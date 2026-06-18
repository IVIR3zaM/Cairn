package commit

// InferBump classifies every commit message in history against the convention v and returns
// the highest bump any of them implies — the level a single release covering them all should
// take. Because the Bump values are ordered (none < patch < minor < major), aggregating a
// range is just their maximum: one feat among many chores ⇒ minor, any breaking change ⇒
// major. An empty history (no commits since the last tag, or none release-worthy) yields
// BumpNone, whose Level() is "" — i.e. "nothing to release".
func InferBump(v Validator, history []string) Bump {
	highest := BumpNone
	for _, msg := range history {
		if b := v.Classify(msg); b > highest {
			highest = b
		}
	}
	return highest
}
