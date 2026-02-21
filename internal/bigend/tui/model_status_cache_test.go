package tui

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

type fakeAggStatus struct {
	state aggregator.State
}

func (f *fakeAggStatus) GetState() aggregator.State { return f.state }
func (f *fakeAggStatus) Refresh(ctx context.Context) error { return nil }
func (f *fakeAggStatus) NewSession(string, string, string) error { return nil }
func (f *fakeAggStatus) RestartSession(string, string, string) error { return nil }
func (f *fakeAggStatus) RenameSession(string, string) error { return nil }
func (f *fakeAggStatus) ForkSession(string, string, string) error { return nil }
func (f *fakeAggStatus) AttachSession(string) error { return nil }
func (f *fakeAggStatus) StartMCP(context.Context, string, string) error { return nil }
func (f *fakeAggStatus) StopMCP(string, string) error { return nil }

func TestSessionStatusFromAggregatorState(t *testing.T) {
	agg := &fakeAggStatus{state: aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "a", UnifiedState: icdata.StatusActive},
			{Name: "b", UnifiedState: icdata.StatusWaiting},
			{Name: "c", UnifiedState: icdata.StatusUnknown},
		},
	}}
	m := New(agg, "")
	m.updateLists()

	items := m.sessionList.Items()
	// Items may include group headers, so extract SessionItems
	var sessions []SessionItem
	for _, item := range items {
		if si, ok := item.(SessionItem); ok {
			sessions = append(sessions, si)
		}
	}

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	tests := []struct {
		name   string
		expect icdata.UnifiedStatus
	}{
		{"a", icdata.StatusActive},
		{"b", icdata.StatusWaiting},
		{"c", icdata.StatusUnknown},
	}
	for i, tt := range tests {
		if sessions[i].Status != tt.expect {
			t.Errorf("session %q: expected status %v, got %v", tt.name, tt.expect, sessions[i].Status)
		}
	}
}
