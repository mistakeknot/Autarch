package arbiter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/confidence"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/consistency"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/thinking"
	"gopkg.in/yaml.v3"
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
	mu          sync.Mutex
	projectPath string
	generator   *Generator
	consistency *consistency.Engine
	confidence  *confidence.Calculator
	scanner     QuickScanner
	research    ResearchProvider // nil = no-research mode
	state       *SprintState    // current sprint state (nil before Start)
}

// NewOrchestrator creates a new Orchestrator for the given project path.
// Uses a stub scanner; call SetScanner to inject the real Pollard scanner.
func NewOrchestrator(projectPath string) *Orchestrator {
	return &Orchestrator{
		projectPath: projectPath,
		generator:   NewGenerator(),
		consistency: consistency.NewEngine(),
		confidence:  confidence.NewCalculator(),
		scanner:     &stubScanner{},
	}
}

// SetScanner replaces the quick scanner implementation.
// Use this to inject the real Pollard scanner (internal/pollard/quick.Scanner).
func (o *Orchestrator) SetScanner(s QuickScanner) {
	o.scanner = s
}

// NewOrchestratorWithResearch creates an Orchestrator with Intermute research integration.
func NewOrchestratorWithResearch(projectPath string, research ResearchProvider) *Orchestrator {
	o := NewOrchestrator(projectPath)
	o.research = research
	return o
}

// State returns the current sprint state, or nil if no sprint is active.
func (o *Orchestrator) State() *SprintState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// Start initializes a new sprint and generates the Problem draft.
// If a ResearchProvider is configured, it also creates an Intermute Spec
// to track research findings for this sprint.
func (o *Orchestrator) Start(ctx context.Context, userInput string) (*SprintState, error) {
	state := NewSprintState(o.projectPath)
	projectCtx := o.readProjectContext()

	// Create Intermute Spec if research provider is available
	if o.research != nil {
		title := userInput
		if len(title) > 200 {
			title = title[:200]
		}
		specID, err := o.research.CreateSpec(ctx, state.ID, title)
		if err != nil {
			// Non-fatal: sprint can proceed without research tracking
			_ = err
		} else {
			state.SpecID = specID
		}
	}

	draft, err := o.generator.GenerateDraft(ctx, PhaseVision, projectCtx, userInput)
	if err != nil {
		return nil, fmt.Errorf("generating vision draft: %w", err)
	}

	state.Sections[PhaseVision] = draft
	state.UpdatedAt = time.Now()

	// Auto-discover vision spec for vertical consistency
	state.VisionContext = o.LoadVisionContext()

	o.mu.Lock()
	o.state = state
	o.mu.Unlock()

	return state, nil
}

// StartWithScan initializes a sprint seeded with lossless codebase scan artifacts.
// The first three phases (Vision, Problem, Users) incorporate evidence, quality
// scores, and resolved questions from the kickoff scan.
func (o *Orchestrator) StartWithScan(ctx context.Context, userInput string, artifacts *scan.Artifacts) (*SprintState, error) {
	state, err := o.Start(ctx, userInput)
	if err != nil {
		return nil, err
	}
	if artifacts == nil {
		return state, nil
	}
	state.ScanArtifacts = artifacts

	// Re-generate first 3 phases with scan evidence injected
	projectCtx := o.readProjectContext()
	phaseMap := map[Phase]*scan.PhaseData{
		PhaseVision:  artifacts.Vision,
		PhaseProblem: artifacts.Problem,
		PhaseUsers:   artifacts.Users,
	}
	for phase, pd := range phaseMap {
		if pd == nil {
			continue
		}
		draft, err := o.generator.GenerateDraft(ctx, phase, projectCtx, userInput, pd)
		if err != nil {
			continue // best-effort: fall back to draft without evidence
		}
		state.Sections[phase] = draft
	}

	return state, nil
}

