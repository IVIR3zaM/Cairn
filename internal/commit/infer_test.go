package commit

import "testing"

// TestInferBump checks that inference takes the strongest signal across a range: the highest
// bump any commit implies wins, breaking trumps feat trumps fix, and a history with nothing
// release-worthy (or no commits at all) infers no bump.
func TestInferBump(t *testing.T) {
	v, ok := ValidatorFor("conventional")
	if !ok {
		t.Fatal("conventional validator not registered")
	}
	cases := []struct {
		name    string
		history []string
		want    Bump
	}{
		{"empty history", nil, BumpNone},
		{"chores only", []string{"docs: tidy readme", "chore: deps"}, BumpNone},
		{"a fix", []string{"chore: deps", "fix: npe"}, BumpPatch},
		{"feat outranks fix", []string{"fix: npe", "feat: add flag", "docs: x"}, BumpMinor},
		{"breaking outranks all", []string{"feat: add flag", "fix!: drop api", "fix: npe"}, BumpMajor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferBump(v, tc.history); got != tc.want {
				t.Errorf("InferBump = %v, want %v", got, tc.want)
			}
		})
	}
}
