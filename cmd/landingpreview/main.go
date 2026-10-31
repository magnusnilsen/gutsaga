// Landing page preview. Serves web/landing.html with the OAuth static chrome.
//
//	go run ./cmd/landingpreview
//	open http://127.0.0.1:8790/
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8790", "listen address")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		log.Fatal(err)
	}
	landing := filepath.Join(root, "web", "landing.html")
	static := filepath.Join(root, "api", "static")
	if _, err := os.Stat(landing); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(static); err != nil {
		log.Fatal(err)
	}

	webDir := filepath.Join(root, "web")
	maskTool := filepath.Join(webDir, "mask-tool.html")
	mux := http.NewServeMux()
	mux.Handle("/oauth/static/", http.StripPrefix("/oauth/static/", http.FileServer(http.Dir(static))))
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(webDir))))
	mux.HandleFunc("/mask", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, maskTool)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, landing)
	})

	fmt.Printf("Sagalore landing preview: http://%s/\n", *addr)
	fmt.Printf("Screenshot mask tool:    http://%s/mask\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}
