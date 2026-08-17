package matrix_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gearshell/inject-proxy/matrix"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func deployOutput(t *testing.T, out matrix.Output) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("deploy output is not JSON: %v (raw: %s)", err, out)
	}
	return body
}

func TestLocalDeployCopiesAssets(t *testing.T) {
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())
	dist := t.TempDir()
	writeTree(t, dist, map[string]string{
		"index.html":       "<h1>hello</h1>",
		"assets/app.js":    "console.log('app')",
		"assets/style.css": "body{}",
	})

	out, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "myapp",
		DistDir:     dist,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body := deployOutput(t, out)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if url := body["url"]; url != "/data/myapp/" {
		t.Errorf("url = %v, want /data/myapp/", url)
	}
	if body["files"] != float64(3) {
		t.Errorf("files = %v, want 3", body["files"])
	}
	if _, ok := body["warning"]; ok {
		t.Errorf("unexpected warning: %v", body["warning"])
	}

	target := h.DataDir + "/myapp"
	if readFile(t, target+"/index.html") != "<h1>hello</h1>" {
		t.Errorf("index.html content mismatch")
	}
	if readFile(t, target+"/assets/app.js") != "console.log('app')" {
		t.Errorf("app.js content mismatch")
	}
	if readFile(t, target+"/assets/style.css") != "body{}" {
		t.Errorf("style.css content mismatch")
	}
}

func TestLocalDeployDefaultsProjectNameToDistBasename(t *testing.T) {
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())
	dist := t.TempDir() + "/my-dist"
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTree(t, dist, map[string]string{"index.html": "x"})

	out, err := h.Deploy(context.Background(), &matrix.DeployRequest{DistDir: dist})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body := deployOutput(t, out)
	if body["project_name"] != "my-dist" {
		t.Errorf("project_name = %v, want my-dist", body["project_name"])
	}
	if _, err := os.Stat(h.DataDir + "/my-dist/index.html"); err != nil {
		t.Errorf("asset not at data/my-dist: %v", err)
	}
}

func TestLocalDeployRejectsPathTraversal(t *testing.T) {
	data := t.TempDir()
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), data)
	dist := t.TempDir()
	writeTree(t, dist, map[string]string{"index.html": "x"})

	for _, name := range []string{"../evil", "/abs", "a/b", `a\b`} {
		_, err := h.Deploy(context.Background(), &matrix.DeployRequest{
			ProjectName: name,
			DistDir:     dist,
		})
		if err == nil {
			t.Errorf("project_name %q: expected error", name)
		}
	}
	// Nothing may have escaped the data dir: the parent of data must not
	// contain any of the attempted project names.
	parent := filepath.Dir(data)
	for _, name := range []string{"evil", "abs", "b"} {
		if _, err := os.Stat(filepath.Join(parent, name)); !os.IsNotExist(err) {
			t.Errorf("path traversal wrote outside data dir: %v (err=%v)", name, err)
		}
	}
}

func TestLocalDeploySkipsDevDirsAndSymlinks(t *testing.T) {
	data := t.TempDir()
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), data)
	dist := t.TempDir()
	writeTree(t, dist, map[string]string{
		"index.html":       "x",
		"node_modules/pkg": "should not copy",
		".git/config":      "should not copy",
		"src/app.ts":       "kept",
		"assets/icon.svg":  "kept",
	})
	// A symlink pointing outside the dist tree must be skipped.
	if err := os.Symlink(data, dist+"/escape"); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app",
		DistDir:     dist,
	}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	target := data + "/app"
	if _, err := os.Stat(target + "/node_modules"); !os.IsNotExist(err) {
		t.Errorf("node_modules was copied (err=%v)", err)
	}
	if _, err := os.Stat(target + "/.git"); !os.IsNotExist(err) {
		t.Errorf(".git was copied (err=%v)", err)
	}
	if _, err := os.Stat(target + "/escape"); !os.IsNotExist(err) {
		t.Errorf("symlink escape was copied (err=%v)", err)
	}
	if readFile(t, target+"/src/app.ts") != "kept" {
		t.Errorf("src/app.ts missing")
	}
}

func TestLocalDeployWarnsWithoutIndexHTML(t *testing.T) {
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())
	dist := t.TempDir()
	writeTree(t, dist, map[string]string{"robots.txt": "x"})

	out, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app",
		DistDir:     dist,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body := deployOutput(t, out)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok (warning, not error)", body["status"])
	}
	if body["warning"] == nil {
		t.Errorf("expected warning for missing index.html, got %v", body)
	}
}

func TestLocalDeployMissingDist(t *testing.T) {
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())
	_, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app",
		DistDir:     t.TempDir() + "/nope",
	})
	if err == nil {
		t.Fatal("expected error for missing dist_dir")
	}
}

func TestLocalDeployReplacesPreviousRelease(t *testing.T) {
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())

	dist1 := t.TempDir()
	writeTree(t, dist1, map[string]string{"index.html": "v1", "old.txt": "stale"})
	if _, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app", DistDir: dist1,
	}); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}

	dist2 := t.TempDir()
	writeTree(t, dist2, map[string]string{"index.html": "v2"})
	if _, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app", DistDir: dist2,
	}); err != nil {
		t.Fatalf("deploy v2: %v", err)
	}

	target := h.DataDir + "/app"
	if readFile(t, target+"/index.html") != "v2" {
		t.Errorf("index.html = %q, want v2", readFile(t, target+"/index.html"))
	}
	if _, err := os.Stat(target + "/old.txt"); !os.IsNotExist(err) {
		t.Errorf("old.txt from previous release still present (err=%v)", err)
	}
}

func TestLocalDeployOtherToolsDelegate(t *testing.T) {
	// Tools other than deploy must keep working through the wrapped handler.
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), t.TempDir())
	out, err := h.GetVoiceList(context.Background(), &matrix.GetVoiceListRequest{})
	if err != nil {
		t.Fatalf("GetVoiceList: %v", err)
	}
	if !json.Valid(out) {
		t.Errorf("delegated output is not JSON: %s", out)
	}
}
