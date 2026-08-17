// Command matrix runs the high-fidelity replica of the MiniMax matrix MCP
// server, built on the modelcontextprotocol/go-sdk.
//
// It serves the same 22 tools with the exact same input schemas as the real
// matrix MCP server. Tool calls are forwarded to the real backend when
// --url/--token are given (default behavior), otherwise a local mock serves
// deterministic responses.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gearshell/inject-proxy/matrix"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	var (
		url     = flag.String("url", os.Getenv("MATRIX_URL"), "real matrix MCP HTTP endpoint (default $MATRIX_URL)")
		token   = flag.String("token", os.Getenv("MATRIX_SK"), "matrix sk token (default $MATRIX_SK)")
		source  = flag.String("source", envOr("MATRIX_SOURCE", "hermes"), "source label (default $MATRIX_SOURCE or hermes)")
		mode    = flag.String("mode", "auto", "handler mode: auto | proxy | mock")
		addr    = flag.String("http", "", "if set, serve streamable HTTP on this address (e.g. :8080)")
		timeout = flag.Duration("timeout", 5*time.Minute, "upstream request timeout for proxy mode")
	)
	flag.Parse()

	var handler matrix.Handler
	switch *mode {
	case "mock":
		handler = matrix.NewMockHandler()
	case "proxy":
		handler = mustProxy(*url, *token, *source, *timeout)
	default: // auto
		if *url != "" && *token != "" {
			handler = mustProxy(*url, *token, *source, *timeout)
		} else {
			handler = matrix.NewMockHandler()
			log.Printf("no --url/--token given, using mock handler")
		}
	}

	server, err := matrix.NewServer(handler)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	if *addr != "" {
		opts := &mcp.StreamableHTTPOptions{Stateless: true}
		h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, opts)
		log.Printf("matrix replica listening on %s (streamable HTTP)", *addr)
		if err := http.ListenAndServe(*addr, h); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func mustProxy(url, token, source string, timeout time.Duration) matrix.Handler {
	if url == "" || token == "" {
		log.Fatalf("proxy mode requires both --url and --token")
	}
	cfg := matrix.ProxyConfig{
		URL:        url,
		Token:      token,
		Source:     source,
		HTTPClient: &http.Client{Timeout: timeout},
	}
	return matrix.NewProxyHandler(cfg)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
