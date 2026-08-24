package matrix

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gearshell/matrix/rewrite"
)

// writeSite creates dataDir/project/index.html (and optional extra files).
func writeSite(t *testing.T, dataDir, project, body string, extra ...string) {
	t.Helper()
	dir := filepath.Join(dataDir, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range extra {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func getHost(t *testing.T, srv *httptest.Server, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSiteHandlerServesProjectBySubdomain(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "hello alpha", "asset.txt")
	writeSite(t, dir, "beta", "hello beta")

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "alpha.localhost", "/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, "hello alpha") {
		t.Fatalf("alpha /: status %d body %q", res.StatusCode, body)
	}
	res = getHost(t, srv, "beta.localhost", "/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, "hello beta") {
		t.Fatalf("beta /: status %d body %q", res.StatusCode, body)
	}
	res = getHost(t, srv, "alpha.localhost", "/asset.txt")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || body != "asset.txt" {
		t.Fatalf("alpha asset: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerServesNestedPaths(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "root")
	if err := os.MkdirAll(filepath.Join(dir, "alpha", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha", "sub", "page.html"), []byte("nested page"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "alpha.localhost", "/sub/page.html")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || body != "nested page" {
		t.Fatalf("nested page: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerNotFound(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "hello")
	// A file named like a project must not be served as a directory.
	if err := os.WriteFile(filepath.Join(dir, "evil"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	cases := []struct {
		host string
		path string
	}{
		{"unknown.localhost", "/"},
		{"alpha.example.com", "/"}, // wrong apex domain
		{"a.b.localhost", "/"},     // deeper subdomain
		{"evil.localhost", "/"},    // project name is a file, not a dir
		{"localhost", "/nope"},     // apex with a non-root path
	}
	for _, tc := range cases {
		res := getHost(t, srv, tc.host, tc.path)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s%s: status %d, want 404", tc.host, tc.path, res.StatusCode)
		}
		res.Body.Close()
	}
}

func TestSiteHandlerIgnoresPortInHost(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "hello")

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "alpha.localhost:9999", "/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, "hello") {
		t.Fatalf("host with port: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerCaseInsensitiveDomain(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "hello")

	srv := httptest.NewServer(NewSiteHandler(dir, "LocalHost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "ALPHA.localhost", "/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, "hello") {
		t.Fatalf("case-insensitive: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerApexListsProjects(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "beta", "b")
	writeSite(t, dir, "alpha", "a")

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "localhost", "/")
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("apex: status %d", res.StatusCode)
	}
	if !strings.Contains(body, "http://alpha.localhost/") || !strings.Contains(body, "http://beta.localhost/") {
		t.Fatalf("apex listing missing project links: %s", body)
	}
}

func TestSiteHandlerApexEmpty(t *testing.T) {
	srv := httptest.NewServer(NewSiteHandler(t.TempDir(), "localhost"))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "localhost", "/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, "no sites deployed yet") {
		t.Fatalf("empty apex: status %d body %q", res.StatusCode, body)
	}
}

const rewriteSnippet = `<script src="/probe.js"></script>`

func TestSiteHandlerRewritesIndexHTML(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>", "asset.txt")
	if err := os.MkdirAll(filepath.Join(dir, "alpha", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha", "sub", "index.html"), []byte("<p>sub</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewSiteHandlerWithInjector(dir, "localhost", rewrite.New(rewriteSnippet)))
	t.Cleanup(srv.Close)

	res := getHost(t, srv, "alpha.localhost", "/")
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, rewriteSnippet) {
		t.Fatalf("root index: status %d body %q", res.StatusCode, body)
	}
	if i, j := strings.Index(body, rewriteSnippet), strings.Index(body, "</body>"); i == -1 || i > j {
		t.Fatalf("snippet must precede </body> (snippet@%d body@%d)", i, j)
	}
	res = getHost(t, srv, "alpha.localhost", "/sub/")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || !strings.Contains(body, rewriteSnippet) {
		t.Fatalf("nested index: status %d body %q", res.StatusCode, body)
	}
	// Non-HTML assets are served untouched.
	res = getHost(t, srv, "alpha.localhost", "/asset.txt")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || body != "asset.txt" {
		t.Fatalf("asset: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerRewriteIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")

	srv := httptest.NewServer(NewSiteHandlerWithInjector(dir, "localhost", rewrite.New(rewriteSnippet)))
	t.Cleanup(srv.Close)

	first := readBody(t, getHost(t, srv, "alpha.localhost", "/"))
	second := readBody(t, getHost(t, srv, "alpha.localhost", "/"))
	if got := strings.Count(second, rewriteSnippet); got != 1 {
		t.Fatalf("snippet appears %d times on second request, want 1 (idempotent rewrite)", got)
	}
	if first != second {
		t.Fatal("rewritten page differs between requests")
	}
}

func TestSiteHandlerNoRewriteWithoutInjector(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	if body := readBody(t, getHost(t, srv, "alpha.localhost", "/")); strings.Contains(body, rewriteSnippet) {
		t.Fatalf("snippet injected without injector: %q", body)
	}
}

func TestSiteHandlerRewriteLeavesPlainPagesAndListingsAlone(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")
	if err := os.WriteFile(filepath.Join(dir, "alpha", "about.html"), []byte("<p>about</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "alpha", "raw"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewSiteHandlerWithInjector(dir, "localhost", rewrite.New(rewriteSnippet)))
	t.Cleanup(srv.Close)

	// Plain .html pages are not rewritten, only index.html.
	res := getHost(t, srv, "alpha.localhost", "/about.html")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || strings.Contains(body, rewriteSnippet) {
		t.Fatalf("about.html: status %d body %q", res.StatusCode, body)
	}
	// A directory without index.html 404s: the real server never renders
	// directory listings.
	res = getHost(t, srv, "alpha.localhost", "/raw/")
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("listing: status %d, want 404 (no listings like the real server)", res.StatusCode)
	}
	// Explicit /index.html URLs are served directly (200, rewritten), not
	// redirected like http.FileServer does.
	res = getHost(t, srv, "alpha.localhost", "/index.html")
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, rewriteSnippet) {
		t.Fatalf("explicit /index.html: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerSPAFallback(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	cases := []struct {
		path string
		want int
	}{
		// Missing extensionless paths fall back to index.html, like the real
		// server's SPA fallback (.git/HEAD, /plain-missing -> 200).
		{"/plain-missing", http.StatusOK},
		{"/some/extensionless", http.StatusOK},
		{"/.git/HEAD", http.StatusOK},
		{"/sub/deep/missing-page", http.StatusOK},
		// Missing paths with an extension 404.
		{"/definitely-missing.js", http.StatusNotFound},
		{"/missing.html", http.StatusNotFound},
		// Directory-looking missing paths 404.
		{"/definitely-missing/", http.StatusNotFound},
	}
	for _, tc := range cases {
		res := getHost(t, srv, "alpha.localhost", tc.path)
		body := readBody(t, res)
		if res.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d", tc.path, res.StatusCode, tc.want)
			continue
		}
		if tc.want == http.StatusOK && !strings.Contains(body, "hello") {
			t.Errorf("%s: fallback body %q must contain index.html content", tc.path, body)
		}
	}
}

func TestSiteHandlerSPAFallbackRespectsInjector(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")

	srv := httptest.NewServer(NewSiteHandlerWithInjector(dir, "localhost", rewrite.New(rewriteSnippet)))
	t.Cleanup(srv.Close)

	// The fallback serves the rewritten index.html.
	res := getHost(t, srv, "alpha.localhost", "/plain-missing")
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, rewriteSnippet) {
		t.Fatalf("fallback: status %d body %q", res.StatusCode, body)
	}
	if i, j := strings.Index(body, rewriteSnippet), strings.Index(body, "</body>"); i == -1 || i > j {
		t.Fatalf("snippet must precede </body> (snippet@%d body@%d)", i, j)
	}
}

func TestSiteHandlerNoIndexHTMLIs404(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha", "plain.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewSiteHandler(dir, "localhost"))
	t.Cleanup(srv.Close)

	// A site without index.html 404s at the root (no listing), while real
	// files still serve — matching the real server's noidx deployment.
	res := getHost(t, srv, "alpha.localhost", "/")
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("root of index-less site: status %d, want 404", res.StatusCode)
	}
	res = getHost(t, srv, "alpha.localhost", "/plain.txt")
	if body := readBody(t, res); res.StatusCode != http.StatusOK || body != "hi" {
		t.Fatalf("plain.txt: status %d body %q", res.StatusCode, body)
	}
}

func TestSiteHandlerRewriteHeadRequest(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, "alpha", "<html><body>hello</body></html>")

	srv := httptest.NewServer(NewSiteHandlerWithInjector(dir, "localhost", rewrite.New(rewriteSnippet)))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodHead, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "alpha.localhost"
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("HEAD: status %d", res.StatusCode)
	}
	// Content-Length reflects the rewritten page, the body is empty.
	got := readBody(t, getHost(t, srv, "alpha.localhost", "/"))
	if res.ContentLength != int64(len(got)) {
		t.Fatalf("HEAD Content-Length = %d, want %d (rewritten body)", res.ContentLength, len(got))
	}
	if b, _ := io.ReadAll(res.Body); len(b) != 0 {
		t.Fatalf("HEAD returned a body: %q", b)
	}
}
