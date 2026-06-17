package changelog

import (
	"fmt"
	"regexp"
	"time"

	version "github.com/IVIR3zaM/Cairn/internal/version"
)

// keepachangelog is the Keep a Changelog format (https://keepachangelog.com): `## [Unreleased]`
// collects `### Added/Changed/...` groups and a release moves them under `## [X.Y.Z] - DATE`,
// maintaining the compare-link references at the bottom. It is the default root standard.
func init() {
	register("keepachangelog", style{
		unreleased: regexp.MustCompile(`(?i)^##\s*\[Unreleased\]\s*$`),
		released: func(ver version.Version, date time.Time) string {
			return fmt.Sprintf("## [%s] - %s", ver.String(), date.Format("2006-01-02"))
		},
		links: true,
	})
}
