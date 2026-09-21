// Package prioritize provides agent-powered feature ranking.
// It synthesizes signals, research findings, and execution state into
// ranked feature recommendations with reasoning.
package prioritize

import (
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/signals"
)

// RankingInput assembles all context for the ranking agent.
type RankingInput struct {
	Spec     *specs.Spec
	Signals  []signals.Signal
	Research []ResearchSummary
}

// ResearchSummary is a condensed research finding for the ranking prompt.
type ResearchSummary struct {
	Title      string
	Source     string
	SourceType string
	Relevance  float64
}

// RankedItem is a single feature recommendation with reasoning.
type RankedItem struct {
	FeatureID  string   `json:"feature_id"`
	Title      string   `json:"title"`
	Rank       int      `json:"rank"`
	Reasoning  string   `json:"reasoning"` // 2-3 sentences from agent
	Signals    []string `json:"signals"`   // signal IDs that influenced
	Confidence float64  `json:"confidence"`
}

// RankingResult holds the complete output from the ranking agent.
type RankingResult struct {
	Items   []RankedItem
	Summary string // overall recommendation summary
}

// Ranker coordinates the agent-powered ranking workflow.
type Ranker struct{}

// NewRanker creates a feature ranker.
func NewRanker() *Ranker {
	return &Ranker{}
}

// Rank synthesizes input into ranked feature recommendations.
// Uses the agent prompt from prompt.go to call an LLM agent.
func (r *Ranker) Rank(input RankingInput) (*RankingResult, error) {
	prompt := BuildRankingPrompt(input)

	// For now, return a structured placeholder.
	// The real implementation will call an agent process
	// using the same AgentHunter pattern from Pollard.
	items := extractFeaturesFromSpec(input.Spec)

	return &RankingResult{
		Items:   items,
		Summary: "Ranking based on: " + prompt[:min(100, len(prompt))] + "...",
	}, nil
}

func extractFeaturesFromSpec(spec *specs.Spec) []RankedItem {
	var items []RankedItem
	for i, goal := range spec.Goals {
		items = append(items, RankedItem{
			FeatureID:  goal.ID,
			Title:      goal.Description,
			Rank:       i + 1,
			Reasoning:  "Ranked by goal order — agent-powered reranking pending",
			Confidence: 0.5,
		})
	}
	return items
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
