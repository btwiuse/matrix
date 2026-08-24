package matrix

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ToolError makes a handler return a JSON tool error whose text matches the
// real matrix server: the JSON body goes into content[0].text. IsError
// mirrors the real server's per-error result flag (true for validation
// errors, false for the missing-dist case).
type ToolError struct {
	JSON    string
	IsError bool
}

func (e *ToolError) Error() string { return e.JSON }

func toolError(v any) error {
	return &ToolError{JSON: pyJSON(v), IsError: true}
}

// softToolError is a tool error the real server returns as a regular
// (non-error) result: the JSON body still carries the error text in
// content[0].text, but isError stays false.
func softToolError(v any) error {
	return &ToolError{JSON: pyJSON(v)}
}

// pyJSON renders v like Python's json.dumps with default separators
// (", " between items, ": " between keys and values), matching the text
// the real matrix server puts into content[0].text. Map keys are sorted
// like Python's json.dumps sorts dict keys.
func pyJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		return string(b)
	}
	var sb strings.Builder
	pyWrite(&sb, x)
	return sb.String()
}

func pyWrite(b *strings.Builder, x any) {
	switch t := x.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		q, _ := json.Marshal(t)
		b.Write(q)
	case float64:
		q, _ := json.Marshal(t)
		b.Write(q)
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteString(", ")
			}
			pyWrite(b, e)
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			q, _ := json.Marshal(k)
			b.Write(q)
			b.WriteString(": ")
			pyWrite(b, t[k])
		}
		b.WriteByte('}')
	default:
		q, _ := json.Marshal(t)
		b.Write(q)
	}
}

// toolErr is shorthand for a tool error whose body is {"error": message}.
func toolErr(format string, args ...any) error {
	return toolError(map[string]string{"error": fmt.Sprintf(format, args...)})
}

// DeployConfig configures the local deploy implementation.
type DeployConfig struct {
	// DataDir is the root directory that deployed assets are copied into,
	// one subdirectory per published site.
	DataDir string
	// WorkspaceDir is the sandbox root: dist_dir must be a sub-directory of
	// it, mirroring the real server's /workspace requirement.
	WorkspaceDir string
	// Domain, when set, makes website_url an absolute <scheme>://<site>.<domain>/
	// URL (like the real server's per-deployment subdomain) instead of the
	// relative /data/<site>/ path.
	Domain string
	// Scheme is the URL scheme used for absolute URLs (default "http";
	// "https" when the server is served behind TLS).
	Scheme string
}

// LocalDeploy implements deploy locally with the same contract as the real
// matrix server: dist_dir must live under the workspace, and the result is
// {"website_id", "website_url", "screenshot_url"}. Instead of uploading to a
// CDN it copies the assets into a fresh DataDir/<random-id> directory per
// deployment, mirroring the real server's per-deployment random subdomain:
// previous releases are kept, never overwritten. website_url is either the
// relative "/data/<random-id>/" path or, when a Domain is configured,
// "http://<random-id>.<domain>/".
//
// Every other tool is delegated to the wrapped Handler, so LocalDeploy can
// wrap a mock or a proxy handler without duplicating the other 21 tools.
type LocalDeploy struct {
	Handler
	cfg DeployConfig
}

// NewLocalDeploy wraps h so that deploy writes assets into cfg.DataDir.
func NewLocalDeploy(h Handler, cfg DeployConfig) *LocalDeploy {
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = "/workspace"
	}
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}
	return &LocalDeploy{Handler: h, cfg: cfg}
}

// siteURL renders the absolute URL of a published site: the random id as
// subdomain of the configured domain, with the configured scheme.
func (d *LocalDeploy) siteURL(site string) string {
	return d.cfg.Scheme + "://" + site + "." + d.cfg.Domain
}

// websiteIDBase is the fixed 3-digit prefix of the real server's website
// ids: they are 15-digit numbers in [431000000000000, 431999999999999]
// whose remaining 12 digits are random per deployment, not sequential.
const websiteIDBase = int64(431000000000000)

// newWebsiteID returns a website id in the real server's range: the fixed
// 431 prefix plus 12 random digits.
func newWebsiteID() int64 {
	return websiteIDBase + rand.Int64N(1_000_000_000_000)
}

// siteIDChars mirrors the charset of the real server's random subdomains.
const siteIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// newSiteID returns a random lowercase alphanumeric id like the one the
// real server embeds in each deployment's website_url.
func newSiteID(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = siteIDChars[rand.IntN(len(siteIDChars))]
	}
	return string(b)
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

