package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/backend"
	"github.com/magnusnilsen/gutsaga/internal/config"
)

func TestLoginUsesDevTokenBypass(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/auth/dev" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","session_token":"gss_test","expires_at":"2030-01-01T00:00:00Z","player":{"id":"p1","battletag":"Player#1234","display_name":"Player"}}`))
	}))
	defer srv.Close()
	t.Setenv("OURSAGAS_DEV_TOKEN", "dev-secret")
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	a := &App{
		Config: config.Config{APIBaseURL: srv.URL, Client: "forever"},
		Client: &backend.Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Log:    discardLog(),
	}
	if err := a.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer dev-secret" {
		t.Fatalf("auth %q", got)
	}
	if a.Config.SessionToken != "gss_test" || a.Config.BattleTag != "Player#1234" {
		t.Fatalf("session %+v", a.Config)
	}
	if os.Getenv("OURSAGAS_DEV_TOKEN") == "" {
		t.Fatal("dev token cleared")
	}
}
