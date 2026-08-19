// Static file server for the ronggur.my.id unikernel image.
//
// Built with CGO_ENABLED=0 so the binary is statically linked: the unikernel
// rootfs is FROM scratch and carries no loader and no shared libraries.
// The platform offers no probes, no logs, and no exec (limits register L5, L6),
// so everything this process needs must come from env and everything it
// reports must be observable over HTTP.
package main

import (
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	port := env("PORT", "8080")
	root := env("SITE_ROOT", "/site")

	// Go's builtin MIME table misses the extensions this site actually uses.
	for ext, typ := range map[string]string{
		".ico": "image/x-icon",
		".wav": "audio/wav",
		".ogg": "audio/ogg",
		".mp3": "audio/mpeg",
	} {
		if err := mime.AddExtensionType(ext, typ); err != nil {
			log.Printf("mime %s: %v", ext, err)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		// Fail loudly at boot rather than serving 404s forever: a missing
		// site root means the rootfs was assembled wrong.
		log.Fatalf("site root %q has no index.html: %v", root, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("/", &siteHandler{root: root})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("serving %s on :%s", root, port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("listen: %v", err)
	}
}

type siteHandler struct{ root string }

func (h *siteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	upath := filepath.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	if strings.HasSuffix(r.URL.Path, "/") {
		upath = filepath.Join(upath, "index.html")
	}

	name := filepath.Join(h.root, filepath.FromSlash(upath))
	info, err := os.Stat(name)
	if err != nil || info.IsDir() {
		// No directory listing, and no filesystem detail in the response.
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
