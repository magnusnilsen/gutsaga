package main

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Landing is the marketing site (web/landing.html + assets). Enabled when
// LANDING_DIR points at the web folder (Docker sets /web).
func (s *Server) mountLanding(mux *http.ServeMux) {
	dir := strings.TrimSpace(os.Getenv("LANDING_DIR"))
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		slog.Warn("landing disabled", "err", err)
		return
	}
	index := filepath.Join(abs, "landing.html")
	if st, err := os.Stat(index); err != nil || st.IsDir() {
		slog.Warn("landing disabled: landing.html missing", "dir", abs)
		return
	}
	root := http.Dir(abs)
	assets := http.FileServer(root)
	mux.HandleFunc("GET /assets/{path...}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		base := path.Base(name)
		if name == "" || base == "." || base == ".." || strings.Contains(name, "..") ||
			base == "mask-tool.html" || strings.HasPrefix(base, "_") {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name
		assets.ServeHTTP(w, r2)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serveLandingHTML(w, r, index)
	})
	mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	slog.Info("landing enabled", "dir", abs)
}

func serveLandingHTML(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeContent(w, r, "landing.html", st.ModTime(), f)
}
