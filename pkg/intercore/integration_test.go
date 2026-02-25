//go:build integration

package intercore

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestIntegrationRoundTrip(t *testing.T) {
	// Skip if ic is not available.
	if _, err := exec.LookPath("ic"); err != nil {
		t.Skip("ic binary not found, skipping integration test")
	}

	ctx := context.Background()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a run.
	runID, err := c.RunCreate(ctx, ".", "integration-test-"+time.Now().Format("150405"),
		WithComplexity(1),
	)
	if err != nil {
		t.Fatalf("RunCreate: %v", err)
	}
	if runID == "" {
		t.Fatal("RunCreate returned empty ID")
	}
	t.Logf("Created run: %s", runID)

	// Check status.
	run, err := c.RunStatus(ctx, runID)
	if err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if run.ID != runID {
		t.Errorf("RunStatus ID = %q, want %q", run.ID, runID)
	}
	if run.Phase != "brainstorm" {
		t.Errorf("Phase = %q, want brainstorm", run.Phase)
	}
	if !run.IsActive() {
		t.Error("run should be active")
	}

	// List active runs — should include ours.
	runs, err := c.RunList(ctx, true)
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	found := false
	for _, r := range runs {
		if r.ID == runID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RunList(active=true) didn't include run %s", runID)
	}

	// Check gate (should fail — no artifacts).
	gate, err := c.GateCheck(ctx, runID)
	if err != nil && gate == nil {
		t.Fatalf("GateCheck: %v", err)
	}
	if gate != nil && gate.Passed() {
		t.Error("GateCheck should fail — no artifacts")
	}

	// Advance (should work even with soft gate failure).
	adv, err := c.RunAdvance(ctx, runID)
	if err != nil {
		t.Fatalf("RunAdvance: %v", err)
	}
	if !adv.Succeeded() {
		t.Error("RunAdvance should succeed")
	}
	t.Logf("Advanced: %s → %s", adv.FromPhase, adv.ToPhase)

	// Get phase.
	phase, err := c.RunPhase(ctx, runID)
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if phase != adv.ToPhase {
		t.Errorf("RunPhase = %q, want %q", phase, adv.ToPhase)
	}

	// List artifacts (should be empty).
	arts, err := c.ArtifactList(ctx, runID, "")
	if err != nil {
		t.Fatalf("ArtifactList: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("ArtifactList should be empty, got %d", len(arts))
	}

	// Dispatch list (should be empty or contain no entries for this run).
	dispatches, err := c.DispatchList(ctx, false)
	if err != nil {
		t.Fatalf("DispatchList: %v", err)
	}
	_ = dispatches // just verify no error

	// Gate rules.
	rules, err := c.GateRules(ctx)
	if err != nil {
		t.Fatalf("GateRules: %v", err)
	}
	if len(rules) == 0 {
		t.Error("GateRules should return at least one rule")
	}

	// State set/get.
	err = c.StateSet(ctx, "test.integration", runID, `"hello"`)
	if err != nil {
		t.Fatalf("StateSet: %v", err)
	}
	val, err := c.StateGet(ctx, "test.integration", runID)
	if err != nil {
		t.Fatalf("StateGet: %v", err)
	}
	if val != `"hello"` {
		t.Errorf("StateGet = %q, want %q", val, `"hello"`)
	}

	// Clean up state.
	_ = c.StateDelete(ctx, "test.integration", runID)

	// Cancel the run.
	if err := c.RunCancel(ctx, runID); err != nil {
		t.Fatalf("RunCancel: %v", err)
	}

	// Verify cancelled.
	run2, err := c.RunStatus(ctx, runID)
	if err != nil {
		t.Fatalf("RunStatus after cancel: %v", err)
	}
	if run2.Status != "cancelled" {
		t.Errorf("Status after cancel = %q, want cancelled", run2.Status)
	}
}

func TestIntegrationEventsTail(t *testing.T) {
	if _, err := exec.LookPath("ic"); err != nil {
		t.Skip("ic binary not found")
	}

	ctx := context.Background()
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create and advance a run to generate events.
	runID, err := c.RunCreate(ctx, ".", "events-test-"+time.Now().Format("150405"))
	if err != nil {
		t.Fatalf("RunCreate: %v", err)
	}
	defer c.RunCancel(ctx, runID) //nolint:errcheck

	_, err = c.RunAdvance(ctx, runID)
	if err != nil {
		t.Fatalf("RunAdvance: %v", err)
	}

	// Tail events (non-follow — returns batch).
	ch, err := c.EventsTail(ctx, runID, false)
	if err != nil {
		t.Fatalf("EventsTail: %v", err)
	}

	var events []Event
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Log("Warning: no events returned (may be expected for some ic versions)")
	} else {
		t.Logf("Got %d events", len(events))
	}
}
