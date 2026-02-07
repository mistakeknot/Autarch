package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// scanProgressWithContinuation wraps a progress message with a continuation command.
type scanProgressWithContinuation struct {
	tui.ScanProgressMsg
	nextCmd tea.Cmd
}

// agentStreamEvent represents a single streaming output or final result.
type agentStreamEvent struct {
	line  string
	epics []epics.EpicProposal
	tasks []tasks.TaskProposal
	err   error
}

// agentStreamWithContinuation wraps a stream message with a continuation command.
type agentStreamWithContinuation struct {
	tui.AgentStreamMsg
	nextCmd tea.Cmd
}

// --- Pure helper functions extracted from unified_app.go ---

// ExtractString extracts a string value from a map.
func ExtractString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// ExtractVisionSummary extracts the vision summary from phase artifacts.
func ExtractVisionSummary(a *agent.PhaseArtifacts) string {
	if a != nil && a.Vision != nil && a.Vision.Summary != "" {
		return a.Vision.Summary
	}
	return ""
}

// ExtractProblemSummary extracts the problem summary from phase artifacts.
func ExtractProblemSummary(a *agent.PhaseArtifacts) string {
	if a != nil && a.Problem != nil && a.Problem.Summary != "" {
		return a.Problem.Summary
	}
	return ""
}

// ExtractUsersSummary extracts the users summary from phase artifacts or exploration result.
func ExtractUsersSummary(a *agent.PhaseArtifacts, exploreResult map[string]any) string {
	// Try personas from artifacts first
	if a != nil && a.Users != nil && len(a.Users.Personas) > 0 {
		var names []string
		for _, p := range a.Users.Personas {
			names = append(names, p.Name)
		}
		return strings.Join(names, ", ")
	}
	// Fall back to summary from exploration result
	if users, ok := exploreResult["users"].(map[string]any); ok {
		if summary, ok := users["summary"].(string); ok {
			return summary
		}
	}
	return ""
}

// --- Conversion functions ---

// ToPhaseArtifacts converts agent.PhaseArtifacts to tui.PhaseArtifacts.
func ToPhaseArtifacts(artifacts *agent.PhaseArtifacts) *tui.PhaseArtifacts {
	if artifacts == nil {
		return nil
	}
	return &tui.PhaseArtifacts{
		Vision:  ToVisionArtifact(artifacts.Vision),
		Problem: ToProblemArtifact(artifacts.Problem),
		Users:   ToUsersArtifact(artifacts.Users),
	}
}

// ToVisionArtifact converts agent.VisionArtifact to tui.VisionArtifact.
func ToVisionArtifact(artifact *agent.VisionArtifact) *tui.VisionArtifact {
	if artifact == nil {
		return nil
	}
	return &tui.VisionArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Summary:       artifact.Summary,
		Goals:         append([]string{}, artifact.Goals...),
		NonGoals:      append([]string{}, artifact.NonGoals...),
		Evidence:      ToEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       ToQualityScores(artifact.Quality),
	}
}

// ToProblemArtifact converts agent.ProblemArtifact to tui.ProblemArtifact.
func ToProblemArtifact(artifact *agent.ProblemArtifact) *tui.ProblemArtifact {
	if artifact == nil {
		return nil
	}
	return &tui.ProblemArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Summary:       artifact.Summary,
		PainPoints:    append([]string{}, artifact.PainPoints...),
		Impact:        artifact.Impact,
		Evidence:      ToEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       ToQualityScores(artifact.Quality),
	}
}

// ToUsersArtifact converts agent.UsersArtifact to tui.UsersArtifact.
func ToUsersArtifact(artifact *agent.UsersArtifact) *tui.UsersArtifact {
	if artifact == nil {
		return nil
	}
	return &tui.UsersArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Personas:      ToPersonas(artifact.Personas),
		Evidence:      ToEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       ToQualityScores(artifact.Quality),
	}
}

// ToEvidenceItems converts agent.EvidenceItem slice to tui.EvidenceItem slice.
func ToEvidenceItems(items []agent.EvidenceItem) []tui.EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]tui.EvidenceItem, 0, len(items))
	for _, item := range items {
		out = append(out, tui.EvidenceItem{
			Type:       item.Type,
			Path:       item.Path,
			Quote:      item.Quote,
			Confidence: item.Confidence,
		})
	}
	return out
}

// ToPersonas converts agent.Persona slice to tui.Persona slice.
func ToPersonas(items []agent.Persona) []tui.Persona {
	if len(items) == 0 {
		return nil
	}
	out := make([]tui.Persona, 0, len(items))
	for _, item := range items {
		out = append(out, tui.Persona{
			Name:    item.Name,
			Needs:   append([]string{}, item.Needs...),
			Context: item.Context,
		})
	}
	return out
}

// ToQualityScores converts agent.QualityScores to tui.QualityScores.
func ToQualityScores(scores agent.QualityScores) tui.QualityScores {
	return tui.QualityScores{
		Clarity:      scores.Clarity,
		Completeness: scores.Completeness,
		Grounding:    scores.Grounding,
		Consistency:  scores.Consistency,
	}
}

// --- Scan conversion functions ---

