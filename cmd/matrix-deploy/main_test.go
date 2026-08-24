package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArchiveBytesDirPacksRootContents verifies a directory becomes a
// .tar.gz whose root holds the files (index.html at the archive root).
func TestArchiveBytesDirPacksRootContents(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>cli</h1>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "a.css"), []byte("body{}"), 0o644)

	data, err := archiveBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	names := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names[hdr.Name] = true
	}
	if !names["index.html"] || !names["assets/"] || !names["assets/a.css"] {
		t.Errorf("archive entries = %v, want index.html + assets/ + assets/a.css", names)
	}
}

// TestCLIDeploysViaAPI runs the full CLI flow against a fake deploy API.
func TestCLIDeploysViaAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/deploy" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"website_url": "https://fake.example.com/site"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>cli</h1>"), 0o644)

	var out strings.Builder
	os.Args = []string{"matrix-deploy", "--server", srv.URL, dir}
	runCLI(&out)
	if got := out.String(); got != "https://fake.example.com/site\n" {
		t.Errorf("stdout = %q", got)
	}
}

// TestCLIJSONFlag prints the full response.
func TestCLIJSONFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"website_url": "https://x.example.com/", "website_id": 431}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)

	var out strings.Builder
	os.Args = []string{"matrix-deploy", "--server", srv.URL, "--json", dir}
	runCLI(&out)
	var body map[string]any
	if err := json.Unmarshal([]byte(out.String()), &body); err != nil {
		t.Fatalf("--json output is not JSON: %v (%s)", err, out.String())
	}
}
