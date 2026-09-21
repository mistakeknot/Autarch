package proposal

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/pollard/config"
)

// AgendaSelector applies selected agendas to the Pollard configuration.
type AgendaSelector struct {
	projectPath string
}

// NewAgendaSelector creates a selector for the given project.
func NewAgendaSelector(projectPath string) *AgendaSelector {
	return &AgendaSelector{
		projectPath: projectPath,
	}
}

// ApplySelectedAgendas updates config with queries from selected agendas.
func (s *AgendaSelector) ApplySelectedAgendas(selectedIDs []string, proposals *ProposalResult) error {
	cfg, err := config.Load(s.projectPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Build a map of selected agendas
	selectedMap := make(map[string]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	// Process each selected agenda
	for _, agenda := range proposals.Agendas {
		if !selectedMap[agenda.ID] {
			continue
		}

		// Add agenda questions to suggested hunters
		for _, hunterName := range agenda.SuggestedHunters {
			if hunterCfg, ok := cfg.Hunters[hunterName]; ok {
				// Add questions that aren't already present
				hunterCfg.Queries = mergeQueries(hunterCfg.Queries, agenda.Questions)
				// Ensure hunter is enabled
				hunterCfg.Enabled = true
				cfg.Hunters[hunterName] = hunterCfg
			} else {
				// Create new hunter config if it doesn't exist
				cfg.Hunters[hunterName] = config.HunterConfig{
					Enabled:    true,
					Queries:    agenda.Questions,
					MaxResults: 50,
					Interval:   "6h",
				}
			}
		}
	}

	// Save updated config
	if err := cfg.Save(s.projectPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// mergeQueries adds new queries while avoiding duplicates.
func mergeQueries(existing, new []string) []string {
	seen := make(map[string]bool)
	for _, q := range existing {
		seen[normalizeQuery(q)] = true
	}

	result := existing
	for _, q := range new {
		normalized := normalizeQuery(q)
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, q)
		}
	}

	return result
}

// normalizeQuery normalizes a query for comparison.
func normalizeQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}

// SelectionResult summarizes what was applied.
type SelectionResult struct {
	SelectedAgendas []string       // IDs of selected agendas
	UpdatedHunters  map[string]int // hunter name -> number of new queries added
	NewHunters      []string       // hunters that were created
	TotalQueries    int            // total new queries added
}

// ApplySelectedAgendasWithResult applies agendas and returns detailed result.
func (s *AgendaSelector) ApplySelectedAgendasWithResult(selectedIDs []string, proposals *ProposalResult) (*SelectionResult, error) {
	cfg, err := config.Load(s.projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	result := &SelectionResult{
		SelectedAgendas: selectedIDs,
		UpdatedHunters:  make(map[string]int),
	}

	selectedMap := make(map[string]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	for _, agenda := range proposals.Agendas {
		if !selectedMap[agenda.ID] {
			continue
		}

		for _, hunterName := range agenda.SuggestedHunters {
			hunterCfg, exists := cfg.Hunters[hunterName]
			if !exists {
				hunterCfg = config.HunterConfig{
					Enabled:    true,
					MaxResults: 50,
					Interval:   "6h",
				}
				result.NewHunters = append(result.NewHunters, hunterName)
			}

			originalCount := len(hunterCfg.Queries)
			hunterCfg.Queries = mergeQueries(hunterCfg.Queries, agenda.Questions)
			hunterCfg.Enabled = true

			newCount := len(hunterCfg.Queries) - originalCount
			if newCount > 0 {
				result.UpdatedHunters[hunterName] += newCount
				result.TotalQueries += newCount
			}

			cfg.Hunters[hunterName] = hunterCfg
		}
	}

	if err := cfg.Save(s.projectPath); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	return result, nil
}

// GetAgenda finds an agenda by ID.
func GetAgenda(agendas []ResearchAgenda, id string) *ResearchAgenda {
	for i := range agendas {
		if agendas[i].ID == id {
			return &agendas[i]
		}
	}
	return nil
}

// FormatAgendaSummary creates a human-readable summary of an agenda.
func FormatAgendaSummary(a *ResearchAgenda) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**%s** (%s priority, %s scope)\n", a.Title, a.Priority, a.EstimatedScope))
	sb.WriteString(fmt.Sprintf("  %s\n", a.Description))

	if len(a.Questions) > 0 {
		sb.WriteString("  Questions:\n")
		for _, q := range a.Questions {
			sb.WriteString(fmt.Sprintf("    - %s\n", q))
		}
	}

	if len(a.SuggestedHunters) > 0 {
		sb.WriteString(fmt.Sprintf("  Hunters: %s\n", strings.Join(a.SuggestedHunters, ", ")))
	}

	return sb.String()
}

// FormatSelectionResult creates a human-readable summary of the selection.
func FormatSelectionResult(r *SelectionResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Applied %d agenda(s)\n", len(r.SelectedAgendas)))
	sb.WriteString(fmt.Sprintf("Added %d new queries\n", r.TotalQueries))

	if len(r.NewHunters) > 0 {
		sb.WriteString(fmt.Sprintf("Created hunters: %s\n", strings.Join(r.NewHunters, ", ")))
	}

	if len(r.UpdatedHunters) > 0 {
		sb.WriteString("Updated hunters:\n")
		for hunter, count := range r.UpdatedHunters {
			sb.WriteString(fmt.Sprintf("  %s: +%d queries\n", hunter, count))
		}
	}

	return sb.String()
}
