package intercore

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Test JSON parsing against real ic output samples.

func TestUnmarshalRun(t *testing.T) {
	raw := `{"auto_advance":true,"complexity":3,"created_at":1772048200,"force_full":false,"goal":"[autarch] Go wrapper for ic CLI","id":"q9m1soaj","phase":"brainstorm","project_dir":"/home/mk/projects/Demarch","status":"active","updated_at":1772048200}`

	var r Run
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal Run: %v", err)
	}
	if r.ID != "q9m1soaj" {
		t.Errorf("ID = %q, want %q", r.ID, "q9m1soaj")
	}
	if r.Phase != "brainstorm" {
		t.Errorf("Phase = %q, want %q", r.Phase, "brainstorm")
	}
	if !r.AutoAdvance {
		t.Error("AutoAdvance should be true")
	}
	if r.Complexity != 3 {
		t.Errorf("Complexity = %d, want 3", r.Complexity)
	}
	if !r.IsActive() {
		t.Error("IsActive should be true")
	}
	if r.CreatedTime().IsZero() {
		t.Error("CreatedTime should not be zero")
	}
}

func TestUnmarshalRunWithBudget(t *testing.T) {
	raw := `{"auto_advance":true,"budget_warn_pct":80,"complexity":3,"created_at":1771831094,"force_full":false,"goal":"Dashboard views","id":"liq11b4x","phase":"planned","phases":["brainstorm","brainstorm-reviewed","strategized","planned"],"project_dir":"/home/mk/projects/Demarch","scope_id":"iv-o9yh","status":"active","token_budget":250000,"updated_at":1771831379}`

	var r Run
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal Run: %v", err)
	}
	if r.ScopeID != "iv-o9yh" {
		t.Errorf("ScopeID = %q, want %q", r.ScopeID, "iv-o9yh")
	}
	if r.TokenBudget != 250000 {
		t.Errorf("TokenBudget = %d, want 250000", r.TokenBudget)
	}
	if r.BudgetWarnPct != 80 {
		t.Errorf("BudgetWarnPct = %d, want 80", r.BudgetWarnPct)
	}
	if len(r.Phases) != 4 {
		t.Errorf("len(Phases) = %d, want 4", len(r.Phases))
	}
}

func TestUnmarshalRunList(t *testing.T) {
	raw := `[{"id":"a","goal":"first","phase":"brainstorm","status":"active","created_at":1000,"updated_at":1000},{"id":"b","goal":"second","phase":"executing","status":"active","created_at":2000,"updated_at":2000}]`

	var runs []Run
	if err := json.Unmarshal([]byte(raw), &runs); err != nil {
		t.Fatalf("unmarshal []Run: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
	if runs[0].ID != "a" {
		t.Errorf("runs[0].ID = %q, want %q", runs[0].ID, "a")
	}
	if runs[1].Phase != "executing" {
		t.Errorf("runs[1].Phase = %q, want %q", runs[1].Phase, "executing")
	}
}

func TestUnmarshalGateResult(t *testing.T) {
	raw := `{"evidence":{"conditions":[{"check":"artifact_exists","phase":"brainstorm","result":"fail","count":0,"detail":"no artifacts found for phase \"brainstorm\""}]},"from_phase":"brainstorm","result":"fail","run_id":"q9m1soaj","tier":"soft","to_phase":"brainstorm-reviewed"}`

	var g GateResult
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("unmarshal GateResult: %v", err)
	}
	if g.Passed() {
		t.Error("Passed() should be false")
	}
	if g.Tier != "soft" {
		t.Errorf("Tier = %q, want %q", g.Tier, "soft")
	}
	if g.Evidence == nil {
		t.Fatal("Evidence should not be nil")
	}
	if len(g.Evidence.Conditions) != 1 {
		t.Fatalf("len(Conditions) = %d, want 1", len(g.Evidence.Conditions))
	}
	cond := g.Evidence.Conditions[0]
	if cond.Check != "artifact_exists" {
		t.Errorf("Check = %q, want %q", cond.Check, "artifact_exists")
	}
	if cond.Result != "fail" {
		t.Errorf("Result = %q, want %q", cond.Result, "fail")
	}
}

func TestUnmarshalAdvanceResult(t *testing.T) {
	raw := `{"advanced":true,"event_type":"advance","from_phase":"brainstorm","gate_result":"none","gate_tier":"none","reason":"","to_phase":"brainstorm-reviewed"}`

	var a AdvanceResult
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal AdvanceResult: %v", err)
	}
	if !a.Succeeded() {
		t.Error("Succeeded() should be true")
	}
	if a.FromPhase != "brainstorm" {
		t.Errorf("FromPhase = %q, want %q", a.FromPhase, "brainstorm")
	}
	if a.ToPhase != "brainstorm-reviewed" {
		t.Errorf("ToPhase = %q, want %q", a.ToPhase, "brainstorm-reviewed")
	}
}

