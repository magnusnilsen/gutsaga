package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLandingRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "landing.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mask-tool.html"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANDING_DIR", dir)

	s := &Server{}
	mux := http.NewServeMux()
	s.mountLanding(mux)

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET /: %d", res.Code)
	}
	if got := res.Body.String(); got != "<html>ok</html>" {
		t.Fatalf("body %q", got)
	}

	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/assets/shot.jpg", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET /assets/shot.jpg: %d", res.Code)
	}

	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/assets/mask-tool.html", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("mask tool should be hidden, got %d", res.Code)
	}
}
