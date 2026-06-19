package changelog

import (
	"fmt"
	"regexp"
	"time"

	version "github.com/IVIR3zaM/Cairn/internal/version"
)

// dart is the plain, pub.dev-idiomatic changelog style each package in a Dart workspace keeps:
// `## Unreleased` (no brackets) promoted to `## X.Y.Z - DATE` (no brackets, no compare links).
// It is the typical per-package style in a multi-package repo whose root uses Keep a Changelog.
func init() {
	register("dart", style{
		unreleased: regexp.MustCompile(`(?i)^##\s+Unreleased\s*$`),
		// A non-bracketed level-2 heading — `## Unreleased` or `## 1.2.3 - 2026-06-09` — is the
		// pub.dev signature; the bracketed form belongs to Keep a Changelog, so this never fires on
		// one. Lets Detect recognise a per-package dart changelog so `cairn init` records it.
		signature: regexp.MustCompile(`(?im)^##\s+(?:unreleased\b|\d)`),
		released: func(ver version.Version, date time.Time) string {
			return fmt.Sprintf("## %s - %s", ver.String(), date.Format("2006-01-02"))
		},
		links: false,
	})
}
