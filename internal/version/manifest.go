package version

import (
	"fmt"
	"regexp"
	"sort"
)

// Manager writes the canonical version into one language manifest format. Each format
// lives in its own manifest_<name>.go and self-registers in init(); bump resolves it via
// ManagerFor. Implementations are pure byte-transforms (safe regex) so they are trivially
// testable and never shell out — native bumpers (e.g. `npm version`) are a later opt-in,
// documented not stubbed (ADR-006: a registry is earned, entries are added, never engines).
type Manager interface {
	// Filename is the manifest this manager owns, e.g. "package.json".
	Filename() string
	// SetVersion returns content with its version set to v and whether anything changed
	// (false when already correct, so bump can skip an untouched file). It errors only
	// when the manifest declares no version this manager can locate.
	SetVersion(content []byte, v Version) ([]byte, bool, error)
}

// managers maps a manifest filename to its writer, populated by each manifest_<name>.go
// init(). Adding a manifest format is dropping one self-registering file — no edits here.
var managers = map[string]Manager{}

// register wires a manifest manager. Call it from a manifest_<name>.go init(). It panics
// on a duplicate filename to catch a copy-paste mistake at startup.
func register(m Manager) {
	if _, dup := managers[m.Filename()]; dup {
		panic("version: duplicate manager registered for " + m.Filename())
	}
	managers[m.Filename()] = m
}

// ManagerFor returns the manager owning filename, or false when none is registered.
func ManagerFor(filename string) (Manager, bool) {
	m, ok := managers[filename]
	return m, ok
}

// Managers lists every registered manager, sorted by filename for a stable scan/output.
func Managers() []Manager {
	out := make([]Manager, 0, len(managers))
	for _, m := range managers {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Filename() < out[j].Filename() })
	return out
}

// setVia rewrites the first capture group of re (which must capture the version literal)
// to v across content, reporting whether it changed. It is the shared core of every regex
// manager: the manager only supplies the locating regex. A no-match is an error so a bump
// never silently leaves a manifest behind.
func setVia(content []byte, re *regexp.Regexp, v Version, what string) ([]byte, bool, error) {
	loc := re.FindSubmatchIndex(content)
	if loc == nil {
		return nil, false, fmt.Errorf("no %s found", what)
	}
	// loc[2]:loc[3] is the first submatch — the version literal to replace.
	start, end := loc[2], loc[3]
	if string(content[start:end]) == v.String() {
		return content, false, nil
	}
	out := make([]byte, 0, len(content)+len(v.String()))
	out = append(out, content[:start]...)
	out = append(out, v.String()...)
	out = append(out, content[end:]...)
	return out, true, nil
}
