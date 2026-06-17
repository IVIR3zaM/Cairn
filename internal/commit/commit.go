// Package commit is the CommitConventions bounded context: it validates a commit message
// against the project's chosen convention and classifies it (feat/fix/breaking) so `bump`
// can infer the next SemVer level from history. Like every other multi-implementation
// choice in Cairn, the convention is a self-registering registry (ADR-006): each lives in
// its own `conv_<name>.go` and plugs in via register() in init(); callers resolve the
// configured one through ValidatorFor without a switch. conventional is the first (and
// currently only) participant; gitmoji and none are future one-file additions, documented
// not stubbed.
package commit

import "sort"

// Bump is the SemVer level a commit (or a whole history) implies. The values are ordered
// so the strongest signal across many commits is just their max (none < patch < minor <
// major), which is exactly how bump inference aggregates a range.
type Bump int

const (
	BumpNone Bump = iota
	BumpPatch
	BumpMinor
	BumpMajor
)

// Level renders the bump as a version.Next level ("major"/"minor"/"patch"), or "" for
// BumpNone — so a caller can feed it straight into the version math or treat "" as "no
// release-worthy change since the last tag".
func (b Bump) Level() string {
	switch b {
	case BumpMajor:
		return "major"
	case BumpMinor:
		return "minor"
	case BumpPatch:
		return "patch"
	default:
		return ""
	}
}

// Validator checks one commit message against a convention and classifies it for bump
// inference. Validate enforces the message shape (and an optional DCO sign-off); Classify
// maps it to the SemVer bump it implies (BumpNone when it implies none, e.g. a non-
// conforming or chore-only message).
type Validator interface {
	Validate(msg string, signoff bool) error
	Classify(msg string) Bump
}

// registry holds each convention's validator, populated by the init() in its conv_<name>.go
// file. register panics on a duplicate key to catch copy-paste mistakes at startup.
var registry = map[string]Validator{}

// register wires a convention's validator. Call it from a conv_<name>.go init().
func register(name string, v Validator) {
	if _, dup := registry[name]; dup {
		panic("commit: duplicate validator registered for " + name)
	}
	registry[name] = v
}

// ValidatorFor returns the validator for a convention (e.g. "conventional"), or false when
// none is registered for it yet — so callers can skip validation/inference for a convention
// whose validator is still a future one-file addition (e.g. "none").
func ValidatorFor(convention string) (Validator, bool) {
	v, ok := registry[convention]
	return v, ok
}

// Conventions lists the registered convention keys (sorted) so the init wizard and docs can
// enumerate choices without a hardcoded list.
func Conventions() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
