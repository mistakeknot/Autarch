package homeask

import "testing"

// The GitHub sync planner (integrations/bb-plugin-autarch/ghsync.ts) files a pr move with only a url. Its own test pins
// that shape; this one runs that exact JSON through the Go validator the filer uses, so the two cannot drift apart.
func TestGhSyncPlannerMoveParses(t *testing.T) {
	body := `{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/mistakeknot/Clavain/pull/30"}}`
	if _, err := ParseMove(body); err != nil {
		t.Fatal(err)
	}
}
