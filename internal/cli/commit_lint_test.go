package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRunCommitLint pins the commit-msg hook job's contract: a conforming message passes, a
// non-conforming one fails with an actionable error, and git comment lines are ignored (git
// strips them before committing, so the hook must judge the same text). Regression for the
// generated hook calling `cairn commit-lint` — a command that previously did not exist.
func TestRunCommitLint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "schema: \"2\"\nversion: 0.1.0\ncommits:\n  convention: conventional\n")

	cases := []struct {
		name    string
		msg     string
		wantErr bool
	}{
		{"valid feat", "feat: add thing\n", false},
		{"valid with comments", "fix: a bug\n\n# Please enter the commit message.\n# On branch main\n", false},
		{"invalid header", "broken message\n", true},
		{"unknown type", "fet: typo in type\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgFile := filepath.Join(dir, "MSG")
			writeFile(t, dir, "MSG", tc.msg)
			err := runCommitLint(dir, msgFile)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.msg)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.msg, err)
			}
		})
	}
}

// TestStripComments confirms git comment lines and leading blanks are removed while the body
// (including blank separators inside the message) is preserved for validation.
func TestStripComments(t *testing.T) {
	in := "\nfeat: x\n\nbody line\n# a comment\n"
	got := stripComments(in)
	want := "feat: x\n\nbody line\n"
	if got != want {
		t.Errorf("stripComments mismatch:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "#") {
		t.Errorf("comment leaked through: %q", got)
	}
}
