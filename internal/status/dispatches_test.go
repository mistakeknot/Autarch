package status

import (
	"strings"
	"testing"
)

func TestDispatchPaneEmpty(t *testing.T) {
	p := NewDispatchPane()
	p.SetSize(80, 20)

	view := p.View()
	if !strings.Contains(view, "No dispatches") {
		t.Fatal("expected 'No dispatches' in empty pane")
	}
}

func TestDispatchPaneRenderWithData(t *testing.T) {
	p := NewDispatchPane()
	p.SetSize(80, 20)

	name := "reviewer"
	model := "opus"
	started := int64(1771606930)

	p.SetDispatches("run1", []Dispatch{
		{
			ID:        "d1",
			AgentType: "claude",
			Status:    "running",
			Name:      &name,
			Model:     &model,
			InTokens:  1000,
			OutTokens: 500,
			StartedAt: &started,
		},
		{
			ID:        "d2",
			AgentType: "codex",
			Status:    "completed",
			// Name is nil — should fall back to AgentType
		},
	})

	view := p.View()

	// Header should include run ID
	if !strings.Contains(view, "DISPATCHES") {
		t.Error("expected DISPATCHES header")
	}
	if !strings.Contains(view, "run1") {
		t.Error("expected run ID in header")
	}

	// First dispatch: named, with model
	if !strings.Contains(view, "reviewer") {
		t.Error("expected dispatch name 'reviewer'")
	}
	if !strings.Contains(view, "opus") {
		t.Error("expected model name 'opus'")
	}
	if !strings.Contains(view, "running") {
		t.Error("expected status 'running'")
	}

	// Second dispatch: falls back to agent_type
	if !strings.Contains(view, "codex") {
		t.Error("expected fallback to agent_type 'codex'")
	}
	if !strings.Contains(view, "completed") {
		t.Error("expected status 'completed'")
	}
}

func TestDispatchPaneRenderLongName(t *testing.T) {
	p := NewDispatchPane()
	p.SetSize(80, 20)

	longName := "this-is-a-very-long-dispatch-name"
	p.SetDispatches("run1", []Dispatch{
		{
			ID:        "d1",
			AgentType: "claude",
			Status:    "running",
			Name:      &longName,
		},
	})

	view := p.View()

	// Full name should NOT appear (truncated at 20 chars)
	if strings.Contains(view, longName) {
		t.Error("expected long name to be truncated")
	}
	// Truncated prefix should appear
	if !strings.Contains(view, longName[:20]) {
		t.Errorf("expected truncated prefix %q in view", longName[:20])
	}
}

func TestDispatchPaneRenderNilFields(t *testing.T) {
	p := NewDispatchPane()
	p.SetSize(80, 20)

	p.SetDispatches("run1", []Dispatch{
		{
			ID:        "d1",
			AgentType: "claude",
			Status:    "waiting",
			// Name, Model, StartedAt, CompletedAt, ScopeID all nil
		},
	})

	view := p.View()

	// Should not panic and should render em dash for duration
	if !strings.Contains(view, "—") {
		t.Error("expected em dash '—' for nil StartedAt duration")
	}
	// Agent type fallback
	if !strings.Contains(view, "claude") {
		t.Error("expected agent_type fallback")
	}
}
