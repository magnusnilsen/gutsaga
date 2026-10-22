package smia

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type memCache struct {
	mu sync.Mutex
	m  map[string]cached
}

type cached struct {
	id    Identity
	until int64
}

func (c *memCache) Get(_ context.Context, hash string) (Identity, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.m[hash]
	if !ok || item.until <= time.Now().Unix() {
		return Identity{}, false, nil
	}
	return item.id, true, nil
}

func (c *memCache) Put(_ context.Context, hash string, id Identity, until int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cached{}
	}
	c.m[hash] = cached{id: id, until: until}
	return nil
}

func TestMCPVerifierCallsCheckAccessAndCaches(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer smia-token" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		calls++
		w.Header().Set("Mcp-Session-Id", "sess")
		switch {
		case strings.Contains(string(body), "initialize"):
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
		case strings.Contains(string(body), "check_access"):
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Authenticated as magnus@example.com\nHubs:\n- wow (WoW) id=22222222-2222-2222-2222-222222222222 — role: member\n  · server: https://smia.tools/wow/mcp (mcp)\n"}]}}`))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()
	v := MCPVerifier{ResourceURL: srv.URL, HTTP: srv.Client(), Cache: &memCache{}, Now: time.Now}
	id, err := v.Verify(context.Background(), "smia-token")
	if err != nil || !id.Member || id.Email != "magnus@example.com" {
		t.Fatalf("%+v %v", id, err)
	}
	if _, err := v.Verify(context.Background(), "smia-token"); err != nil {
		t.Fatal(err)
	}
	if calls > 3 {
		t.Fatalf("cache missed, calls=%d", calls)
	}
}