// StartWithResearch initializes a sprint and imports Pollard insights.
// Each Pollard finding is published as an Intermute insight linked to the sprint's spec.
// Requires a ResearchProvider; returns an error if none is configured.
func (o *Orchestrator) StartWithResearch(ctx context.Context, userInput string, pollardFindings []ResearchFinding) (*SprintState, error) {
	state, err := o.Start(ctx, userInput)
	if err != nil {
		return nil, err
	}

	if o.research == nil || state.SpecID == "" || len(pollardFindings) == 0 {
		return state, nil
	}

	for _, f := range pollardFindings {
		_, _ = o.research.PublishInsight(ctx, state.SpecID, f)
	}

	findings, err := o.research.FetchLinkedInsights(ctx, state.SpecID)
	if err == nil && len(findings) > 0 {
		state.Findings = findings
	}

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

	// Trigger quick scan when advancing to FeaturesGoals (legacy)
	if state.Phase == PhaseFeaturesGoals {
		o.runQuickScan(ctx, state)
	}

	// Trigger phase-specific deep research if research provider is available
	if o.research != nil && state.SpecID != "" {
		o.runPhaseResearch(ctx, state)
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
			ID:          "spec",
			Label:       "Export Spec",
			Description: "Export as a structured Spec (YAML-compatible)",
			Recommended: false,
		},
		{
			ID:          "export",
			Label:       "Export PRD",
			Description: "Export the spec as Markdown",
			Recommended: false,
		},
	}
}

// ExportSpec converts a sprint state to a structured Spec.
func (o *Orchestrator) ExportSpec(state *SprintState) (*specs.Spec, error) {
	return ExportToSpec(state)
}

// StartVision initializes a new sprint for a vision-type spec.
func (o *Orchestrator) StartVision(ctx context.Context, userInput string) (*SprintState, error) {
	state, err := o.Start(ctx, userInput)
	if err != nil {
		return nil, err
	}
	// Mark the sprint as producing a vision spec (used by ExportSpec)
	state.SpecType = "vision"
	state.IsReview = false
	return state, nil
}

// StartReview loads an existing vision spec into a new sprint for review.
// Sections without active signals are pre-accepted (AutoAccept=true).
// Sections with signals are left pending for human review.
func (o *Orchestrator) StartReview(ctx context.Context, spec *specs.Spec, activeSignalIDs []string) (*SprintState, error) {
	state := NewSprintState(o.projectPath)
	state.IsReview = true
	state.ReviewingSpecID = spec.ID

	// Build a set of affected fields from signal IDs for quick lookup
	signalFields := make(map[string][]string) // phase-field → signal IDs
	for _, sid := range activeSignalIDs {
		// Signal IDs carry no phase info; we'll match by field name below
		signalFields[sid] = nil
	}

	for _, phase := range AllPhases() {
		draft := extractSectionFromSpec(spec, phase)
		draft.Status = DraftAccepted
		draft.AutoAccept = true

		// Check if any active signals target fields mapped to this phase
		phaseSignals := signalsForPhase(phase, activeSignalIDs)
		if len(phaseSignals) > 0 {
			draft.Status = DraftPending
			draft.AutoAccept = false
			draft.ActiveSignals = phaseSignals
		}
		state.Sections[phase] = draft
	}

	state.VisionContext = o.LoadVisionContext()
	return state, nil
}

