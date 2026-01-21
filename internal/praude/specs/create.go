package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func CreateTemplate(dir string, now time.Time) (string, error) {
	id, err := NextID(dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".yaml")
	doc := fmt.Sprintf(`id: "%s"
title: "Example PRD Title"
created_at: "%s"
strategic_context:
  cuj_id: "CUJ-1"
  cuj_name: "Example Journey"
  feature_id: "example-feature"
  mvp_included: true
user_story:
  text: "As a user, I want X so that Y."
  hash: "pending"
summary: |
  One paragraph describing what to build and why.
requirements:
  - "REQ-001: Requirement one"
acceptance_criteria:
  - id: "ac-1"
    description: "Acceptance criterion one"
files_to_modify:
  - action: "create"
    path: "path/to/file"
    description: "Why this file"
critical_user_journeys:
  - id: "CUJ-001"
    title: "Primary Journey"
    priority: "high"
    steps:
      - "Step one"
    success_criteria:
      - "Success outcome"
    linked_requirements:
      - "REQ-001"
  - id: "CUJ-002"
    title: "Maintenance"
    priority: "low"
    steps:
      - "Routine upkeep"
    success_criteria:
      - "System remains stable"
    linked_requirements:
      - "REQ-001"
market_research:
  - id: "MR-001"
    claim: "Market is growing"
    evidence_refs:
      - path: ".praude/research/PRD-001-YYYYMMDD-HHMMSS.md"
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
      - path: ".praude/research/PRD-001-YYYYMMDD-HHMMSS.md"
        anchor: "section-2"
        note: "Source quote"
research:
  - ".praude/research/PRD-001-YYYYMMDD-HHMMSS.md"
complexity: "medium"
estimated_minutes: 25
priority: 1
`, id, now.UTC().Format(time.RFC3339))
	return path, os.WriteFile(path, []byte(doc), 0o644)
}
