package intercore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Contract tests: a fake ic binary replays golden JSON through the real Client
// methods. Files named *.golden.json were captured from a live `ic --json`;
// *.from-source.json were written from the emitter structs in the ic source
// (core/intercore) because no live data was available for those commands.

func fakeIC(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n  health*) echo ok; exit 0;;\n"
	for args, file := range routes {
		abs, err := filepath.Abs(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		script += "  \"--json " + args + "\") cat '" + abs + "'; exit 0;;\n"
	}
	script += "  *) echo \"unexpected args: $*\" >&2; exit 2;;\nesac\n"
	bin := filepath.Join(dir, "ic")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := New(WithBinPath(bin))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestContractRunEvents(t *testing.T) {
	c := fakeIC(t, map[string]string{"run events r1": "run_events.golden.json"})
	evs, err := c.RunEvents(context.Background(), "r1")
	if err != nil || len(evs) != 1 {
		t.Fatalf("RunEvents: %v %v", evs, err)
	}
	e := evs[0]
	if e.ID != 40 || e.RunID != "277bvi7e" || e.Type != "cancel" ||
		e.FromState != "brainstorm" || e.ToState != "brainstorm" || e.Timestamp != 1774015332 {
		t.Errorf("event not decoded from real ic keys: %+v", e)
	}
	if e.EventTime().IsZero() || e.Source == "" {
		t.Errorf("event time/source unset: %+v", e)
	}
}

func TestContractRunAgentList(t *testing.T) {
	c := fakeIC(t, map[string]string{"run agent list r1": "run_agent_list.golden.json"})
	as, err := c.RunAgentList(context.Background(), "r1")
	if err != nil || len(as) != 1 {
		t.Fatalf("RunAgentList: %v %v", as, err)
	}
	if as[0].AgentType != "session" || as[0].ID != "sw154048" || as[0].Status != "active" {
		t.Errorf("agent not decoded: %+v", as[0])
	}
}

func TestContractRunBudget(t *testing.T) {
	c := fakeIC(t, map[string]string{"run budget r1": "run_budget.golden.json"})
	b, err := c.RunBudget(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if b.TokenBudget != 250000 || b.TokensUsed != 0 || b.WarnPct != 80 || b.Exceeded || b.RunID != "277bvi7e" {
		t.Errorf("budget not decoded: %+v", b)
	}
}

func TestContractRunBudgetNonzero(t *testing.T) {
	c := fakeIC(t, map[string]string{"run budget r2": "run_budget_nonzero.from-source.json"})
	b, err := c.RunBudget(context.Background(), "r2")
	if err != nil {
		t.Fatal(err)
	}
	if b.TokenBudget != 250000 || b.TokensUsed != 210000 || !b.Warning || b.UsagePct != 84 || b.WarnPct != 80 {
		t.Errorf("budget not decoded: %+v", b)
	}
}

func TestContractDispatch(t *testing.T) {
	c := fakeIC(t, map[string]string{
		"dispatch list":            "dispatch_list.from-source.json",
		"dispatch status d2def567": "dispatch_status.from-source.json",
	})
	ds, err := c.DispatchList(context.Background(), false)
	if err != nil || len(ds) != 2 {
		t.Fatalf("DispatchList: %v %v", ds, err)
	}
	if ds[0].AgentType != "codex" || ds[0].DisplayName() != "reviewer" || ds[0].InputTokens != 1200 {
		t.Errorf("list[0]: %+v", ds[0])
	}
	d, err := c.DispatchStatus(context.Background(), "d2def567")
	if err != nil {
		t.Fatal(err)
	}
	if d.AgentType != "claude" || d.DisplayName() != "claude" || d.ExitCode == nil {
		t.Errorf("status: %+v", d)
	}
}

func TestContractCostBaseline(t *testing.T) {
	c := fakeIC(t, map[string]string{"cost baseline --days=30": "cost_baseline.from-source.json"})
	cb, err := c.CostBaseline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cb.LandedChanges != 12 || cb.Source != "landed_changes" || cb.Stats.Count != 9 {
		t.Errorf("baseline: %+v", cb)
	}
}
