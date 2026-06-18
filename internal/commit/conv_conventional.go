package commit

import (
	"fmt"
	"regexp"
	"strings"
)

// conventional implements Conventional Commits (https://www.conventionalcommits.org):
// a header `type(scope)?!?: description`, an optional body, and footers. For bump
// inference, `feat` ⇒ minor, `fix` ⇒ patch, and a `!` after the type/scope or a
// `BREAKING CHANGE:` footer ⇒ major; everything else (docs, chore, …) implies no release.
func init() { register("conventional", conventional{}) }

type conventional struct{}

// headerRe matches the first line: a type, an optional `(scope)`, an optional breaking `!`,
// then `: ` and a non-empty description.
var headerRe = regexp.MustCompile(`^(?P<type>[a-z]+)(?:\((?P<scope>[^)]+)\))?(?P<bang>!)?: (?P<desc>.+)$`)

// knownTypes is the Angular/commitlint set; an unrecognized type is an invalid header so a
// typo ("fet:") is caught rather than silently classified as no-bump.
var knownTypes = map[string]bool{
	"feat": true, "fix": true, "docs": true, "style": true, "refactor": true,
	"perf": true, "test": true, "build": true, "ci": true, "chore": true, "revert": true,
}

// breakingFooterRe matches a `BREAKING CHANGE:` / `BREAKING-CHANGE:` footer anywhere in the
// body — either spelling per the spec.
var breakingFooterRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE: .+`)

// signoffRe matches a DCO `Signed-off-by: Name <email>` trailer.
var signoffRe = regexp.MustCompile(`(?m)^Signed-off-by: .+ <.+>`)

// IsSignedOff reports whether msg carries a DCO `Signed-off-by:` trailer. It is exposed so
// onboarding (`cairn init`) can learn from history whether sign-off is the repo's norm and set
// commits.signoff accordingly, reusing the exact trailer definition the validator enforces —
// one source of truth for "what counts as signed off".
func IsSignedOff(msg string) bool { return signoffRe.MatchString(msg) }

// header returns the regex submatches for msg's first line, or nil when it doesn't conform.
func (conventional) header(msg string) []string {
	line := strings.SplitN(strings.TrimSpace(msg), "\n", 2)[0]
	return headerRe.FindStringSubmatch(strings.TrimSpace(line))
}

func (c conventional) Validate(msg string, signoff bool) error {
	m := c.header(msg)
	if m == nil {
		return fmt.Errorf(`commit message must match "type(scope): description" (Conventional Commits)`)
	}
	if typ := m[headerRe.SubexpIndex("type")]; !knownTypes[typ] {
		return fmt.Errorf("unknown commit type %q (want one of feat fix docs style refactor perf test build ci chore revert)", typ)
	}
	if signoff && !signoffRe.MatchString(msg) {
		return fmt.Errorf("missing Signed-off-by trailer (commits.signoff is on)")
	}
	return nil
}

func (c conventional) Classify(msg string) Bump {
	m := c.header(msg)
	if m == nil {
		return BumpNone
	}
	if m[headerRe.SubexpIndex("bang")] == "!" || breakingFooterRe.MatchString(msg) {
		return BumpMajor
	}
	switch m[headerRe.SubexpIndex("type")] {
	case "feat":
		return BumpMinor
	case "fix":
		return BumpPatch
	default:
		return BumpNone
	}
}
