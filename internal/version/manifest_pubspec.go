package version

import (
	"bytes"
	"fmt"
	"regexp"
)

// pubspecVersion captures the top-level `version:` of a pubspec.yaml — a YAML key at the
// start of a line, value unquoted. Anchored to the line start (and only the first match via
// setVia) so a dependency's nested `version:` key is never mistaken for the package version.
var pubspecVersion = regexp.MustCompile(`(?m)^version:\s*(` + versionToken + `)`)

// pubspecName captures a pubspec's own package name (top-level `name:` key) — the identity
// that lets a workspace tell a sibling dependency apart from an external one, by name.
var pubspecName = regexp.MustCompile(`(?m)^name:\s*(\S+)`)

// pubspecDep matches an indented dependency line carrying an inline version constraint, e.g.
// "  signing_local: ^0.1.0" — capturing the dependency name (1), an optional caret (2), and
// the version literal (3). Anchored to end-of-line so multi-part ranges (">=x <y") and
// path/git deps (no inline version) are left untouched; the top-level `version:` is at column
// zero so it never matches.
var pubspecDep = regexp.MustCompile(`(?m)^\s+([A-Za-z0-9_]+):\s*(\^?)(` + versionToken + `)\s*$`)

// pubspec is the Dart manifest manager. It also implements Workspace because a pub workspace
// references its members by name with `^X.Y.Z` constraints that must move in lockstep.
type pubspec struct{}

func init() { register(pubspec{}) }

func (pubspec) Filename() string { return "pubspec.yaml" }

// SetVersion sets the package's own `version:`, like every other manager. Member-to-member
// constraints are the Workspace pass's job (SetSiblings), not SetVersion's.
func (pubspec) SetVersion(content []byte, v Version) ([]byte, bool, error) {
	return setVia(content, pubspecVersion, v, "version in pubspec.yaml")
}

// PackageID implements Workspace: the member's own package name.
func (pubspec) PackageID(content []byte) (string, bool) {
	m := pubspecName.FindSubmatch(content)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// SetSiblings implements Workspace: rewrite every dependency that names a member (a key in
// members) to v, preserving an existing caret. An external dependency — whose key is not a
// member — is left untouched, however its version happens to read.
func (pubspec) SetSiblings(content []byte, members map[string]bool, v Version) ([]byte, bool) {
	changed := false
	out := pubspecDep.ReplaceAllFunc(content, func(line []byte) []byte {
		g := pubspecDep.FindSubmatch(line)
		name, ver := string(g[1]), string(g[3])
		if !members[name] || ver == v.String() {
			return line
		}
		changed = true
		idx := bytes.LastIndex(line, g[3]) // swap only the version literal, keep name+caret
		out := make([]byte, 0, len(line))
		out = append(out, line[:idx]...)
		out = append(out, v.String()...)
		return append(out, line[idx+len(g[3]):]...)
	})
	return out, changed
}

// CheckSiblings implements Workspace: report each dependency on a member that pins a version
// other than v — drift the per-file version: check is blind to.
func (pubspec) CheckSiblings(content []byte, members map[string]bool, v Version) []string {
	var reasons []string
	for _, g := range pubspecDep.FindAllSubmatch(content, -1) {
		name, ver := string(g[1]), string(g[3])
		if !members[name] {
			continue
		}
		if pv, err := Parse(ver); err != nil || pv.Compare(v) != 0 {
			reasons = append(reasons, fmt.Sprintf("depends on member %s at %s", name, ver))
		}
	}
	return reasons
}
