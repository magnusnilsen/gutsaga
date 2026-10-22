// Package smiaoauth is the companion's MCP OAuth public client.
// Smia has no device-code grant. The primary redirect is the loopback
// http://127.0.0.1:<port>/callback, which Smia has not confirmed. If dynamic
// registration rejects that URI, the client tries http://localhost:<port>/callback
// and, if the browser never comes back, asks for a pasted code.
package smiaoauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed logo.png
var logoPNG []byte

const (
	DefaultResourceMetadata   = "https://smia.tools/.well-known/oauth-protected-resource/wow/mcp"
	DefaultASMetadata         = "https://app.smia.io/.well-known/oauth-authorization-server"
	DefaultResource           = "https://smia.tools/wow/mcp"
	Scope                     = "openid profile mcp"
	errRedirectRejectedMarker = "redirect_rejected"
)

var ErrRedirectRejected = errors.New(errRedirectRejectedMarker)

// Client performs authorization-code + PKCE S256 as a public client.
type Client struct {
	ResourceMetadataURL   string
	FallbackASMetadataURL string
	HTTP                  *http.Client
	OpenBrowser           func(string) error
	// Prompt is optional. It is used when the loopback redirect does not arrive.
	Prompt func() (string, error)
}

type Result struct {
	ClientID        string
	AccessToken     string
	RefreshToken    string
	AccessExpiresAt time.Time
	Resource        string
	TokenEndpoint   string
}

type Stored struct {
	ClientID        string
	RefreshToken    string
	AccessToken     string
	AccessExpiresAt time.Time
	Resource        string
	TokenEndpoint   string
}

type asMeta struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
}

type resourceMeta struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

func (c *Client) Login(ctx context.Context) (Result, error) {
	meta, resource, err := c.discover(ctx)
	if err != nil {
		return Result{}, err
	}
	var last error
	for _, host := range []string{"127.0.0.1", "localhost"} {
		res, err := c.loginHost(ctx, meta, resource, host)
		if err == nil {
			return res, nil
		}
		last = err
		if !errors.Is(err, ErrRedirectRejected) {
			return Result{}, err
		}
	}
	if last == nil {
		last = errors.New("innlogging feilet")
	}
	return Result{}, fmt.Errorf("loopback-omdirigering ble avvist av Smia (ikke bekreftet for 127.0.0.1 eller localhost): %w", last)
}

func (c *Client) Refresh(ctx context.Context, stored Stored) (Result, error) {
	if stored.RefreshToken == "" || stored.ClientID == "" {
		return Result{}, errors.New("mangler refresh-token")
	}
	meta := asMeta{TokenEndpoint: stored.TokenEndpoint}
	resource := stored.Resource
	if meta.TokenEndpoint == "" || resource == "" {
		var err error
		meta, resource, err = c.discover(ctx)
		if err != nil {
			return Result{}, err
		}
	}
	tok, err := c.token(ctx, meta.TokenEndpoint, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {stored.RefreshToken},
		"client_id":     {stored.ClientID},
		"resource":      {resource},
	})
	if err != nil {
		return Result{}, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = stored.RefreshToken
	}
	return Result{
		ClientID: stored.ClientID, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		AccessExpiresAt: time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
		Resource:        resource, TokenEndpoint: meta.TokenEndpoint,
	}, nil
}

func (c *Client) loginHost(ctx context.Context, meta asMeta, resource, host string) (Result, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Result{}, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://%s:%d/callback", host, port)
	clientID, err := c.register(ctx, meta.RegistrationEndpoint, redirectURI)
	if err != nil {
		_ = ln.Close()
		return Result{}, err
	}
	verifier, challenge, err := pkce()
	if err != nil {
		_ = ln.Close()
		return Result{}, err
	}
	state, err := randomToken(16)
	if err != nil {
		_ = ln.Close()
		return Result{}, err
	}
	authURL := authorizeURL(meta.AuthorizationEndpoint, url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scope},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {resource},
	})
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "" && q.Get("state") != state {
			http.Error(w, "state matcher ikke", http.StatusBadRequest)
			errCh <- errors.New("state matcher ikke")
			return
		}
		if q.Get("error") != "" {
			http.Error(w, q.Get("error"), http.StatusBadRequest)
			errCh <- fmt.Errorf("%s", q.Get("error"))
			return
		}
		if q.Get("code") == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, loggedInPage())
		codeCh <- q.Get("code")
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if c.OpenBrowser != nil {
		if err := c.OpenBrowser(authURL); err != nil {
			fmt.Println("Klarte ikke å åpne nettleseren. Åpne adressen selv:")
			fmt.Println(authURL)
		}
	}
	wait := 3 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < wait {
			wait = d
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var code string
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case err := <-errCh:
		return Result{}, err
	case code = <-codeCh:
	case <-timer.C:
		if c.Prompt == nil {
			return Result{}, fmt.Errorf("fikk ingen omdirigering til %s. Loopback er ikke bekreftet hos Smia. Åpne %s og lim inn koden om du blir spurt", redirectURI, authURL)
		}
		pasted, err := c.Prompt()
		if err != nil {
			return Result{}, err
		}
		code = codeFromPaste(pasted)
		if code == "" {
			return Result{}, errors.New("fant ingen kode i det du limte inn")
		}
	}
	tok, err := c.token(ctx, meta.TokenEndpoint, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
		"resource":      {resource},
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		ClientID: clientID, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		AccessExpiresAt: time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
		Resource:        resource, TokenEndpoint: meta.TokenEndpoint,
	}, nil
}

