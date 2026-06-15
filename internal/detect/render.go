package detect

import (
	"fmt"
	"io"
)

// Glyphs for tool status. A richer colored Reporter arrives in a later iteration;
// doctor stays plain for now so its output is stable and CI-friendly.
const (
	glyphPresent = "✓"
	glyphMissing = "✗"
)

// Render writes a compact per-language installed/missing table for doctor. Missing
// tools carry their install hint so the next step is obvious.
func Render(w io.Writer, r *Result) {
	if len(r.Languages) == 0 {
		fmt.Fprintln(w, "No supported languages detected.")
		return
	}
	for i, l := range r.Languages {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s  (dir: %s, pkg: %s)\n", l.Name, l.Dir, l.PackageManager)
		for _, t := range l.Tools {
			if t.Installed {
				fmt.Fprintf(w, "  %s %s\n", glyphPresent, t.Tool.Name)
			} else {
				fmt.Fprintf(w, "  %s %s  → %s\n", glyphMissing, t.Tool.Name, t.Tool.Hint)
			}
		}
	}
}
