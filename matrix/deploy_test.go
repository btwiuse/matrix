package matrix_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gearshell/inject-proxy/matrix"
)

// newTestDeploy builds a LocalDeploy with an isolated workspace+data dir.
func newTestDeploy(t *testing.T) (*matrix.LocalDeploy, string, string) {
	t.Helper()
	root := t.TempDir()
	ws := root + "/workspace"
	data := root + "/data"
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	h := matrix.NewLocalDeploy(matrix.NewMockHandler(), matrix.DeployConfig{
		DataDir:      data,
		WorkspaceDir: ws,
	})
	return h, ws, data
}

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

// deployErr asserts the returned error is a JSON tool error and returns its
// parsed body.
func deployErr(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatal("expected tool error, got nil")
	}
	var te *matrix.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("error is %T, want *matrix.ToolError: %v", err, err)
	}
	return deployOutput(t, []byte(te.JSON))
}

func TestLocalDeployCopiesAssets(t *testing.T) {
	h, ws, data := newTestDeploy(t)
	dist := ws + "/myapp-dist"
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
	// Output shape must match the real server: website_id/website_url/screenshot_url.
	if _, ok := body["website_id"].(float64); !ok {
		t.Errorf("website_id = %v (%T), want number", body["website_id"], body["website_id"])
	}
	if url := body["website_url"]; url != "/data/myapp/" {
		t.Errorf("website_url = %v, want /data/myapp/", url)
	}
	if ss := body["screenshot_url"]; ss != "" {
		t.Errorf("screenshot_url = %v, want empty", ss)
	}
	for _, k := range []string{"status", "url", "warning", "files", "size_bytes"} {
		if _, ok := body[k]; ok {
			t.Errorf("unexpected key %q in output (must match real shape): %v", k, body)
		}
	}

	target := data + "/myapp"
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
	h, ws, data := newTestDeploy(t)
	dist := ws + "/my-dist"
	writeTree(t, dist, map[string]string{"index.html": "x"})

	out, err := h.Deploy(context.Background(), &matrix.DeployRequest{DistDir: dist})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := os.Stat(data + "/my-dist/index.html"); err != nil {
		t.Errorf("asset not at data/my-dist: %v", err)
	}
	if ss := deployOutput(t, out)["screenshot_url"]; ss != "" {
		t.Errorf("screenshot_url = %v, want empty", ss)
	}
}

func TestLocalDeployRejectsDistOutsideWorkspace(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	dist := t.TempDir() + "/outside"
	writeTree(t, dist, map[string]string{"index.html": "x"})

	_, err := h.Deploy(context.Background(), &matrix.DeployRequest{DistDir: dist})
	body := deployErr(t, err)
	msg, _ := body["error"].(string)
	if msg == "" || !strings.Contains(msg, "must be a sub-directory under "+ws) {
		t.Errorf("error message = %q, want workspace constraint", msg)
	}
}

func TestLocalDeployRejectsWorkspaceItself(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	// The workspace root itself is not accepted, like the real server.
	_, err := h.Deploy(context.Background(), &matrix.DeployRequest{DistDir: ws})
	body := deployErr(t, err)
	if msg, _ := body["error"].(string); !strings.Contains(msg, "not accepted") {
		t.Errorf("error message = %q, want 'not accepted'", msg)
	}
}

func TestLocalDeployMissingDist(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	_, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app",
		DistDir:     ws + "/nope",
	})
	body := deployErr(t, err)
	if body["error"] != "dist directory does not exist" {
		t.Errorf("error = %v, want 'dist directory does not exist'", body["error"])
	}
	if body["message"] == nil {
		t.Errorf("expected message key in error body, got %v", body)
	}
}

func TestLocalDeployDefaultsDistDirToWorkspaceDist(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	// Missing dist_dir defaults to <workspace>/dist, like the real server.
	_, err := h.Deploy(context.Background(), &matrix.DeployRequest{ProjectName: "app"})
	body := deployErr(t, err)
	if body["error"] != "dist directory does not exist" {
		t.Errorf("error = %v, want 'dist directory does not exist' (default <workspace>/dist)", body["error"])
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, ws+"/dist") {
		t.Errorf("message = %q, want default path %s/dist", msg, ws)
	}
}

