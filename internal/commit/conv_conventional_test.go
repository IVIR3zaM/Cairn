package commit

import "testing"

// Classify is the contract bump inference relies on: type → SemVer level, with `!` and a
// BREAKING CHANGE footer both trumping the type, and anything non-release (or malformed)
// landing at none.
func TestConventionalClassify(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want Bump
	}{
		{"feat is minor", "feat: add widget", BumpMinor},
		{"fix is patch", "fix(parser): handle empty input", BumpPatch},
		{"bang is major", "feat!: drop v1 API", BumpMajor},
		{"bang beats fix", "fix(core)!: rename flag", BumpMajor},
		{"breaking footer is major", "feat: redo\n\nBREAKING CHANGE: config moved", BumpMajor},
		{"breaking-change hyphen footer is major", "chore: x\n\nBREAKING-CHANGE: gone", BumpMajor},
		{"chore is none", "chore: tidy imports", BumpNone},
		{"docs is none", "docs: fix typo", BumpNone},
		{"non-conforming is none", "just some message", BumpNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (conventional{}).Classify(tc.msg); got != tc.want {
				t.Fatalf("Classify(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

// Validate guards the header shape, the known-type set, and the optional DCO sign-off.
func TestConventionalValidate(t *testing.T) {
	c := conventional{}
	if err := c.Validate("feat(api): add endpoint", false); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	if err := c.Validate("nope: bad type", false); err == nil {
		t.Fatal("unknown type accepted")
	}
	if err := c.Validate("missing colon and type", false); err == nil {
		t.Fatal("non-conforming header accepted")
	}
	if err := c.Validate("feat: needs signoff", true); err == nil {
		t.Fatal("missing sign-off accepted when required")
	}
	if err := c.Validate("feat: ok\n\nSigned-off-by: Dev <d@e.x>", true); err != nil {
		t.Fatalf("present sign-off rejected: %v", err)
	}
}

// The convention resolves through the registry by its config key, and is enumerable.
func TestRegistryResolution(t *testing.T) {
	if _, ok := ValidatorFor("conventional"); !ok {
		t.Fatal(`ValidatorFor("conventional") not registered`)
	}
	if _, ok := ValidatorFor("gitmoji"); ok {
		t.Fatal(`ValidatorFor("gitmoji") should be absent (future addition)`)
	}
	found := false
	for _, name := range Conventions() {
		if name == "conventional" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Conventions() = %v, missing conventional", Conventions())
	}
}
