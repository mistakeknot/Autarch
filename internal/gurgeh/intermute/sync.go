// Package intermute provides Intermute integration for Gurgeh PRD generation.
// It synchronizes Gurgeh PRDs with Intermute Specs for cross-tool visibility.
package intermute

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// SpecManager defines the interface for managing specs in Intermute.
type SpecManager interface {
	CreateSpec(ctx context.Context, spec intermute.Spec) (intermute.Spec, error)
	UpdateSpec(ctx context.Context, spec intermute.Spec) (intermute.Spec, error)
	GetSpec(ctx context.Context, id string) (intermute.Spec, error)
}

// PRDSyncer synchronizes Gurgeh PRDs with Intermute Specs.
type PRDSyncer struct {
	client  SpecManager
	project string
}

// NewPRDSyncer creates a new PRD syncer.
// If client is nil, sync operations become no-ops (graceful degradation).
func NewPRDSyncer(client SpecManager, project string) *PRDSyncer {
	return &PRDSyncer{
		client:  client,
		project: project,
	}
}

// SyncPRD creates a new Intermute Spec from a Gurgeh PRD.
// Use this for initial sync when no Intermute ID exists yet.
func (s *PRDSyncer) SyncPRD(ctx context.Context, prd *specs.PRD) (intermute.Spec, error) {
	if s.client == nil {
		return intermute.Spec{}, nil
	}

	spec := mapPRDToSpec(prd, s.project)
	return s.client.CreateSpec(ctx, spec)
}

// SyncPRDWithID updates an existing Intermute Spec from a Gurgeh PRD.
// Use this when you have a known Intermute ID from a previous sync.
func (s *PRDSyncer) SyncPRDWithID(ctx context.Context, prd *specs.PRD, intermuteID string) (intermute.Spec, error) {
	if s.client == nil {
		return intermute.Spec{}, nil
	}

	spec := mapPRDToSpec(prd, s.project)
	spec.ID = intermuteID
	return s.client.UpdateSpec(ctx, spec)
}

// mapPRDToSpec converts a Gurgeh PRD to an Intermute Spec.
func mapPRDToSpec(prd *specs.PRD, project string) intermute.Spec {
	now := time.Now()
	return intermute.Spec{
		// ID is assigned by Intermute on creation
		Project:   project,
		Title:     prd.Title,
		Vision:    extractVisionFromFeatures(prd.Features),
		Users:     extractUsersFromFeatures(prd.Features),
		Problem:   extractProblemFromFeatures(prd.Features),
		Status:    mapPRDStatusToSpecStatus(prd.Status),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// mapPRDStatusToSpecStatus converts Gurgeh PRD status to Intermute Spec status.
// Mapping:
//   - draft       -> draft
//   - approved    -> research (validated enough to research)
//   - in_progress -> validated (actively being built)
//   - done        -> archived
func mapPRDStatusToSpecStatus(status specs.PRDStatus) intermute.SpecStatus {
	switch status {
	case specs.PRDStatusDraft:
		return intermute.SpecStatusDraft
	case specs.PRDStatusApproved:
		return intermute.SpecStatusResearch
	case specs.PRDStatusInProgress:
		return intermute.SpecStatusValidated
	case specs.PRDStatusDone:
		return intermute.SpecStatusArchived
	default:
		return intermute.SpecStatusDraft
	}
}

// extractVisionFromFeatures builds a vision statement from PRD features.
func extractVisionFromFeatures(features []specs.Feature) string {
	if len(features) == 0 {
		return ""
	}

	var parts []string
	for _, f := range features {
		if f.Summary != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", f.Title, f.Summary))
		} else {
			parts = append(parts, f.Title)
		}
	}

	return fmt.Sprintf("Product vision including: %s", strings.Join(parts, "; "))
}

// extractUsersFromFeatures extracts user information from features.
// This looks at CUJs and requirements for user references.
func extractUsersFromFeatures(features []specs.Feature) string {
	var userRefs []string
	seen := make(map[string]bool)

	for _, f := range features {
		for _, cuj := range f.CriticalUserJourneys {
			// CUJ titles often describe user actions
			if !seen[cuj.Title] {
				userRefs = append(userRefs, cuj.Title)
				seen[cuj.Title] = true
			}
		}
	}

	if len(userRefs) == 0 {
		return ""
	}
	return fmt.Sprintf("Users who: %s", strings.Join(userRefs, ", "))
}

// extractProblemFromFeatures extracts problem context from features.
// This uses requirements and summaries to understand the problems being solved.
func extractProblemFromFeatures(features []specs.Feature) string {
	var problems []string
	for _, f := range features {
		if len(f.Requirements) > 0 {
			problems = append(problems, f.Requirements[0])
		}
	}

	if len(problems) == 0 {
		return ""
	}
	return strings.Join(problems, "; ")
}