// extractSectionFromSpec pulls content from a Spec for the given phase.
func extractSectionFromSpec(spec *specs.Spec, phase Phase) *SectionDraft {
	var content string
	switch phase {
	case PhaseVision:
		content = spec.Summary
	case PhaseProblem:
		content = spec.UserStory.Text
	case PhaseUsers:
		content = spec.UserStory.Text
	case PhaseFeaturesGoals:
		var parts []string
		for _, g := range spec.Goals {
			parts = append(parts, g.Description)
		}
		content = strings.Join(parts, "\n")
	case PhaseRequirements:
		content = strings.Join(spec.Requirements, "\n")
	case PhaseScopeAssumptions:
		var parts []string
		for _, a := range spec.Assumptions {
			parts = append(parts, a.Description)
		}
		content = strings.Join(parts, "\n")
	case PhaseCUJs:
		var parts []string
		for _, c := range spec.CriticalUserJourneys {
			parts = append(parts, c.Title)
		}
		content = strings.Join(parts, "\n")
	case PhaseAcceptanceCriteria:
		var parts []string
		for _, ac := range spec.Acceptance {
			parts = append(parts, ac.Description)
		}
		content = strings.Join(parts, "\n")
	}
	return &SectionDraft{
		Content:   content,
		Status:    DraftPending,
		UpdatedAt: time.Now(),
	}
}

// signalsForPhase returns signal IDs that are relevant to a given phase.
// Mapping is based on the signal's affected_field convention:
//
//	"goals", "scope" → PhaseFeaturesGoals
//	"assumptions" → PhaseScopeAssumptions
//	"hypotheses" → PhaseRequirements
//	"health" → PhaseVision
func signalsForPhase(phase Phase, signalIDs []string) []string {
	// Without the actual signal objects (just IDs), we can't filter by field.
	// In a review sprint, all active signals are shown on PhaseScopeAssumptions
	// (assumptions phase) as the primary review surface. Individual phase
	// routing requires the store to be passed — deferred to TUI layer.
	if phase == PhaseScopeAssumptions && len(signalIDs) > 0 {
		return signalIDs
	}
	return nil
}

// LoadVisionContext scans .gurgeh/specs/ for a type=vision spec and loads it
// as VisionContext for vertical consistency checks. Returns nil if no vision spec exists.
func (o *Orchestrator) LoadVisionContext() *VisionContext {
	specsDir := o.projectPath + "/.gurgeh/specs"
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(specsDir + "/" + entry.Name())
		if err != nil {
			continue
		}
		var spec specs.Spec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		if spec.EffectiveType() != specs.SpecTypeVision {
			continue
		}
		// Found a vision spec — extract context
		vc := &VisionContext{SpecID: spec.ID}
		for _, g := range spec.Goals {
			vc.Goals = append(vc.Goals, g.Description)
		}
		for _, a := range spec.Assumptions {
			vc.Assumptions = append(vc.Assumptions, a.Description)
		}
		for _, c := range spec.CriticalUserJourneys {
			vc.CUJs = append(vc.CUJs, c.Title)
		}
		for _, h := range spec.Hypotheses {
			vc.Hypotheses = append(vc.Hypotheses, h.Statement)
		}
		return vc
	}
	return nil
}

// stubScanner is a no-op scanner used when no real scanner is injected.
type stubScanner struct{}

