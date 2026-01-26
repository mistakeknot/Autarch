package navigator

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/cuj"
)

func TestIdentifyGaps_NoRecovery(t *testing.T) {
	nav := NewNavigator(nil)

	cujs := []*cuj.CUJ{
		{
			ID:         "CUJ-001",
			Title:      "Critical Flow",
			Priority:   cuj.PriorityHigh,
			EntryPoint: "homepage",
			ExitPoint:  "success",
			Steps:      []cuj.Step{{Action: "click button"}},
			// No ErrorRecovery
		},
	}

	flowMap := &FlowMap{SpecID: "SPEC-001"}
	gaps := nav.identifyGaps(cujs, flowMap)

	foundRecoveryGap := false
	for _, gap := range gaps {
		if gap.Type == GapTypeNoRecovery {
			foundRecoveryGap = true
			break
		}
	}

	if !foundRecoveryGap {
		t.Error("expected gap for missing error recovery on high-priority CUJ")
	}
}

func TestIdentifyGaps_NoSuccessCriteria(t *testing.T) {
	nav := NewNavigator(nil)

	cujs := []*cuj.CUJ{
		{
			ID:         "CUJ-001",
			Title:      "Test Flow",
			Priority:   cuj.PriorityMedium,
			EntryPoint: "homepage",
			ExitPoint:  "success",
			Steps:      []cuj.Step{{Action: "click button"}},
			// No SuccessCriteria
		},
	}

	flowMap := &FlowMap{SpecID: "SPEC-001"}
	gaps := nav.identifyGaps(cujs, flowMap)

	foundGap := false
	for _, gap := range gaps {
		if gap.Type == GapTypeNoSuccessCriteria {
			foundGap = true
			break
		}
	}

	if !foundGap {
		t.Error("expected gap for missing success criteria")
	}
}

func TestIdentifyGaps_MissingEntryExit(t *testing.T) {
	nav := NewNavigator(nil)

	cujs := []*cuj.CUJ{
		{
			ID:       "CUJ-001",
			Title:    "Incomplete Flow",
			Priority: cuj.PriorityMedium,
			// No EntryPoint or ExitPoint
		},
	}

	flowMap := &FlowMap{SpecID: "SPEC-001"}
	gaps := nav.identifyGaps(cujs, flowMap)

	deadEndCount := 0
	for _, gap := range gaps {
		if gap.Type == GapTypeDeadEnd {
			deadEndCount++
		}
	}

	// Should have gaps for missing entry, exit, and steps
	if deadEndCount < 2 {
		t.Errorf("expected at least 2 dead end gaps, got %d", deadEndCount)
	}
}

func TestIdentifyOverlaps_SameEntry(t *testing.T) {
	nav := NewNavigator(nil)

	cujs := []*cuj.CUJ{
		{ID: "CUJ-001", Title: "Flow 1", EntryPoint: "homepage"},
		{ID: "CUJ-002", Title: "Flow 2", EntryPoint: "homepage"},
	}

	overlaps := nav.identifyOverlaps(cujs)

	found := false
	for _, overlap := range overlaps {
		if overlap.OverlapType == "same_entry" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected overlap for same entry point")
	}
}

func TestIdentifyOverlaps_SameExit(t *testing.T) {
	nav := NewNavigator(nil)

	cujs := []*cuj.CUJ{
		{ID: "CUJ-001", Title: "Flow 1", ExitPoint: "dashboard"},
		{ID: "CUJ-002", Title: "Flow 2", ExitPoint: "dashboard"},
	}

	overlaps := nav.identifyOverlaps(cujs)

	found := false
	for _, overlap := range overlaps {
		if overlap.OverlapType == "same_exit" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected overlap for same exit point")
	}
}

func TestIdentifyOverlaps_SharedSteps(t *testing.T) {
	nav := NewNavigator(nil)

	sharedSteps := []cuj.Step{
		{Action: "open menu"},
		{Action: "select option"},
		{Action: "confirm choice"},
	}

	cujs := []*cuj.CUJ{
		{ID: "CUJ-001", Title: "Flow 1", Steps: sharedSteps},
		{ID: "CUJ-002", Title: "Flow 2", Steps: sharedSteps},
	}

	overlaps := nav.identifyOverlaps(cujs)

	found := false
	for _, overlap := range overlaps {
		if overlap.OverlapType == "same_steps" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected overlap for shared steps")
	}
}

func TestValidateCompleteness(t *testing.T) {
	nav := NewNavigator(nil)

	// Flow map with critical gap
	flowMap := &FlowMap{
		SpecID: "SPEC-001",
		Paths: []FlowPath{
			{CUJID: "CUJ-001", Title: "Flow 1"},
		},
		Gaps: []FlowGap{
			{Type: GapTypeNoRecovery, Severity: SeverityCritical, Description: "Critical issue"},
		},
	}

	result := nav.ValidateCompleteness(flowMap)

	if result.Complete {
		t.Error("expected incomplete when critical gaps exist")
	}
	if len(result.CriticalIssues) == 0 {
		t.Error("expected critical issues to be listed")
	}
	if result.TotalPaths != 1 {
		t.Errorf("TotalPaths = %d, want 1", result.TotalPaths)
	}
}

func TestFormatFlowMap(t *testing.T) {
	flowMap := &FlowMap{
		SpecID: "SPEC-001",
		EntryPoints: []EntryPoint{
			{Name: "homepage", Personas: []string{"new user"}},
		},
		ExitPoints: []ExitPoint{
			{Name: "dashboard"},
		},
		Paths: []FlowPath{
			{
				CUJID:    "CUJ-001",
				Title:    "Onboarding",
				Persona:  "new user",
				Entry:    "homepage",
				Exit:     "dashboard",
				Priority: "high",
				Steps:    []string{"sign up", "verify email", "complete profile"},
			},
		},
		Gaps: []FlowGap{
			{Type: GapTypeNoRecovery, Severity: SeverityHigh, Description: "Missing recovery", Suggestion: "Add recovery"},
		},
	}

	result := FormatFlowMap(flowMap)

	if !strings.Contains(result, "User Flow Map") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "homepage") {
		t.Error("should contain entry point")
	}
	if !strings.Contains(result, "Onboarding") {
		t.Error("should contain path title")
	}
	if !strings.Contains(result, "dashboard") {
		t.Error("should contain exit point")
	}
	if !strings.Contains(result, "Gaps Identified") {
		t.Error("should contain gaps section")
	}
}

func TestCountStepOverlap(t *testing.T) {
	steps1 := []cuj.Step{
		{Action: "open menu"},
		{Action: "select option"},
		{Action: "confirm"},
	}
	steps2 := []cuj.Step{
		{Action: "open menu"},
		{Action: "select option"},
		{Action: "cancel"},
	}

	overlap := countStepOverlap(steps1, steps2)

	if overlap != 2 {
		t.Errorf("countStepOverlap = %d, want 2", overlap)
	}
}

func TestCollectPersonas(t *testing.T) {
	cujs := []*cuj.CUJ{
		{Persona: "new user"},
		{Persona: "admin"},
		{Persona: "new user"}, // Duplicate
		{Persona: ""},         // Empty
	}

	personas := collectPersonas(cujs)

	if len(personas) != 2 {
		t.Errorf("expected 2 unique personas, got %d", len(personas))
	}
}
