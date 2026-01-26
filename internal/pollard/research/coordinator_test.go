package research

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/hunters"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// mockInsightCreator implements intermute_pkg.InsightCreator for testing
type mockInsightCreator struct {
	mu       sync.Mutex
	insights []intermute.Insight
	err      error
}

func (m *mockInsightCreator) CreateInsight(ctx context.Context, insight intermute.Insight) (intermute.Insight, error) {
	if m.err != nil {
		return intermute.Insight{}, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	insight.ID = "int-insight-" + insight.Title[:8]
	m.insights = append(m.insights, insight)
	return insight, nil
}

func (m *mockInsightCreator) getInsights() []intermute.Insight {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]intermute.Insight(nil), m.insights...)
}

// mockHunter implements hunters.Hunter for testing
type mockHunter struct {
	name   string
	result *hunters.HuntResult
	err    error
}

func (m *mockHunter) Name() string { return m.name }

func (m *mockHunter) Hunt(ctx context.Context, cfg hunters.HunterConfig) (*hunters.HuntResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return &hunters.HuntResult{
		SourcesCollected: 5,
		InsightsCreated:  3,
	}, nil
}

func TestCoordinator_PublishesToIntermute(t *testing.T) {
	// Create mock insight creator
	mock := &mockInsightCreator{insights: make([]intermute.Insight, 0)}

	// Create registry with test hunter
	registry := hunters.NewRegistry()
	registry.Register(&mockHunter{
		name: "test-hunter",
		result: &hunters.HuntResult{
			SourcesCollected: 10,
			InsightsCreated:  5,
		},
	})

	// Create coordinator with publisher
	coord := NewCoordinator(registry)
	coord.SetIntermutePublisher(mock, "autarch")

	// Start a run
	ctx := context.Background()
	topics := []TopicConfig{
		{Key: "tech", Queries: []string{"AI research"}},
	}

	run, err := coord.StartRun(ctx, "test-project", []string{"test-hunter"}, topics)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Wait for run to complete
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for run to complete")
		case <-ticker.C:
			if run.IsDone() {
				goto done
			}
		}
	}
done:

	// Verify findings were published to Intermute
	insights := mock.getInsights()
	if len(insights) == 0 {
		t.Error("expected findings to be published to Intermute")
	}

	// Verify insight has correct project
	if len(insights) > 0 && insights[0].Project != "autarch" {
		t.Errorf("expected project 'autarch', got %s", insights[0].Project)
	}
}

func TestCoordinator_GracefulDegradationWithoutIntermute(t *testing.T) {
	// Create registry with test hunter
	registry := hunters.NewRegistry()
	registry.Register(&mockHunter{
		name: "test-hunter",
		result: &hunters.HuntResult{
			SourcesCollected: 3,
			InsightsCreated:  2,
		},
	})

	// Create coordinator WITHOUT publisher (nil client)
	coord := NewCoordinator(registry)
	// Don't call SetIntermutePublisher - should still work

	// Start a run
	ctx := context.Background()
	topics := []TopicConfig{
		{Key: "general", Queries: []string{"test query"}},
	}

	run, err := coord.StartRun(ctx, "test-project", []string{"test-hunter"}, topics)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Wait for run to complete
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for run to complete")
		case <-ticker.C:
			if run.IsDone() {
				goto done
			}
		}
	}
done:

	// Verify run completed successfully without Intermute
	if run.TotalFindings() == 0 {
		t.Error("expected findings even without Intermute publisher")
	}
}

func TestCoordinator_SetIntermutePublisher(t *testing.T) {
	coord := NewCoordinator(nil)

	// Set publisher - should not panic
	mock := &mockInsightCreator{}
	coord.SetIntermutePublisher(mock, "test-project")

	// The fact that it doesn't panic means it's set
	// We verify the actual publishing works in TestCoordinator_PublishesToIntermute
}
