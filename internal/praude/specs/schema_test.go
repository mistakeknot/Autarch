package specs

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSpecSchemaIncludesEvidenceSections(t *testing.T) {
	raw := []byte(`id: "PRD-001"
title: "Example"
summary: "Summary"
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "high"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "req-1"
market_research:
  - id: "MR-001"
    claim: "Market is growing"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source quote"
    confidence: "medium"
    date: "2026-01-15"
competitive_landscape:
  - id: "COMP-001"
    name: "Competitor"
    positioning: "Low cost"
    strengths:
      - "Speed"
    weaknesses:
      - "Reliability"
    risk: "Medium"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-2"
        note: "Source quote"
`)
	var doc Spec
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.CriticalUserJourneys) != 1 {
		t.Fatalf("expected cujs parsed")
	}
	if len(doc.MarketResearch) != 1 {
		t.Fatalf("expected market research parsed")
	}
	if len(doc.CompetitiveLandscape) != 1 {
		t.Fatalf("expected competitive landscape parsed")
	}
	if doc.MarketResearch[0].EvidenceRefs[0].Path == "" {
		t.Fatalf("expected evidence ref path")
	}
}
