package exploration

import (
	"github.com/mistakeknot/autarch/internal/autarch/agent"
)

// MergeIntoArtifacts takes exploration results and merges them into existing PhaseArtifacts.
// If artifacts is nil, creates new PhaseArtifacts.
// Exploration evidence is appended to existing evidence with type "exploration".
func MergeIntoArtifacts(result map[string]any, artifacts *agent.PhaseArtifacts) *agent.PhaseArtifacts {
	if artifacts == nil {
		artifacts = &agent.PhaseArtifacts{}
	}

	// Extract and merge Vision
	if vision, ok := result["vision"].(map[string]any); ok {
		if artifacts.Vision == nil {
			artifacts.Vision = &agent.VisionArtifact{}
		}
		mergeVision(vision, artifacts.Vision)
	}

	// Extract and merge Problem
	if problem, ok := result["problem"].(map[string]any); ok {
		if artifacts.Problem == nil {
			artifacts.Problem = &agent.ProblemArtifact{}
		}
		mergeProblem(problem, artifacts.Problem)
	}

	// Extract and merge Users
	if users, ok := result["users"].(map[string]any); ok {
		if artifacts.Users == nil {
			artifacts.Users = &agent.UsersArtifact{}
		}
		mergeUsers(users, artifacts.Users)
	}

	return artifacts
}

func mergeVision(src map[string]any, dst *agent.VisionArtifact) {
	// Set summary if not already set
	if dst.Summary == "" {
		if summary, ok := src["summary"].(string); ok {
			dst.Summary = summary
		}
	}

	// Extract evidence from quotes
	evidence := extractEvidence(src)
	dst.Evidence = append(dst.Evidence, evidence...)
}

func mergeProblem(src map[string]any, dst *agent.ProblemArtifact) {
	if dst.Summary == "" {
		if summary, ok := src["summary"].(string); ok {
			dst.Summary = summary
		}
	}

	evidence := extractEvidence(src)
	dst.Evidence = append(dst.Evidence, evidence...)
}

func mergeUsers(src map[string]any, dst *agent.UsersArtifact) {
	evidence := extractEvidence(src)
	dst.Evidence = append(dst.Evidence, evidence...)

	// Extract personas if available
	if personas, ok := src["personas"].([]any); ok {
		for _, p := range personas {
			if pm, ok := p.(map[string]any); ok {
				persona := agent.Persona{}
				if role, ok := pm["role"].(string); ok {
					persona.Name = role
				}
				if workflow, ok := pm["workflow"].(string); ok {
					persona.Context = workflow
				}
				if persona.Name != "" {
					dst.Personas = append(dst.Personas, persona)
				}
			}
		}
	}
}

// extractEvidence pulls evidence items from exploration output.
// Handles both "quotes" array and "evidence" array formats.
func extractEvidence(src map[string]any) []agent.EvidenceItem {
	var items []agent.EvidenceItem

	// Try "quotes" format (array of {text, source/file} objects)
	if quotes, ok := src["quotes"].([]any); ok {
		for _, q := range quotes {
			if qm, ok := q.(map[string]any); ok {
				item := agent.EvidenceItem{
					Type:       "exploration",
					Confidence: 0.9, // High confidence for verbatim quotes
				}
				if text, ok := qm["text"].(string); ok {
					item.Quote = text
				}
				if source, ok := qm["source"].(string); ok {
					item.Path = source
				} else if file, ok := qm["file"].(string); ok {
					item.Path = file
				}
				if item.Quote != "" {
					items = append(items, item)
				}
			}
		}
	}

	// Try "evidence" format (array of strings or objects)
	if evidence, ok := src["evidence"].([]any); ok {
		for _, e := range evidence {
			switch ev := e.(type) {
			case string:
				items = append(items, agent.EvidenceItem{
					Type:       "exploration",
					Quote:      ev,
					Confidence: 0.8,
				})
			case map[string]any:
				item := agent.EvidenceItem{
					Type:       "exploration",
					Confidence: 0.9,
				}
				if text, ok := ev["text"].(string); ok {
					item.Quote = text
				}
				if path, ok := ev["path"].(string); ok {
					item.Path = path
				}
				if item.Quote != "" {
					items = append(items, item)
				}
			}
		}
	}

	return items
}
