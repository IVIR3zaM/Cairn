package changelog

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	version "github.com/IVIR3zaM/Cairn/internal/version"
)

// style is the shared promotion engine behind every changelog standard: the standards differ
// only in how the "unreleased" heading is spelled, how a released heading is formatted, and
// whether compare links are maintained. Each std_<name>.go registers a style, so adding a
// changelog convention is declaring those three knobs — not re-implementing promotion. A
// style implements Writer, so it plugs straight into the registry.
type style struct {
	// unreleased matches the unreleased heading (e.g. `## [Unreleased]` or `## Unreleased`).
	unreleased *regexp.Regexp
	// signature matches a line that is distinctive to this standard, used by Detect to
	// recognise an existing changelog so `cairn init` can record the right standard. It must be
	// specific enough not to fire on a foreign format's file.
	signature *regexp.Regexp
	// released formats the new released heading for a version dated date.
	released func(ver version.Version, date time.Time) string
	// links, when true, maintains Keep a Changelog compare-link references at the bottom.
	links bool
}

var (
	h2Re           = regexp.MustCompile(`^##\s`)  // any level-2 heading — the section boundary
	groupHeadingRe = regexp.MustCompile(`^###\s`) // an empty `### Added/Changed/...` group
	linkDefRe      = regexp.MustCompile(`^\[[^\]]+\]:\s`)
	// unreleasedLinkRe splits the [Unreleased] compare link so the previous tag (group 3)
	// can become a released-version link and the Unreleased link can point at the new tag.
	unreleasedLinkRe = regexp.MustCompile(`^(\[Unreleased\]:\s*)(\S*?/compare/)(.+?)(\.\.\.HEAD\s*)$`)
)

// Promote moves the unreleased entries under a new released heading for ver dated date, leaves
// a fresh empty unreleased section, and (for link-style standards) refreshes compare links.
// Promotion is idempotent: once moved, the unreleased section is empty, so a second run finds
// nothing to promote and reports Empty. The body ends at the next h2 heading or the link block,
// so the moved entries land above the previous release with the file's spacing preserved.
func (s style) Promote(content []byte, ver version.Version, date time.Time) (Result, error) {
	lines := strings.Split(string(content), "\n")

	head := -1
	for i, ln := range lines {
		if s.unreleased.MatchString(strings.TrimSpace(ln)) {
			head = i
			break
		}
	}
	if head == -1 {
		// No unreleased heading at all — the file doesn't use the convention, so it is skipped
		// (not an error and not an empty release). Mirrors the reference bump script.
		return Result{Content: content, Found: false}, nil
	}

	end := len(lines)
	for i := head + 1; i < len(lines); i++ {
		if h2Re.MatchString(lines[i]) || linkDefRe.MatchString(lines[i]) {
			end = i
			break
		}
	}
	body := trimBlank(lines[head+1 : end])
	if changelogEmpty(body) {
		return Result{Content: content, Found: true, Empty: true}, nil
	}

	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:head+1]...) // through the unreleased heading
	out = append(out, "")                // keep the unreleased section empty
	out = append(out, s.released(ver, date))
	out = append(out, "")
	out = append(out, body...)
	out = append(out, "")
	out = append(out, lines[end:]...)

	if s.links {
		out = refreshLinks(out, ver)
	}
	return Result{Content: []byte(strings.Join(out, "\n")), Changed: true, Found: true}, nil
}

// refreshLinks rewrites the `[Unreleased]` compare link to start at the new tag and inserts a
// `[X.Y.Z]` link spanning the previous tag to the new one, right below it. It is a no-op when
// the file carries no such link (Cairn won't fabricate a repo URL it doesn't know).
func refreshLinks(lines []string, ver version.Version) []string {
	for i, ln := range lines {
		m := unreleasedLinkRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		prefix, base, prevTag, head := m[1], m[2], m[3], m[4]
		verTag := tagLike(prevTag, ver)
		lines[i] = prefix + base + verTag + head
		newLink := fmt.Sprintf("[%s]: %s%s...%s", ver.String(), base, prevTag, verTag)
		return append(lines[:i+1], append([]string{newLink}, lines[i+1:]...)...)
	}
	return lines
}

// tagLike renders ver with the same "v" prefix convention the previous tag used, so a repo
// tagging "v1.2.3" keeps the prefix and one tagging "1.2.3" stays bare.
func tagLike(prevTag string, ver version.Version) string {
	if strings.HasPrefix(prevTag, "v") {
		return "v" + ver.String()
	}
	return ver.String()
}

// changelogEmpty reports whether the unreleased body has no real entries — only blank lines
// and empty `### Added/Changed/...` group headings count as empty, so an unreleased section
// with stale group headers but no items still counts as nothing to release.
func changelogEmpty(body []string) bool {
	for _, ln := range body {
		t := strings.TrimSpace(ln)
		if t == "" || groupHeadingRe.MatchString(t) {
			continue
		}
		return false
	}
	return true
}

// trimBlank drops leading and trailing blank lines, keeping internal spacing intact.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
