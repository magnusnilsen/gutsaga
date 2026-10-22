package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/smiaoauth"
)

func (a *App) ensureLogin(ctx context.Context) error {
	a.mu.Lock()
	if a.Client.APIKey != "" {
		a.mu.Unlock()
		return nil
	}
	if a.loginTried {
		a.mu.Unlock()
		return errors.New("ikke innlogget med Smia")
	}
	a.loginTried = true
	a.mu.Unlock()
	return a.Login(ctx)
}

// Login opens the Smia browser flow, or uses GUTSAGA_DEV_TOKEN when the API
// is in dev mode. The Smia access token is exchanged for a GutSaga session.
func (a *App) Login(ctx context.Context) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if dev := strings.TrimSpace(os.Getenv("GUTSAGA_DEV_TOKEN")); dev != "" {
		return a.exchange(ctx, dev)
	}
	if a.Config.Smia.RefreshToken != "" && a.Config.Smia.ClientID != "" {
		oauth := a.oauthClient()
		res, err := oauth.Refresh(ctx, smiaoauth.Stored{
			ClientID: a.Config.Smia.ClientID, RefreshToken: a.Config.Smia.RefreshToken,
			Resource: a.Config.Smia.Resource, TokenEndpoint: a.Config.Smia.TokenEndpoint,
		})
		if err == nil {
			a.storeSmia(res)
			if err := a.exchange(ctx, res.AccessToken); err == nil {
				return nil
			}
		}
		a.Log.Warn("Smia refresh feilet, åpner nettleseren", "err", err)
	}
	res, err := a.oauthClient().Login(ctx)
	if err != nil {
		return err
	}
	a.storeSmia(res)
	return a.exchange(ctx, res.AccessToken)
}

func (a *App) oauthClient() *smiaoauth.Client {
	return &smiaoauth.Client{OpenBrowser: smiaoauth.Open, Prompt: a.Prompt}
}

func (a *App) storeSmia(res smiaoauth.Result) {
	a.mu.Lock()
	a.Config.Smia = config.Smia{
		ClientID: res.ClientID, RefreshToken: res.RefreshToken, AccessToken: res.AccessToken,
		AccessExpiresAt: res.AccessExpiresAt.UTC().Format(time.RFC3339),
		Resource:        res.Resource, TokenEndpoint: res.TokenEndpoint,
	}
	cfg := a.Config
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		a.Log.Warn("klarte ikke å lagre Smia-token", "err", err)
	}
}

func (a *App) exchange(ctx context.Context, smiaToken string) error {
	out, err := a.Client.CreateSession(ctx, smiaToken)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.Config.SessionToken = out.SessionToken
	a.Config.APIKey = out.SessionToken
	a.Config.Normalize()
	a.Client.APIKey = a.Config.Token()
	cfg := a.Config
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		a.Log.Warn("klarte ikke å lagre økten", "err", err)
	}
	a.Log.Info("logget inn med Smia", "player", out.Player.DisplayName)
	return nil
}

func (a *App) clearSession() {
	a.mu.Lock()
	a.Config.SessionToken = ""
	if strings.HasPrefix(a.Config.APIKey, "gss_") {
		a.Config.APIKey = ""
	}
	a.Client.APIKey = a.Config.Token()
	a.loginTried = false
	cfg := a.Config
	a.mu.Unlock()
	_ = config.Save(cfg)
}
