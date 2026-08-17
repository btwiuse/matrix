// Command deploy-server serves the matrix deploy data directory over
// second-level domains: every project deployed by cmd/matrix --data-dir is
// reachable at http://<project>.<domain>/, backed by DataDir/<project>.
// The bare apex domain lists all deployed sites.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gearshell/inject-proxy/matrix"
	"github.com/gearshell/inject-proxy/rewrite"
	"github.com/spf13/cobra"
)

func main() {
	var (
		dataDir    string
		domain     string
		addr       string
		injectFile string
		injectHTML string
	)

	root := &cobra.Command{
		Use:   "deploy-server",
		Short: "serve matrix deploy output over subdomains",
		Long: "Serves the data directory written by the matrix deploy tool at " +
			"http://<project>.<domain>/: the project name becomes the second-level " +
			"domain, and the apex domain lists every deployed site. With --inject " +
			"or --inject-html, every served index.html is rewritten with the snippet, " +
			"reusing inject-proxy's idempotent HTML injection.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("creating data dir %s: %w", dataDir, err)
			}
			injection, err := loadInjection(injectFile, injectHTML)
			if err != nil {
				return err
			}
			var inj *rewrite.Injector
			if strings.TrimSpace(injection) != "" {
				inj = rewrite.New(injection)
				log.Printf("served index.html pages rewritten with a %d byte snippet", len(injection))
			}
			h := matrix.NewSiteHandlerWithInjector(dataDir, domain, inj)
			log.Printf("serving %s at http://<project>.%s/ (apex http://%s/)", dataDir, domain, domain)
			return http.ListenAndServe(addr, h)
		},
	}

	root.Flags().StringVar(&dataDir, "data-dir", envOr("MATRIX_DATA_DIR", "./data"), "deploy data directory (default $MATRIX_DATA_DIR or ./data)")
	root.Flags().StringVar(&domain, "domain", envOr("MATRIX_DOMAIN", "localhost"), "apex domain for project subdomains (default $MATRIX_DOMAIN or localhost)")
	root.Flags().StringVar(&addr, "http", defaultHTTPAddr(), "listen address (default $PORT or :8080)")
	root.Flags().StringVar(&injectFile, "inject", "", "HTML snippet file injected into served index.html pages")
	root.Flags().StringVar(&injectHTML, "inject-html", "", "inline HTML snippet injected into served index.html pages")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
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
