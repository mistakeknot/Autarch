package scheduler

import (
	"github.com/mistakeknot/autarch/internal/mycroft"
)

// AllowlistCheck tests whether a bead is eligible for T2 auto-dispatch
// based on the tier2_dispatch_allowlist in config. Returns true if any
// allowlist entry matches (type, priority, complexity).
//
// An empty allowlist means nothing is auto-dispatchable at T2.
// Unknown complexity escalates (returns false) unless the entry says "any".
func AllowlistCheck(bead mycroft.BeadView, allowlist []mycroft.AllowlistEntry) bool {
	for _, entry := range allowlist {
		if matchesEntry(bead, entry) {
			return true
		}
	}
	return false
}

func matchesEntry(bead mycroft.BeadView, entry mycroft.AllowlistEntry) bool {
	// Type must match.
	if entry.Type != "" && entry.Type != bead.Type {
		return false
	}

	// Priority must be within limit (higher number = lower priority = safer).
	if bead.Priority < entry.MaxPriority {
		return false
	}

	// Complexity must be within limit.
	if !complexityWithin(bead.Complexity, entry.MaxComplexity) {
		return false
	}

	return true
}

// complexityWithin returns true if the bead's complexity is at or below the max.
// "any" allows all complexities. "unknown" always fails (escalate to user).
func complexityWithin(beadComplexity, maxComplexity string) bool {
	// Unknown bead complexity always escalates — safe default per brainstorm.
	if beadComplexity == "unknown" || beadComplexity == "" {
		return false
	}
	if maxComplexity == "any" {
		return true
	}
	return complexityRank(beadComplexity) <= complexityRank(maxComplexity)
}
