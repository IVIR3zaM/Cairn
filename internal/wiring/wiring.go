// Package wiring is the Wiring bounded context: it installs Cairn into a repo so the
// local hook and CI both call the same `cairn verify` (ADR-005). It owns two outputs of
// `init`: git hooks (a tracked hooks dir wired via core.hooksPath) and a generated CI
// workflow. The CI provider is a self-registering registry (ADR-006): each provider lives
// in its own `ci_<name>.go` and plugs in via register() in init(); GenerateCI resolves the
// configured one through ProviderFor without a switch. github is the first participant;
// other providers are future one-file additions, documented not stubbed.
package wiring

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// Provider renders a CI workflow for a project. It is a pure transform (no I/O): it returns
// the repo-relative path and byte content, so GenerateCI owns writing and tests drive it on
// in-memory output. Generating from anything richer than config earns its place only once a
// provider needs it (keep it simple); a workflow that shells out to `cairn` is all CI needs.
type Provider interface {
	Workflow(cfg *config.Config) (path string, content []byte)
}

// registry holds each provider, populated by the init() in its ci_<name>.go file. register
// panics on a duplicate key to catch copy-paste mistakes at startup.
var registry = map[string]Provider{}

// register wires a CI provider. Call it from a ci_<name>.go init().
func register(name string, p Provider) {
	if _, dup := registry[name]; dup {
		panic("wiring: duplicate CI provider registered for " + name)
	}
	registry[name] = p
}

// ProviderFor returns the provider for a CI system (e.g. "github"), or false when none is
// registered for it yet — so callers can report an actionable error rather than guess.
func ProviderFor(name string) (Provider, bool) {
	p, ok := registry[name]
	return p, ok
}

// Providers lists the registered provider keys (sorted) so the init wizard and docs can
// enumerate choices without a hardcoded list.
func Providers() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GenerateCI writes the CI workflow for cfg.CI.Provider into the repo rooted at dir, creating
// parent directories as needed, and returns the repo-relative path written. It is idempotent:
// re-running overwrites the file with identical content. An unregistered provider is an
// actionable error, not a silent no-op.
func GenerateCI(dir string, cfg *config.Config) (string, error) {
	p, ok := ProviderFor(cfg.CI.Provider)
	if !ok {
		return "", fmt.Errorf("ci: no provider registered for %q (have %v)", cfg.CI.Provider, Providers())
	}
	rel, content := p.Workflow(cfg)
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
