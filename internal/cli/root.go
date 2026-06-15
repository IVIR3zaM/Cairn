// Package cli wires Cairn's cobra command tree. It owns glue only: the actual
// quality/versioning logic lives in the bounded-context packages it calls.
package cli

import (
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags
// "-X github.com/IVIR3zaM/Cairn/internal/cli.version=<v>".
var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cairn",
		Short:         "Cairn — one orchestrator for quality, versioning, changelog, and commit hygiene",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newVerifyCmd())
	return root
}

// Execute runs the root command. Errors are returned so main can choose the
// exit code; cobra is told to stay silent so we don't double-print.
func Execute() error {
	return newRootCmd().Execute()
}
