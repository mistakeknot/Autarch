package scoring

import (
	"fmt"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/pipeline"
)

func makeItems(n int) []pipeline.SynthesizedItem {
	items := make([]pipeline.SynthesizedItem, n)
	now := time.Now()
	for i := range items {
		items[i] = pipeline.SynthesizedItem{
			Fetched: pipeline.FetchedItem{
				Raw: pipeline.RawItem{
					ID:          fmt.Sprintf("item-%d", i),
					Type:        []string{"github_repo", "hn_story", "arxiv_paper"}[i%3],
					Title:       fmt.Sprintf("Research Item %d: Advanced Techniques in %s", i, []string{"ML", "Systems", "Security"}[i%3]),
					URL:         fmt.Sprintf("https://example.com/item-%d", i),
					CollectedAt: now.Add(-time.Duration(i) * time.Hour),
					Metadata: map[string]any{
						"stars":    1000 + i*100,
						"comments": 50 + i*10,
					},
				},
				Content:      fmt.Sprintf("This is the content of item %d with some detailed technical description.", i),
				ContentType:  "readme",
				FetchedAt:    now.Add(-time.Duration(i) * 30 * time.Minute),
				FetchSuccess: true,
			},
			Synthesis: pipeline.Synthesis{
				Summary:            fmt.Sprintf("Summary of item %d covering advanced techniques.", i),
				KeyFeatures:        []string{"feature-a", "feature-b", "feature-c"},
				RelevanceRationale: "Highly relevant to current research direction.",
				Recommendations:    []string{"Consider integrating", "Monitor for updates"},
				Confidence:         0.7 + float64(i%3)*0.1,
				SynthesizedAt:      now.Add(-time.Duration(i) * 15 * time.Minute),
			},
		}
	}
	return items
}

func BenchmarkScoreBatch10(b *testing.B) {
	scorer := NewDefaultScorer()
	items := makeItems(10)
	query := "machine learning optimization techniques"

	b.ResetTimer()
	for b.Loop() {
		_ = scorer.ScoreBatch(items, query)
	}
}

func BenchmarkScoreBatch50(b *testing.B) {
	scorer := NewDefaultScorer()
	items := makeItems(50)
	query := "distributed systems observability monitoring"

	b.ResetTimer()
	for b.Loop() {
		_ = scorer.ScoreBatch(items, query)
	}
}

func BenchmarkScoreOne(b *testing.B) {
	scorer := NewDefaultScorer()
	items := makeItems(1)
	query := "agent orchestration framework"
	now := time.Now()

	b.ResetTimer()
	for b.Loop() {
		_ = scorer.ScoreOne(items[0], query, now)
	}
}
