package version

import (
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

func TestResolverForDir(t *testing.T) {
	proj := config.Project{
		CanonicalVersion: "1.0.0",
		Versioning:       "semver",
		Packages: []config.PackageVersion{
			{Path: "packages/api", Version: "2.3.0"},
			{Path: "packages/api/internal", Version: "0.9.0", Versioning: "calver"},
			{Path: "./apps/web/", Version: "4.0.0"},
		},
	}
	r := NewResolver(proj)

	tests := []struct {
		name string
		dir  string
		want Target
	}{
		{"matching unit gets its package version, inherits project scheme",
			"packages/api", Target{Version: "2.3.0", Versioning: "semver"}},
		{"nested path resolves to the most-specific entry with its scheme override",
			"packages/api/internal", Target{Version: "0.9.0", Versioning: "calver"}},
		{"dir under a package inherits that package, not an ancestor",
			"packages/api/handlers", Target{Version: "2.3.0", Versioning: "semver"}},
		{"config path is cleaned so ./apps/web/ matches apps/web",
			"apps/web", Target{Version: "4.0.0", Versioning: "semver"}},
		{"unmatched unit falls back to canonical version and project scheme",
			"tools/cli", Target{Version: "1.0.0", Versioning: "semver"}},
		{"sibling sharing a path prefix is not treated as nested",
			"packages/apiv2", Target{Version: "1.0.0", Versioning: "semver"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ForDir(tt.dir); got != tt.want {
				t.Errorf("ForDir(%q) = %+v, want %+v", tt.dir, got, tt.want)
			}
		})
	}
}

// A root "." package covers every otherwise-unmatched unit, so a repo can declare one
// repo-wide version line distinct from canonical without listing every dir.
func TestResolverRootPackageCoversRepo(t *testing.T) {
	r := NewResolver(config.Project{
		CanonicalVersion: "1.0.0",
		Versioning:       "semver",
		Packages: []config.PackageVersion{
			{Path: ".", Version: "5.5.5"},
			{Path: "packages/api", Version: "2.0.0"},
		},
	})
	if got := r.ForDir("anything/here"); got.Version != "5.5.5" {
		t.Errorf("root package should cover unmatched dir, got %+v", got)
	}
	if got := r.ForDir("packages/api"); got.Version != "2.0.0" {
		t.Errorf("more-specific entry should win over root, got %+v", got)
	}
}

// With no project.packages the resolver is the lockstep case: every unit gets canonical.
func TestResolverNoPackagesIsLockstep(t *testing.T) {
	r := NewResolver(config.Project{CanonicalVersion: "3.1.4", Versioning: "calver"})
	want := Target{Version: "3.1.4", Versioning: "calver"}
	for _, dir := range []string{".", "packages/api", "deep/nested/unit"} {
		if got := r.ForDir(dir); got != want {
			t.Errorf("ForDir(%q) = %+v, want %+v", dir, got, want)
		}
	}
}
