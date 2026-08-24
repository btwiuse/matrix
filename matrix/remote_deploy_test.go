package matrix_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gearshell/inject-proxy/matrix"
)

// zipBytes builds a zip archive from files (name -> content).
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tarGzBytes builds a .tar.gz archive from files (name -> content).
func tarGzBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractArchiveZipAndTarGz(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bytes func(*testing.T, map[string]string) []byte
	}{
		{"zip", zipBytes},
		{"tar.gz", tarGzBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.bytes(t, map[string]string{
				"dist/index.html": "<h1>hi</h1>",
				"dist/app.js":     "console.log(1)",
			})
			root, err := matrix.ExtractArchiveForTest(t.TempDir(), data)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			defer os.RemoveAll(root)
			if got := readFile(t, filepath.Join(root, "index.html")); got != "<h1>hi</h1>" {
				t.Errorf("index.html = %q", got)
			}
			if got := readFile(t, filepath.Join(root, "app.js")); got != "console.log(1)" {
				t.Errorf("app.js = %q", got)
			}
		})
	}
}

func TestExtractArchiveRejectsTraversal(t *testing.T) {
	z := zipBytes(t, map[string]string{"../evil.txt": "pwned"})
	if _, err := matrix.ExtractArchiveForTest(t.TempDir(), z); err == nil {
		t.Fatal("zip-slip entry must be rejected")
	}
	tg := tarGzBytes(t, map[string]string{"../../etc/passwd": "x"})
	if _, err := matrix.ExtractArchiveForTest(t.TempDir(), tg); err == nil {
		t.Fatal("tar traversal entry must be rejected")
	}
}

func TestExtractArchiveSkipsSymlinks(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.WriteHeader(&tar.Header{Name: "ok.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("ok"))
	tw.Close()
	gz.Close()
	root, err := matrix.ExtractArchiveForTest(t.TempDir(), buf.Bytes())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	defer os.RemoveAll(root)
	if _, err := os.Lstat(filepath.Join(root, "link")); !os.IsNotExist(err) {
		t.Error("symlink must be skipped")
	}
	if got := readFile(t, filepath.Join(root, "ok.txt")); got != "ok" {
		t.Errorf("ok.txt = %q", got)
	}
}

func TestExtractArchiveRejectsUnknownFormat(t *testing.T) {
	if _, err := matrix.ExtractArchiveForTest(t.TempDir(), []byte("not an archive")); err == nil {
		t.Fatal("unknown format must be rejected")
	}
}

// TestLocalDeployRemoteDeployArchiveData publishes from an inline base64
// zip and checks the result shape and the published files.
func TestLocalDeployRemoteDeployArchiveData(t *testing.T) {
	h, _, data := newTestDeploy(t)
	out, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{
		ArchiveData: base64.StdEncoding.EncodeToString(zipBytes(t, map[string]string{
			"index.html": "<h1>remote</h1>",
			"assets/app.css": "body{}",
		})),
	})
	if err != nil {
		t.Fatalf("RemoteDeploy: %v", err)
	}
	body := deployOutput(t, out)
	if id, ok := body["website_id"].(float64); !ok || id < 431000000000000 || id > 431999999999999 {
		t.Errorf("website_id out of range: %v", body["website_id"])
	}
	url, _ := body["website_url"].(string)
	if !strings.HasPrefix(url, "/data/") || !strings.HasSuffix(url, "/") {
		t.Errorf("website_url = %q, want /data/<site>/", url)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(url, "/data/"), "/")
	if got := readFile(t, filepath.Join(data, site, "index.html")); got != "<h1>remote</h1>" {
		t.Errorf("published index.html = %q", got)
	}
	if got := readFile(t, filepath.Join(data, site, "assets/app.css")); got != "body{}" {
		t.Errorf("published app.css = %q", got)
	}
}

// TestLocalDeployRemoteDeployArchiveURL downloads the archive from a URL.
func TestLocalDeployRemoteDeployArchiveURL(t *testing.T) {
	h, _, _ := newTestDeploy(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarGzBytes(t, map[string]string{"site/index.html": "<h1>from url</h1>"}))
	}))
	defer srv.Close()
	out, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{
		ArchiveURL: srv.URL + "/site.tar.gz",
	})
	if err != nil {
		t.Fatalf("RemoteDeploy: %v", err)
	}
	if u, _ := deployOutput(t, out)["website_url"].(string); u == "" {
		t.Fatal("missing website_url")
	}
}

