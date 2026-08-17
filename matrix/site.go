package matrix

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gearshell/inject-proxy/rewrite"
)

// SiteHandler serves the deploy data directory over second-level domains:
// a project deployed as DataDir/<project> is reachable at
// http://<project>.<domain>/. The bare apex domain lists all deployed
// projects.
type SiteHandler struct {
	dataDir string
	domain  string
	// injector, when set, rewrites every served index.html with the
	// snippet, reusing inject-proxy's idempotent HTML rewrite core.
	injector *rewrite.Injector
}

// NewSiteHandler returns a handler serving the projects deployed under
// dataDir at <project>.<domain>. The domain is matched case-insensitively
// and any port in the Host header is ignored.
func NewSiteHandler(dataDir, domain string) *SiteHandler {
	return NewSiteHandlerWithInjector(dataDir, domain, nil)
}

// NewSiteHandlerWithInjector is NewSiteHandler with optional index
// rewriting: when inj is non-nil, every request that resolves to a
// directory's index.html is served with the snippet injected (see
// SiteHandler). A nil injector serves files untouched.
func NewSiteHandlerWithInjector(dataDir, domain string, inj *rewrite.Injector) *SiteHandler {
	if dataDir == "" {
		dataDir = "."
	}
	if domain == "" {
		domain = "localhost"
	}
	return &SiteHandler{dataDir: dataDir, domain: strings.ToLower(domain), injector: inj}
}

// Domain returns the normalized apex domain this handler routes on.
func (s *SiteHandler) Domain() string { return s.domain }

// ServeHTTP routes by Host: <project>.<domain> serves that project's
// directory; the bare domain serves a listing of all deployed projects;
// anything else is a 404.
func (s *SiteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)

	if host == s.domain {
		s.serveApex(w, r)
		return
	}
	if !strings.HasSuffix(host, "."+s.domain) {
		http.NotFound(w, r)
		return
	}
	project := host[:len(host)-len(s.domain)-1]
	// Only one level of subdomain is meaningful: a project name with a dot
	// would be a deeper subdomain, not a deployed project.
	if project == "" || strings.Contains(project, ".") || !validDirName(project) {
		http.NotFound(w, r)
		return
	}
	dir := filepath.Join(s.dataDir, project)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		http.NotFound(w, r)
		return
	}
	s.serveSite(w, r, dir)
}

// serveSite serves one project's directory, mirroring the real server's
// file gateway: requests resolving to a directory's index.html serve it
// (rewritten when an injector is configured), existing files serve as-is,
// directories without index.html and missing paths are 404s, and missing
// extensionless paths fall back to index.html (SPA fallback).
func (s *SiteHandler) serveSite(w http.ResponseWriter, r *http.Request, dir string) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	clean := path.Clean(upath)
	full := filepath.Join(dir, filepath.FromSlash(clean))
	// Guard against escaping the project directory: path traversal must
	// never read outside the project.
	if rel, err := filepath.Rel(dir, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	fi, err := os.Stat(full)
	switch {
	case err != nil:
		// Missing path: the real server falls back to index.html for
		// extensionless non-directory paths, and 404s everything else
		// (paths with an extension, and directory-looking paths).
		if !strings.HasSuffix(upath, "/") && path.Ext(clean) == "" {
			if s.serveIndex(w, r, dir) {
				return
			}
		}
		http.NotFound(w, r)
	case fi.IsDir():
		if s.serveIndex(w, r, full) {
			return
		}
		http.NotFound(w, r) // no index.html: the real server 404s, no listing
	default:
		// Existing plain file. Explicit /index.html URLs are served
		// directly (no redirect), rewritten when an injector is configured.
		if s.injector != nil && path.Base(clean) == "index.html" {
			if s.serveIndex(w, r, filepath.Dir(full)) {
				return
			}
		}
		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	}
}

// serveIndex serves dir/index.html: rewritten with the injector when one
// is configured (with Content-Type, Last-Modified and Content-Length set,
// HEAD getting an empty body), otherwise verbatim through ServeContent. It
// reports whether the request was taken over (index.html existed); when it
// returns false the caller should 404.
func (s *SiteHandler) serveIndex(w http.ResponseWriter, r *http.Request, dir string) bool {
	index := filepath.Join(dir, "index.html")
	info, err := os.Stat(index)
	if err != nil || info.IsDir() {
		return false
	}
	if s.injector == nil {
		f, err := os.Open(index)
		if err != nil {
			http.NotFound(w, r)
			return true
		}
		defer f.Close()
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		return true
	}
	b, err := os.ReadFile(index)
	if err != nil {
		http.NotFound(w, r)
		return true
	}
	out, _ := s.injector.Inject(string(b))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	if r.Method != http.MethodHead {
		io.WriteString(w, out)
	}
	return true
}

// serveApex renders a small directory of the deployed projects, each
// linked to its own subdomain.
func (s *SiteHandler) serveApex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}
	sort.Strings(projects)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>matrix sites</title></head><body>")
	fmt.Fprint(w, "<h1>matrix sites</h1>")
	if len(projects) == 0 {
		fmt.Fprint(w, "<p>no sites deployed yet</p>")
	}
	fmt.Fprint(w, "<ul>")
	for _, p := range projects {
		fmt.Fprintf(w, "<li><a href=\"http://%s.%s/\">%s</a></li>", p, s.domain, html.EscapeString(p))
	}
	fmt.Fprint(w, "</ul></body></html>")
}
