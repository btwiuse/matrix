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

// serveSite serves one project's directory. With an injector configured,
// requests that resolve to a directory's index.html are rewritten before
// serving; everything else behaves exactly like http.FileServer (redirects,
// listings, ranges, explicit /index.html URLs).
func (s *SiteHandler) serveSite(w http.ResponseWriter, r *http.Request, dir string) {
	if s.injector == nil || !s.tryRewriteIndex(w, r, dir) {
		http.FileServer(http.Dir(dir)).ServeHTTP(w, r)
	}
}

// tryRewriteIndex takes over the response when the request resolves to a
// directory's index.html, serving it with the snippet injected. Only
// requests ending in "/" resolve to index.html: http.FileServer redirects
// explicit /index.html URLs and serves plain files directly.
func (s *SiteHandler) tryRewriteIndex(w http.ResponseWriter, r *http.Request, dir string) bool {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	if !strings.HasSuffix(upath, "/") {
		return false
	}
	full := filepath.Join(dir, filepath.FromSlash(path.Clean(upath)))
	// Guard against escaping the project directory: http.FileServer also
	// cleans the path, but a rewrite must never read outside the project.
	if rel, err := filepath.Rel(dir, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	fi, err := os.Stat(full)
	if err != nil || !fi.IsDir() {
		return false // not a directory: FileServer serves or 404s it
	}
	index := filepath.Join(full, "index.html")
	info, err := os.Stat(index)
	if err != nil {
		return false // no index.html: FileServer renders the listing
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