func TestLocalDeployRemoteDeployErrors(t *testing.T) {
	h, _, _ := newTestDeploy(t)
	ctx := context.Background()

	if _, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{}); err == nil {
		t.Error("empty request must fail")
	}
	if _, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{
		ArchiveURL: "http://x", ArchiveData: "AAAA",
	}); err == nil {
		t.Error("both sources must fail")
	}
	if _, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{ArchiveData: "%%%"}); err == nil {
		t.Error("invalid base64 must fail")
	}
	if _, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{ArchiveData: base64.StdEncoding.EncodeToString([]byte("junk"))}); err == nil {
		t.Error("unknown archive format must fail")
	}
	if _, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{ArchiveURL: "http://127.0.0.1:1/nope.tar.gz"}); err == nil {
		t.Error("unreachable URL must fail")
	}
}

func TestLocalDeployRemoteDeployAbsoluteURLWithDomain(t *testing.T) {
	root := t.TempDir()
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), matrix.DeployConfig{
		DataDir: root + "/data", WorkspaceDir: root + "/ws", Domain: "matrix.k0s.io",
	})
	out, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{
		ArchiveData: base64.StdEncoding.EncodeToString(zipBytes(t, map[string]string{"index.html": "x"})),
	})
	if err != nil {
		t.Fatalf("RemoteDeploy: %v", err)
	}
	u, _ := deployOutput(t, out)["website_url"].(string)
	if !strings.HasPrefix(u, "http://") || !strings.HasSuffix(u, ".matrix.k0s.io") {
		t.Errorf("website_url = %q, want http://<site>.matrix.k0s.io", u)
	}
}

// TestLocalDeployUploadToCDN publishes a server file and returns a
// reachable cdn_url.
func TestLocalDeployUploadToCDN(t *testing.T) {
	h, ws, data := newTestDeploy(t)
	writeTree(t, ws, map[string]string{"site.tar.gz": "archive-bytes"})

	out, err := h.UploadToCDN(context.Background(), &matrix.UploadToCDNRequest{
		FilePath: ws + "/site.tar.gz",
	})
	if err != nil {
		t.Fatalf("UploadToCDN: %v", err)
	}
	body := deployOutput(t, out)
	url, _ := body["cdn_url"].(string)
	if !strings.HasPrefix(url, "/data/") || !strings.HasSuffix(url, "/site.tar.gz") {
		t.Fatalf("cdn_url = %q, want /data/<site>/site.tar.gz", url)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(url, "/data/"), "/site.tar.gz")
	if got := readFile(t, filepath.Join(data, site, "site.tar.gz")); got != "archive-bytes" {
		t.Errorf("uploaded content = %q", got)
	}
}

func TestLocalDeployUploadToCDNErrors(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	if _, err := h.UploadToCDN(context.Background(), &matrix.UploadToCDNRequest{}); err == nil {
		t.Error("empty file_path must fail")
	}
	if _, err := h.UploadToCDN(context.Background(), &matrix.UploadToCDNRequest{FilePath: ws + "/missing.tar.gz"}); err == nil {
		t.Error("missing file must fail")
	}
	if _, err := h.UploadToCDN(context.Background(), &matrix.UploadToCDNRequest{FilePath: ws}); err == nil {
		t.Error("directory must fail")
	}
}

// TestLocalDeployRemoteDeployFromUploadedArchive runs the two-step flow:
// upload_to_cdn then remote_deploy with the returned /data/ path (read
// locally, no network).
func TestLocalDeployRemoteDeployFromUploadedArchive(t *testing.T) {
	h, ws, data := newTestDeploy(t)
	writeTree(t, ws, map[string]string{
		"site.tar.gz": string(tarGzBytes(t, map[string]string{"index.html": "<h1>cdn flow</h1>"})),
	})

	up, err := h.UploadToCDN(context.Background(), &matrix.UploadToCDNRequest{FilePath: ws + "/site.tar.gz"})
	if err != nil {
		t.Fatalf("UploadToCDN: %v", err)
	}
	cdnURL, _ := deployOutput(t, up)["cdn_url"].(string)

	out, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{ArchiveURL: cdnURL})
	if err != nil {
		t.Fatalf("RemoteDeploy: %v", err)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(deployOutput(t, out)["website_url"].(string), "/data/"), "/")
	if got := readFile(t, filepath.Join(data, site, "index.html")); got != "<h1>cdn flow</h1>" {
		t.Errorf("published index.html = %q", got)
	}
}

