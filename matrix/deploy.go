package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// ToolError makes a handler return a JSON tool error whose text matches the
// real matrix server: the JSON body goes into content[0].text and the result
// is flagged isError=true.
type ToolError struct{ JSON string }

func (e *ToolError) Error() string { return e.JSON }

func toolError(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding tool error: %w", err)
	}
	return &ToolError{JSON: string(b)}
}

// DeployConfig configures the local deploy implementation.
type DeployConfig struct {
	// DataDir is the root directory that deployed assets are copied into,
	// one subdirectory per project.
	DataDir string
	// WorkspaceDir is the sandbox root: dist_dir must be a sub-directory of
	// it, mirroring the real server's /workspace requirement.
	WorkspaceDir string
}

// LocalDeploy implements deploy locally with the same contract as the real
// matrix server: dist_dir must live under the workspace, and the result is
// {"website_id", "website_url", "screenshot_url"}. Instead of uploading to a
// CDN it copies the assets into DataDir/<project>; the website_url is the
// "/data/<project>/" path a future HTTP server over the data directory will
// serve.
//
// Every other tool is delegated to the wrapped Handler, so LocalDeploy can
// wrap a mock or a proxy handler without duplicating the other 21 tools.
type LocalDeploy struct {
	Handler
	cfg DeployConfig
	seq int64 // monotonically increasing website id sequence
}

// NewLocalDeploy wraps h so that deploy writes assets into cfg.DataDir.
func NewLocalDeploy(h Handler, cfg DeployConfig) *LocalDeploy {
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = "/workspace"
	}
	return &LocalDeploy{Handler: h, cfg: cfg}
}

// websiteIDBase keeps generated ids in the same magnitude as the real
// server's website ids (15 digits).
const websiteIDBase = int64(431000000000000)

// ignoredDirs are development directories skipped during deployment,
// matching the real server's behavior described in the deploy tool schema
// (".git, node_modules, etc. are automatically ignored").
var ignoredDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"__pycache__":  true,
}

// Deploy copies the dist directory into DataDir/<project>, replacing any
// previous release of the same project. The contract mirrors the real tool,
// verified against the live server: dist_dir defaults to <workspace>/dist,
// paths outside the workspace are rejected, missing dist yields the same
// JSON error shape, and a missing index.html does not produce a warning.
func (d *LocalDeploy) Deploy(ctx context.Context, in *DeployRequest) (Output, error) {
	if in == nil {
		return nil, toolError(map[string]string{"error": "nil request"})
	}

	dist := in.DistDir
	if dist == "" {
		dist = filepath.Join(d.cfg.WorkspaceDir, "dist")
	}
	abs, err := filepath.Abs(dist)
	if err != nil {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("resolving dist_dir: %v", err)})
	}
	if rel, err := filepath.Rel(d.cfg.WorkspaceDir, abs); err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return nil, toolError(map[string]string{"error": fmt.Sprintf(
			"dist_dir must be a sub-directory under %s, e.g. '%s/dist' or '%s/build'. Got: '%s'. Note: '%s' itself is not accepted — please append a sub-path (your built output directory, typically '%s/dist').",
			d.cfg.WorkspaceDir, d.cfg.WorkspaceDir, d.cfg.WorkspaceDir, abs, d.cfg.WorkspaceDir, d.cfg.WorkspaceDir)})
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, toolError(map[string]string{
			"error":   "dist directory does not exist",
			"message": fmt.Sprintf("Please ensure that the directory %s exists and contains built files.", abs),
		})
	}
	if !info.IsDir() {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("dist_dir %s is not a directory", abs)})
	}

	name := in.ProjectName
	if name == "" {
		name = filepath.Base(abs)
	}
	if !validDirName(name) {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("invalid project_name %q", name)})
	}
	if d.cfg.DataDir == "" {
		return nil, toolError(map[string]string{"error": "data dir not configured"})
	}
	target := filepath.Join(d.cfg.DataDir, name)

	// Deploying publishes a new release: clear the previous one so files
	// removed from the dist do not linger.
	if err := os.RemoveAll(target); err != nil {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("clearing %s: %v", target, err)})
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("creating %s: %v", target, err)})
	}
	st := copyStats{}
	if err := copyTree(abs, target, &st); err != nil {
		return nil, toolError(map[string]string{"error": fmt.Sprintf("copying %s: %v", abs, err)})
	}

	id := atomic.AddInt64(&d.seq, 1) + websiteIDBase
	// website_url is the "/data/<project>/" path the future data HTTP
	// server will serve; swap in the absolute URL once that exists.
	return json.Marshal(map[string]any{
		"website_id":     id,
		"website_url":    "/data/" + name + "/",
		"screenshot_url": "",
	})
}

// validDirName rejects project names that could escape the data directory.
func validDirName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

type copyStats struct {
	files     int
	bytes     int64
	indexHTML int
}

// copyTree recursively copies src into dst, skipping development
// directories and never following symlinks (so the copy cannot escape the
// dist tree).
func copyTree(src, dst string, st *copyStats) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && ignoredDirs[e.Name()] {
			continue
		}
		s := filepath.Join(src, e.Name())
		t := filepath.Join(dst, e.Name())
		switch {
		case e.Type()&os.ModeSymlink != 0:
			continue
		case e.IsDir():
			if err := os.MkdirAll(t, 0o755); err != nil {
				return err
			}
			if err := copyTree(s, t, st); err != nil {
				return err
			}
		default:
			if err := copyFile(s, t, st); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, st *copyStats) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if filepath.Base(dst) == "index.html" {
		st.indexHTML++
	}
	st.files++
	st.bytes += n
	return nil
}
