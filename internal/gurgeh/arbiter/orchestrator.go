package arbiter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/confidence"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/consistency"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/quick"
)

// ErrBlocker is returned when a blocker conflict prevents advancing.
var ErrBlocker = errors.New("blocker conflict prevents advance")

// IsBlockerError returns true if the error is or wraps ErrBlocker.
func IsBlockerError(err error) bool {
	return errors.Is(err, ErrBlocker)
}

// HandoffOption represents a post-sprint action.
type HandoffOption struct {
	ID          string
	Label       string
	Description string
	Recommended bool
}

// Orchestrator manages the full spec sprint flow.
type Orchestrator struct {
	projectPath string
	generator   *Generator
	consistency *consistency.Engine
	confidence  *confidence.Calculator
	scanner     *quick.Scanner
}

// NewOrchestrator creates a new Orchestrator for the given project path.
func NewOrchestrator(projectPath string) *Orchestrator {
	return &Orchestrator{
		projectPath: projectPath,
		generator:   NewGenerator(),
		consistency: consistency.NewEngine(),
		confidence:  confidence.NewCalculator(),
		scanner:     quick.NewScanner(),
	}
}

// Start initializes a new sprint and generates the Problem draft.
func (o *Orchestrator) Start(ctx context.Context, userInput string) (*SprintState, error) {
	state := NewSprintState(o.projectPath)
	projectCtx := o.readProjectContext()

	draft, err := o.generator.GenerateDraft(ctx, PhaseProblem, projectCtx, userInput)
	if err != nil {
		return nil, fmt.Errorf("generating problem draft: %w", err)
	}

	state.Sections[PhaseProblem] = draft
	state.UpdatedAt = time.Now()
	return state, nil
}

// Advance runs consistency checks, updates confidence, and moves to the next phase.
func (o *Orchestrator) Advance(ctx context.Context, state *SprintState) (*SprintState, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	// Run consistency checks
	conflicts := o.checkConsistency(state)
	state.Conflicts = conflicts

	// Block on blockers
	for _, c := range state.Conflicts {
		if c.Severity == SeverityBlocker {
			return state, fmt.Errorf("%w: %s", ErrBlocker, c.Message)
		}
	}

	// Update confidence
	o.updateConfidence(state)

	// Advance to next phase
	phases := AllPhases()
	for i, p := range phases {
		if p == state.Phase && i+1 < len(phases) {
			state.Phase = phases[i+1]
			break
		}
	}

	// Trigger quick scan when advancing to FeaturesGoals
	if state.Phase == PhaseFeaturesGoals {
		o.runQuickScan(ctx, state)
	}

	// Generate draft for the new phase
	projectCtx := o.readProjectContext()
	draft, err := o.generator.GenerateDraft(ctx, state.Phase, projectCtx, "")
	if err != nil {
		return nil, fmt.Errorf("generating draft for %s: %w", state.Phase, err)
	}
	state.Sections[state.Phase] = draft
	state.UpdatedAt = time.Now()

	return state, nil
}

// AcceptDraft marks the current phase's draft as accepted.
func (o *Orchestrator) AcceptDraft(state *SprintState) *SprintState {
	if section, ok := state.Sections[state.Phase]; ok {
		section.Status = DraftAccepted
		section.UpdatedAt = time.Now()
	}
	state.UpdatedAt = time.Now()
	return state
}

// ReviseDraft updates the current phase's draft with new content.
func (o *Orchestrator) ReviseDraft(state *SprintState, newContent string, reason string) *SprintState {
	if section, ok := state.Sections[state.Phase]; ok {
		edit := Edit{
			Before:    section.Content,
			After:     newContent,
			Reason:    reason,
			Timestamp: time.Now(),
		}
		section.UserEdits = append(section.UserEdits, edit)
		section.Content = newContent
		section.Status = DraftNeedsRevision
		section.UpdatedAt = time.Now()
	}
	state.UpdatedAt = time.Now()
	return state
}

// GetHandoffOptions returns available post-sprint actions.
func (o *Orchestrator) GetHandoffOptions(state *SprintState) []HandoffOption {
	return []HandoffOption{
		{
			ID:          "research",
			Label:       "Deep Research",
			Description: "Run Pollard hunters for competitive analysis and prior art",
			Recommended: state.Confidence.Research < 0.7,
		},
		{
			ID:          "tasks",
			Label:       "Generate Tasks",
			Description: "Break the PRD into implementation tasks via Coldwine",
			Recommended: state.Confidence.Total() >= 0.7,
		},
		{
			ID:          "export",
			Label:       "Export PRD",
			Description: "Export the spec as Markdown",
			Recommended: false,
		},
	}
}

// readProjectContext is a stub that will eventually read project metadata.
func (o *Orchestrator) readProjectContext() *ProjectContext {
	return nil
}

// checkConsistency converts state to the consistency package's format and checks.
func (o *Orchestrator) checkConsistency(state *SprintState) []Conflict {
	sections := make(map[int]*consistency.SectionInfo)
	for phase, section := range state.Sections {
		sections[int(phase)] = &consistency.SectionInfo{
			Content:  section.Content,
			Accepted: section.Status == DraftAccepted,
		}
	}

	cConflicts := o.consistency.Check(sections)
	var conflicts []Conflict
	for _, cc := range cConflicts {
		var phases []Phase
		for _, s := range cc.Sections {
			phases = append(phases, Phase(s))
		}
		conflicts = append(conflicts, Conflict{
			Type:     ConflictType(cc.TypeCode),
			Severity: Severity(cc.Severity),
			Message:  cc.Message,
			Sections: phases,
		})
	}
	return conflicts
}

// updateConfidence computes and sets the confidence score on the state.
func (o *Orchestrator) updateConfidence(state *SprintState) {
	phases := AllPhases()
	accepted := 0
	for _, p := range phases {
		if s, ok := state.Sections[p]; ok && s.Status == DraftAccepted {
			accepted++
		}
	}

	score := o.confidence.Calculate(len(phases), accepted, len(state.Conflicts), state.ResearchCtx != nil)
	state.Confidence = ConfidenceScore{
		Completeness: score.Completeness,
		Consistency:  score.Consistency,
		Specificity:  score.Specificity,
		Research:     score.Research,
		Assumptions:  score.Assumptions,
	}
}

// runQuickScan extracts a topic from the sprint state and runs a quick scan.
func (o *Orchestrator) runQuickScan(ctx context.Context, state *SprintState) {
	topic := ""
	if section, ok := state.Sections[PhaseProblem]; ok && section.Content != "" {
		topic = section.Content
		if len(topic) > 100 {
			topic = topic[:100]
		}
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return
	}

	result, err := o.scanner.Scan(ctx, topic, o.projectPath)
	if err != nil {
		return
	}
	state.ResearchCtx = &QuickScanResult{
		Topic:     result.Topic,
		Summary:   result.Summary,
		ScannedAt: result.ScannedAt,
	}
}
