package smiaoauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginLoopbackThenLocalhostFallback(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/oauth-protected-resource/wow/mcp"):
			_, _ = io.WriteString(w, `{"resource":"https://smia.tools/wow/mcp","authorization_servers":["`+srv.URL+`/issuer"]}`)
		case strings.Contains(r.URL.Path, "/issuer/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/.well-known/oauth-authorization-server"):
			_, _ = io.WriteString(w, `{"issuer":"https://smia.tools/","authorization_endpoint":"`+srv.URL+`/authorize","token_endpoint":"`+srv.URL+`/token","registration_endpoint":"`+srv.URL+`/register"}`)
		case r.URL.Path == "/register":
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "127.0.0.1") {
				http.Error(w, `{"error":"invalid_redirect_uri"}`, http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"client_id":"gut-public"}`)
		case r.URL.Path == "/authorize":
			q := r.URL.Query()
			if q.Get("code_challenge_method") != "S256" || q.Get("resource") == "" {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			dest := q.Get("redirect_uri") + "?code=abc&state=" + url.QueryEscape(q.Get("state"))
			http.Redirect(w, r, dest, http.StatusFound)
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "abc" || r.Form.Get("code_verifier") == "" || r.Form.Get("client_id") != "gut-public" {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"smia-access","refresh_token":"smia-refresh","expires_in":3600,"token_type":"Bearer"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{
		ResourceMetadataURL:   srv.URL + "/.well-known/oauth-protected-resource/wow/mcp",
		FallbackASMetadataURL: srv.URL + "/.well-known/oauth-authorization-server",
		HTTP:                  srv.Client(),
		OpenBrowser: func(raw string) error {
			req, err := http.NewRequest(http.MethodGet, raw, nil)
			if err != nil {
				return err
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Login(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.AccessToken != "smia-access" || res.RefreshToken != "smia-refresh" || res.ClientID != "gut-public" {
		t.Fatalf("%+v", res)
	}
}

func TestCodeFromPaste(t *testing.T) {
	if codeFromPaste("http://127.0.0.1:9/callback?code=xyz&state=1") != "xyz" {
		t.Fatal("url")
	}
	if codeFromPaste("xyz") != "xyz" {
		t.Fatal("raw")
	}
}
