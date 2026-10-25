package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/install"
)

func (a *App) ensureLogin(ctx context.Context) error {
	a.mu.Lock()
	if a.Client.APIKey != "" {
		a.mu.Unlock()
		return nil
	}
	if a.signedOut || a.loginTried {
		a.mu.Unlock()
		return errors.New("not logged in")
	}
	a.loginTried = true
	a.mu.Unlock()
	return a.Login(ctx)
}

// ConfigLanguage is the saved en or nb choice. Empty means the addon uses GetLocale().
func (a *App) ConfigLanguage() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Config.Language
}

// LoggedIn reports whether the companion has a OurSagas API credential.
func (a *App) LoggedIn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Config.Token() != ""
}

// Login uses OURSAGAS_DEV_TOKEN when it is set. Otherwise it opens the browser
// on the API Battle.net start URL and polls until the API stores a session.
func (a *App) Login(ctx context.Context) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	a.mu.Lock()
	a.signedOut = false
	a.mu.Unlock()
	if dev := strings.TrimSpace(os.Getenv("OURSAGAS_DEV_TOKEN")); dev != "" {
		out, err := a.Client.DevLogin(ctx, dev)
		if err != nil {
			return err
		}
		return a.storeSession(out)
	}
	attempt, err := randomAttempt()
	if err != nil {
		return err
	}
	start := strings.TrimRight(a.Config.APIBaseURL, "/") + "/v1/auth/bnet/start?attempt=" + attempt
	// Register the attempt on the API before opening the browser. /start only
	// creates the row when hit; polling first would race and get "unknown attempt".
	if err := registerBnetAttempt(ctx, a.Client.HTTP, start); err != nil {
		return err
	}
	if err := openBrowser(start); err != nil {
		a.Log.Warn("could not open the browser", "err", err, "url", start)
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("Battle.net login timed out")
		}
		out, pending, err := a.Client.PollBnet(ctx, attempt)
		if err != nil {
			return err
		}
		if !pending {
			return a.storeSession(out)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// registerBnetAttempt hits /v1/auth/bnet/start without following the Battle.net
// redirect so the poll endpoint knows the attempt id before the browser opens.
func registerBnetAttempt(ctx context.Context, httpClient *http.Client, startURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, startURL, nil)
	if err != nil {
		return err
	}
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Battle.net start failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *App) storeSession(out contract.SessionResponse) error {
	a.mu.Lock()
	a.Config.SessionToken = out.SessionToken
	a.Config.APIKey = out.SessionToken
	a.storePlayerLocked(out.Player)
	a.Config.Normalize()
	a.Client.APIKey = a.Config.Token()
	cfg := a.Config
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		a.Log.Warn("could not save the session", "err", err)
	}
	a.Log.Info("logged in", "player", out.Player.DisplayName, "battletag", out.Player.BattleTag)
	a.notifyAccount()
	return nil
}

func (a *App) clearSession() {
	a.mu.Lock()
	a.clearSessionLocked(false)
	cfg := a.Config
	a.mu.Unlock()
	_ = config.Save(cfg)
	a.notifyAccount()
}

// Logout clears the OurSagas session. Sync will not open the browser again
// until the user chooses Log in.
func (a *App) Logout() {
	a.mu.Lock()
	a.signedOut = true
	a.loginTried = true
	a.sync = contract.SyncResponse{}
	a.clearSessionLocked(true)
	cfg := a.Config
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		a.Log.Warn("could not save logout", "err", err)
	}
	a.Log.Info("logged out")
	a.notifyAccount()
}

func (a *App) clearSessionLocked(keepSignedOut bool) {
	a.Config.SessionToken = ""
	if strings.HasPrefix(a.Config.APIKey, "gss_") || strings.HasPrefix(a.Config.APIKey, "gst_") {
		a.Config.APIKey = ""
	}
	a.Config.PlayerName = ""
	a.Config.PlayerEmail = ""
	a.Config.BattleTag = ""
	a.player = contract.Player{}
	a.Client.APIKey = a.Config.Token()
	if !keepSignedOut {
		a.loginTried = false
	}
}

// MintPersonalToken creates a gst_ token, stores its id, and writes the raw token once.
func (a *App) MintPersonalToken(ctx context.Context) (string, error) {
	if err := a.ensureLogin(ctx); err != nil {
		return "", err
	}
	tok, err := a.Client.MintToken(ctx, "companion")
	if err != nil {
		return "", err
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "personal-token.txt")
	if err := os.WriteFile(path, []byte(tok.Token+"\n"), 0o600); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.Config.PersonalTokenID = tok.ID
	cfg := a.Config
	a.mu.Unlock()
	_ = config.Save(cfg)
	return path, nil
}

// RevokePersonalToken revokes the token minted from this companion.
func (a *App) RevokePersonalToken(ctx context.Context) error {
	a.mu.Lock()
	id := a.Config.PersonalTokenID
	a.mu.Unlock()
	if id == "" {
		return errors.New("no personal token to revoke")
	}
	if err := a.Client.RevokeToken(ctx, id); err != nil {
		return err
	}
	a.mu.Lock()
	a.Config.PersonalTokenID = ""
	cfg := a.Config
	a.mu.Unlock()
	_ = config.Save(cfg)
	if dir, err := config.Dir(); err == nil {
		_ = os.Remove(filepath.Join(dir, "personal-token.txt"))
	}
	return nil
}

// SetLanguage stores en or nb and rewrites the addon language file.
func (a *App) SetLanguage(lang string) error {
	switch lang {
	case "en", "nb":
	default:
		return errors.New("language must be en or nb")
	}
	a.mu.Lock()
	a.Config.Language = lang
	cfg := a.Config
	wow := a.WowDir
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		return err
	}
	if wow != "" {
		return install.WriteLanguage(wow, lang)
	}
	return nil
}

func (a *App) storePlayerLocked(p contract.Player) {
	a.player = p
	if tag := strings.TrimSpace(p.BattleTag); tag != "" {
		a.Config.BattleTag = tag
	}
	a.Config.PlayerName = strings.TrimSpace(p.DisplayName)
}

func randomAttempt() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Start()
}
