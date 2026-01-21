package suggestions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"gopkg.in/yaml.v3"
)

type Suggestion struct {
	Summary              string
	Requirements         []string
	CriticalUserJourneys []specs.CriticalUserJourney
	MarketResearch       []specs.MarketResearchItem
	CompetitiveLandscape []specs.CompetitiveLandscapeItem
}

func LoadLatest(dir, id string) (Suggestion, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Suggestion{}, "", err
	}
	var latestPath string
	var latestName string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, id+"-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		if name > latestName {
			latestName = name
			latestPath = filepath.Join(dir, name)
		}
	}
	if latestPath == "" {
		return Suggestion{}, "", fmt.Errorf("no suggestions for %s", id)
	}
	raw, err := os.ReadFile(latestPath)
	if err != nil {
		return Suggestion{}, "", err
	}
	return parseSuggestion(raw), latestPath, nil
}

func Create(dir, id string, now time.Time) (string, error) {
	name := fmt.Sprintf("%s-%s.md", id, now.UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	body := fmt.Sprintf(`# Suggestions for %s

## Summary
- status: pending
- suggestion: ""

## Requirements
- status: pending
- suggestion:
  - "REQ-001: Add requirement"

## Critical User Journeys
- status: pending
- suggestion:
  - id: "CUJ-001"
    title: "Primary Journey"
    priority: "high"
    steps:
      - "Step"
    success_criteria:
      - "Outcome"
    linked_requirements:
      - "REQ-001"

## Market Research
- status: pending
- suggestion:
  - id: "MR-001"
    claim: "Market claim"
    evidence_refs:
      - path: ".praude/research/%s-YYYYMMDD-HHMMSS.md"
        anchor: "section-1"
        note: "Source quote"
    confidence: "medium"
    date: "2026-01-15"

## Competitive Landscape
- status: pending
- suggestion:
  - id: "COMP-001"
    name: "Competitor"
    positioning: "Positioning"
    strengths:
      - "Strength"
    weaknesses:
      - "Weakness"
    risk: "Medium"
    evidence_refs:
      - path: ".praude/research/%s-YYYYMMDD-HHMMSS.md"
        anchor: "section-2"
        note: "Source quote"
`, id, id, id)
	return path, os.WriteFile(path, []byte(body), 0o644)
}

func Apply(path string, suggestion Suggestion) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc specs.Spec
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if suggestion.Summary != "" {
		doc.Summary = suggestion.Summary
	}
	if len(suggestion.Requirements) > 0 {
		doc.Requirements = suggestion.Requirements
	}
	if len(suggestion.CriticalUserJourneys) > 0 {
		doc.CriticalUserJourneys = suggestion.CriticalUserJourneys
	}
	if len(suggestion.MarketResearch) > 0 {
		doc.MarketResearch = suggestion.MarketResearch
	}
	if len(suggestion.CompetitiveLandscape) > 0 {
		doc.CompetitiveLandscape = suggestion.CompetitiveLandscape
	}
	updated, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func ParseReady(raw []byte) Suggestion {
	return parseSuggestionWithFilter(raw, func(_ string, status string) bool {
		return strings.EqualFold(status, "ready")
	})
}

func IsEmpty(s Suggestion) bool {
	return s.Summary == "" &&
		len(s.Requirements) == 0 &&
		len(s.CriticalUserJourneys) == 0 &&
		len(s.MarketResearch) == 0 &&
		len(s.CompetitiveLandscape) == 0
}

func parseSuggestion(raw []byte) Suggestion {
	return parseSuggestionWithFilter(raw, func(_ string, _ string) bool {
		return true
	})
}

func parseSuggestionWithFilter(raw []byte, include func(section, status string) bool) Suggestion {
	lines := strings.Split(string(raw), "\n")
	out := Suggestion{}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		section := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		status, block := extractSection(lines, i+1)
		if !include(section, status) {
			continue
		}
		switch section {
		case "Summary":
			out.Summary = parseSummaryBlock(block)
		case "Requirements":
			out.Requirements = parseStringListBlock(block)
		case "Critical User Journeys":
			out.CriticalUserJourneys = parseCUJBlock(block)
		case "Market Research":
			out.MarketResearch = parseMarketBlock(block)
		case "Competitive Landscape":
			out.CompetitiveLandscape = parseCompetitiveBlock(block)
		}
	}
	return out
}

func firstQuoted(lines []string) string {
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- suggestion:") && strings.Contains(trim, "\"") {
			return betweenQuotes(trim)
		}
		if strings.HasPrefix(trim, "- suggestion:") {
			continue
		}
		if strings.HasPrefix(trim, "- ") && strings.Contains(trim, "\"") {
			return betweenQuotes(trim)
		}
	}
	return ""
}

func betweenQuotes(line string) string {
	first := strings.Index(line, "\"")
	last := strings.LastIndex(line, "\"")
	if first >= 0 && last > first {
		return line[first+1 : last]
	}
	return ""
}

func extractSuggestionBlock(lines []string, start int) []string {
	_, block := extractSection(lines, start)
	return block
}

func extractSection(lines []string, start int) (string, []string) {
	status := ""
	for i := start; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "## ") {
			return status, nil
		}
		if strings.HasPrefix(trim, "- status:") {
			status = strings.TrimSpace(strings.TrimPrefix(trim, "- status:"))
			status = strings.Trim(status, "\"")
			continue
		}
		if strings.HasPrefix(trim, "- suggestion:") {
			block := []string{lines[i]}
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if strings.HasPrefix(next, "## ") {
					break
				}
				block = append(block, lines[j])
			}
			return status, block
		}
	}
	return status, nil
}

func parseSummaryBlock(block []string) string {
	if len(block) == 0 {
		return ""
	}
	if inline := betweenQuotes(block[0]); inline != "" {
		return inline
	}
	return firstQuoted(block[1:])
}

func parseStringListBlock(block []string) []string {
	if len(block) == 0 {
		return nil
	}
	if inline := betweenQuotes(block[0]); inline != "" {
		return []string{inline}
	}
	var wrapper struct {
		Items []string `yaml:"items"`
	}
	if !parseBlock(block[1:], &wrapper) {
		return nil
	}
	return wrapper.Items
}

func parseCUJBlock(block []string) []specs.CriticalUserJourney {
	var wrapper struct {
		Items []specs.CriticalUserJourney `yaml:"items"`
	}
	if !parseBlock(block[1:], &wrapper) {
		return nil
	}
	return wrapper.Items
}

func parseMarketBlock(block []string) []specs.MarketResearchItem {
	var wrapper struct {
		Items []specs.MarketResearchItem `yaml:"items"`
	}
	if !parseBlock(block[1:], &wrapper) {
		return nil
	}
	return wrapper.Items
}

func parseCompetitiveBlock(block []string) []specs.CompetitiveLandscapeItem {
	var wrapper struct {
		Items []specs.CompetitiveLandscapeItem `yaml:"items"`
	}
	if !parseBlock(block[1:], &wrapper) {
		return nil
	}
	return wrapper.Items
}

func parseBlock(lines []string, out interface{}) bool {
	if len(lines) == 0 {
		return false
	}
	var b strings.Builder
	b.WriteString("items:\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := yaml.Unmarshal([]byte(b.String()), out); err != nil {
		return false
	}
	return true
}
