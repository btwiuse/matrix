package matrix_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gearshell/inject-proxy/matrix"
)

// TestRouterDispatch verifies Host-based dispatch: the site namespace
// (<site>.<domain> and the apex listing at "/") goes to the site handler;
// the apex with any other path, IP addresses and unknown hosts go to the
// MCP handler.
func TestRouterDispatch(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("site page"), 0o644); err != nil {
		t.Fatal(err)
	}

	mcpHits := 0
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mcpHits++
		io.WriteString(w, "mcp:"+r.URL.Path)
	})
	router := matrix.NewRouter(mcpHandler, matrix.NewSiteHandler(dir, "localhost"))

	serve := func(host, path string) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
		req.Host = host
		router.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	cases := []struct {
		name, host, path, want string
	}{
		{"site subdomain", "alpha.localhost", "/", "site page"},
		{"site subdomain with port", "alpha.localhost:8080", "/", "site page"},
		{"site subdomain case-insensitive", "ALPHA.LocalHost", "/", "site page"},
		{"apex listing", "localhost", "/", "alpha"},
		{"unknown site 404", "nope.localhost", "/", "404"},
		{"deeper subdomain 404", "a.b.localhost", "/", "404"},
		{"apex mcp path", "localhost", "/mcp/message", "mcp:/mcp/message"},
		{"plain IP", "127.0.0.1:9999", "/", "mcp:/"},
		{"unknown host", "example.com", "/", "mcp:/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, body := serve(tc.host, tc.path)
			if !strings.Contains(body, tc.want) {
				t.Errorf("%s%s: body %q, want %q", tc.host, tc.path, body, tc.want)
			}
		})
	}

	if mcpHits != 3 {
		t.Errorf("mcp handler hit %d times, want 3", mcpHits)
	}
}
