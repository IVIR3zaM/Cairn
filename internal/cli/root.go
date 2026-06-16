// Package cli wires Cairn's cobra command tree. It owns glue only: the actual
// quality/versioning logic lives in the bounded-context packages it calls.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// errSilent marks a failure a command has already reported itself (e.g. verify's compact
// summary). Execute returns non-zero for it but prints nothing, avoiding a duplicate line.
var errSilent = errors.New("already reported")

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
	root.AddCommand(newBumpCmd())
	return root
}

// Execute runs the root command and is the single place errors reach the user. cobra is
// told to stay silent (SilenceErrors) so a command that renders its own failure summary —
// verify — isn't double-printed; everything else surfaces here on stderr. Without this an
// error (unset canonical, a guard rejection, an unknown command) would exit non-zero with
// no message at all. The error is still returned so main sets the exit code.
func Execute() error {
	err := newRootCmd().Execute()
	reportError(os.Stderr, err)
	return err
}

// reportError prints err for the user unless it is nil or an already-reported failure.
func reportError(w io.Writer, err error) {
	if err != nil && !errors.Is(err, errSilent) {
		fmt.Fprintln(w, "Error:", err)
	}
}
