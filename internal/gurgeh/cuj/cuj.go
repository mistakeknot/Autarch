// Package cuj provides Critical User Journey management for Gurgeh PRD creation.
// CUJs are first-class entities that define user journeys through the product,
// with steps, success criteria, and error recovery paths.
package cuj

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/intermute"
)

// Priority represents the priority level of a CUJ
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Status represents the lifecycle status of a CUJ
type Status string

const (
	StatusDraft     Status = "draft"
	StatusValidated Status = "validated"
	StatusArchived  Status = "archived"
)

// Step represents a single step in a Critical User Journey
type Step struct {
	Order        int      `yaml:"order" json:"order"`
	Action       string   `yaml:"action" json:"action"`                                 // "User clicks sign up"
	Expected     string   `yaml:"expected" json:"expected"`                             // "Registration form appears"
	Alternatives []string `yaml:"alternatives,omitempty" json:"alternatives,omitempty"` // Edge cases
}

// CUJ represents a Critical User Journey
type CUJ struct {
	ID              string    `yaml:"id" json:"id"`
	SpecID          string    `yaml:"spec_id" json:"spec_id"`
	Project         string    `yaml:"project" json:"project"`
	Title           string    `yaml:"title" json:"title"`
	Persona         string    `yaml:"persona,omitempty" json:"persona,omitempty"`
	Priority        Priority  `yaml:"priority" json:"priority"`
	EntryPoint      string    `yaml:"entry_point,omitempty" json:"entry_point,omitempty"`
	ExitPoint       string    `yaml:"exit_point,omitempty" json:"exit_point,omitempty"`
	Steps           []Step    `yaml:"steps,omitempty" json:"steps,omitempty"`
	SuccessCriteria []string  `yaml:"success_criteria,omitempty" json:"success_criteria,omitempty"`
	ErrorRecovery   []string  `yaml:"error_recovery,omitempty" json:"error_recovery,omitempty"`
	Status          Status    `yaml:"status" json:"status"`
	Version         int64     `yaml:"version,omitempty" json:"version,omitempty"`
	CreatedAt       time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt       time.Time `yaml:"updated_at" json:"updated_at"`
}

// FeatureLink represents a link between a CUJ and a feature
type FeatureLink struct {
	CUJID     string    `yaml:"cuj_id" json:"cuj_id"`
	FeatureID string    `yaml:"feature_id" json:"feature_id"`
	Project   string    `yaml:"project" json:"project"`
	LinkedAt  time.Time `yaml:"linked_at" json:"linked_at"`
}

// ValidationResult contains the results of validating a CUJ
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// Service provides CUJ management operations
type Service struct {
	client *intermute.Client
}

// NewService creates a new CUJ service with optional Intermute client
func NewService(client *intermute.Client) *Service {
	return &Service{client: client}
}

// Validate checks if a CUJ is complete and valid
func (s *Service) Validate(cuj *CUJ) ValidationResult {
	result := ValidationResult{Valid: true}

	// Required fields
	if strings.TrimSpace(cuj.Title) == "" {
		result.Errors = append(result.Errors, "title is required")
		result.Valid = false
	}
	if strings.TrimSpace(cuj.SpecID) == "" {
		result.Errors = append(result.Errors, "spec_id is required")
		result.Valid = false
	}

	// Steps validation
	if len(cuj.Steps) == 0 {
		result.Warnings = append(result.Warnings, "no steps defined")
	} else {
		for i, step := range cuj.Steps {
			if strings.TrimSpace(step.Action) == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("step %d: action is required", i+1))
				result.Valid = false
			}
			if strings.TrimSpace(step.Expected) == "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("step %d: expected outcome not defined", i+1))
			}
		}
	}

	// Success criteria
	if len(cuj.SuccessCriteria) == 0 {
		result.Warnings = append(result.Warnings, "no success criteria defined")
	}

	// Error recovery for high-priority CUJs
	if cuj.Priority == PriorityHigh && len(cuj.ErrorRecovery) == 0 {
		result.Warnings = append(result.Warnings, "high-priority CUJ should have error recovery defined")
	}

	// Entry/exit points
	if strings.TrimSpace(cuj.EntryPoint) == "" {
		result.Warnings = append(result.Warnings, "entry point not defined")
	}
	if strings.TrimSpace(cuj.ExitPoint) == "" {
		result.Warnings = append(result.Warnings, "exit point (success state) not defined")
	}

	return result
}

