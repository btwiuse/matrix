package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalDeploy implements deploy locally: instead of uploading the dist
// directory to a remote web server, it copies the assets into
// DataDir/<project> (one subdirectory per project). A future HTTP server can
// then serve the data directory; until then the returned URL is the
// relative "/data/<project>/" path it will resolve to.
//
// Every other tool is delegated to the wrapped Handler, so LocalDeploy can
// wrap a mock or a proxy handler without duplicating the other 21 tools.
type LocalDeploy struct {
	Handler
	DataDir string
}

// NewLocalDeploy wraps h so that deploy writes assets into dataDir.
func NewLocalDeploy(h Handler, dataDir string) *LocalDeploy {
	return &LocalDeploy{Handler: h, DataDir: dataDir}
}

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
// previous release of the same project. It mirrors the real tool's contract:
// dist_dir defaults to "dist", project_name defaults to the dist directory
// name, and a missing index.html yields a warning instead of an error.
func (d *LocalDeploy) Deploy(ctx context.Context, in *DeployRequest) (Output, error) {
	if in == nil {
		return nil, fmt.Errorf("deploy: nil request")
	}
	dist := in.DistDir
	if dist == "" {
		dist = "dist"
	}
	info, err := os.Stat(dist)
	if err != nil {
		return nil, fmt.Errorf("deploy: dist_dir %q: %w", dist, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("deploy: dist_dir %q is not a directory", dist)
	}

	name := in.ProjectName
	if name == "" {
		name = filepath.Base(filepath.Clean(dist))
	}
	if !validDirName(name) {
		return nil, fmt.Errorf("deploy: invalid project_name %q", name)
	}
	if d.DataDir == "" {
		return nil, fmt.Errorf("deploy: data dir not configured")
	}
	target := filepath.Join(d.DataDir, name)

	// Deploying publishes a new release: clear the previous one so files
	// removed from the dist do not linger.
	if err := os.RemoveAll(target); err != nil {
		return nil, fmt.Errorf("deploy: clearing %s: %w", target, err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, fmt.Errorf("deploy: creating %s: %w", target, err)
	}

	st := copyStats{}
	if err := copyTree(dist, target, &st); err != nil {
		return nil, fmt.Errorf("deploy: copying %s: %w", dist, err)
	}

	out := map[string]any{
		"status":       "ok",
		"url":          "/data/" + name + "/",
		"project_name": name,
		"dist_dir":     dist,
		"data_dir":     target,
		"files":        st.files,
		"size_bytes":   st.bytes,
	}
	if in.ProjectType != "" {
		out["project_type"] = in.ProjectType
	}
	if st.indexHTML == 0 {
		out["warning"] = "no index.html found in dist; the site may not serve correctly"
	}
	return json.Marshal(out)
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
