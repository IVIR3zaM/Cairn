package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/commit"
	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/spf13/cobra"
)

// newCommitLintCmd validates a commit message against the configured convention. It is the
// job the generated commit-msg hook runs (`cairn commit-lint "$@"`, where $1 is the path to
// the file git wrote the proposed message into), so a non-conforming message aborts the
// commit. The convention (and whether a DCO sign-off is required) is resolved from the repo's
// cairn.yaml via the per-directory Tree, like every other context.
func newCommitLintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commit-lint <message-file>",
		Short: "Validate a commit message against the configured convention (commit-msg hook job)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCommitLint(wd, args[0])
		},
	}
}

// runCommitLint is the testable core: resolve the convention/sign-off from the cairn.yaml at
// wd via the per-directory Tree, read the proposed message from msgFile, and validate it. A
// convention without a registered validator (e.g. "none") asserts nothing and never blocks the
// commit.
func runCommitLint(wd, msgFile string) error {
	tree, err := config.LoadTree(os.DirFS(wd))
	if err != nil {
		return err
	}
	root, _ := tree.Resolve(".")
	commits := config.Default().Commits
	if root.Commits != nil {
		commits = *root.Commits
	}

	validator, ok := commit.ValidatorFor(commits.Convention)
	if !ok {
		return nil
	}

	raw, err := os.ReadFile(msgFile)
	if err != nil {
		return fmt.Errorf("read commit message %s: %w", msgFile, err)
	}
	if err := validator.Validate(stripComments(string(raw)), commits.Signoff); err != nil {
		return fmt.Errorf("invalid commit message: %w", err)
	}
	return nil
}

// stripComments drops git's comment lines (those beginning with '#', after the COMMIT_EDITMSG
// template) and leading blank lines so validation sees the author's actual message. git itself
// removes these before committing, so the hook must judge the same text.
func stripComments(msg string) string {
	var kept []string
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimLeft(strings.Join(kept, "\n"), "\n")
}
