package matrix_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gearshell/matrix/matrix"
)

// TestDeployAPIUploadPublishesSite posts a raw archive body and checks the
// deploy result plus the published files on disk.
func TestDeployAPIUploadPublishesSite(t *testing.T) {
	h, _, data := newTestDeploy(t)
	srv := httptest.NewServer(matrix.NewDeployAPI(h))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/deploy", "application/gzip",
		bytes.NewReader(tarGzBytes(t, map[string]string{"dist/index.html": "<h1>api</h1>"})))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	url, _ := body["website_url"].(string)
	if !strings.HasPrefix(url, "/data/") {
		t.Fatalf("website_url = %q", url)
	}
	site := strings.TrimSuffix(strings.TrimPrefix(url, "/data/"), "/")
	if got := readFile(t, filepath.Join(data, site, "index.html")); got != "<h1>api</h1>" {
		t.Errorf("published index.html = %q", got)
	}
}

func TestDeployAPIRejectsBadInput(t *testing.T) {
	h, _, _ := newTestDeploy(t)
	srv := httptest.NewServer(matrix.NewDeployAPI(h))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/deploy", "application/octet-stream",
		bytes.NewReader([]byte("not an archive")))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad archive: status = %d, want 400", res.StatusCode)
	}

	res2, err := http.Get(srv.URL + "/api/deploy")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want 405", res2.StatusCode)
	}
}