// Create creates a new CUJ and persists it to Intermute if client is available
func (s *Service) Create(ctx context.Context, cuj *CUJ) (*CUJ, error) {
	// Validate first
	result := s.Validate(cuj)
	if !result.Valid {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(result.Errors, "; "))
	}

	// Set defaults
	if cuj.Status == "" {
		cuj.Status = StatusDraft
	}
	if cuj.Priority == "" {
		cuj.Priority = PriorityMedium
	}
	now := time.Now()
	if cuj.CreatedAt.IsZero() {
		cuj.CreatedAt = now
	}
	cuj.UpdatedAt = now

	// Normalize step orders
	for i := range cuj.Steps {
		cuj.Steps[i].Order = i + 1
	}

	// Persist to Intermute if client available
	if s.client != nil {
		created, err := s.client.CreateCUJ(ctx, toIntermuteCUJ(cuj))
		if err != nil {
			return nil, fmt.Errorf("failed to persist CUJ: %w", err)
		}
		return fromIntermuteCUJ(&created), nil
	}

	return cuj, nil
}

// Get retrieves a CUJ by ID
func (s *Service) Get(ctx context.Context, id string) (*CUJ, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Intermute client not configured")
	}
	cuj, err := s.client.GetCUJ(ctx, id)
	if err != nil {
		return nil, err
	}
	return fromIntermuteCUJ(&cuj), nil
}

// List retrieves all CUJs for a spec
func (s *Service) List(ctx context.Context, specID string) ([]*CUJ, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Intermute client not configured")
	}
	cujs, err := s.client.ListCUJs(ctx, specID)
	if err != nil {
		return nil, err
	}
	result := make([]*CUJ, len(cujs))
	for i := range cujs {
		result[i] = fromIntermuteCUJ(&cujs[i])
	}
	return result, nil
}

// Update updates an existing CUJ
func (s *Service) Update(ctx context.Context, cuj *CUJ) (*CUJ, error) {
	result := s.Validate(cuj)
	if !result.Valid {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(result.Errors, "; "))
	}

	cuj.UpdatedAt = time.Now()

	// Normalize step orders
	for i := range cuj.Steps {
		cuj.Steps[i].Order = i + 1
	}

	if s.client != nil {
		updated, err := s.client.UpdateCUJ(ctx, toIntermuteCUJ(cuj))
		if err != nil {
			return nil, fmt.Errorf("failed to update CUJ: %w", err)
		}
		return fromIntermuteCUJ(&updated), nil
	}

	return cuj, nil
}

// Delete removes a CUJ
func (s *Service) Delete(ctx context.Context, id string) error {
	if s.client == nil {
		return fmt.Errorf("Intermute client not configured")
	}
	return s.client.DeleteCUJ(ctx, id)
}

// LinkToFeature links a CUJ to a feature
func (s *Service) LinkToFeature(ctx context.Context, cujID, featureID string) error {
	if s.client == nil {
		return fmt.Errorf("Intermute client not configured")
	}
	return s.client.LinkCUJToFeature(ctx, cujID, featureID)
}

// UnlinkFromFeature removes a link between a CUJ and a feature
func (s *Service) UnlinkFromFeature(ctx context.Context, cujID, featureID string) error {
	if s.client == nil {
		return fmt.Errorf("Intermute client not configured")
	}
	return s.client.UnlinkCUJFromFeature(ctx, cujID, featureID)
}

