// Package scoring provides unified quality scoring for research items.
package scoring

import (
	"math"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
)

// Scorer calculates quality scores for research items.
type Scorer struct {
	weights    pipeline.ScoreWeights
	halfLives  pipeline.HalfLives
	thresholds pipeline.ScoreThresholds
}

// NewScorer creates a scorer with the given configuration.
func NewScorer(opts pipeline.ScoreOpts) *Scorer {
	return &Scorer{
		weights:    opts.Weights,
		halfLives:  opts.HalfLives,
		thresholds: opts.Thresholds,
	}
}

// NewDefaultScorer creates a scorer with default configuration.
func NewDefaultScorer() *Scorer {
	return NewScorer(pipeline.DefaultScoreOpts())
}

// ScoreBatch calculates quality scores for multiple items.
func (s *Scorer) ScoreBatch(items []pipeline.SynthesizedItem, query string) []pipeline.ScoredItem {
	results := make([]pipeline.ScoredItem, len(items))
	now := time.Now()

	for i, item := range items {
		results[i] = s.ScoreOne(item, query, now)
	}

	return results
}

// ScoreOne calculates the quality score for a single item.
func (s *Scorer) ScoreOne(item pipeline.SynthesizedItem, query string, now time.Time) pipeline.ScoredItem {
	factors := make(map[string]float64)

	// Calculate individual factors
	factors["engagement"] = s.calculateEngagement(item)
	factors["citations"] = s.calculateCitations(item)
	factors["recency"] = s.calculateRecency(item, now)
	factors["query_match"] = s.calculateQueryMatch(item, query)
	factors["synthesis"] = s.calculateSynthesisScore(item)

	// Calculate weighted final score
	finalScore := factors["engagement"]*s.weights.Engagement +
		factors["citations"]*s.weights.Citations +
		factors["recency"]*s.weights.Recency +
		factors["query_match"]*s.weights.QueryMatch +
		factors["synthesis"]*s.weights.Synthesis

	// Clamp to 0-1 range
	finalScore = math.Max(0, math.Min(1, finalScore))

	// Determine level
	level := s.levelFromScore(finalScore)

	// Calculate confidence based on data completeness
	confidence := s.calculateConfidence(item, factors)

	return pipeline.ScoredItem{
		Synthesized: item,
		Score: pipeline.QualityScore{
			Value:      finalScore,
			Level:      level,
			Factors:    factors,
			Confidence: confidence,
			ScoredAt:   now,
		},
	}
}

// calculateEngagement normalizes engagement metrics (stars, points, comments).
func (s *Scorer) calculateEngagement(item pipeline.SynthesizedItem) float64 {
	metadata := item.Fetched.Raw.Metadata
	if metadata == nil {
		return 0.5 // Default to middle if no data
	}

	switch item.Fetched.Raw.Type {
	case "github_repo":
		stars := extractInt(metadata, "stars")
		// Log scale: 100 stars = 0.5, 1000 = 0.7, 10000 = 0.9
		if stars <= 0 {
			return 0.1
		}
		return math.Min(1.0, 0.1+0.2*math.Log10(float64(stars)))

	case "hn_story":
		points := extractInt(metadata, "points")
		comments := extractInt(metadata, "comments")
		// Combined score: points dominate
		engagement := float64(points) + 0.5*float64(comments)
		// Log scale: 100 = 0.5, 500 = 0.7, 2000 = 0.9
		if engagement <= 0 {
			return 0.1
		}
		return math.Min(1.0, 0.1+0.2*math.Log10(engagement))

	case "arxiv_paper", "openalex_work", "pubmed_article":
		// For academic, engagement = citations handled separately
		return 0.5

	default:
		return 0.5
	}
}

// calculateCitations normalizes citation counts for academic content.
func (s *Scorer) calculateCitations(item pipeline.SynthesizedItem) float64 {
	metadata := item.Fetched.Raw.Metadata
	if metadata == nil {
		return 0.5
	}

	citations := extractInt(metadata, "citations")
	if citations <= 0 {
		// No citations data - not necessarily bad for recent papers
		return 0.5
	}

	// Log scale: 10 citations = 0.5, 100 = 0.7, 1000 = 0.9
	return math.Min(1.0, 0.3+0.2*math.Log10(float64(citations)))
}

