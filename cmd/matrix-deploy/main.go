// matrix-deploy: deploy a local dist directory or archive to the matrix
// deploy API over plain HTTP, printing the public site URL.
//
//	go run ./cmd/matrix-deploy ./dist
//	go run ./cmd/matrix-deploy --server https://matrix.k0s.io ./dist
//	go run ./cmd/matrix-deploy --json ./dist
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func main() {
	runCLI(os.Stdout)
}

// runCLI executes the command, writing output to out (injected for tests).
func runCLI(out io.Writer) {
	var (
		server string
		asJSON bool
	)
	root := &cobra.Command{
		Use:   "matrix-deploy <dist-dir|archive.tar.gz|archive.zip>",
		Short: "Deploy a local site via the matrix deploy API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := archiveBytes(args[0])
			if err != nil {
				return err
			}
			url := strings.TrimSuffix(server, "/") + "/api/deploy"
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/gzip")
			resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
			if err != nil {
				return fmt.Errorf("deploying: %w", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("deploy API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			if asJSON {
				fmt.Fprintln(out, strings.TrimSpace(string(body)))
				return nil
			}
			var result struct {
				WebsiteURL string `json:"website_url"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			fmt.Fprintln(out, result.WebsiteURL)
			return nil
		},
	}
	root.Flags().StringVar(&server, "server", envOr("MATRIX_SERVER", "https://matrix.k0s.io"), "deploy API base URL (default $MATRIX_SERVER or https://matrix.k0s.io)")
	root.Flags().BoolVar(&asJSON, "json", false, "print the full JSON response instead of just the URL")
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(out, err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// archiveBytes returns the upload payload: an existing archive is passed
// through, a directory is packed into .tar.gz with its contents at the
// archive root (index.html must be at the root).
func archiveBytes(arg string) ([]byte, error) {
	info, err := os.Stat(arg)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", arg, err)
	}
	if !info.IsDir() {
		return os.ReadFile(arg)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == arg {
			return nil
		}
		rel, err := filepath.Rel(arg, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{Name: rel + "/", Mode: 0o755, Typeflag: tar.TypeDir})
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("packing %s: %w", arg, err)
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