// Deploy copies the dist directory into a fresh DataDir/<random-id>
// directory, one per deployment, mirroring the real server's per-deployment
// random subdomain. The contract otherwise mirrors the real tool, verified
// against the live server: dist_dir defaults to <workspace>/dist, paths
// outside the workspace are rejected, missing dist yields the same JSON
// error shape, and a missing index.html does not produce a warning.
func (d *LocalDeploy) Deploy(ctx context.Context, in *DeployRequest) (Output, error) {
	if in == nil {
		return nil, toolErr("nil request")
	}

	dist := in.DistDir
	if dist == "" {
		dist = filepath.Join(d.cfg.WorkspaceDir, "dist")
	}
	abs, err := filepath.Abs(dist)
	if err != nil {
		return nil, toolErr("resolving dist_dir: %v", err)
	}
	if rel, err := filepath.Rel(d.cfg.WorkspaceDir, abs); err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		// The trailing slash after the workspace path mirrors the real
		// server's wording verbatim ("under /workspace/").
		return nil, toolErr(
			"dist_dir must be a sub-directory under %s/, e.g. '%s/dist' or '%s/build'. Got: '%s'. Note: '%s' itself is not accepted — please append a sub-path (your built output directory, typically '%s/dist').",
			d.cfg.WorkspaceDir, d.cfg.WorkspaceDir, d.cfg.WorkspaceDir, abs, d.cfg.WorkspaceDir, d.cfg.WorkspaceDir)
	}
	info, err := os.Stat(abs)
	if err != nil {
		// The real server appends the file gateway's 404 detail to the
		// message; locally the os.Stat failure is the equivalent detail.
		// The real server returns this case with isError=false, so use the
		// soft error to match.
		return nil, softToolError(map[string]string{
			"error":   "dist directory does not exist",
			"message": fmt.Sprintf("Please ensure that the directory %s exists and contains built files. Error: %v", abs, err),
		})
	}
	if !info.IsDir() {
		return nil, toolErr("dist_dir %s is not a directory", abs)
	}

	// project_name is ignored entirely, like on the real server (even
	// path-traversal-looking values deploy fine and do not determine the
	// published location).
	if d.cfg.DataDir == "" {
		return nil, toolErr("data dir not configured")
	}
	// Publishing a site is append-only, like the real server: every
	// deployment gets its own directory and URL, previous releases stay.
	site := newSiteID(12)
	target := filepath.Join(d.cfg.DataDir, site)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, toolErr("creating %s: %v", target, err)
	}
	st := copyStats{}
	if err := copyTree(abs, target, &st); err != nil {
		os.RemoveAll(target)
		return nil, toolErr("copying %s: %v", abs, err)
	}

	id := newWebsiteID()
	// website_url mirrors the real server's per-deployment URL: the site id
	// becomes the subdomain, or the path when no domain is configured. The
	// absolute form carries no trailing slash, like the real server's
	// https://<site>.space.mcode.cn.
	url := "/data/" + site + "/"
	if d.cfg.Domain != "" {
		url = d.siteURL(site)
	}
	return deploySuccess(id, url), nil
}

// RemoteDeploy publishes a site from an uploaded archive (.tar.gz or .zip)
// instead of a workspace directory: the server downloads ArchiveURL or
// decodes ArchiveData (base64), unpacks it and publishes the result exactly
// like Deploy. This is the extension that makes deploy usable through a
// public server without any files on the server side.
func (d *LocalDeploy) RemoteDeploy(ctx context.Context, in *RemoteDeployRequest) (Output, error) {
	if in == nil {
		return nil, toolErr("nil request")
	}
	if d.cfg.DataDir == "" {
		return nil, toolErr("data dir not configured")
	}
	if in.ArchiveURL != "" && in.ArchiveData != "" {
		return nil, toolErr("provide either archive_url or archive_data, not both")
	}
	if in.ArchiveURL == "" && in.ArchiveData == "" {
		return nil, toolErr("provide either archive_url or archive_data")
	}

	data, err := fetchArchive(ctx, in, d.cfg.DataDir)
	if err != nil {
		return nil, toolErr("%v", err)
	}
	if len(data) == 0 {
		return nil, toolErr("archive is empty")
	}

	root, err := extractArchive(os.TempDir(), data)
	if err != nil {
		return nil, toolErr("extracting archive: %v", err)
	}
	defer os.RemoveAll(root)

	site := newSiteID(12)
	target := filepath.Join(d.cfg.DataDir, site)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, toolErr("creating %s: %v", target, err)
	}
	st := copyStats{}
	if err := copyTree(root, target, &st); err != nil {
		os.RemoveAll(target)
		return nil, toolErr("copying %s: %v", root, err)
	}

	url := "/data/" + site + "/"
	if d.cfg.Domain != "" {
		url = d.siteURL(site)
	}
	return deploySuccess(newWebsiteID(), url), nil
}