func TestLocalDeployRemoteDeployRejectsEscapingDataPath(t *testing.T) {
	h, _, _ := newTestDeploy(t)
	if _, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{
		ArchiveURL: "/data/../../etc/passwd",
	}); err == nil {
		t.Error("escaping /data path must fail")
	}
}

// TestLocalDeployUploadFile publishes base64 content from the caller.
func TestLocalDeployUploadFile(t *testing.T) {
	h, _, data := newTestDeploy(t)
	out, err := h.UploadFile(context.Background(), &matrix.UploadFileRequest{
		Data:     base64.StdEncoding.EncodeToString([]byte("local bytes")),
		Filename: "my.tar.gz",
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	url, _ := deployOutput(t, out)["cdn_url"].(string)
	if !strings.HasSuffix(url, "/my.tar.gz") {
		t.Fatalf("cdn_url = %q, want /data/<site>/my.tar.gz", url)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(url, "/data/"), "/my.tar.gz")
	if got := readFile(t, filepath.Join(data, site, "my.tar.gz")); got != "local bytes" {
		t.Errorf("uploaded content = %q", got)
	}
}

func TestLocalDeployUploadFileErrors(t *testing.T) {
	h, _, _ := newTestDeploy(t)
	if _, err := h.UploadFile(context.Background(), &matrix.UploadFileRequest{}); err == nil {
		t.Error("empty data must fail")
	}
	if _, err := h.UploadFile(context.Background(), &matrix.UploadFileRequest{Data: "%%%"}); err == nil {
		t.Error("invalid base64 must fail")
	}
	// path-traversal-looking filenames collapse to their base name.
	out, err := h.UploadFile(context.Background(), &matrix.UploadFileRequest{
		Data:     base64.StdEncoding.EncodeToString([]byte("x")),
		Filename: "../../evil.tar.gz",
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	url, _ := deployOutput(t, out)["cdn_url"].(string)
	if strings.Contains(url, "..") {
		t.Errorf("cdn_url must not contain traversal: %q", url)
	}
}

// TestLocalDeployLocalFileEndToEnd runs the full local-file flow: upload_file
// then remote_deploy via the returned /data/ path.
func TestLocalDeployLocalFileEndToEnd(t *testing.T) {
	h, _, data := newTestDeploy(t)
	up, err := h.UploadFile(context.Background(), &matrix.UploadFileRequest{
		Data:     base64.StdEncoding.EncodeToString(tarGzBytes(t, map[string]string{"index.html": "<h1>local end to end</h1>"})),
		Filename: "site.tar.gz",
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	cdnURL, _ := deployOutput(t, up)["cdn_url"].(string)

	out, err := h.RemoteDeploy(context.Background(), &matrix.RemoteDeployRequest{ArchiveURL: cdnURL})
	if err != nil {
		t.Fatalf("RemoteDeploy: %v", err)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(deployOutput(t, out)["website_url"].(string), "/data/"), "/")
	if got := readFile(t, filepath.Join(data, site, "index.html")); got != "<h1>local end to end</h1>" {
		t.Errorf("published index.html = %q", got)
	}
}

// TestDeploySchemeHttps checks that a configured https scheme lands in the
// absolute deploy and CDN URLs.
func TestDeploySchemeHttps(t *testing.T) {
	root := t.TempDir()
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), matrix.DeployConfig{
		DataDir: root + "/data", WorkspaceDir: root + "/ws",
		Domain: "matrix.k0s.io", Scheme: "https",
	})
	ctx := context.Background()

	writeTree(t, root+"/ws", map[string]string{"dist/index.html": "<h1>s</h1>"})
	out, err := h.Deploy(ctx, &matrix.DeployRequest{DistDir: root + "/ws/dist"})
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := deployOutput(t, out)["website_url"].(string); !strings.HasPrefix(u, "https://") {
		t.Errorf("deploy website_url = %q, want https", u)
	}
	up, err := h.UploadFile(ctx, &matrix.UploadFileRequest{
		Data: base64.StdEncoding.EncodeToString([]byte("x")), Filename: "a.tar.gz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := deployOutput(t, up)["cdn_url"].(string); !strings.HasPrefix(u, "https://") {
		t.Errorf("upload_file cdn_url = %q, want https", u)
	}
	rd, err := h.RemoteDeploy(ctx, &matrix.RemoteDeployRequest{
		ArchiveData: base64.StdEncoding.EncodeToString(zipBytes(t, map[string]string{"index.html": "x"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := deployOutput(t, rd)["website_url"].(string); !strings.HasPrefix(u, "https://") {
		t.Errorf("remote_deploy website_url = %q, want https", u)
	}
}
