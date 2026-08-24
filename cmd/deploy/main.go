// deploy: standalone entry for the deploy subcommand (also available as
// "matrix deploy").
package main

import (
	"fmt"
	"os"

	"github.com/gearshell/inject-proxy/internal/deploycmd"
)

func main() {
	if err := deploycmd.New(os.Stdout).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
