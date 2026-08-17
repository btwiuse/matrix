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
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gearshell/inject-proxy/matrix"
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
	)

	root := &cobra.Command{
		Use:   "matrix",
		Short: "MiniMax matrix MCP server replica",
		Long: "High-fidelity replica of the MiniMax matrix MCP server: the same 22 " +
			"tools with the exact same input schemas. Tool calls are forwarded to the " +
			"real backend when --url/--token are given (default behavior), otherwise a " +
			"local mock serves deterministic responses.",
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
			default: // auto
				if url != "" && token != "" {
					h, err := proxyHandler(url, token, source, timeout)
					if err != nil {
						return err
					}
					handler = h
				} else {
					handler = matrix.NewMockHandler()
					log.Printf("no --url/--token given, using mock handler")
				}
			}
			if dataDir != "" {
				handler = matrix.NewLocalDeploy(handler, matrix.DeployConfig{
					DataDir:      dataDir,
					WorkspaceDir: workspaceDir,
				})
				log.Printf("deploy writes assets under %s (workspace %s)", dataDir, workspaceDir)
			}

			server, err := matrix.NewServer(handler)
			if err != nil {
				return err
			}

			ctx := context.Background()
			if addr != "" {
				opts := &mcp.StreamableHTTPOptions{Stateless: true}
				h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, opts)
				log.Printf("matrix replica listening on %s (streamable HTTP)", addr)
				return http.ListenAndServe(addr, h)
			}

			return server.Run(ctx, &mcp.StdioTransport{})
		},
	}

	root.Flags().StringVar(&url, "url", os.Getenv("MATRIX_URL"), "real matrix MCP HTTP endpoint (default $MATRIX_URL)")
	root.Flags().StringVar(&token, "token", os.Getenv("MATRIX_SK"), "matrix sk token (default $MATRIX_SK)")
	root.Flags().StringVar(&source, "source", envOr("MATRIX_SOURCE", "hermes"), "source label (default $MATRIX_SOURCE or hermes)")
	root.Flags().StringVar(&mode, "mode", "auto", "handler mode: auto | proxy | mock")
	root.Flags().StringVar(&addr, "http", "", "if set, serve streamable HTTP on this address (e.g. :8080)")
	root.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "upstream request timeout for proxy mode")
	root.Flags().StringVar(&dataDir, "data-dir", os.Getenv("MATRIX_DATA_DIR"), "deploy writes assets under this directory (empty = deploy is forwarded/mocked like the rest)")
	root.Flags().StringVar(&workspaceDir, "workspace-dir", envOr("MATRIX_WORKSPACE", "/workspace"), "workspace root; deploy rejects dist_dir outside it")

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
	return matrix.NewProxyHandler(cfg), nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
