package arbiter

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/intermute"
)

func TestNewSprintState_GeneratesID(t *testing.T) {
	state := NewSprintState("/some/path")
	if state.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if len(state.ID) != 32 {
		t.Fatalf("expected 32-char hex ID, got %d chars: %s", len(state.ID), state.ID)
	}
}

func TestNewSprintState_UniqueIDs(t *testing.T) {
	s1 := NewSprintState("/a")
	s2 := NewSprintState("/b")
	if s1.ID == s2.ID {
		t.Fatalf("expected unique IDs, both got %s", s1.ID)
	}
}

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "hello", "hello"},
		{"newlines", "hello\nworld\r\n!", "hello world  !"},
		{"long", strings.Repeat("a", 300), strings.Repeat("a", 200)},
		{"whitespace", "  trimmed  ", "trimmed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// mockClient implements the subset of intermute.Client methods used by ResearchBridge.
type mockClient struct {
	specs    map[string]intermute.Spec
	insights []intermute.Insight
	links    map[string]string // insightID -> specID
	nextID   int
}

func newMockClient() *mockClient {
	return &mockClient{
		specs: make(map[string]intermute.Spec),
		links: make(map[string]string),
	}
}

// mockResearchBridge wraps a mockClient and implements ResearchProvider for testing.
type mockResearchBridge struct {
	mock *mockClient
}

func (m *mockResearchBridge) CreateSpec(_ context.Context, id, title string) (string, error) {
	m.mock.nextID++
	specID := fmt.Sprintf("spec-%d", m.mock.nextID)
	m.mock.specs[specID] = intermute.Spec{
		ID:      specID,
		Title:   sanitizeTitle(title),
		Status:  intermute.SpecStatusDraft,
		Project: "test-project",
	}
	return specID, nil
}

func (m *mockResearchBridge) LinkInsight(_ context.Context, insightID, specID string) error {
	m.mock.links[insightID] = specID
	return nil
}

func (m *mockResearchBridge) PublishInsight(_ context.Context, specID string, finding ResearchFinding) (string, error) {
	m.mock.nextID++
	id := fmt.Sprintf("insight-%d", m.mock.nextID)
	m.mock.insights = append(m.mock.insights, intermute.Insight{
		ID:       id,
		SpecID:   specID,
		Title:    finding.Title,
		Body:     finding.Summary,
		Source:   finding.SourceType,
		Category: strings.Join(finding.Tags, ","),
		URL:      finding.Source,
		Score:    finding.Relevance,
	})
	return id, nil
}

func (m *mockResearchBridge) FetchLinkedInsights(_ context.Context, specID string) ([]ResearchFinding, error) {
	var findings []ResearchFinding
	for _, in := range m.mock.insights {
		if in.SpecID == specID {
			findings = append(findings, ResearchFinding{
				ID:         in.ID,
				Title:      in.Title,
				Summary:    in.Body,
				Source:     in.URL,
				SourceType: in.Source,
				Relevance:  in.Score,
				Tags:       []string{in.Category},
			})
		}
	}
	return findings, nil
}

func (m *mockResearchBridge) StartDeepScan(_ context.Context, specID string) (string, error) {
	return "scan-" + specID, nil
}

func (m *mockResearchBridge) CheckDeepScan(_ context.Context, scanID string) (bool, error) {
	return true, nil
}

func TestResearchBridge_CreateSpec(t *testing.T) {
	mock := newMockClient()
	bridge := &mockResearchBridge{mock: mock}

	specID, err := bridge.CreateSpec(context.Background(), "sprint-123", "My\nSpec Title")
	if err != nil {
		t.Fatalf("CreateSpec: %v", err)
	}
	if specID == "" {
		t.Fatal("expected non-empty spec ID")
	}
	spec := mock.specs[specID]
	if spec.Title != "My Spec Title" {
		t.Errorf("expected sanitized title, got %q", spec.Title)
	}
	if spec.Status != intermute.SpecStatusDraft {
		t.Errorf("expected draft status, got %s", spec.Status)
	}
}

func TestResearchBridge_LinkInsight(t *testing.T) {
	mock := newMockClient()
	bridge := &mockResearchBridge{mock: mock}

	err := bridge.LinkInsight(context.Background(), "insight-1", "spec-1")
	if err != nil {
		t.Fatalf("LinkInsight: %v", err)
	}
	if mock.links["insight-1"] != "spec-1" {
		t.Error("expected insight linked to spec")
	}
}

func TestResearchBridge_FetchLinkedInsights(t *testing.T) {
	mock := newMockClient()
	mock.insights = []intermute.Insight{
		{ID: "i1", SpecID: "spec-1", Title: "Finding 1", Body: "Summary", Source: "github", Category: "competitive", URL: "https://example.com", Score: 0.9},
		{ID: "i2", SpecID: "spec-2", Title: "Unrelated", Body: "Nope", Source: "hn", Category: "trends"},
	}
	bridge := &mockResearchBridge{mock: mock}

	findings, err := bridge.FetchLinkedInsights(context.Background(), "spec-1")
	if err != nil {
		t.Fatalf("FetchLinkedInsights: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.ID != "i1" || f.Title != "Finding 1" || f.SourceType != "github" || f.Relevance != 0.9 {
		t.Errorf("unexpected finding: %+v", f)
	}
}

// Verify ResearchProvider interface compliance
var _ ResearchProvider = (*mockResearchBridge)(nil)
