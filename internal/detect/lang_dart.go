package detect

import "strings"

func init() {
	register(langSpec{
		name:     "dart",
		markers:  []marker{{"pubspec.yaml", "pub"}},
		tools:    []Tool{{"dart", "https://dart.dev/get-dart"}},
		skipDirs: []string{".dart_tool"},
		// A Dart pub workspace root (Dart 3.6+) lists its members under a top-level
		// `workspace:` key and owns no package code itself; detection defers to those
		// members so each is verified in its own dir.
		workspace: isPubWorkspace,
	})
}

// isPubWorkspace reports whether a pubspec.yaml declares a top-level `workspace:` key,
// marking it a workspace aggregator rather than a real package. Members instead carry
// `resolution: workspace` (an indented value), so only the root matches.
func isPubWorkspace(manifest []byte) bool {
	for _, line := range strings.Split(string(manifest), "\n") {
		if strings.HasPrefix(line, "workspace:") {
			return true
		}
	}
	return false
}
