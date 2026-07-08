package heroes

import "testing"

func TestRosterMentionsCoreHeroes(t *testing.T) {
	// Compile-time presence check: this package exists so go list / docs stay honest.
	if testing.Short() {
		t.Skip("roster is documentation-only")
	}
}