func (c *Client) discover(ctx context.Context) (asMeta, string, error) {
	resURL := c.ResourceMetadataURL
	if resURL == "" {
		resURL = DefaultResourceMetadata
	}
	var resource resourceMeta
	if err := c.getJSON(ctx, resURL, &resource); err != nil {
		return asMeta{}, "", fmt.Errorf("beskyttet ressurs: %w", err)
	}
	if resource.Resource == "" {
		resource.Resource = DefaultResource
	}
	var first error
	if len(resource.AuthorizationServers) > 0 {
		metaURL := strings.TrimRight(resource.AuthorizationServers[0], "/") + "/.well-known/oauth-authorization-server"
		var meta asMeta
		if err := c.getJSON(ctx, metaURL, &meta); err == nil && meta.AuthorizationEndpoint != "" && meta.TokenEndpoint != "" && meta.RegistrationEndpoint != "" {
			return meta, resource.Resource, nil
		} else if err != nil {
			first = err
		}
	}
	fallback := c.FallbackASMetadataURL
	if fallback == "" {
		fallback = DefaultASMetadata
	}
	var meta asMeta
	if err := c.getJSON(ctx, fallback, &meta); err != nil {
		if first != nil {
			return asMeta{}, "", fmt.Errorf("autorisasjonstjener: %v; reserve: %w", first, err)
		}
		return asMeta{}, "", err
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" || meta.RegistrationEndpoint == "" {
		return asMeta{}, "", errors.New("autorisasjonstjeneren mangler endepunkt")
	}
	return meta, resource.Resource, nil
}

func (c *Client) register(ctx context.Context, endpoint, redirectURI string) (string, error) {
	body := map[string]any{
		"client_name":                "GutSaga",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      Scope,
		"application_type":           "native",
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusBadRequest || strings.Contains(strings.ToLower(string(b)), "redirect") {
			return "", fmt.Errorf("%w: %s", ErrRedirectRejected, strings.TrimSpace(string(b)))
		}
		return "", fmt.Errorf("klientregistrering %d", resp.StatusCode)
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.ClientID == "" {
		return "", errors.New("klientregistrering mangler client_id")
	}
	return out.ClientID, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

func (c *Client) token(ctx context.Context, endpoint string, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out tokenResponse
	_ = json.Unmarshal(b, &out)
	if resp.StatusCode >= 400 || out.AccessToken == "" {
		if out.Error != "" {
			return tokenResponse{}, errors.New(out.Error)
		}
		return tokenResponse{}, fmt.Errorf("token-endepunkt %d", resp.StatusCode)
	}
	if out.ExpiresIn == 0 {
		out.ExpiresIn = 3600
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.Unmarshal(b, dest)
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func loggedInPage() string {
	return `<!doctype html><meta charset=utf-8><title>GutSaga</title>` +
		`<body style="margin:0;background:#120e0a;color:#f4e7cf;font-family:Georgia,serif;text-align:center;padding:48px 16px">` +
		`<img alt="GutSaga" width="240" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(logoPNG) + `">` +
		`<p>Du er logget inn. Du kan lukke dette vinduet.</p>`
}

func authorizeURL(endpoint string, q url.Values) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint + "?" + q.Encode()
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func pkce() (verifier, challenge string, err error) {
	verifier, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeFromPaste(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Query().Get("code") != "" {
		return u.Query().Get("code")
	}
	if i := strings.Index(s, "code="); i >= 0 {
		rest := s[i+len("code="):]
		if j := strings.IndexAny(rest, "& \t\r\n"); j >= 0 {
			rest = rest[:j]
		}
		if decoded, err := url.QueryUnescape(rest); err == nil {
			return decoded
		}
		return rest
	}
	return s
}
