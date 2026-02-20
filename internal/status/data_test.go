package status

import (
	"encoding/json"
	"testing"
)

func TestRunParsing(t *testing.T) {
	raw := `[{"id":"tkjd6vhn","goal":"Cost-aware scheduling","phase":"brainstorm","phases":["brainstorm","strategized","planned","executing","done"],"status":"active","scope_id":"iv-suzr","complexity":3,"created_at":1771606927}]`

	var runs []Run
	if err := json.Unmarshal([]byte(raw), &runs); err != nil {
		t.Fatalf("parse runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	r := runs[0]
	if r.ID != "tkjd6vhn" {
		t.Errorf("ID = %q, want tkjd6vhn", r.ID)
	}
	if r.Phase != "brainstorm" {
		t.Errorf("Phase = %q, want brainstorm", r.Phase)
	}
	if len(r.Phases) != 5 {
		t.Errorf("Phases length = %d, want 5", len(r.Phases))
	}
	if r.Status != "active" {
		t.Errorf("Status = %q, want active", r.Status)
	}
}

func TestRunParsingNoPhasesArray(t *testing.T) {
	// Some runs don't have a phases array
	raw := `[{"id":"psrboftd","goal":"Status tool","phase":"brainstorm","status":"active"}]`

	var runs []Run
	if err := json.Unmarshal([]byte(raw), &runs); err != nil {
		t.Fatalf("parse runs: %v", err)
	}
	if runs[0].Phases != nil {
		t.Errorf("expected nil Phases, got %v", runs[0].Phases)
	}
}

func TestDispatchParsing(t *testing.T) {
	raw := `[{"id":"d1","agent_type":"claude","status":"running","name":"reviewer","model":"opus","in_tokens":1000,"out_tokens":500,"created_at":1771606927,"started_at":1771606930}]`

	var dispatches []Dispatch
	if err := json.Unmarshal([]byte(raw), &dispatches); err != nil {
		t.Fatalf("parse dispatches: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	d := dispatches[0]
	if d.DisplayName() != "reviewer" {
		t.Errorf("DisplayName = %q, want reviewer", d.DisplayName())
	}
	if d.DisplayModel() != "opus" {
		t.Errorf("DisplayModel = %q, want opus", d.DisplayModel())
	}
	if d.InTokens != 1000 {
		t.Errorf("InTokens = %d, want 1000", d.InTokens)
	}
}

func TestDispatchDisplayNameFallback(t *testing.T) {
	d := Dispatch{AgentType: "codex"}
	if d.DisplayName() != "codex" {
		t.Errorf("DisplayName = %q, want codex", d.DisplayName())
	}
	if d.DisplayModel() != "" {
		t.Errorf("DisplayModel = %q, want empty", d.DisplayModel())
	}
}

func TestEventParsing(t *testing.T) {
	raw := `{"id":1,"run_id":"r1","source":"phase","type":"advance","from_state":"brainstorm","to_state":"strategized","timestamp":"2026-02-20T09:15:00Z"}`

	var ev Event
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("parse event: %v", err)
	}
	if ev.Type != "advance" {
		t.Errorf("Type = %q, want advance", ev.Type)
	}
	if ev.FromState != "brainstorm" {
		t.Errorf("FromState = %q, want brainstorm", ev.FromState)
	}
}

func TestTokenParsing(t *testing.T) {
	raw := `{"run_id":"r1","input_tokens":12450,"output_tokens":3200,"total_tokens":15650,"cache_hits":0}`

	var ts TokenSummary
	if err := json.Unmarshal([]byte(raw), &ts); err != nil {
		t.Fatalf("parse tokens: %v", err)
	}
	if ts.InputTokens != 12450 {
		t.Errorf("InputTokens = %d, want 12450", ts.InputTokens)
	}
	if ts.TotalTokens != 15650 {
		t.Errorf("TotalTokens = %d, want 15650", ts.TotalTokens)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{12450, "12,450"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
		{1234567890, "1,234,567,890"},
	}
	for _, tt := range tests {
		got := formatNumber(tt.n)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
