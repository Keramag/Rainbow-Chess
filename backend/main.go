package main

import (
	"log"
	"net/http"
	"os"
	"path"
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

// isAllowedStaticPath whitelists the only paths the static file server may serve:
// the shell (index.html), the stylesheet, and the ES-module tree under /js/.
// Everything else under the served directory — in the container that root is
// /app, which also holds the Go source and the SQLite games database under
// backend/data — is off limits. Without this the FileServer would happily hand
// out e.g. /backend/data/games.db (the full game history) or, in dev where the
// root is the repo, all source and the .git directory.
func isAllowedStaticPath(p string) bool {
	switch p {
	case "/", "/index.html", "/style.css":
		return true
	}
	return strings.HasPrefix(p, "/js/") && strings.HasSuffix(p, ".js")
}

// staticAssetGuard 404s any request that isAllowedStaticPath rejects before it
// reaches the file server, scoping the served tree to the frontend assets only.
func staticAssetGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowedStaticPath(path.Clean(r.URL.Path)) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	store, err := OpenStore(databasePath())
	if err != nil {
		log.Fatalf("Failed to open game store: %v", err)
	}
	defer store.Close()

	hub := newHub()
	// Wire the Task 9 game-end seam to SQLite persistence: every finished game is
	// saved asynchronously, off the hub goroutine.
	hub.gameEnded = func(g *Game) { store.SaveGame(g) }
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
	http.Handle("/", staticAssetGuard(noCacheMiddleware(fs)))

	log.Println("Server starting on :8080")
	log.Printf("Serving static files from: %s", staticDir)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
