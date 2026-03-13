// Package scheduler implements bead ranking, dispatch execution, and
// pre-dispatch conflict checking.
package scheduler

import (
	"sort"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// RankBeads returns beads sorted by dispatch priority.
// Ranking criteria (ordered):
//  1. Priority (P0 > P1 > P2 > P3 > P4 — lower number = higher priority)
//  2. Dependency-readiness (resolved deps first; unresolved excluded)
//  3. Age (oldest first within same priority)
//  4. Complexity match (simple beads first when agents are available)
func RankBeads(beads []mycroft.BeadView) []mycroft.BeadView {
	// Filter out beads with unresolved dependencies.
	eligible := make([]mycroft.BeadView, 0, len(beads))
	for _, b := range beads {
		if b.DepsResolved {
			eligible = append(eligible, b)
		}
	}

	sort.Slice(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]

		// 1. Priority (lower = more urgent).
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}

		// 2. Age (older first).
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}

		// 3. Complexity (simple first — faster dispatch).
		return complexityRank(a.Complexity) < complexityRank(b.Complexity)
	})

	return eligible
}

// complexityRank maps complexity labels to sort order.
func complexityRank(c string) int {
	switch c {
	case "simple":
		return 0
	case "medium":
		return 1
	case "complex":
		return 2
	default:
		return 3 // unknown = escalate (sort last)
	}
}

// SelectForAgent picks the top-N beads from the ranked list that an agent
// can handle, checking for interlock conflicts.
func SelectForAgent(ranked []mycroft.BeadView, agent mycroft.AgentView, conflicts []mycroft.ConflictView, maxSuggestions int) []mycroft.BeadView {
	if maxSuggestions <= 0 {
		maxSuggestions = 3
	}

	conflictFiles := make(map[string]bool)
	for _, c := range conflicts {
		for _, holder := range c.Holders {
			if holder == agent.Name {
				conflictFiles[c.File] = true
			}
		}
	}

	var selected []mycroft.BeadView
	for _, b := range ranked {
		if len(selected) >= maxSuggestions {
			break
		}

		// Skip beads already claimed by another agent.
		if b.ClaimedBy != "" && b.ClaimedBy != agent.Name {
			continue
		}

		selected = append(selected, b)
	}

	return selected
}