func (s *stubScanner) Scan(_ context.Context, topic string, _ string) (*QuickScanResult, error) {
	return &QuickScanResult{
		Topic:     topic,
		Summary:   "Quick scan results for: " + topic,
		ScannedAt: time.Now(),
	}, nil
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

	// Pass vision context to consistency engine for vertical checks
	if state.VisionContext != nil {
		o.consistency.SetVision(&consistency.VisionInfo{
			Goals:       state.VisionContext.Goals,
			Assumptions: state.VisionContext.Assumptions,
		})
	} else {
		o.consistency.SetVision(nil)
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

// researchQuality computes a 0.0–1.0 score from sprint research state.
// The score combines finding count (30%), source diversity (30%), and
// average relevance (40%). Returns 0.0 if no research has been performed.
func researchQuality(state *SprintState) float64 {
	// Count all findings across Intermute and legacy quick scan
	findingCount := len(state.Findings)
	if state.ResearchCtx != nil {
		findingCount += len(state.ResearchCtx.GitHubHits) + len(state.ResearchCtx.HNHits)
	}
	if findingCount == 0 {
		return 0.0
	}

	// Source diversity: count distinct source types
	sources := make(map[string]bool)
	for _, f := range state.Findings {
		if f.SourceType != "" {
			sources[f.SourceType] = true
		}
	}
	if state.ResearchCtx != nil {
		if len(state.ResearchCtx.GitHubHits) > 0 {
			sources["github"] = true
		}
		if len(state.ResearchCtx.HNHits) > 0 {
			sources["hackernews"] = true
		}
	}

	// Average relevance (default 0.5 for GitHub/HN hits without scores)
	var relevanceSum float64
	var relevanceCount int
	for _, f := range state.Findings {
		relevanceSum += f.Relevance
		relevanceCount++
	}
	if state.ResearchCtx != nil {
		for range state.ResearchCtx.GitHubHits {
			relevanceSum += 0.5
			relevanceCount++
		}
		for range state.ResearchCtx.HNHits {
			relevanceSum += 0.5
			relevanceCount++
		}
	}
	avgRelevance := 0.0
	if relevanceCount > 0 {
		avgRelevance = relevanceSum / float64(relevanceCount)
	}

	// Weighted formula: 30% count + 30% diversity + 40% relevance
	countScore := clamp01(float64(findingCount) / 10.0)
	diversityScore := clamp01(float64(len(sources)) / 3.0)
	return 0.3*countScore + 0.3*diversityScore + 0.4*avgRelevance
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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

	// Build shapes-used map from phase defaults + overrides
	shapesUsed := make(map[string]thinking.Shape)
	for _, p := range phases {
		if s, ok := state.Sections[p]; ok && s.Status == DraftAccepted {
			if state.ShapeOverrides != nil {
				if shape, has := state.ShapeOverrides[p]; has {
					shapesUsed[p.String()] = shape
					continue
				}
			}
			if shape, has := thinking.PhaseDefault[p.String()]; has {
				shapesUsed[p.String()] = shape
			}
		}
	}
	score := o.confidence.Calculate(len(phases), accepted, len(state.Conflicts), researchQuality(state), shapesUsed)

	// Blend scan quality scores if available
	if state.ScanArtifacts != nil {
		// Average quality across all available scan phases
		var qualities []scan.QualityScores
		for _, pd := range []*scan.PhaseData{state.ScanArtifacts.Vision, state.ScanArtifacts.Problem, state.ScanArtifacts.Users} {
			if pd != nil {
				qualities = append(qualities, pd.Quality)
			}
		}
		if len(qualities) > 0 {
			avg := &scan.QualityScores{}
			for _, q := range qualities {
				avg.Clarity += q.Clarity
				avg.Completeness += q.Completeness
				avg.Grounding += q.Grounding
				avg.Consistency += q.Consistency
			}
			n := float64(len(qualities))
			avg.Clarity /= n
			avg.Completeness /= n
			avg.Grounding /= n
			avg.Consistency /= n
			score = o.confidence.ApplyQualityScores(score, avg)
		}
	}

	state.Confidence = ConfidenceScore{
		Completeness: score.Completeness,
		Consistency:  score.Consistency,
		Specificity:  score.Specificity,
		Research:     score.Research,
		Assumptions:  score.Assumptions,
	}
}

// runPhaseResearch triggers Pollard targeted research for the current phase.
func (o *Orchestrator) runPhaseResearch(ctx context.Context, state *SprintState) {
	cfg := ResearchConfigForPhase(state.Phase)
	if cfg == nil {
		return
	}

	query := cfg.QueryExtractor(state)
	if query == "" {
		return
	}

	// Fire targeted research via the provider (non-blocking, best-effort)
	_ = o.research.RunTargetedScan(ctx, state.SpecID, cfg.Hunters, cfg.Mode, query)

	// Refresh findings after research
	findings, err := o.research.FetchLinkedInsights(ctx, state.SpecID)
	if err == nil && len(findings) > 0 {
		state.Findings = findings
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
	state.ResearchCtx = result

	// Publish scan result as an Intermute Insight and fetch all linked findings
	if o.research != nil && state.SpecID != "" {
		_, _ = o.research.PublishInsight(ctx, state.SpecID, ResearchFinding{
			Title:      "Quick Scan: " + result.Topic,
			Summary:    result.Summary,
			SourceType: "quick-scan",
			Relevance:  0.5,
			Tags:       []string{"quick-scan"},
		})

		findings, err := o.research.FetchLinkedInsights(ctx, state.SpecID)
		if err == nil && len(findings) > 0 {
			state.Findings = findings
		}
	}
}

// ProcessChatMessage handles a user message in the context of the current phase.
// It returns a channel that streams the agent's response chunks. The channel is
// closed when the response is complete.
//
// The caller MUST cancel ctx when navigating away to prevent goroutine leaks.
// Basic retry with backoff (3 attempts) is applied on generation failure.
func (o *Orchestrator) ProcessChatMessage(ctx context.Context, msg string) <-chan string {
	ch := make(chan string, 8)

	go func() {
		defer close(ch)

		o.mu.Lock()
		state := o.state
		o.mu.Unlock()

		if state == nil {
			select {
			case ch <- "No active sprint. Start a sprint first.":
			case <-ctx.Done():
			}
			return
		}

		projectCtx := o.readProjectContext()

		// Retry with backoff: 3 attempts
		var draft *SectionDraft
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Inject user message as additional context for draft generation
			combined := msg
			if section, ok := state.Sections[state.Phase]; ok && section.Content != "" {
				combined = section.Content + "\n\nUser feedback: " + msg
			}

			draft, err = o.generator.GenerateDraft(ctx, state.Phase, projectCtx, combined)
			if err == nil {
				break
			}

			// Backoff: 100ms, 300ms, 900ms
			backoff := time.Duration(100*(attempt*3+1)) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}

		if err != nil {
			select {
			case ch <- fmt.Sprintf("Failed to generate response: %v", err):
			case <-ctx.Done():
			}
			return
		}

		// Update state with the new draft
		o.mu.Lock()
		if o.state != nil {
			o.state.Sections[state.Phase] = draft
			o.state.UpdatedAt = time.Now()
		}
		o.mu.Unlock()

		// Stream the response (single chunk for template-based generation)
		select {
		case ch <- draft.Content:
		case <-ctx.Done():
		}
	}()

	return ch
}

// ChatAcceptDraft accepts the current phase draft, runs consistency checks,
// and advances to the next phase. Returns the updated state or an error
// if a blocker conflict prevents advancing.
func (o *Orchestrator) ChatAcceptDraft(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == nil {
		return fmt.Errorf("no active sprint")
	}

	// Mark current draft as accepted
	if section, ok := o.state.Sections[o.state.Phase]; ok {
		section.Status = DraftAccepted
		section.UpdatedAt = time.Now()
	}
	o.state.UpdatedAt = time.Now()

	// Run consistency + advance (unlock briefly for potentially slow operations)
	state := o.state
	o.mu.Unlock()

	updated, err := o.Advance(ctx, state)

	o.mu.Lock()
	if updated != nil {
		o.state = updated
	}
	return err
}

// ChatReviseDraft requests revision of the current phase draft with user feedback.
// The feedback is recorded as an edit and the draft status is set to NeedsRevision.
func (o *Orchestrator) ChatReviseDraft(feedback string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == nil {
		return fmt.Errorf("no active sprint")
	}

	section, ok := o.state.Sections[o.state.Phase]
	if !ok {
		return fmt.Errorf("no draft for current phase %s", o.state.Phase)
	}

	edit := Edit{
		Before:    section.Content,
		After:     section.Content, // content unchanged until ProcessChatMessage regenerates
		Reason:    feedback,
		Timestamp: time.Now(),
	}
	section.UserEdits = append(section.UserEdits, edit)
	section.Status = DraftNeedsRevision
	section.UpdatedAt = time.Now()
	o.state.UpdatedAt = time.Now()

	return nil
}