func TestUnmarshalGateRules(t *testing.T) {
	raw := `[{"checks":[{"check":"artifact_exists","phase":"brainstorm"}],"from":"brainstorm","to":"brainstorm-reviewed"},{"checks":[{"check":"agents_complete"}],"from":"executing","to":"review"}]`

	var rules []GateRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		t.Fatalf("unmarshal []GateRule: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len = %d, want 2", len(rules))
	}
	if rules[0].From != "brainstorm" {
		t.Errorf("rules[0].From = %q, want %q", rules[0].From, "brainstorm")
	}
	if len(rules[0].Checks) != 1 {
		t.Fatalf("len(rules[0].Checks) = %d, want 1", len(rules[0].Checks))
	}
}

func TestUnmarshalEvent(t *testing.T) {
	raw := `{"id":42,"run_id":"abc123","source":"gate","type":"advance","from_state":"brainstorm","to_state":"brainstorm-reviewed","timestamp":1772048200}`

	var ev Event
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal Event: %v", err)
	}
	if ev.ID != 42 {
		t.Errorf("ID = %d, want 42", ev.ID)
	}
	if ev.RunID != "abc123" {
		t.Errorf("RunID = %q, want %q", ev.RunID, "abc123")
	}
	if ev.EventTime().IsZero() {
		t.Error("EventTime should not be zero")
	}
}

func TestUnmarshalEmptyList(t *testing.T) {
	raw := `[]`
	var dispatches []Dispatch
	if err := json.Unmarshal([]byte(raw), &dispatches); err != nil {
		t.Fatalf("unmarshal []Dispatch: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("len = %d, want 0", len(dispatches))
	}
}

func TestUnmarshalRunAgent(t *testing.T) {
	raw := `{"id":"agent-1","run_id":"run-1","type":"claude","name":"reviewer","status":"active"}`

	var a RunAgent
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal RunAgent: %v", err)
	}
	if a.Type != "claude" {
		t.Errorf("Type = %q, want %q", a.Type, "claude")
	}
}

func TestErrUnavailableWhenBinaryMissing(t *testing.T) {
	_, err := New(WithBinPath("/nonexistent/ic-fake-binary"))
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestRunHelpers(t *testing.T) {
	r := Run{Status: "active", CreatedAt: 1772048200}
	if !r.IsActive() {
		t.Error("IsActive should be true for status=active")
	}

	r.Status = "done"
	if r.IsActive() {
		t.Error("IsActive should be false for status=done")
	}

	ct := r.CreatedTime()
	if ct.Year() < 2026 {
		t.Errorf("CreatedTime year = %d, expected >= 2026", ct.Year())
	}
}

func TestGateResultHelpers(t *testing.T) {
	g := GateResult{Result: "pass"}
	if !g.Passed() {
		t.Error("Passed should be true")
	}
	g.Result = "fail"
	if g.Passed() {
		t.Error("Passed should be false")
	}
}

func TestAdvanceResultHelpers(t *testing.T) {
	a := AdvanceResult{Advanced: true}
	if !a.Succeeded() {
		t.Error("Succeeded should be true")
	}
	a.Advanced = false
	if a.Succeeded() {
		t.Error("Succeeded should be false")
	}
}

func TestIsNoRunError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"ic run current: no active run", true},
		{"ic run status: not found", true},
		{"ic run status: database locked", false},
		{"", false},
	}
	for _, tt := range tests {
		var err error
		if tt.msg != "" {
			err = &testError{tt.msg}
		}
		if got := isNoRunError(err); got != tt.want {
			t.Errorf("isNoRunError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestStringReader(t *testing.T) {
	r := stringReader(`{"key": "value"}`)
	buf := make([]byte, 100)
	n, _ := r.Read(buf)
	s := string(buf[:n])
	if s != "{\"key\": \"value\"}\n" {
		t.Errorf("stringReader output = %q", s)
	}
}

func TestBaseArgsJSON(t *testing.T) {
	c := &Client{timeout: DefaultTimeout}
	args := c.baseArgs(true)
	if len(args) != 1 || args[0] != "--json" {
		t.Errorf("baseArgs(true) = %v, want [--json]", args)
	}

	args = c.baseArgs(false)
	if len(args) != 0 {
		t.Errorf("baseArgs(false) = %v, want []", args)
	}
}

func TestBaseArgsWithDB(t *testing.T) {
	c := &Client{dbPath: "/tmp/test.db", timeout: DefaultTimeout}
	args := c.baseArgs(true)
	if len(args) != 2 || args[0] != "--db=/tmp/test.db" || args[1] != "--json" {
		t.Errorf("baseArgs = %v, want [--db=/tmp/test.db --json]", args)
	}
}

func TestContextTimeout(t *testing.T) {
	// Verify that context cancellation works with the client.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond) // ensure timeout fires

	c := &Client{binPath: "/bin/sleep", timeout: 0}
	_, err := c.execRaw(ctx, "10")
	if err == nil {
		t.Error("expected context deadline error")
	}
}
