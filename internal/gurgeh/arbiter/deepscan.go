package arbiter

import (
	"context"
	"fmt"
	"time"
)

// StartDeepScan kicks off an async deep scan via Intermute.
// Requires a ResearchProvider and an active spec on the sprint state.
// Returns an error if already running or prerequisites are missing.
func (o *Orchestrator) StartDeepScan(ctx context.Context, state *SprintState) error {
	if o.research == nil {
		return fmt.Errorf("no research provider configured")
	}
	if state.SpecID == "" {
		return fmt.Errorf("sprint has no spec ID; start with research first")
	}
	if state.DeepScan.Status == DeepScanRunning {
		return fmt.Errorf("deep scan already running (scan ID: %s)", state.DeepScan.ScanID)
	}

	scanID, err := o.research.StartDeepScan(ctx, state.SpecID)
	if err != nil {
		state.DeepScan = DeepScanState{
			Status: DeepScanFailed,
			Error:  err.Error(),
		}
		return fmt.Errorf("starting deep scan: %w", err)
	}

	state.DeepScan = DeepScanState{
		Status:    DeepScanRunning,
		ScanID:    scanID,
		StartedAt: time.Now(),
	}
	return nil
}

// CheckDeepScan polls the status of a running deep scan.
// Updates state.DeepScan.Status to DeepScanComplete or DeepScanFailed.
// Returns true if the scan is complete and results are ready to import.
func (o *Orchestrator) CheckDeepScan(ctx context.Context, state *SprintState) (bool, error) {
	if o.research == nil {
		return false, fmt.Errorf("no research provider configured")
	}
	if state.DeepScan.Status != DeepScanRunning {
		return state.DeepScan.Status == DeepScanComplete, nil
	}

	done, err := o.research.CheckDeepScan(ctx, state.DeepScan.ScanID)
	if err != nil {
		state.DeepScan.Status = DeepScanFailed
		state.DeepScan.Error = err.Error()
		return false, fmt.Errorf("checking deep scan: %w", err)
	}

	if done {
		state.DeepScan.Status = DeepScanComplete
	}
	return done, nil
}

// ImportDeepScanResults fetches completed deep scan findings and merges
// them into the sprint's Findings slice, deduplicating by title.
func (o *Orchestrator) ImportDeepScanResults(ctx context.Context, state *SprintState) error {
	if o.research == nil {
		return fmt.Errorf("no research provider configured")
	}
	if state.DeepScan.Status != DeepScanComplete {
		return fmt.Errorf("deep scan not complete (status: %d)", state.DeepScan.Status)
	}

	findings, err := o.research.FetchLinkedInsights(ctx, state.SpecID)
	if err != nil {
		return fmt.Errorf("importing deep scan results: %w", err)
	}

	// Deduplicate by title against existing findings
	existing := make(map[string]bool, len(state.Findings))
	for _, f := range state.Findings {
		existing[f.Title] = true
	}
	for _, f := range findings {
		if !existing[f.Title] {
			state.Findings = append(state.Findings, f)
			existing[f.Title] = true
		}
	}

	return nil
}
