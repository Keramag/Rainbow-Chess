package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

// noCacheMiddleware disables caching for JS/CSS so the no-build-step frontend
// never serves stale modules during development or after a redeploy.
func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	hub := newHub()
	go hub.run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	// Static frontend lives at the repo root in development (one level up from
	// backend/) and at /app inside the container image.
	staticDir := "../"
	if _, err := os.Stat("/app/index.html"); err == nil {
		staticDir = "/app"
	}

	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", noCacheMiddleware(fs))

	log.Println("Server starting on :8080")
	log.Printf("Serving static files from: %s", staticDir)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
