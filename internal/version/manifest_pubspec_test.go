package version

import (
	"strings"
	"testing"
	"testing/fstest"
)

// SetVersion sets only the package's own version:, like every other manager; a versionless
// pubspec errors. Interdependency constraints are the workspace pass's job, not SetVersion's.
func TestPubspecSetVersion(t *testing.T) {
	m, ok := ManagerFor("pubspec.yaml")
	if !ok {
		t.Fatal("pubspec.yaml manager not registered")
	}
	out, changed, err := m.SetVersion([]byte("name: app\nversion: 1.0.0\n\ndependencies:\n  http: ^1.5.0\n"), Version{2, 0, 0})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v, want true/nil", changed, err)
	}
	got := string(out)
	if !strings.Contains(got, "version: 2.0.0") || !strings.Contains(got, "http: ^1.5.0") {
		t.Errorf("SetVersion should set version: only, leaving deps untouched:\n%s", got)
	}
	if _, changed, err := m.SetVersion([]byte("name: app\nversion: 2.0.0\n"), Version{2, 0, 0}); err != nil || changed {
		t.Errorf("already-correct: changed=%v err=%v, want false/nil", changed, err)
	}
	if _, _, err := m.SetVersion([]byte("name: app\n"), Version{1, 0, 0}); err == nil {
		t.Error("a pubspec without a version should error")
	}
}

// The pubspec Workspace capability rewrites a sibling constraint (identified by member name,
// whatever stale version it held) to the new version, while leaving an external dep alone.
func TestPubspecSetSiblings(t *testing.T) {
	members := map[string]bool{"core": true, "signing": true}
	in := []byte("name: app\ndependencies:\n  core: ^1.0.0\n  signing: ^0.3.0\n  http: ^1.5.0\n")
	out, changed := pubspec{}.SetSiblings(in, members, Version{2, 0, 0})
	got := string(out)
	if !changed {
		t.Error("changed = false, want true")
	}
	if !strings.Contains(got, "core: ^2.0.0") {
		t.Errorf("member at current version not bumped:\n%s", got)
	}
	if !strings.Contains(got, "signing: ^2.0.0") {
		t.Errorf("member pinned at a *stale* version not repaired:\n%s", got)
	}
	if !strings.Contains(got, "http: ^1.5.0") {
		t.Errorf("external dependency must be left untouched:\n%s", got)
	}
}

// CheckWorkspace is the language-agnostic honesty assertion the per-file version: check is
// blind to: across the workspace it flags a member-to-member constraint pinned at an old
// version (resolving the manager generically via the unit's manifest name), passing when all
// agree. The pubspec format participates only because it implements version.Workspace.
func TestCheckWorkspace(t *testing.T) {
	// Two members, both at the canonical version; signing depends on core but still pins the
	// OLD version — exactly the drift the user hit. The version: fields look perfectly honest.
	fsys := fstest.MapFS{
		"core/pubspec.yaml":    {Data: []byte("name: core\nversion: 2.0.0\n")},
		"signing/pubspec.yaml": {Data: []byte("name: signing\nversion: 2.0.0\n\ndependencies:\n  core: ^1.0.0\n  http: ^1.5.0\n")},
	}
	units := []ManifestUnit{
		{Dir: "core", Manifests: []string{"pubspec.yaml"}},
		{Dir: "signing", Manifests: []string{"pubspec.yaml"}},
	}

	drifts, err := CheckWorkspace(fsys, "2.0.0", units)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 {
		t.Fatalf("drifts = %+v, want exactly the stale core constraint", drifts)
	}
	if drifts[0].Path != "signing/pubspec.yaml" || !strings.Contains(drifts[0].Reason(), "member core at 1.0.0") {
		t.Errorf("reason = %q, want the stale signing→core constraint", drifts[0].Reason())
	}

	// Honest workspace: bump the constraint to canonical and the drift is gone.
	fsys["signing/pubspec.yaml"] = &fstest.MapFile{Data: []byte("name: signing\nversion: 2.0.0\n\ndependencies:\n  core: ^2.0.0\n")}
	if d, err := CheckWorkspace(fsys, "2.0.0", units); err != nil || len(d) != 0 {
		t.Errorf("honest workspace: drifts=%v err=%v, want none/nil", d, err)
	}
}

// Empty canonical or no units is a no-op: the check costs nothing until configured.
func TestCheckWorkspaceNoOp(t *testing.T) {
	units := []ManifestUnit{{Dir: "a", Manifests: []string{"pubspec.yaml"}}}
	if d, err := CheckWorkspace(fstest.MapFS{}, "", units); err != nil || d != nil {
		t.Errorf("empty canonical: drifts=%v err=%v, want nil/nil", d, err)
	}
}

// CheckManifests is the read-only version: honesty assertion: it flags a drifted manifest,
// passes an honest one, counts only manifests it could examine, and skips files it cannot.
func TestCheckManifests(t *testing.T) {
	fsys := fstest.MapFS{
		"pubspec.yaml":       {Data: []byte("name: root\nversion: 2.0.0\n")},     // honest
		"pkg/a/pubspec.yaml": {Data: []byte("name: a\nversion: 1.0.0\n")},        // drifted
		"pkg/b/Cargo.toml":   {Data: []byte("[package]\nversion = \"2.0.0\"\n")}, // honest, other lang
		"app/pubspec.yaml":   {Data: []byte("name: app\n")},                      // no version → skipped
	}
	units := []ManifestUnit{
		{Dir: ".", Manifests: []string{"pubspec.yaml"}},
		{Dir: "pkg/a", Manifests: []string{"pubspec.yaml"}},
		{Dir: "pkg/b", Manifests: []string{"Cargo.toml"}},
		{Dir: "app", Manifests: []string{"pubspec.yaml"}},
		{Dir: "gone", Manifests: []string{"pubspec.yaml"}}, // missing file → skipped
	}
	drifts, checked, err := CheckManifests(fsys, "2.0.0", units)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 3 { // root + pkg/a + pkg/b examined; app(no version) & gone(missing) skipped
		t.Errorf("checked = %d, want 3", checked)
	}
	if len(drifts) != 1 || drifts[0].Path != "pkg/a/pubspec.yaml" {
		t.Fatalf("drifts = %+v, want only pkg/a/pubspec.yaml", drifts)
	}
	if !strings.Contains(drifts[0].Reason(), "canonical 2.0.0") {
		t.Errorf("reason = %q, want canonical mention", drifts[0].Reason())
	}
}

// No canonical (and no units) is a no-op: the check costs nothing until a project sets one.
func TestCheckManifestsNoOp(t *testing.T) {
	d, checked, err := CheckManifests(fstest.MapFS{}, "", []ManifestUnit{{Dir: ".", Manifests: []string{"pubspec.yaml"}}})
	if err != nil || d != nil || checked != 0 {
		t.Errorf("empty canonical: got drifts=%v checked=%d err=%v, want nil/0/nil", d, checked, err)
	}
}
