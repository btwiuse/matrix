package matrix

import (
	"net"
	"net/http"
	"strings"
)

// Router dispatches incoming requests by Host between the MCP endpoint and
// the site handler, letting one process serve both on a single port.
//
// The site namespace is the apex domain (the listing page at "/") and every
// <site>.<domain> subdomain; everything else is the MCP endpoint: bare IP
// addresses (the natural way MCP clients point at http://127.0.0.1:PORT),
// the apex at any non-root path (http://<domain>/mcp/...), and any unknown
// hostname.
type Router struct {
	mcp    http.Handler
	sites  *SiteHandler
	domain string
}

// NewRouter builds a Host-dispatching handler: requests inside the site
// namespace go to sites, all others to mcpHandler.
func NewRouter(mcpHandler http.Handler, sites *SiteHandler) *Router {
	return &Router{mcp: mcpHandler, sites: sites, domain: sites.Domain()}
}

// ServeHTTP routes by Host (port stripped, case-insensitive).
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)

	switch {
	case host == r.domain:
		if req.URL.Path == "/" {
			r.sites.ServeHTTP(w, req)
			return
		}
		r.mcp.ServeHTTP(w, req)
	case strings.HasSuffix(host, "."+r.domain):
		r.sites.ServeHTTP(w, req)
	default:
		r.mcp.ServeHTTP(w, req)
	}
}