// GetFeatureLinks retrieves all feature links for a CUJ
func (s *Service) GetFeatureLinks(ctx context.Context, cujID string) ([]FeatureLink, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Intermute client not configured")
	}
	links, err := s.client.GetCUJFeatureLinks(ctx, cujID)
	if err != nil {
		return nil, err
	}
	result := make([]FeatureLink, len(links))
	for i, link := range links {
		result[i] = FeatureLink{
			CUJID:     link.CUJID,
			FeatureID: link.FeatureID,
			Project:   link.Project,
			LinkedAt:  link.LinkedAt,
		}
	}
	return result, nil
}

// MarkValidated updates a CUJ's status to validated
func (s *Service) MarkValidated(ctx context.Context, id string) (*CUJ, error) {
	cuj, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Re-validate before marking validated
	result := s.Validate(cuj)
	if !result.Valid {
		return nil, fmt.Errorf("cannot validate CUJ: %s", strings.Join(result.Errors, "; "))
	}
	if len(result.Warnings) > 0 {
		// For now, just log warnings but allow validation
		// In the future, we might want to require resolution of certain warnings
	}

	cuj.Status = StatusValidated
	return s.Update(ctx, cuj)
}

// CanSpecAdvance checks if a spec can advance to "validated" status
// based on whether all its CUJs are validated
func (s *Service) CanSpecAdvance(ctx context.Context, specID string) (bool, []string, error) {
	cujs, err := s.List(ctx, specID)
	if err != nil {
		return false, nil, err
	}

	if len(cujs) == 0 {
		return false, []string{"spec has no CUJs defined"}, nil
	}

	var blockers []string
	for _, cuj := range cujs {
		if cuj.Status != StatusValidated {
			blockers = append(blockers, fmt.Sprintf("CUJ %q (%s) is not validated (status: %s)", cuj.Title, cuj.ID, cuj.Status))
		}
	}

	return len(blockers) == 0, blockers, nil
}

// --- Conversion helpers ---

func toIntermuteCUJ(cuj *CUJ) intermute.CriticalUserJourney {
	steps := make([]intermute.CUJStep, len(cuj.Steps))
	for i, s := range cuj.Steps {
		steps[i] = intermute.CUJStep{
			Order:        s.Order,
			Action:       s.Action,
			Expected:     s.Expected,
			Alternatives: s.Alternatives,
		}
	}
	return intermute.CriticalUserJourney{
		ID:              cuj.ID,
		SpecID:          cuj.SpecID,
		Project:         cuj.Project,
		Title:           cuj.Title,
		Persona:         cuj.Persona,
		Priority:        intermute.CUJPriority(cuj.Priority),
		EntryPoint:      cuj.EntryPoint,
		ExitPoint:       cuj.ExitPoint,
		Steps:           steps,
		SuccessCriteria: cuj.SuccessCriteria,
		ErrorRecovery:   cuj.ErrorRecovery,
		Status:          intermute.CUJStatus(cuj.Status),
		Version:         cuj.Version,
		CreatedAt:       cuj.CreatedAt,
		UpdatedAt:       cuj.UpdatedAt,
	}
}

func fromIntermuteCUJ(cuj *intermute.CriticalUserJourney) *CUJ {
	steps := make([]Step, len(cuj.Steps))
	for i, s := range cuj.Steps {
		steps[i] = Step{
			Order:        s.Order,
			Action:       s.Action,
			Expected:     s.Expected,
			Alternatives: s.Alternatives,
		}
	}
	return &CUJ{
		ID:              cuj.ID,
		SpecID:          cuj.SpecID,
		Project:         cuj.Project,
		Title:           cuj.Title,
		Persona:         cuj.Persona,
		Priority:        Priority(cuj.Priority),
		EntryPoint:      cuj.EntryPoint,
		ExitPoint:       cuj.ExitPoint,
		Steps:           steps,
		SuccessCriteria: cuj.SuccessCriteria,
		ErrorRecovery:   cuj.ErrorRecovery,
		Status:          Status(cuj.Status),
		Version:         cuj.Version,
		CreatedAt:       cuj.CreatedAt,
		UpdatedAt:       cuj.UpdatedAt,
	}
}
