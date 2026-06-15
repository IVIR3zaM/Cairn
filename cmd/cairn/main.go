// Command cairn is the polyglot quality/versioning orchestrator CLI.
package main

import (
	"os"

	"github.com/IVIR3zaM/Cairn/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
