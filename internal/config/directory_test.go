package config

import "testing"

func strptr(s string) *string { return &s }
func boolptr(b bool) *bool    { return &b }

// overlay must resolve field-by-field: a field set in the higher layer wins, an unset one
// inherits the lower layer — the single contract the whole cascade is built on.
func TestOverlayFieldLevel(t *testing.T) {
	base := Directory{
		Version:    strptr("1.0.0"),
		Versioning: strptr("semver"),
		Enabled:    boolptr(true),
	}
	over := Directory{
		Version: strptr("2.0.0"), // set ⇒ wins
		// Versioning unset ⇒ inherits base
		// Enabled unset ⇒ inherits base
	}
	got := overlay(base, over)
	if got.Version == nil || *got.Version != "2.0.0" {
		t.Errorf("Version: set field should win, got %v", deref(got.Version))
	}
	if got.Versioning == nil || *got.Versioning != "semver" {
		t.Errorf("Versioning: unset should inherit base, got %v", deref(got.Versioning))
	}
	if got.Enabled == nil || *got.Enabled != true {
		t.Errorf("Enabled: unset should inherit base, got %v", got.Enabled)
	}
}

// Languages merge by name: an override entry replaces that language's block while the
// other languages survive — so a directory tweaks one language without restating the rest.
func TestOverlayLanguagesMergeByName(t *testing.T) {
	base := Directory{Languages: map[string]Language{
		"go":   {Enabled: true},
		"dart": {Enabled: true, Strict: boolptr(true)},
	}}
	over := Directory{Languages: map[string]Language{
		"dart": {Enabled: true, Strict: boolptr(false)}, // replaces dart only
	}}
	got := overlay(base, over)
	if _, ok := got.Languages["go"]; !ok {
		t.Error("go entry should survive an override that only names dart")
	}
	d := got.Languages["dart"]
	if d.Strict == nil || *d.Strict != false {
		t.Errorf("dart: override entry should win, got strict=%v", d.Strict)
	}
}

// cascade folds low→high so the nearest layer that sets a field wins; the precedence
// example: a root directories.<path> entry (highest) beats the directory's own file.
func TestCascadeNearestWins(t *testing.T) {
	baseline := Directory{Languages: map[string]Language{"dart": {Strict: boolptr(true)}}}
	ownFile := Directory{Languages: map[string]Language{"dart": {Strict: boolptr(false)}}}
	rootDirEntry := Directory{Languages: map[string]Language{"dart": {Strict: boolptr(true)}}}

	got := cascade(baseline, ownFile, rootDirEntry)
	if s := got.Languages["dart"].Strict; s == nil || *s != true {
		t.Errorf("root directories entry should beat own file (layer 3 over layer 2), got %v", s)
	}

	// Second example: no root entry ⇒ the own file (layer 2) beats the baseline (layer 1).
	got2 := cascade(baseline, ownFile)
	if s := got2.Languages["dart"].Strict; s == nil || *s != false {
		t.Errorf("own file should beat baseline when root is silent, got %v", s)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