// calculateRecency applies temporal decay based on content type.
func (s *Scorer) calculateRecency(item pipeline.SynthesizedItem, now time.Time) float64 {
	metadata := item.Fetched.Raw.Metadata
	if metadata == nil {
		return 0.5
	}

	// Try to get timestamp
	var itemTime time.Time

	if t, ok := metadata["updated_at"].(time.Time); ok {
		itemTime = t
	} else if t, ok := metadata["created_at"].(time.Time); ok {
		itemTime = t
	} else if t, ok := metadata["published_at"].(time.Time); ok {
		itemTime = t
	} else if ts, ok := metadata["updated_at"].(string); ok {
		itemTime, _ = time.Parse(time.RFC3339, ts)
	} else if ts, ok := metadata["created_at"].(string); ok {
		itemTime, _ = time.Parse(time.RFC3339, ts)
	}

	if itemTime.IsZero() {
		return 0.5 // Default if no timestamp
	}

	age := now.Sub(itemTime)
	if age < 0 {
		age = 0 // Future timestamp, treat as current
	}

	// Determine half-life based on content type
	var halfLife time.Duration
	switch item.Fetched.Raw.Type {
	case "hn_story":
		halfLife = s.halfLives.Trends
	case "arxiv_paper", "openalex_work", "pubmed_article":
		halfLife = s.halfLives.Research
	case "github_repo":
		halfLife = s.halfLives.Repos
	default:
		halfLife = s.halfLives.Repos
	}

	return TemporalDecay(age, halfLife)
}

// calculateQueryMatch scores how well the item matches the search query.
func (s *Scorer) calculateQueryMatch(item pipeline.SynthesizedItem, query string) float64 {
	if query == "" {
		return 0.5
	}

	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower)

	// Check title match
	titleLower := strings.ToLower(item.Fetched.Raw.Title)
	titleMatch := 0.0
	for _, term := range queryTerms {
		if strings.Contains(titleLower, term) {
			titleMatch += 1.0 / float64(len(queryTerms))
		}
	}

	// Check description/content match
	contentMatch := 0.0
	if desc, ok := item.Fetched.Raw.Metadata["description"].(string); ok {
		descLower := strings.ToLower(desc)
		for _, term := range queryTerms {
			if strings.Contains(descLower, term) {
				contentMatch += 0.5 / float64(len(queryTerms))
			}
		}
	}

	// Check topics match (for GitHub)
	topicMatch := 0.0
	if topics, ok := item.Fetched.Raw.Metadata["topics"].([]string); ok {
		for _, topic := range topics {
			topicLower := strings.ToLower(topic)
			for _, term := range queryTerms {
				if strings.Contains(topicLower, term) || strings.Contains(term, topicLower) {
					topicMatch += 0.3 / float64(len(queryTerms))
				}
			}
		}
	}

	// Combine scores (title most important)
	return math.Min(1.0, titleMatch*0.5+contentMatch*0.3+topicMatch*0.2)
}

// calculateSynthesisScore uses the agent's confidence from synthesis.
func (s *Scorer) calculateSynthesisScore(item pipeline.SynthesizedItem) float64 {
	synthesis := item.Synthesis

	// If no synthesis was done, return neutral
	if synthesis.Summary == "" || synthesis.Confidence == 0 {
		return 0.5
	}

	// Use the agent's confidence directly (it's already 0-1)
	return synthesis.Confidence
}

// calculateConfidence estimates how reliable the overall score is.
func (s *Scorer) calculateConfidence(item pipeline.SynthesizedItem, factors map[string]float64) float64 {
	// Start with base confidence
	confidence := 0.5

	// Increase confidence if we have more data
	metadata := item.Fetched.Raw.Metadata
	if metadata != nil {
		if _, ok := metadata["stars"]; ok {
			confidence += 0.1
		}
		if _, ok := metadata["citations"]; ok {
			confidence += 0.1
		}
		if _, ok := metadata["updated_at"]; ok {
			confidence += 0.1
		}
	}

	// Increase confidence if synthesis was done
	if item.Synthesis.Confidence > 0 {
		confidence += 0.2
	}

	return math.Min(1.0, confidence)
}

// levelFromScore converts a numeric score to a level string.
func (s *Scorer) levelFromScore(score float64) string {
	if score >= s.thresholds.High {
		return "high"
	}
	if score >= s.thresholds.Medium {
		return "medium"
	}
	return "low"
}

// TemporalDecay calculates exponential decay based on age and half-life.
// Returns 1.0 for brand new, 0.5 for age = halfLife, approaching 0 as age → ∞
func TemporalDecay(age, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 0.5
	}
	if age <= 0 {
		return 1.0
	}
	return math.Pow(0.5, float64(age)/float64(halfLife))
}

// extractInt safely extracts an int from metadata.
func extractInt(metadata map[string]any, key string) int {
	if v, ok := metadata[key].(int); ok {
		return v
	}
	if v, ok := metadata[key].(float64); ok {
		return int(v)
	}
	return 0
}
