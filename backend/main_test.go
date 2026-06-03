package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIsAllowedStaticPath pins the whitelist: only the shell, the stylesheet,
// and the /js/ module tree are serveable. Anything else under the served root —
// notably the SQLite games database and the Go source, which both live under it
// in the container — must be rejected.
func TestIsAllowedStaticPath(t *testing.T) {
	allowed := []string{"/", "/index.html", "/style.css", "/js/app.js", "/js/board-model.js"}
	for _, p := range allowed {
		if !isAllowedStaticPath(p) {
			t.Errorf("isAllowedStaticPath(%q) = false, want true", p)
		}
	}

	blocked := []string{
		"/backend/data/games.db", // the game history database
		"/backend/hub.go",        // server source
		"/go.mod",
		"/.git/config",
		"/docs/plans/x.md",
		"/js/",        // directory listing, not a module
		"/js/app.txt", // non-module file under js/
		"/Prd.md",
	}
	for _, p := range blocked {
		if isAllowedStaticPath(p) {
			t.Errorf("isAllowedStaticPath(%q) = true, want false", p)
		}
	}
}

// TestStaticAssetGuard confirms the guard 404s disallowed paths before they
// reach the file server, and lets allowed ones through to next.
func TestStaticAssetGuard(t *testing.T) {
	reached := ""
	guard := staticAssetGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		path       string
		wantStatus int
		wantNext   bool
	}{
		{"/js/app.js", http.StatusOK, true},
		{"/index.html", http.StatusOK, true},
		{"/backend/data/games.db", http.StatusNotFound, false},
		{"/../backend/data/games.db", http.StatusNotFound, false}, // traversal still cleaned + blocked
		{"/.git/config", http.StatusNotFound, false},
	}
	for _, tc := range cases {
		reached = ""
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		guard.ServeHTTP(rec, req)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s: status = %d, want %d", tc.path, rec.Code, tc.wantStatus)
		}
		if (reached != "") != tc.wantNext {
			t.Errorf("%s: next reached = %v, want %v", tc.path, reached != "", tc.wantNext)
		}
	}
}