func TestLocalDeployRejectsPathTraversal(t *testing.T) {
	h, ws, data := newTestDeploy(t)
	dist := ws + "/app"
	writeTree(t, dist, map[string]string{"index.html": "x"})

	for _, name := range []string{"../evil", "/abs", "a/b", `a\b`} {
		_, err := h.Deploy(context.Background(), &matrix.DeployRequest{
			ProjectName: name,
			DistDir:     dist,
		})
		deployErr(t, err)
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
	h, ws, data := newTestDeploy(t)
	dist := ws + "/app"
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

func TestLocalDeployNoIndexHTMLSucceedsWithoutWarning(t *testing.T) {
	h, ws, _ := newTestDeploy(t)
	dist := ws + "/app"
	writeTree(t, dist, map[string]string{"robots.txt": "x"})

	// The real server deploys successfully with no warning when index.html
	// is missing; the replica must behave identically.
	out, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app",
		DistDir:     dist,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body := deployOutput(t, out)
	if _, ok := body["warning"]; ok {
		t.Errorf("unexpected warning key: %v", body)
	}
	if _, ok := body["website_id"]; !ok {
		t.Errorf("expected website_id, got %v", body)
	}
}

func TestLocalDeployReplacesPreviousRelease(t *testing.T) {
	h, ws, data := newTestDeploy(t)

	dist1 := ws + "/app-v1"
	writeTree(t, dist1, map[string]string{"index.html": "v1", "old.txt": "stale"})
	if _, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app", DistDir: dist1,
	}); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}

	dist2 := ws + "/app-v2"
	writeTree(t, dist2, map[string]string{"index.html": "v2"})
	if _, err := h.Deploy(context.Background(), &matrix.DeployRequest{
		ProjectName: "app", DistDir: dist2,
	}); err != nil {
		t.Fatalf("deploy v2: %v", err)
	}

	target := data + "/app"
	if readFile(t, target+"/index.html") != "v2" {
		t.Errorf("index.html = %q, want v2", readFile(t, target+"/index.html"))
	}
	if _, err := os.Stat(target + "/old.txt"); !os.IsNotExist(err) {
		t.Errorf("old.txt from previous release still present (err=%v)", err)
	}
}

func TestLocalDeployFreshWebsiteIDPerDeployment(t *testing.T) {
	h, ws, _ := newTestDeploy(t)

	dist1 := ws + "/app-v1"
	writeTree(t, dist1, map[string]string{"index.html": "v1"})
	out1, err := h.Deploy(context.Background(), &matrix.DeployRequest{ProjectName: "app", DistDir: dist1})
	if err != nil {
		t.Fatalf("deploy 1: %v", err)
	}
	dist2 := ws + "/app-v2"
	writeTree(t, dist2, map[string]string{"index.html": "v2"})
	out2, err := h.Deploy(context.Background(), &matrix.DeployRequest{ProjectName: "app", DistDir: dist2})
	if err != nil {
		t.Fatalf("deploy 2: %v", err)
	}
	id1 := deployOutput(t, out1)["website_id"].(float64)
	id2 := deployOutput(t, out2)["website_id"].(float64)
	if id1 == id2 {
		t.Errorf("website_id did not change between deployments: %v", id1)
	}
	if id2 <= id1 {
		t.Errorf("website_id not increasing: %v -> %v", id1, id2)
	}
}

func TestLocalDeployOtherToolsDelegate(t *testing.T) {
	// Tools other than deploy must keep working through the wrapped handler.
	h, _, _ := newTestDeploy(t)
	out, err := h.GetVoiceList(context.Background(), &matrix.GetVoiceListRequest{})
	if err != nil {
		t.Fatalf("GetVoiceList: %v", err)
	}
	if !json.Valid(out) {
		t.Errorf("delegated output is not JSON: %s", out)
	}
}
