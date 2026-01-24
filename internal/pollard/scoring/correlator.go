// Package scoring provides unified quality scoring for research items.
package scoring

import (
	"regexp"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
)

// CorrelatedTopic represents a topic that appears across multiple hunters.
type CorrelatedTopic struct {
	Topic       string   `yaml:"topic"`
	Sources     []string `yaml:"sources"`      // Hunter names where this appeared
	BoostFactor float64  `yaml:"boost_factor"` // Score multiplier
	ItemCount   int      `yaml:"item_count"`   // Total items mentioning this
}

// Correlator detects and boosts topics that appear across multiple hunters.
type Correlator struct {
	minHunters int // Minimum hunters required for correlation
}

// NewCorrelator creates a new correlator.
func NewCorrelator() *Correlator {
	return &Correlator{
		minHunters: 2, // Require at least 2 different sources
	}
}

// CorrelateAndBoost identifies cross-hunter topics and boosts their scores.
func (c *Correlator) CorrelateAndBoost(items []pipeline.ScoredItem) ([]pipeline.ScoredItem, []CorrelatedTopic) {
	// Extract topics from all items
	topicHunters := make(map[string]map[string][]int) // topic -> hunter -> item indices

	for i, item := range items {
		topics := c.extractTopics(item)
		hunterName := c.getHunterName(item)

		for _, topic := range topics {
			if topicHunters[topic] == nil {
				topicHunters[topic] = make(map[string][]int)
			}
			topicHunters[topic][hunterName] = append(topicHunters[topic][hunterName], i)
		}
	}

	// Find correlated topics
	var correlatedTopics []CorrelatedTopic
	boostMap := make(map[int]float64) // item index -> boost factor

	for topic, hunters := range topicHunters {
		if len(hunters) < c.minHunters {
			continue
		}

		// Calculate boost: 1.25x for 2 hunters, 1.5x for 3, etc.
		boostFactor := 1.0 + 0.25*float64(len(hunters)-1)

		// Collect source names and total items
		var sources []string
		itemCount := 0
		for hunter, indices := range hunters {
			sources = append(sources, hunter)
			itemCount += len(indices)

			// Apply boost to each item
			for _, idx := range indices {
				if existing, ok := boostMap[idx]; ok {
					// Take the higher boost if item matches multiple correlated topics
					if boostFactor > existing {
						boostMap[idx] = boostFactor
					}
				} else {
					boostMap[idx] = boostFactor
				}
			}
		}

		correlatedTopics = append(correlatedTopics, CorrelatedTopic{
			Topic:       topic,
			Sources:     sources,
			BoostFactor: boostFactor,
			ItemCount:   itemCount,
		})
	}

	// Apply boosts to scores
	results := make([]pipeline.ScoredItem, len(items))
	for i, item := range items {
		results[i] = item

		if boost, ok := boostMap[i]; ok {
			// Apply boost but cap at 1.0
			newScore := item.Score.Value * boost
			if newScore > 1.0 {
				newScore = 1.0
			}
			results[i].Score.Value = newScore

			// Update level if score changed
			if newScore >= 0.7 {
				results[i].Score.Level = "high"
			} else if newScore >= 0.4 {
				results[i].Score.Level = "medium"
			}

			// Add boost factor to metadata
			results[i].Score.Factors["cross_hunter_boost"] = boost
		}
	}

	return results, correlatedTopics
}

// extractTopics extracts relevant topics from an item for correlation.
func (c *Correlator) extractTopics(item pipeline.ScoredItem) []string {
	var topics []string
	seen := make(map[string]bool)

	addTopic := func(topic string) {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" || len(topic) < 2 {
			return
		}
		// Skip common words
		if c.isStopWord(topic) {
			return
		}
		if !seen[topic] {
			seen[topic] = true
			topics = append(topics, topic)
		}
	}

	// Extract from explicit topics/tags
	if topicList, ok := item.Synthesized.Fetched.Raw.Metadata["topics"].([]string); ok {
		for _, t := range topicList {
			addTopic(t)
		}
	}

	// Extract from title using word extraction
	titleWords := c.extractSignificantWords(item.Synthesized.Fetched.Raw.Title)
	for _, w := range titleWords {
		addTopic(w)
	}

	// Extract from synthesis key features
	for _, feature := range item.Synthesized.Synthesis.KeyFeatures {
		featureWords := c.extractSignificantWords(feature)
		for _, w := range featureWords {
			addTopic(w)
		}
	}

	return topics
}

// extractSignificantWords extracts meaningful words from text.
func (c *Correlator) extractSignificantWords(text string) []string {
	// Remove punctuation and convert to lowercase
	reg := regexp.MustCompile(`[^a-zA-Z0-9\s-]`)
	cleaned := reg.ReplaceAllString(strings.ToLower(text), "")

	words := strings.Fields(cleaned)
	var significant []string

	for _, word := range words {
		// Skip short words and stop words
		if len(word) < 3 || c.isStopWord(word) {
			continue
		}
		significant = append(significant, word)
	}

	return significant
}

// getHunterName extracts the hunter/source name from an item.
func (c *Correlator) getHunterName(item pipeline.ScoredItem) string {
	// Try to get from metadata
	if source, ok := item.Synthesized.Fetched.Raw.Metadata["source"].(string); ok {
		return source
	}

	// Infer from type
	switch item.Synthesized.Fetched.Raw.Type {
	case "github_repo":
		return "github-scout"
	case "hn_story":
		return "hackernews-trendwatcher"
	case "arxiv_paper":
		return "arxiv-scout"
	case "openalex_work":
		return "openalex"
	case "pubmed_article":
		return "pubmed"
	case "competitor_change":
		return "competitor-tracker"
	default:
		return "unknown"
	}
}

// isStopWord returns true if the word is a common stop word.
func (c *Correlator) isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"as": true, "is": true, "was": true, "are": true, "were": true,
		"been": true, "be": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "i": true, "you": true, "we": true,
		"they": true, "he": true, "she": true, "my": true, "your": true,
		"our": true, "their": true, "all": true, "any": true, "both": true,
		"each": true, "few": true, "more": true, "most": true, "other": true,
		"some": true, "such": true, "no": true, "not": true, "only": true,
		"own": true, "same": true, "so": true, "than": true, "too": true,
		"very": true, "just": true, "also": true, "now": true, "new": true,
		"use": true, "using": true, "used": true,
	}
	return stopWords[word]
}
