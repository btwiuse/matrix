package matrix

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
