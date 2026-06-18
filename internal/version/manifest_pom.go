package version

import (
	"fmt"
	"regexp"
)

// maven writes the project version into a Maven pom.xml. The reactor's single source of
// truth is the root (aggregator) pom's own <version>; submodules inherit it through their
// <parent> block. This manager sets that own version and is deliberately blind to a
// submodule's inherited (parentless) version — a child pom that declares no own <version>
// states nothing this manager can contradict, so SetVersion reports "no version" and the
// honesty engine skips it (it is honest by inheritance, not a lie).
//
// Multi-module reconciliation — rewriting every child's <parent><version> in lockstep with
// the root on a bump — is not done here: Java detection is single-root (the build runs once
// at the reactor root), so submodule poms are not surfaced as units to reconcile. That
// remains a follow-up; this manager makes `verify` assert the root pom's version and `bump`
// write it.
type maven struct{}

func init() { register(maven{}) }

func (maven) Filename() string { return "pom.xml" }

// pomVersionCore captures the numeric core of a <version>X</version> element. A Maven
// version routinely carries a qualifier (e.g. "0.3.1-SNAPSHOT", "1.0.0-RC1"); only the
// X.Y.Z core is captured so the qualifier is preserved across a rewrite.
var pomVersionCore = regexp.MustCompile(`<version>\s*(` + versionToken + `)`)

// pomParentBlock spans a <parent>…</parent> element. The project's own version is searched
// after it, so a child module's <parent><version> (the parent's version) is never taken for
// the module's own.
var pomParentBlock = regexp.MustCompile(`(?s)<parent>.*?</parent>`)

// pomSectionBoundary marks the first element that may itself contain <version> tags
// (dependency or plugin declarations). A pom's own coordinates always precede these, so the
// own <version> is sought only in the header region before the earliest boundary — a
// dependency or plugin version is never mistaken for the project version.
var pomSectionBoundary = regexp.MustCompile(`<(dependencies|dependencyManagement|build|profiles|reporting|distributionManagement)\b`)

// SetVersion sets the project's own version (the <version> in the header region, outside the
// <parent> block and before any dependency/build section), preserving a trailing qualifier
// like -SNAPSHOT. It reports changed=false when already correct and errors when the pom
// declares no own version (an inheriting submodule), so the engine skips it rather than
// rewriting a parent reference or a dependency pin.
func (maven) SetVersion(content []byte, v Version) ([]byte, bool, error) {
	start := 0
	if loc := pomParentBlock.FindIndex(content); loc != nil {
		start = loc[1]
	}
	end := len(content)
	if loc := pomSectionBoundary.FindIndex(content[start:]); loc != nil {
		end = start + loc[0]
	}
	loc := pomVersionCore.FindSubmatchIndex(content[start:end])
	if loc == nil {
		return nil, false, fmt.Errorf("no project version in pom.xml")
	}
	s, e := start+loc[2], start+loc[3] // the X.Y.Z capture, offset back into content
	if string(content[s:e]) == v.String() {
		return content, false, nil
	}
	out := make([]byte, 0, len(content)-(e-s)+len(v.String()))
	out = append(out, content[:s]...)
	out = append(out, v.String()...)
	out = append(out, content[e:]...)
	return out, true, nil
}

// ReadVersion returns the project's own version core (the same header-region <version> that
// SetVersion writes), dropping any -SNAPSHOT/-RC qualifier. It reports false for an inheriting
// submodule that declares no own version — so `cairn init` seeds from the reactor root pom and
// never mistakes a parent reference or dependency pin for the project version.
func (maven) ReadVersion(content []byte) (Version, bool) {
	start := 0
	if loc := pomParentBlock.FindIndex(content); loc != nil {
		start = loc[1]
	}
	end := len(content)
	if loc := pomSectionBoundary.FindIndex(content[start:]); loc != nil {
		end = start + loc[0]
	}
	return readVia(content[start:end], pomVersionCore)
}
