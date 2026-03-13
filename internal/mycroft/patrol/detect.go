package patrol

import (
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// StuckThresholds are phase-aware timeouts for detecting stuck agents.
var StuckThresholds = map[string]time.Duration{
	"brainstorm": 15 * time.Minute,
	"research":   15 * time.Minute,
	"plan":       10 * time.Minute,
	"build":      5 * time.Minute,
	"test":       5 * time.Minute,
	"review":     10 * time.Minute,
	"default":    5 * time.Minute,
}

// ClassifyFailure determines the failure class of an agent based on its
// current state, git status, and interlock reservations.
func ClassifyFailure(agent mycroft.AgentView, gitDirty bool, gitCorrupted bool, phase string) mycroft.FailureClass {
	// Healthy agents need no recovery.
	if agent.Status == "active" || agent.Status == "idle" {
		return mycroft.FailureHealthy
	}

	// Corrupted detection: known-bad git state.
	if gitCorrupted {
		return mycroft.FailureCorrupted
	}

	// Stuck or crashed with no uncommitted work — clean failure.
	if !gitDirty {
		return mycroft.FailureClean
	}

	// Degraded: high activity but no bead progress.
	if agent.Status == "stuck" && agent.Health.IsHealthy {
		return mycroft.FailureDegraded
	}

	// Dirty: uncommitted changes present.
	return mycroft.FailureDirty
}

// IsStuck returns true if an agent has been inactive beyond the phase threshold.
func IsStuck(lastSeen time.Time, phase string) bool {
	threshold, ok := StuckThresholds[phase]
	if !ok {
		threshold = StuckThresholds["default"]
	}
	return time.Since(lastSeen) > threshold
}

// IsClaimStale returns true if a bead claim is older than the staleness threshold.
// Two-phase TTL: 90s dispatch→heartbeat, 45min running.
func IsClaimStale(claimedAt time.Time, hasHeartbeat bool) bool {
	if hasHeartbeat {
		// Phase 2: running agent, 45-minute TTL.
		return time.Since(claimedAt) > 45*time.Minute
	}
	// Phase 1: dispatched but no heartbeat yet, 90-second TTL.
	return time.Since(claimedAt) > 90*time.Second
}