// ScanResultToArtifacts converts a CodebaseScanResultMsg to scan.Artifacts
// for seeding the SprintView's orchestrator.
func ScanResultToArtifacts(r *tui.CodebaseScanResultMsg) *scan.Artifacts {
	if r == nil {
		return nil
	}
	a := &scan.Artifacts{}
	if r.PhaseArtifacts != nil {
		if v := r.PhaseArtifacts.Vision; v != nil {
			a.Vision = &scan.PhaseData{
				Summary:           v.Summary,
				Evidence:          ToScanEvidence(v.Evidence),
				ResolvedQuestions: ToScanResolved(v.ResolvedQuestions),
				Quality:           scan.QualityScores{Clarity: v.Quality.Clarity, Completeness: v.Quality.Completeness, Grounding: v.Quality.Grounding, Consistency: v.Quality.Consistency},
			}
		}
		if p := r.PhaseArtifacts.Problem; p != nil {
			a.Problem = &scan.PhaseData{
				Summary:           p.Summary,
				Evidence:          ToScanEvidence(p.Evidence),
				ResolvedQuestions: ToScanResolved(p.ResolvedQuestions),
				Quality:           scan.QualityScores{Clarity: p.Quality.Clarity, Completeness: p.Quality.Completeness, Grounding: p.Quality.Grounding, Consistency: p.Quality.Consistency},
			}
		}
		if u := r.PhaseArtifacts.Users; u != nil {
			summary := r.Users
			if summary == "" && len(u.Personas) > 0 {
				summary = u.Personas[0].Name
			}
			a.Users = &scan.PhaseData{
				Summary:           summary,
				Evidence:          ToScanEvidence(u.Evidence),
				ResolvedQuestions: ToScanResolved(u.ResolvedQuestions),
				Quality:           scan.QualityScores{Clarity: u.Quality.Clarity, Completeness: u.Quality.Completeness, Grounding: u.Quality.Grounding, Consistency: u.Quality.Consistency},
			}
		}
	}
	// If no phase artifacts, create minimal ones from top-level fields
	if a.Vision == nil && r.Vision != "" {
		a.Vision = &scan.PhaseData{Summary: r.Vision}
	}
	if a.Problem == nil && r.Problem != "" {
		a.Problem = &scan.PhaseData{Summary: r.Problem}
	}
	if a.Users == nil && r.Users != "" {
		a.Users = &scan.PhaseData{Summary: r.Users}
	}
	return a
}

// ToScanEvidence converts tui.EvidenceItem slice to scan.EvidenceItem slice.
func ToScanEvidence(items []tui.EvidenceItem) []scan.EvidenceItem {
	out := make([]scan.EvidenceItem, 0, len(items))
	for _, item := range items {
		out = append(out, scan.EvidenceItem{
			Type:       item.Type,
			FilePath:   item.Path,
			Quote:      item.Quote,
			Confidence: item.Confidence,
		})
	}
	return out
}

// ToScanResolved converts tui.ResolvedQuestion slice to scan.ResolvedQuestion slice.
func ToScanResolved(items []tui.ResolvedQuestion) []scan.ResolvedQuestion {
	out := make([]scan.ResolvedQuestion, 0, len(items))
	for _, item := range items {
		out = append(out, scan.ResolvedQuestion{
			Question: item.Question,
			Answer:   item.Answer,
		})
	}
	return out
}

// --- Text helpers ---

// SafeIndex returns the string at index i, or empty string if out of bounds.
func SafeIndex(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// CreateSpecSummaryFromSprintState extracts display fields from a completed sprint.
// The state pointer is read-only; it was already cloned by Orchestrator.State().
func CreateSpecSummaryFromSprintState(state *arbiter.SprintState) *tui.SpecSummary {
	spec := &tui.SpecSummary{
		ProjectID: state.ID,
	}

	if s, ok := state.Sections[arbiter.PhaseVision]; ok && s.Content != "" {
		spec.Vision = s.Content
		spec.Name = ExtractFirstLine(s.Content)
	}
	if s, ok := state.Sections[arbiter.PhaseProblem]; ok && s.Content != "" {
		spec.Problem = s.Content
	}
	if s, ok := state.Sections[arbiter.PhaseUsers]; ok && s.Content != "" {
		spec.Users = s.Content
	}
	if s, ok := state.Sections[arbiter.PhaseRequirements]; ok && s.Content != "" {
		spec.Requirements = ParseBulletItems(s.Content)
	}
	// Note: Platform/Language not in 8-phase sprint - leave empty

	return spec
}

// ExtractFirstLine returns the first non-empty line of content.
func ExtractFirstLine(content string) string {
	if idx := strings.Index(content, "\n"); idx > 0 {
		return strings.TrimSpace(content[:idx])
	}
	return strings.TrimSpace(content)
}

// ParseBulletItems splits content on newlines and strips bullet prefixes.
func ParseBulletItems(content string) []string {
	lines := strings.Split(content, "\n")
	var items []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Strip common bullet prefixes
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "\u2022 ")
		if line != "" {
			items = append(items, line)
		}
	}
	return items
}

// SummarizeDiff builds a one-line summary of a unified diff.
func SummarizeDiff(diff []string, err error) string {
	if err != nil {
		return "Agent run complete. Diff unavailable."
	}
	if len(diff) == 0 {
		return "Agent run complete. No document changes detected."
	}

	adds := 0
	dels := 0
	for _, line := range diff {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			adds++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			dels++
		}
	}

	return fmt.Sprintf("Agent run complete. +%d -%d lines.", adds, dels)
}

// ToValidationErrors converts agent.ValidationError slice to tui.ValidationError slice.
func ToValidationErrors(errs []agent.ValidationError) []tui.ValidationError {
	if len(errs) == 0 {
		return nil
	}
	out := make([]tui.ValidationError, 0, len(errs))
	for _, err := range errs {
		out = append(out, tui.ValidationError{
			Code:    err.Code,
			Field:   err.Field,
			Message: err.Message,
		})
	}
	return out
}

// Ensure pkgtui is used (referenced by callers for UnifiedDiff).
var _ = pkgtui.UnifiedDiff
