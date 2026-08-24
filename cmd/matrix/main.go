// Command matrix runs the high-fidelity replica of the MiniMax matrix MCP
// server, built on the modelcontextprotocol/go-sdk. It serves the same 22
// tools with the exact same input schemas as the real matrix MCP server,
// over streamable HTTP (the only transport).
//
// With --data-dir set, the same process also hosts the deployed sites:
// requests are dispatched by Host — <site>.<domain> subdomains (and the
// apex listing at "/") serve the deploy output, everything else (the
// listen address, apex paths like /mcp/..., and unknown hostnames) serves
// the MCP endpoint. With --inject/--inject-html, every served index.html
// is rewritten with the snippet.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	htmlinject "github.com/gearshell/inject-proxy"
	"github.com/gearshell/inject-proxy/matrix"
	"github.com/gearshell/inject-proxy/rewrite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func main() {
	var (
		url          string
		token        string
		source       string
		mode         string
		addr         string
		timeout      time.Duration
		dataDir      string
		workspaceDir string
		domain       string
		scheme       string
		injectFile   string
		injectHTML   string
	)

	root := &cobra.Command{
		Use:   "matrix",
		Short: "MiniMax matrix MCP server replica",
		Long: "High-fidelity replica of the MiniMax matrix MCP server: the same 22 " +
			"tools with the exact same input schemas, over streamable HTTP. Tool calls " +
			"are forwarded to the real backend when --url/--token are given (default " +
			"behavior), otherwise a local mock serves deterministic responses. With " +
			"--data-dir the same process also hosts deployed sites by Host.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var handler matrix.Handler
			switch mode {
			case "mock":
				handler = matrix.NewMockHandler()
			case "proxy":
				h, err := proxyHandler(url, token, source, timeout)
				if err != nil {
					return err
				}
				handler = h
			case "auto":
				if url == "" || token == "" {
					handler = matrix.NewMockHandler()
					log.Printf("no --url/--token given, using mock handler")
					break
				}
				h, err := proxyHandler(url, token, source, timeout)
				if err != nil {
					return err
				}
				handler = h
			default:
				return fmt.Errorf("unknown mode %q (want auto, proxy or mock)", mode)
			}
			if dataDir != "" {
				if err := os.MkdirAll(dataDir, 0o755); err != nil {
					return fmt.Errorf("creating data dir %s: %w", dataDir, err)
				}
				handler = matrix.NewLocalDeploy(handler, matrix.DeployConfig{
					DataDir:      dataDir,
					WorkspaceDir: workspaceDir,
					Domain:       domain,
					Scheme:       scheme,
				})
				log.Printf("deploy writes assets under %s (workspace %s)", dataDir, workspaceDir)
			}

			server, err := matrix.NewServer(handler)
			if err != nil {
				return err
			}
			// /mcp/mini/message exposes only the replica extension tools
			// (remote_deploy, upload_file): a public publishing endpoint
			// without the real matrix tool surface.
			miniServer, err := matrix.NewMiniServer(handler)
			if err != nil {
				return err
			}
			opts := &mcp.StreamableHTTPOptions{
				Stateless: true,
				// The SDK's default 4 MiB cap would reject the base64 bodies
				// upload_file accepts (64 MiB decoded ~ 85 MiB encoded).
				MaxRequestBodyBytes: 96 << 20,
			}
			mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
				if r.URL.Path == "/mcp/mini/message" {
					return miniServer
				}
				return server
			}, opts)

			var h http.Handler = matrix.NewEnvelopeRewriter(mcpHandler)
			if dataDir != "" {
				injection, err := loadInjection(injectFile, injectHTML)
				if err != nil {
					return err
				}
				var inj *rewrite.Injector
				if strings.TrimSpace(injection) != "" {
					inj = rewrite.New(injection)
					log.Printf("served index.html pages rewritten with a %d byte snippet", len(injection))
				}
				sites := matrix.NewSiteHandlerWithInjector(dataDir, domain, inj)
				h = matrix.NewRouter(h, sites)
				log.Printf("serving deployed sites at http://<site>.%s/ (apex listing http://%s/)", sites.Domain(), sites.Domain())

				// POST /api/deploy: raw archive body -> deployed site.
				// Must run before the MCP/envelope handler.
				api := matrix.NewDeployAPI(handler.(*matrix.LocalDeploy))
				next := h
				h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/deploy" {
						api.ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			}

			log.Printf("matrix replica listening on %s (streamable HTTP; full tools at /mcp/message, publish-only tools at /mcp/mini/message)", addr)

			// /SKILL.md serves the embedded deploy-site SKILL.md on any host, so
			// the skill is reachable at a stable public URL (used by the
			// context-path sync in crushrc) without a separate CDN publish.
			skill := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/SKILL.md" {
					h.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
				w.Write(htmlinject.DeploySiteSkill)
			})
			// CORS first: every response allows cross-origin calls (the API
			// is meant to be called from anywhere, including browsers).
			var final http.Handler = matrix.NewCORS(skill)
			return http.ListenAndServe(addr, final)
		},
	}

	root.Flags().StringVar(&url, "url", os.Getenv("MATRIX_URL"), "real matrix MCP HTTP endpoint (default $MATRIX_URL)")
	root.Flags().StringVar(&token, "token", os.Getenv("MATRIX_SK"), "matrix sk token (default $MATRIX_SK)")
	root.Flags().StringVar(&source, "source", envOr("MATRIX_SOURCE", "hermes"), "source label (default $MATRIX_SOURCE or hermes)")
	root.Flags().StringVar(&mode, "mode", "auto", "handler mode: auto | proxy | mock")
	root.Flags().StringVar(&addr, "http", defaultHTTPAddr(), "listen address (default $PORT or :8080)")
	root.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "upstream request timeout for proxy mode")
	root.Flags().StringVar(&dataDir, "data-dir", os.Getenv("MATRIX_DATA_DIR"), "deploy writes assets under this directory and serves them by Host (empty = deploy is forwarded/mocked like the rest)")
	root.Flags().StringVar(&workspaceDir, "workspace-dir", envOr("MATRIX_WORKSPACE", "/workspace"), "workspace root; deploy rejects dist_dir outside it")
	root.Flags().StringVar(&domain, "domain", envOr("MATRIX_DOMAIN", "localhost"), "apex domain for deploy website_url and site hosting (default $MATRIX_DOMAIN or localhost)")
	root.Flags().StringVar(&scheme, "scheme", envOr("MATRIX_SCHEME", "http"), "URL scheme (http|https) for absolute deploy/CDN URLs (default $MATRIX_SCHEME or http)")
	root.Flags().StringVar(&injectFile, "inject", "", "HTML snippet file injected into served index.html pages")
	root.Flags().StringVar(&injectHTML, "inject-html", "", "inline HTML snippet injected into served index.html pages")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func proxyHandler(url, token, source string, timeout time.Duration) (matrix.Handler, error) {
	if url == "" || token == "" {
		return nil, fmt.Errorf("proxy mode requires both --url and --token")
	}
	cfg := matrix.ProxyConfig{
		URL:        url,
		Token:      token,
		Source:     source,
		HTTPClient: &http.Client{Timeout: timeout},
	}
	return matrix.NewProxyHandler(cfg)
}

// loadInjection resolves the injection snippet: the file wins over the
// inline value, mirroring inject-proxy's --inject/--inject-html flags.
func loadInjection(file, inline string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading inject file: %w", err)
		}
		return string(b), nil
	}
	return inline, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// defaultHTTPAddr returns the listen address: $PORT when set and
// parseable, otherwise :8080.
func defaultHTTPAddr() string {
	if a := listenAddr(os.Getenv("PORT")); a != "" {
		return a
	}
	return ":8080"
}

// listenAddr normalizes a PORT-style value into a ListenAndServe address:
// "8080" -> ":8080", ":8080" -> ":8080", "127.0.0.1:9000" -> unchanged.
// It returns "" for empty or unrecognized values.
func listenAddr(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, ":") {
		return p
	}
	if _, err := strconv.Atoi(p); err == nil {
		return ":" + p
	}
	if _, _, err := net.SplitHostPort(p); err == nil {
		return p
	}
	return ""
}
