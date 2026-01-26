// Package navigator provides user flow mapping and analysis for CUJs.
// The "navigator" subagent maps user journeys through the spec, identifies
// gaps in journey coverage, and detects overlapping or conflicting paths.
package navigator

import (
	"context"
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/cuj"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// FlowMap represents the complete map of user flows for a spec
type FlowMap struct {
	SpecID      string       `json:"spec_id"`
	EntryPoints []EntryPoint `json:"entry_points"`
	ExitPoints  []ExitPoint  `json:"exit_points"`
	Paths       []FlowPath   `json:"paths"`
	Gaps        []FlowGap    `json:"gaps"`
	Overlaps    []FlowOverlap `json:"overlaps"`
}

// EntryPoint represents where users can enter the system
type EntryPoint struct {
	Name     string   `json:"name"`
	CUJIDs   []string `json:"cuj_ids"`   // CUJs that start here
	Personas []string `json:"personas"`  // User types that enter here
}

// ExitPoint represents where users successfully complete journeys
type ExitPoint struct {
	Name   string   `json:"name"`
	CUJIDs []string `json:"cuj_ids"` // CUJs that end here
}

// FlowPath represents a path through the system
type FlowPath struct {
	CUJID       string   `json:"cuj_id"`
	Title       string   `json:"title"`
	Persona     string   `json:"persona"`
	Entry       string   `json:"entry"`
	Exit        string   `json:"exit"`
	Steps       []string `json:"steps"`
	Priority    string   `json:"priority"`
	HasRecovery bool     `json:"has_recovery"`
}

// FlowGap represents a gap in journey coverage
type FlowGap struct {
	Type        GapType  `json:"type"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	AffectedCUJs []string `json:"affected_cujs,omitempty"`
	Suggestion  string   `json:"suggestion"`
}

// GapType categorizes the type of gap
type GapType string

const (
	GapTypeNoRecovery     GapType = "no_recovery"      // No error recovery defined
	GapTypeDeadEnd        GapType = "dead_end"         // Path leads nowhere
	GapTypeNoEntry        GapType = "no_entry"         // Exit exists but no path to it
	GapTypeMissingPersona GapType = "missing_persona"  // Persona has no journeys
	GapTypeNoSuccessCriteria GapType = "no_success_criteria" // CUJ lacks success criteria
)

// Severity indicates the importance of addressing a gap
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// FlowOverlap represents overlapping or potentially conflicting journeys
type FlowOverlap struct {
	CUJ1ID      string `json:"cuj1_id"`
	CUJ2ID      string `json:"cuj2_id"`
	CUJ1Title   string `json:"cuj1_title"`
	CUJ2Title   string `json:"cuj2_title"`
	OverlapType string `json:"overlap_type"` // same_entry, same_exit, same_steps
	Description string `json:"description"`
}

// Navigator analyzes CUJs and maps user flows
type Navigator struct {
	client *intermute.Client
}

// NewNavigator creates a new flow navigator
func NewNavigator(client *intermute.Client) *Navigator {
	return &Navigator{client: client}
}

// MapFlows analyzes all CUJs for a spec and creates a flow map
func (n *Navigator) MapFlows(ctx context.Context, specID string) (*FlowMap, error) {
	// Get all CUJs for the spec
	cujs, err := n.getCUJs(ctx, specID)
	if err != nil {
		return nil, fmt.Errorf("failed to get CUJs: %w", err)
	}

	flowMap := &FlowMap{
		SpecID: specID,
	}

	// Build entry points
	entryMap := make(map[string]*EntryPoint)
	exitMap := make(map[string]*ExitPoint)

	for _, c := range cujs {
		// Track entry points
		entry := normalizePoint(c.EntryPoint)
		if entry != "" {
			if ep, ok := entryMap[entry]; ok {
				ep.CUJIDs = append(ep.CUJIDs, c.ID)
				if c.Persona != "" && !contains(ep.Personas, c.Persona) {
					ep.Personas = append(ep.Personas, c.Persona)
				}
			} else {
				entryMap[entry] = &EntryPoint{
					Name:     entry,
					CUJIDs:   []string{c.ID},
					Personas: []string{c.Persona},
				}
			}
		}

		// Track exit points
		exit := normalizePoint(c.ExitPoint)
		if exit != "" {
			if xp, ok := exitMap[exit]; ok {
				xp.CUJIDs = append(xp.CUJIDs, c.ID)
			} else {
				exitMap[exit] = &ExitPoint{
					Name:   exit,
					CUJIDs: []string{c.ID},
				}
			}
		}

		// Build flow path
		path := FlowPath{
			CUJID:       c.ID,
			Title:       c.Title,
			Persona:     c.Persona,
			Entry:       entry,
			Exit:        exit,
			Steps:       extractStepActions(c.Steps),
			Priority:    string(c.Priority),
			HasRecovery: len(c.ErrorRecovery) > 0,
		}
		flowMap.Paths = append(flowMap.Paths, path)
	}

	// Convert maps to slices
	for _, ep := range entryMap {
		flowMap.EntryPoints = append(flowMap.EntryPoints, *ep)
	}
	for _, xp := range exitMap {
		flowMap.ExitPoints = append(flowMap.ExitPoints, *xp)
	}

	// Identify gaps
	flowMap.Gaps = n.identifyGaps(cujs, flowMap)

	// Identify overlaps
	flowMap.Overlaps = n.identifyOverlaps(cujs)

	return flowMap, nil
}

// ValidateCompleteness checks if CUJ coverage is sufficient
func (n *Navigator) ValidateCompleteness(flowMap *FlowMap) ValidationResult {
	result := ValidationResult{
		Complete: true,
	}

	// Check for critical gaps
	for _, gap := range flowMap.Gaps {
		if gap.Severity == SeverityCritical {
			result.Complete = false
			result.CriticalIssues = append(result.CriticalIssues, gap.Description)
		} else if gap.Severity == SeverityHigh {
			result.HighPriorityIssues = append(result.HighPriorityIssues, gap.Description)
		}
	}

	// Check for conflicting overlaps
	for _, overlap := range flowMap.Overlaps {
		if overlap.OverlapType == "conflicting" {
			result.Complete = false
			result.CriticalIssues = append(result.CriticalIssues, overlap.Description)
		}
	}

	// Count coverage
	result.TotalPaths = len(flowMap.Paths)
	result.EntryPointCount = len(flowMap.EntryPoints)
	result.ExitPointCount = len(flowMap.ExitPoints)
	result.GapCount = len(flowMap.Gaps)

	return result
}

// ValidationResult contains the result of flow validation
type ValidationResult struct {
	Complete           bool     `json:"complete"`
	CriticalIssues     []string `json:"critical_issues,omitempty"`
	HighPriorityIssues []string `json:"high_priority_issues,omitempty"`
	TotalPaths         int      `json:"total_paths"`
	EntryPointCount    int      `json:"entry_point_count"`
	ExitPointCount     int      `json:"exit_point_count"`
	GapCount           int      `json:"gap_count"`
}

// getCUJs retrieves CUJs from Intermute or returns empty slice if no client
func (n *Navigator) getCUJs(ctx context.Context, specID string) ([]*cuj.CUJ, error) {
	if n.client == nil {
		return nil, nil
	}

	cujSvc := cuj.NewService(n.client)
	return cujSvc.List(ctx, specID)
}

// identifyGaps finds gaps in CUJ coverage
func (n *Navigator) identifyGaps(cujs []*cuj.CUJ, flowMap *FlowMap) []FlowGap {
	var gaps []FlowGap

	for _, c := range cujs {
		// Check for missing error recovery on high-priority CUJs
		if c.Priority == cuj.PriorityHigh && len(c.ErrorRecovery) == 0 {
			gaps = append(gaps, FlowGap{
				Type:        GapTypeNoRecovery,
				Description: fmt.Sprintf("High-priority CUJ %q has no error recovery defined", c.Title),
				Severity:    SeverityHigh,
				AffectedCUJs: []string{c.ID},
				Suggestion:  "Define error recovery paths for critical user journeys",
			})
		}

		// Check for missing success criteria
		if len(c.SuccessCriteria) == 0 {
			gaps = append(gaps, FlowGap{
				Type:        GapTypeNoSuccessCriteria,
				Description: fmt.Sprintf("CUJ %q has no success criteria", c.Title),
				Severity:    SeverityMedium,
				AffectedCUJs: []string{c.ID},
				Suggestion:  "Define clear success criteria for journey completion",
			})
		}

		// Check for missing entry point
		if strings.TrimSpace(c.EntryPoint) == "" {
			gaps = append(gaps, FlowGap{
				Type:        GapTypeDeadEnd,
				Description: fmt.Sprintf("CUJ %q has no entry point defined", c.Title),
				Severity:    SeverityMedium,
				AffectedCUJs: []string{c.ID},
				Suggestion:  "Specify where users begin this journey",
			})
		}

		// Check for missing exit point
		if strings.TrimSpace(c.ExitPoint) == "" {
			gaps = append(gaps, FlowGap{
				Type:        GapTypeDeadEnd,
				Description: fmt.Sprintf("CUJ %q has no exit point (success state) defined", c.Title),
				Severity:    SeverityMedium,
				AffectedCUJs: []string{c.ID},
				Suggestion:  "Specify the success state that ends this journey",
			})
		}

		// Check for empty steps
		if len(c.Steps) == 0 {
			gaps = append(gaps, FlowGap{
				Type:        GapTypeDeadEnd,
				Description: fmt.Sprintf("CUJ %q has no steps defined", c.Title),
				Severity:    SeverityHigh,
				AffectedCUJs: []string{c.ID},
				Suggestion:  "Define the steps users take in this journey",
			})
		}
	}

	// Check for personas without journeys
	personas := collectPersonas(cujs)
	if len(personas) == 0 && len(cujs) > 0 {
		gaps = append(gaps, FlowGap{
			Type:        GapTypeMissingPersona,
			Description: "No personas defined for any CUJ",
			Severity:    SeverityMedium,
			Suggestion:  "Define target personas for each journey",
		})
	}

	return gaps
}

// identifyOverlaps finds overlapping CUJs
func (n *Navigator) identifyOverlaps(cujs []*cuj.CUJ) []FlowOverlap {
	var overlaps []FlowOverlap

	for i := 0; i < len(cujs); i++ {
		for j := i + 1; j < len(cujs); j++ {
			c1, c2 := cujs[i], cujs[j]

			// Same entry point
			if c1.EntryPoint != "" && c1.EntryPoint == c2.EntryPoint {
				overlaps = append(overlaps, FlowOverlap{
					CUJ1ID:      c1.ID,
					CUJ2ID:      c2.ID,
					CUJ1Title:   c1.Title,
					CUJ2Title:   c2.Title,
					OverlapType: "same_entry",
					Description: fmt.Sprintf("CUJs %q and %q share entry point: %s", c1.Title, c2.Title, c1.EntryPoint),
				})
			}

			// Same exit point
			if c1.ExitPoint != "" && c1.ExitPoint == c2.ExitPoint {
				overlaps = append(overlaps, FlowOverlap{
					CUJ1ID:      c1.ID,
					CUJ2ID:      c2.ID,
					CUJ1Title:   c1.Title,
					CUJ2Title:   c2.Title,
					OverlapType: "same_exit",
					Description: fmt.Sprintf("CUJs %q and %q share exit point: %s", c1.Title, c2.Title, c1.ExitPoint),
				})
			}

			// Overlapping steps (significant overlap)
			stepOverlap := countStepOverlap(c1.Steps, c2.Steps)
			if stepOverlap >= 3 {
				overlaps = append(overlaps, FlowOverlap{
					CUJ1ID:      c1.ID,
					CUJ2ID:      c2.ID,
					CUJ1Title:   c1.Title,
					CUJ2Title:   c2.Title,
					OverlapType: "same_steps",
					Description: fmt.Sprintf("CUJs %q and %q share %d steps - consider consolidating", c1.Title, c2.Title, stepOverlap),
				})
			}
		}
	}

	return overlaps
}

// FormatFlowMap formats the flow map as markdown for display
func FormatFlowMap(fm *FlowMap) string {
	var sb strings.Builder

	sb.WriteString("# User Flow Map\n\n")

	// Entry points
	sb.WriteString("## Entry Points\n")
	if len(fm.EntryPoints) == 0 {
		sb.WriteString("_No entry points defined_\n")
	} else {
		for _, ep := range fm.EntryPoints {
			sb.WriteString(fmt.Sprintf("- **%s**", ep.Name))
			if len(ep.Personas) > 0 {
				sb.WriteString(fmt.Sprintf(" (personas: %s)", strings.Join(ep.Personas, ", ")))
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")

	// Paths
	sb.WriteString("## Journey Paths\n")
	for _, path := range fm.Paths {
		sb.WriteString(fmt.Sprintf("### %s\n", path.Title))
		sb.WriteString(fmt.Sprintf("- **Priority:** %s\n", path.Priority))
		if path.Persona != "" {
			sb.WriteString(fmt.Sprintf("- **Persona:** %s\n", path.Persona))
		}
		sb.WriteString(fmt.Sprintf("- **Entry:** %s → **Exit:** %s\n", path.Entry, path.Exit))
		if !path.HasRecovery {
			sb.WriteString("- ⚠️ No error recovery defined\n")
		}
		if len(path.Steps) > 0 {
			sb.WriteString("- **Steps:** ")
			sb.WriteString(strings.Join(path.Steps, " → "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Exit points
	sb.WriteString("## Exit Points (Success States)\n")
	if len(fm.ExitPoints) == 0 {
		sb.WriteString("_No exit points defined_\n")
	} else {
		for _, xp := range fm.ExitPoints {
			sb.WriteString(fmt.Sprintf("- **%s**\n", xp.Name))
		}
	}
	sb.WriteString("\n")

	// Gaps
	if len(fm.Gaps) > 0 {
		sb.WriteString("## Gaps Identified\n")
		for _, gap := range fm.Gaps {
			icon := "ℹ️"
			switch gap.Severity {
			case SeverityCritical:
				icon = "🚨"
			case SeverityHigh:
				icon = "⚠️"
			case SeverityMedium:
				icon = "📋"
			}
			sb.WriteString(fmt.Sprintf("%s **%s** (%s): %s\n", icon, gap.Type, gap.Severity, gap.Description))
			sb.WriteString(fmt.Sprintf("   _Suggestion: %s_\n", gap.Suggestion))
		}
		sb.WriteString("\n")
	}

	// Overlaps
	if len(fm.Overlaps) > 0 {
		sb.WriteString("## Overlapping Journeys\n")
		for _, overlap := range fm.Overlaps {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", overlap.OverlapType, overlap.Description))
		}
	}

	return sb.String()
}

// --- Helper functions ---

func normalizePoint(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func extractStepActions(steps []cuj.Step) []string {
	actions := make([]string, len(steps))
	for i, s := range steps {
		actions[i] = s.Action
	}
	return actions
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func collectPersonas(cujs []*cuj.CUJ) []string {
	seen := make(map[string]bool)
	var personas []string
	for _, c := range cujs {
		if c.Persona != "" && !seen[c.Persona] {
			seen[c.Persona] = true
			personas = append(personas, c.Persona)
		}
	}
	return personas
}

func countStepOverlap(steps1, steps2 []cuj.Step) int {
	count := 0
	for _, s1 := range steps1 {
		for _, s2 := range steps2 {
			if normalizePoint(s1.Action) == normalizePoint(s2.Action) {
				count++
				break
			}
		}
	}
	return count
}