// fetchArchive resolves the archive bytes from the request: a /data/.../
// path published by upload_to_cdn (read locally, no network), a downloaded
// URL (capped at 64 MiB), or a base64-encoded body.
func fetchArchive(ctx context.Context, in *RemoteDeployRequest, dataDir string) ([]byte, error) {
	if in.ArchiveURL != "" {
		if strings.HasPrefix(in.ArchiveURL, "/data/") {
			if dataDir == "" {
				return nil, fmt.Errorf("data dir not configured")
			}
			full := filepath.Join(dataDir, filepath.FromSlash(strings.TrimPrefix(in.ArchiveURL, "/data/")))
			if rel, err := filepath.Rel(dataDir, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("archive path %q escapes the data dir", in.ArchiveURL)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return nil, fmt.Errorf("reading uploaded archive: %v", err)
			}
			if len(data) > 64<<20 {
				return nil, fmt.Errorf("archive exceeds 64 MiB")
			}
			return data, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.ArchiveURL, nil)
		if err != nil {
			return nil, fmt.Errorf("parsing archive_url: %v", err)
		}
		resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
		if err != nil {
			return nil, fmt.Errorf("downloading archive: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("downloading archive: HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20+1))
		if err != nil {
			return nil, fmt.Errorf("reading archive: %v", err)
		}
		if len(data) > 64<<20 {
			return nil, fmt.Errorf("archive exceeds 64 MiB")
		}
		return data, nil
	}
	data, err := base64.StdEncoding.DecodeString(in.ArchiveData)
	if err != nil {
		return nil, fmt.Errorf("archive_data is not valid base64: %v", err)
	}
	return data, nil
}

// UploadToCDN uploads a file from the server (any absolute path) to the
// local CDN: the file is published under a fresh random subdomain and the
// returned cdn_url is publicly reachable. This makes files on the server
// usable by external services, matching the real tool's contract ("Local
// file paths are NOT accessible outside the agent environment. Only CDN
// URLs can be accessed by external services.").
func (d *LocalDeploy) UploadToCDN(_ context.Context, in *UploadToCDNRequest) (Output, error) {
	if in == nil {
		return nil, toolErr("nil request")
	}
	if d.cfg.DataDir == "" {
		return nil, toolErr("data dir not configured")
	}
	if in.FilePath == "" {
		return nil, toolErr("file_path is required")
	}
	info, err := os.Stat(in.FilePath)
	if err != nil {
		return nil, toolErr("file %s does not exist: %v", in.FilePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, toolErr("file %s is not a regular file", in.FilePath)
	}
	if info.Size() > 64<<20 {
		return nil, toolErr("file %s exceeds 64 MiB", in.FilePath)
	}

	site := newSiteID(12)
	target := filepath.Join(d.cfg.DataDir, site)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, toolErr("creating %s: %v", target, err)
	}
	name := filepath.Base(in.FilePath)
	if err := copyFile(in.FilePath, filepath.Join(target, name), &copyStats{}); err != nil {
		os.RemoveAll(target)
		return nil, toolErr("uploading %s: %v", in.FilePath, err)
	}

	url := "/data/" + site + "/" + name
	if d.cfg.Domain != "" {
		url = d.siteURL(site) + "/" + name
	}
	return mockOutput(map[string]any{"status": "ok", "cdn_url": url})
}

// UploadFile uploads base64 file content from the caller to the local CDN:
// the bytes are stored under a fresh random subdomain and the returned
// cdn_url is publicly reachable. This is the counterpart of UploadToCDN for
// clients that hold the file locally (upload_to_cdn only takes server-side
// paths, like on the real server).
func (d *LocalDeploy) UploadFile(_ context.Context, in *UploadFileRequest) (Output, error) {
	if in == nil {
		return nil, toolErr("nil request")
	}
	if d.cfg.DataDir == "" {
		return nil, toolErr("data dir not configured")
	}
	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return nil, toolErr("data is not valid base64: %v", err)
	}
	if len(data) == 0 {
		return nil, toolErr("data is empty")
	}
	if len(data) > 64<<20 {
		return nil, toolErr("file exceeds 64 MiB")
	}
	name := filepath.Base(in.Filename)
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		name = "file"
	}

	site := newSiteID(12)
	target := filepath.Join(d.cfg.DataDir, site)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, toolErr("creating %s: %v", target, err)
	}
	if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
		os.RemoveAll(target)
		return nil, toolErr("writing %s: %v", name, err)
	}

	url := "/data/" + site + "/" + name
	if d.cfg.Domain != "" {
		url = d.siteURL(site) + "/" + name
	}
	return mockOutput(map[string]any{"status": "ok", "cdn_url": url})
}

// deploySuccess renders the deploy result exactly like the real server:
// Python-style JSON (space after colon) with the insertion order
// website_id, website_url, screenshot_url.
func deploySuccess(id int64, url string) []byte {
	return []byte(fmt.Sprintf(`{"website_id": %d, "website_url": %q, "screenshot_url": ""}`, id, url))
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
