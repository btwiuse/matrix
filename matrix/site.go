package matrix

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SiteHandler serves the deploy data directory over second-level domains:
// a project deployed as DataDir/<project> is reachable at
// http://<project>.<domain>/. The bare apex domain lists all deployed
// projects.
type SiteHandler struct {
	dataDir string
	domain  string
}

// NewSiteHandler returns a handler serving the projects deployed under
// dataDir at <project>.<domain>. The domain is matched case-insensitively
// and any port in the Host header is ignored.
func NewSiteHandler(dataDir, domain string) *SiteHandler {
	if dataDir == "" {
		dataDir = "."
	}
	if domain == "" {
		domain = "localhost"
	}
	return &SiteHandler{dataDir: dataDir, domain: strings.ToLower(domain)}
}

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
	http.FileServer(http.Dir(dir)).ServeHTTP(w, r)
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
