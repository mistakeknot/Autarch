// Package pipeline defines the generalized 4-stage research pipeline.
// Each hunter implements: Search → Fetch → Synthesize → Score
package pipeline

import (
	"context"
	"time"
)

// Mode controls the depth of the pipeline execution.
type Mode string

const (
	// ModeQuick runs fast with minimal processing (no synthesis).
	ModeQuick Mode = "quick"
	// ModeBalanced is the default - synthesizes top N items.
	ModeBalanced Mode = "balanced"
	// ModeDeep synthesizes all items with parallel agents.
	ModeDeep Mode = "deep"
)

// Pipeline defines the 4-stage search → fetch → synthesize → score flow.
// Each hunter can implement this interface for enhanced processing.
type Pipeline interface {
	// Search finds candidate items (repos, papers, posts, etc.)
	Search(ctx context.Context, query string, opts SearchOpts) ([]RawItem, error)

	// Fetch retrieves detailed information for each item.
	Fetch(ctx context.Context, items []RawItem, opts FetchOpts) ([]FetchedItem, error)

	// Synthesize uses agent to extract insights and relevance.
	Synthesize(ctx context.Context, items []FetchedItem, query string, opts SynthesizeOpts) ([]SynthesizedItem, error)

	// Score assigns quality scores based on all factors.
	Score(ctx context.Context, items []SynthesizedItem, opts ScoreOpts) ([]ScoredItem, error)
}

// SearchOpts configures the search stage.
type SearchOpts struct {
	MaxResults int
	MinStars   int // For GitHub
	MinPoints  int // For HN
	Categories []string
}

// FetchOpts configures the fetch stage.
type FetchOpts struct {
	Mode        Mode
	FetchREADME bool // For GitHub
	FetchDocs   bool // For papers
	Timeout     time.Duration
}

// SynthesizeOpts configures the synthesis stage.
type SynthesizeOpts struct {
	Mode        Mode
	Limit       int           // Max items to synthesize (0 = all)
	Parallelism int           // Max concurrent agent instances
	Timeout     time.Duration // Per-item timeout
}

// ScoreOpts configures the scoring stage.
type ScoreOpts struct {
	Weights    ScoreWeights
	HalfLives  HalfLives
	Thresholds ScoreThresholds
}

// ScoreWeights defines the relative importance of scoring factors.
type ScoreWeights struct {
	Engagement float64 `yaml:"engagement"` // points, comments, stars
	Citations  float64 `yaml:"citations"`  // academic citations
	Recency    float64 `yaml:"recency"`    // temporal decay
	QueryMatch float64 `yaml:"query_match"` // title/content match
	Synthesis  float64 `yaml:"synthesis"`  // agent analysis confidence
}

// HalfLives defines temporal decay rates for different content types.
type HalfLives struct {
	Trends   time.Duration `yaml:"trends"`   // 7 days for HN/news
	Research time.Duration `yaml:"research"` // 365 days for papers
	Repos    time.Duration `yaml:"repos"`    // 90 days for GitHub
}

// ScoreThresholds define quality level cutoffs.
type ScoreThresholds struct {
	High   float64 `yaml:"high"`   // 0.7
	Medium float64 `yaml:"medium"` // 0.4
}

// RawItem represents a search result before detailed fetching.
type RawItem struct {
	ID          string            `yaml:"id"`
	Type        string            `yaml:"type"` // github_repo, hn_story, arxiv_paper, etc.
	Title       string            `yaml:"title"`
	URL         string            `yaml:"url"`
	Metadata    map[string]any    `yaml:"metadata,omitempty"`
	CollectedAt time.Time         `yaml:"collected_at"`
}

// FetchedItem includes content retrieved in the fetch stage.
type FetchedItem struct {
	Raw          RawItem           `yaml:"raw"`
	Content      string            `yaml:"content,omitempty"`      // README, abstract, etc.
	ContentType  string            `yaml:"content_type,omitempty"` // readme, abstract, description
	ExtraData    map[string]any    `yaml:"extra_data,omitempty"`   // Additional fetched metadata
	FetchedAt    time.Time         `yaml:"fetched_at"`
	FetchSuccess bool              `yaml:"fetch_success"`
	FetchError   string            `yaml:"fetch_error,omitempty"`
}

// SynthesizedItem includes agent-generated analysis.
type SynthesizedItem struct {
	Fetched   FetchedItem `yaml:"fetched"`
	Synthesis Synthesis   `yaml:"synthesis"`
}

// Synthesis contains the agent-generated analysis.
type Synthesis struct {
	Summary            string   `yaml:"summary" json:"summary"`
	KeyFeatures        []string `yaml:"key_features" json:"key_features"`
	RelevanceRationale string   `yaml:"relevance_rationale" json:"relevance_rationale"`
	Recommendations    []string `yaml:"recommendations" json:"recommendations"`
	Confidence         float64  `yaml:"confidence" json:"confidence"`
	AgentUsed          string   `yaml:"agent_used,omitempty"`
	SynthesizedAt      time.Time `yaml:"synthesized_at,omitempty"`
}

// ScoredItem includes the final quality score.
type ScoredItem struct {
	Synthesized SynthesizedItem `yaml:"synthesized"`
	Score       QualityScore    `yaml:"score"`
}

// QualityScore represents the final quality assessment.
type QualityScore struct {
	Value      float64            `yaml:"value"`      // Final score 0.0-1.0
	Level      string             `yaml:"level"`      // high, medium, low
	Factors    map[string]float64 `yaml:"factors"`    // engagement, citations, recency, query_match, synthesis
	Confidence float64            `yaml:"confidence"` // How reliable is this score
	ScoredAt   time.Time          `yaml:"scored_at"`
}

// DefaultWeights returns sensible default scoring weights.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Engagement: 0.25,
		Citations:  0.20,
		Recency:    0.25,
		QueryMatch: 0.15,
		Synthesis:  0.15,
	}
}

// DefaultHalfLives returns sensible default temporal decay rates.
func DefaultHalfLives() HalfLives {
	return HalfLives{
		Trends:   7 * 24 * time.Hour,   // 7 days
		Research: 365 * 24 * time.Hour, // 1 year
		Repos:    90 * 24 * time.Hour,  // 90 days
	}
}

// DefaultThresholds returns sensible default score thresholds.
func DefaultThresholds() ScoreThresholds {
	return ScoreThresholds{
		High:   0.7,
		Medium: 0.4,
	}
}

// DefaultScoreOpts returns default scoring options.
func DefaultScoreOpts() ScoreOpts {
	return ScoreOpts{
		Weights:    DefaultWeights(),
		HalfLives:  DefaultHalfLives(),
		Thresholds: DefaultThresholds(),
	}
}
