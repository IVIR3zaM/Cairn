// Package changelog is the Changelog bounded context: it promotes a project's
// `[Unreleased]` section into a dated release entry on `cairn bump`. Like every other
// multi-implementation choice in Cairn, the changelog *standard* is a self-registering
// registry (ADR-006): each standard lives in its own `std_<name>.go` and plugs in via
// register() in init(); `bump` resolves the configured one through WriterFor without a
// switch. keepachangelog is the first (and currently only) participant; git-cliff and
// conventional-changelog are future one-file additions, documented not stubbed.
package changelog

import (
	"sort"
	"time"

	version "github.com/IVIR3zaM/Cairn/internal/version"
)

// Writer promotes a changelog's unreleased entries into a released section. The Changelog
// port also covers "generate from commits", but that earns its place only once a standard
// needs it (keep it simple); promotion is all `bump` requires today.
type Writer interface {
	// Promote moves the `[Unreleased]` entries under a new released heading for ver dated
	// date, refreshes any compare links, and leaves a fresh empty `[Unreleased]`. It is a
	// pure transform (no I/O) so callers own reading/writing the file and tests drive it on
	// in-memory content.
	Promote(content []byte, ver version.Version, date time.Time) (Result, error)
}

// Result reports the outcome of a Promote: the rewritten content, whether it changed, whether
// an unreleased section was located at all (Found), and whether that section held nothing to
// promote (Empty). A file with no unreleased heading is Found=false (the caller skips it); a
// present-but-empty section is Empty=true (the caller refuses the bump rather than ship notes-
// less). These are distinct: a package simply not using the convention is not an empty release.
type Result struct {
	Content []byte
	Changed bool
	Empty   bool
	Found   bool
}

// registry holds each standard's writer, populated by the init() in its std_<name>.go file.
// register panics on a duplicate key to catch copy-paste mistakes at startup.
var registry = map[string]Writer{}

// register wires a standard's writer. Call it from a std_<name>.go init().
func register(name string, w Writer) {
	if _, dup := registry[name]; dup {
		panic("changelog: duplicate writer registered for " + name)
	}
	registry[name] = w
}

// WriterFor returns the writer for a changelog standard (e.g. "keepachangelog"), or false
// when none is registered for it yet — so `bump` can skip promotion for a standard whose
// writer is still a future one-file addition.
func WriterFor(standard string) (Writer, bool) {
	w, ok := registry[standard]
	return w, ok
}

// Standards lists the registered standard keys (sorted) so the init wizard and docs can
// enumerate choices without a hardcoded list.
func Standards() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
