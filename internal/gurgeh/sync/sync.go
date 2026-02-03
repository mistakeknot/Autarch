// Package sync provides synchronization between Gurgeh's file-based specs
// and the Intermute coordination server. This enables real-time updates
// across all Autarch tools.
package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/cuj"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// Syncer provides bidirectional synchronization between file-based specs
// and Intermute's API.
type Syncer struct {
	client  *intermute.Client
	project string
}

// NewSyncer creates a new spec syncer with an Intermute client.
func NewSyncer(client *intermute.Client, project string) *Syncer {
	return &Syncer{
		client:  client,
		project: project,
	}
}

// PushSpec uploads a spec to Intermute, creating or updating as needed.
func (s *Syncer) PushSpec(ctx context.Context, spec specs.Spec) (intermute.Spec, error) {
	if s.client == nil {
		return intermute.Spec{}, fmt.Errorf("Intermute client not configured")
	}

	iSpec := toIntermuteSpec(spec, s.project)

	// Try to get existing spec first
	existing, err := s.client.GetSpec(ctx, spec.ID)
	if err != nil {
		// Assume it doesn't exist, create new
		return s.client.CreateSpec(ctx, iSpec)
	}

	// Update existing
	iSpec.Version = existing.Version // Preserve version for optimistic locking
	return s.client.UpdateSpec(ctx, iSpec)
}

// PullSpec downloads a spec from Intermute and returns it.
func (s *Syncer) PullSpec(ctx context.Context, id string) (specs.Spec, error) {
	if s.client == nil {
		return specs.Spec{}, fmt.Errorf("Intermute client not configured")
	}

	iSpec, err := s.client.GetSpec(ctx, id)
	if err != nil {
		return specs.Spec{}, err
	}

	return fromIntermuteSpec(iSpec), nil
}

// PushPRD uploads a PRD and all its features to Intermute.
func (s *Syncer) PushPRD(ctx context.Context, prd *specs.PRD) error {
	if s.client == nil {
		return fmt.Errorf("Intermute client not configured")
	}

	// Convert PRD to spec format for Intermute
	iSpec := intermute.Spec{
		ID:      prd.ID,
		Project: s.project,
		Title:   prd.Title,
		Status:  intermute.SpecStatus(prd.Status),
	}

	// Set timestamps
	if prd.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, prd.CreatedAt); err == nil {
			iSpec.CreatedAt = t
		}
	}
	if prd.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, prd.UpdatedAt); err == nil {
			iSpec.UpdatedAt = t
		}
	}

	// Check if spec exists
	existing, err := s.client.GetSpec(ctx, prd.ID)
	if err != nil {
		// Create new
		_, err = s.client.CreateSpec(ctx, iSpec)
		if err != nil {
			return fmt.Errorf("failed to create spec: %w", err)
		}
	} else {
		// Update existing
		iSpec.Version = existing.Version
		_, err = s.client.UpdateSpec(ctx, iSpec)
		if err != nil {
			return fmt.Errorf("failed to update spec: %w", err)
		}
	}

	// Sync CUJs from features
	cujSvc := cuj.NewService(s.client)
	cujs := cuj.MigratePRDCUJs(prd, s.project)
	for _, c := range cujs {
		c.SpecID = prd.ID
		_, err := cujSvc.Create(ctx, c)
		if err != nil {
			// Try update if create fails
			_, err = cujSvc.Update(ctx, c)
			if err != nil {
				return fmt.Errorf("failed to sync CUJ %s: %w", c.ID, err)
			}
		}
	}

	return nil
}

// ListSpecs returns all specs from Intermute.
func (s *Syncer) ListSpecs(ctx context.Context, status string) ([]intermute.Spec, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Intermute client not configured")
	}
	return s.client.ListSpecs(ctx, status)
}

// SyncOnSave is a hook that can be called after saving a spec file.
// It attempts to push the spec to Intermute in the background.
func (s *Syncer) SyncOnSave(ctx context.Context, spec specs.Spec) error {
	_, err := s.PushSpec(ctx, spec)
	return err
}

// --- Conversion helpers ---

func toIntermuteSpec(spec specs.Spec, project string) intermute.Spec {
	iSpec := intermute.Spec{
		ID:      spec.ID,
		Project: project,
		Title:   spec.Title,
		Vision:  spec.Summary, // Map summary to vision
		Problem: spec.UserStory.Text,
		Status:  mapSpecStatus(spec.Status),
	}

	// Parse created_at if present
	if spec.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, spec.CreatedAt); err == nil {
			iSpec.CreatedAt = t
		}
	}
	iSpec.UpdatedAt = time.Now()

	return iSpec
}

func fromIntermuteSpec(iSpec intermute.Spec) specs.Spec {
	return specs.Spec{
		ID:        iSpec.ID,
		Title:     iSpec.Title,
		CreatedAt: iSpec.CreatedAt.Format(time.RFC3339),
		Status:    string(iSpec.Status),
		Summary:   iSpec.Vision,
		UserStory: specs.UserStory{Text: iSpec.Problem},
	}
}

func mapSpecStatus(status string) intermute.SpecStatus {
	switch strings.ToLower(status) {
	case "draft":
		return intermute.SpecStatusDraft
	case "research":
		return intermute.SpecStatusResearch
	case "validated", "approved":
		return intermute.SpecStatusValidated
	case "archived", "done":
		return intermute.SpecStatusArchived
	case "needs_revision":
		return intermute.SpecStatusNeedsRevision
	default:
		return intermute.SpecStatusDraft
	}
}
