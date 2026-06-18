package wiring

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"gopkg.in/yaml.v3"
)

// The github provider self-registers under its key and is enumerable, proving the registry
// works without a hardcoded list (ADR-006).
func TestProviderForGithubSelfRegisters(t *testing.T) {
	if _, ok := ProviderFor("github"); !ok {
		t.Fatal("ProviderFor(\"github\") not registered")
	}
	if _, ok := ProviderFor("nope"); ok {
		t.Fatal("ProviderFor returned a provider for an unregistered key")
	}
	found := false
	for _, p := range Providers() {
		if p == "github" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Providers() = %v, missing github", Providers())
	}
}

// GenerateCI writes a parseable workflow that runs each configured job, and re-running yields
// identical bytes (idempotent).
func TestGenerateCIWritesValidWorkflowIdempotently(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()

	rel, err := GenerateCI(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rel != ".github/workflows/cairn.yml" {
		t.Fatalf("path = %q", rel)
	}

	first, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	// Valid YAML and runs `cairn verify` (the default job).
	var doc map[string]any
	if err := yaml.Unmarshal(first, &doc); err != nil {
		t.Fatalf("workflow is not valid YAML: %v", err)
	}
	if !strings.Contains(string(first), "run: cairn verify") {
		t.Fatalf("workflow does not run cairn verify:\n%s", first)
	}

	if _, err := GenerateCI(dir, cfg); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, rel))
	if string(first) != string(second) {
		t.Fatal("GenerateCI is not idempotent")
	}
}

// A job list other than the default flows into the workflow as a step per job.
func TestGenerateCIHonorsConfiguredJobs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.CI.Jobs = []string{"verify", "bump"}

	rel, err := GenerateCI(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, rel))
	for _, want := range []string{"run: cairn verify", "run: cairn bump"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("workflow missing %q:\n%s", want, content)
		}
	}
}

// An unregistered provider is an actionable error, not a silent no-op.
func TestGenerateCIUnknownProvider(t *testing.T) {
	cfg := config.Default()
	cfg.CI.Provider = "jenkins"
	if _, err := GenerateCI(t.TempDir(), cfg); err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}

// InstallHooks writes runnable, executable hook scripts, points git at the tracked hooks dir,
// and is idempotent across re-installs.
func TestInstallHooksWritesRunnableHooksAndSetsHooksPath(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	cfg := config.Default() // pre_commit: [verify], commit_msg: [commit-lint]

	installed, err := InstallHooks(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(installed, ","); got != "pre-commit,commit-msg" {
		t.Fatalf("installed = %q", got)
	}

	pre := filepath.Join(dir, HooksDir, "pre-commit")
	info, err := os.Stat(pre)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("pre-commit hook is not executable: %v", info.Mode())
	}
	body, _ := os.ReadFile(pre)
	if !strings.Contains(string(body), "cairn verify") {
		t.Fatalf("pre-commit does not run cairn verify:\n%s", body)
	}
	// Runnable: a POSIX shell parses it without syntax error.
	if out, err := exec.Command("sh", "-n", pre).CombinedOutput(); err != nil {
		t.Fatalf("hook is not a valid shell script: %v: %s", err, out)
	}
	// commit-msg forwards the message file path.
	msg, _ := os.ReadFile(filepath.Join(dir, HooksDir, "commit-msg"))
	if !strings.Contains(string(msg), `"$@"`) {
		t.Fatalf("commit-msg does not forward args:\n%s", msg)
	}

	// git was pointed at the tracked dir.
	if got := gitConfig(t, dir, "core.hooksPath"); got != HooksDir {
		t.Fatalf("core.hooksPath = %q, want %q", got, HooksDir)
	}

	// Idempotent: a second install keeps the script identical and executable.
	if _, err := InstallHooks(dir, cfg); err != nil {
		t.Fatal(err)
	}
	body2, _ := os.ReadFile(pre)
	if string(body) != string(body2) {
		t.Fatal("InstallHooks is not idempotent")
	}
	if info2, _ := os.Stat(pre); info2.Mode()&0o111 == 0 {
		t.Fatal("pre-commit lost its executable bit on re-install")
	}
}

// TestInstalledHookFiresOnCommit proves the whole wiring works end-to-end: with
// core.hooksPath pointed at the tracked .cairn/hooks dir, git actually runs the installed
// pre-commit on `git commit`, and a non-zero job aborts the commit. Regression guard for the
// tracked-hooks-dir approach itself (a hook in .cairn/hooks must fire, not just exist).
func TestInstalledHookFiresOnCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitInit(t, dir)
	if out, err := runGit(dir, "config", "user.email", "t@t"); err != nil {
		t.Fatalf("config email: %v: %s", err, out)
	}
	if out, err := runGit(dir, "config", "user.name", "t"); err != nil {
		t.Fatalf("config name: %v: %s", err, out)
	}
	if _, err := InstallHooks(dir, config.Default()); err != nil {
		t.Fatal(err)
	}

	// A stub `cairn` on PATH records that the hook invoked it; its exit code is controlled by
	// a marker file so we can drive both the pass and fail paths.
	binDir := filepath.Join(dir, "stubbin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ranLog := filepath.Join(dir, "ran.log")
	failFlag := filepath.Join(dir, "FAIL")
	stub := "#!/bin/sh\necho \"$@\" >> '" + ranLog + "'\n[ -f '" + failFlag + "' ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "cairn"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}

	commit := func(msg string) (string, error) {
		writeFileWiring(t, dir, "f.txt", msg)
		if out, err := runGit(dir, "add", "f.txt"); err != nil {
			return out, err
		}
		cmd := exec.Command("git", "commit", "-m", msg)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Pass path: the commit succeeds and the hook invoked `cairn verify`.
	if out, err := commit("first"); err != nil {
		t.Fatalf("commit should succeed: %v: %s", err, out)
	}
	if log, _ := os.ReadFile(ranLog); !strings.Contains(string(log), "verify") {
		t.Fatalf("pre-commit hook did not run `cairn verify` (log=%q)", log)
	}

	// Fail path: a non-zero job aborts the commit.
	if err := os.WriteFile(failFlag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := commit("second"); err == nil {
		t.Fatalf("commit should be blocked when the hook job fails, got success:\n%s", out)
	}
}

func writeFileWiring(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if out, err := runGit(dir, "init"); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

func gitConfig(t *testing.T, dir, key string) string {
	t.Helper()
	out, err := runGit(dir, "config", key)
	if err != nil {
		t.Fatalf("git config %s: %v: %s", key, err, out)
	}
	return strings.TrimSpace(out)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
