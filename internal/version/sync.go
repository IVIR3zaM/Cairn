package version

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// placeholder marks where a version appears inside a version_sync pattern (e.g.
// "mylib:{VERSION}"). versionToken matches a version-like literal, optionally "v"-prefixed.
const placeholder = "{VERSION}"

const versionToken = `v?\d+\.\d+\.\d+`

// Drift is one doc-honesty problem: a documented version that disagrees with the canonical
// one, or a configured pattern that never matched (so the doc would silently rot). Found is
// the disagreeing text, or empty when the pattern was not found at all.
type Drift struct {
	Path    string
	Pattern string
	Found   string
	Want    string
}

// Reason renders a one-line, actionable description for the reporter.
func (d Drift) Reason() string {
	if d.Found == "" {
		return fmt.Sprintf("%s: pattern %q not found (want %s)", d.Path, d.Pattern, d.Want)
	}
	return fmt.Sprintf("%s: %q has %s, want %s", d.Path, d.Pattern, d.Found, d.Want)
}

// Check is the non-mutating honesty assertion: for each version_sync file and pattern it
// matches {VERSION} against the file's text and reports every captured version that differs
// from canonical, plus any pattern that never matched. It modifies nothing — 6b's rewrite
// is what fixes the drift Check finds. An empty canonical or no files is a no-op (nil), so
// the check costs nothing until version_sync is configured.
func Check(fsys fs.FS, canonical string, files []config.VersionSyncFile) ([]Drift, error) {
	if canonical == "" || len(files) == 0 {
		return nil, nil
	}
	want, err := Parse(canonical)
	if err != nil {
		return nil, fmt.Errorf("project.canonical_version: %w", err)
	}

	var drifts []Drift
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f.Path)
		if err != nil {
			return nil, fmt.Errorf("version_sync %s: %w", f.Path, err)
		}
		text := string(data)
		for _, pat := range f.Patterns {
			re, err := compile(pat)
			if err != nil {
				return nil, fmt.Errorf("version_sync %s pattern %q: %w", f.Path, pat, err)
			}
			matches := re.FindAllStringSubmatch(text, -1)
			if len(matches) == 0 {
				drifts = append(drifts, Drift{Path: f.Path, Pattern: pat, Want: want.String()})
				continue
			}
			for _, m := range matches {
				for _, got := range m[1:] { // every {VERSION} capture in the pattern
					gv, err := Parse(got)
					if err != nil || gv.Compare(want) != 0 {
						drifts = append(drifts, Drift{Path: f.Path, Pattern: pat, Found: got, Want: want.String()})
					}
				}
			}
		}
	}
	return drifts, nil
}

// Rewrite is the mutating sibling of Check: for each version_sync file it sets every
// {VERSION} pattern to canonical, writing the file back only when it changed. It returns
// the paths it modified so bump can report them. Patterns that never match are left alone
// (Check is what flags those); an empty canonical or no files is a no-op. Paths are joined
// under root so the caller controls the working directory.
func Rewrite(root, canonical string, files []config.VersionSyncFile) ([]string, error) {
	if canonical == "" || len(files) == 0 {
		return nil, nil
	}
	want, err := Parse(canonical)
	if err != nil {
		return nil, fmt.Errorf("project.canonical_version: %w", err)
	}

	var changed []string
	for _, f := range files {
		path := filepath.Join(root, f.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return changed, fmt.Errorf("version_sync %s: %w", f.Path, err)
		}
		updated, did, err := rewriteText(string(data), want, f.Patterns)
		if err != nil {
			return changed, fmt.Errorf("version_sync %s: %w", f.Path, err)
		}
		if !did {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return changed, fmt.Errorf("version_sync %s: %w", f.Path, err)
		}
		changed = append(changed, f.Path)
	}
	return changed, nil
}

// rewriteText sets every {VERSION} occurrence of each pattern in text to want, reporting
// whether anything changed. Because a pattern is literal text around the placeholder, the
// replacement is just the pattern with {VERSION} → want, applied literally so a "$" in the
// surrounding text is never treated as a regex expansion.
func rewriteText(text string, want Version, patterns []string) (string, bool, error) {
	changed := false
	for _, pat := range patterns {
		re, err := compile(pat)
		if err != nil {
			return text, false, fmt.Errorf("pattern %q: %w", pat, err)
		}
		repl := strings.ReplaceAll(pat, placeholder, want.String())
		next := re.ReplaceAllLiteralString(text, repl)
		if next != text {
			changed = true
			text = next
		}
	}
	return text, changed, nil
}

// compile turns a version_sync pattern into a regex: literal text is escaped and each
// {VERSION} becomes a version-capturing group. A pattern without the placeholder is an
// error — it could never track a version, so it is a config mistake worth surfacing.
func compile(pattern string) (*regexp.Regexp, error) {
	if !strings.Contains(pattern, placeholder) {
		return nil, fmt.Errorf("missing %s placeholder", placeholder)
	}
	parts := strings.Split(pattern, placeholder)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.Compile(strings.Join(parts, "("+versionToken+")"))
}
