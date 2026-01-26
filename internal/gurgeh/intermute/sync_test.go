package intermute

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// mockSpecClient implements SpecManager for testing
type mockSpecClient struct {
	specs     []intermute.Spec
	createErr error
	updateErr error
}

func (m *mockSpecClient) CreateSpec(ctx context.Context, spec intermute.Spec) (intermute.Spec, error) {
	if m.createErr != nil {
		return intermute.Spec{}, m.createErr
	}
	spec.ID = "int-spec-" + spec.Title[:8]
	m.specs = append(m.specs, spec)
	return spec, nil
}

func (m *mockSpecClient) UpdateSpec(ctx context.Context, spec intermute.Spec) (intermute.Spec, error) {
	if m.updateErr != nil {
		return intermute.Spec{}, m.updateErr
	}
	for i, s := range m.specs {
		if s.ID == spec.ID {
			m.specs[i] = spec
			return spec, nil
		}
	}
	return spec, nil
}

func (m *mockSpecClient) GetSpec(ctx context.Context, id string) (intermute.Spec, error) {
	for _, s := range m.specs {
		if s.ID == id {
			return s, nil
		}
	}
	return intermute.Spec{}, nil
}

func TestPRDSyncer_SyncPRD(t *testing.T) {
	mock := &mockSpecClient{specs: make([]intermute.Spec, 0)}
	syncer := NewPRDSyncer(mock, "autarch")

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "Product MVP",
		Version: "mvp",
		Status:  specs.PRDStatusDraft,
		Features: []specs.Feature{
			{ID: "FEAT-001", Title: "User Authentication", Summary: "Login and signup"},
			{ID: "FEAT-002", Title: "Dashboard", Summary: "Main dashboard view"},
		},
	}

	intermuteSpec, err := syncer.SyncPRD(context.Background(), prd)
	if err != nil {
		t.Fatalf("SyncPRD failed: %v", err)
	}

	if intermuteSpec.ID == "" {
		t.Error("expected non-empty spec ID")
	}
	if len(mock.specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(mock.specs))
	}

	created := mock.specs[0]
	if created.Title != "Product MVP" {
		t.Errorf("expected title 'Product MVP', got %s", created.Title)
	}
	if created.Status != intermute.SpecStatusDraft {
		t.Errorf("expected status 'draft', got %s", created.Status)
	}
}

func TestPRDSyncer_UpdateExisting(t *testing.T) {
	mock := &mockSpecClient{specs: make([]intermute.Spec, 0)}
	syncer := NewPRDSyncer(mock, "autarch")

	// First sync creates the spec
	prd := &specs.PRD{
		ID:     "V1",
		Title:  "Version 1",
		Status: specs.PRDStatusDraft,
	}

	intermuteSpec, _ := syncer.SyncPRD(context.Background(), prd)

	// Update the PRD
	prd.Title = "Version 1 - Updated"
	prd.Status = specs.PRDStatusApproved

	// Sync again with known ID
	updatedSpec, err := syncer.SyncPRDWithID(context.Background(), prd, intermuteSpec.ID)
	if err != nil {
		t.Fatalf("SyncPRDWithID failed: %v", err)
	}

	if updatedSpec.Title != "Version 1 - Updated" {
		t.Errorf("expected updated title, got %s", updatedSpec.Title)
	}
}

func TestPRDSyncer_MapPRDStatus(t *testing.T) {
	testCases := []struct {
		prdStatus specs.PRDStatus
		expected  intermute.SpecStatus
	}{
		{specs.PRDStatusDraft, intermute.SpecStatusDraft},
		{specs.PRDStatusApproved, intermute.SpecStatusResearch},      // approved -> research
		{specs.PRDStatusInProgress, intermute.SpecStatusValidated},  // in_progress -> validated
		{specs.PRDStatusDone, intermute.SpecStatusArchived},          // done -> archived
	}

	for _, tc := range testCases {
		t.Run(string(tc.prdStatus), func(t *testing.T) {
			result := mapPRDStatusToSpecStatus(tc.prdStatus)
			if result != tc.expected {
				t.Errorf("mapPRDStatusToSpecStatus(%s) = %s, want %s",
					tc.prdStatus, result, tc.expected)
			}
		})
	}
}

func TestPRDSyncer_ExtractVisionFromFeatures(t *testing.T) {
	features := []specs.Feature{
		{Title: "Auth", Summary: "User authentication with OAuth"},
		{Title: "Dashboard", Summary: "Real-time analytics dashboard"},
	}

	vision := extractVisionFromFeatures(features)

	if vision == "" {
		t.Error("expected non-empty vision")
	}
	// Vision should mention the features
	if len(vision) < 20 {
		t.Error("expected longer vision summary")
	}
}

func TestPRDSyncer_NilClientGracefulDegradation(t *testing.T) {
	syncer := NewPRDSyncer(nil, "autarch")

	prd := &specs.PRD{
		ID:    "TEST",
		Title: "Test PRD",
	}

	spec, err := syncer.SyncPRD(context.Background(), prd)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if spec.ID != "" {
		t.Error("expected empty spec for nil client")
	}
}
