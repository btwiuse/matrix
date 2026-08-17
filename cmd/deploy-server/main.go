// Command deploy-server serves the matrix deploy data directory over
// second-level domains: every project deployed by cmd/matrix --data-dir is
// reachable at http://<project>.<domain>/, backed by DataDir/<project>.
// The bare apex domain lists all deployed sites.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gearshell/inject-proxy/matrix"
	"github.com/spf13/cobra"
)

func main() {
	var (
		dataDir string
		domain  string
		addr    string
	)

	root := &cobra.Command{
		Use:   "deploy-server",
		Short: "serve matrix deploy output over subdomains",
		Long: "Serves the data directory written by the matrix deploy tool at " +
			"http://<project>.<domain>/: the project name becomes the second-level " +
			"domain, and the apex domain lists every deployed site.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("creating data dir %s: %w", dataDir, err)
			}
			h := matrix.NewSiteHandler(dataDir, domain)
			log.Printf("serving %s at http://<project>.%s/ (apex http://%s/)", dataDir, domain, domain)
			return http.ListenAndServe(addr, h)
		},
	}

	root.Flags().StringVar(&dataDir, "data-dir", envOr("MATRIX_DATA_DIR", "./data"), "deploy data directory (default $MATRIX_DATA_DIR or ./data)")
	root.Flags().StringVar(&domain, "domain", envOr("MATRIX_DOMAIN", "localhost"), "apex domain for project subdomains (default $MATRIX_DOMAIN or localhost)")
	root.Flags().StringVar(&addr, "http", ":8080", "listen address")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
