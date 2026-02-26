package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/icdata"
)

// KernelState holds aggregated Intercore kernel data across all projects.
type KernelState struct {
	Runs         map[string][]icdata.Run      `json:"runs"`
	Dispatches   map[string][]icdata.Dispatch `json:"dispatches"`
	Events       map[string][]icdata.Event    `json:"events"`
	Metrics      KernelMetrics                `json:"metrics"`
	CostBaseline *icdata.CostBaseline         `json:"cost_baseline,omitempty"`
}

// KernelMetrics holds cross-project kernel aggregate stats.
type KernelMetrics struct {
	ActiveRuns       int               `json:"active_runs"`
	ActiveDispatches int               `json:"active_dispatches"`
	BlockedAgents    int               `json:"blocked_agents"`
	TotalTokensIn    int64             `json:"total_tokens_in"`
	TotalTokensOut   int64             `json:"total_tokens_out"`
	KernelErrors     map[string]string `json:"kernel_errors,omitempty"`
}

// enrichWithKernelState fetches Intercore data for all kernel-aware projects.
// Returns nil if no projects have Intercore.
func (a *Aggregator) enrichWithKernelState(ctx context.Context, projects []discovery.Project) *KernelState {
	// Filter to kernel-aware projects
	var kernelProjects []discovery.Project
	for _, p := range projects {
		if p.HasIntercore {
			kernelProjects = append(kernelProjects, p)
		}
	}
	if len(kernelProjects) == 0 {
		return nil
	}

	ks := &KernelState{
		Runs:       make(map[string][]icdata.Run),
		Dispatches: make(map[string][]icdata.Dispatch),
		Events:     make(map[string][]icdata.Event),
		Metrics: KernelMetrics{
			KernelErrors: make(map[string]string),
		},
	}

	// Bounded concurrency: max 5 parallel project enrichments
	sem := make(chan struct{}, 5)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range kernelProjects {
		wg.Add(1)
		go func(proj discovery.Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Per-project timeout
			projCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			runs, err := a.enrichRuns(projCtx, proj.Path)
			if err != nil {
				slog.Warn("kernel enrichment: runs failed", "project", proj.Path, "error", err)
				mu.Lock()
				ks.Metrics.KernelErrors[proj.Path] = err.Error()
				mu.Unlock()
				return
			}

			dispatches, err := a.enrichDispatches(projCtx, proj.Path)
			if err != nil {
				slog.Warn("kernel enrichment: dispatches failed", "project", proj.Path, "error", err)
			}

			events, err := a.enrichEvents(projCtx, proj.Path)
			if err != nil {
				slog.Warn("kernel enrichment: events failed", "project", proj.Path, "error", err)
			}

			mu.Lock()
			ks.Runs[proj.Path] = runs
			ks.Dispatches[proj.Path] = dispatches
			ks.Events[proj.Path] = events
			mu.Unlock()
		}(p)
	}

	wg.Wait()

	// Cost baseline is global (not per-project), fetched once after per-project data.
	// Silently ignore errors — cost data is non-critical.
	costCtx, costCancel := context.WithTimeout(ctx, 3*time.Second)
	defer costCancel()
	baseline, err := icdata.FetchCostBaseline(costCtx)
	if err == nil {
		ks.CostBaseline = baseline
	}

	a.computeKernelMetrics(ks)
	return ks
}

// enrichRuns fetches active runs for a project.
func (a *Aggregator) enrichRuns(ctx context.Context, projectPath string) ([]icdata.Run, error) {
	return icdata.FetchRuns(ctx, projectPath)
}

// enrichDispatches fetches dispatches for a project.
func (a *Aggregator) enrichDispatches(ctx context.Context, projectPath string) ([]icdata.Dispatch, error) {
	return icdata.FetchDispatches(ctx, projectPath, false)
}

// enrichEvents fetches the most recent events for a project.
func (a *Aggregator) enrichEvents(ctx context.Context, projectPath string) ([]icdata.Event, error) {
	return icdata.FetchEvents(ctx, projectPath, "", 50)
}

// kernelEventsToActivities converts kernel events into Activity entries for the unified feed.
func kernelEventsToActivities(ks *KernelState) []Activity {
	if ks == nil {
		return nil
	}
	var acts []Activity
	for projPath, events := range ks.Events {
		projName := filepath.Base(projPath)
		for _, ev := range events {
			synID := fmt.Sprintf("kernel:%s:%d", projPath, ev.ID)
			ts := parseEventTimestamp(ev.Timestamp)
			acts = append(acts, Activity{
				Time:        ts,
				Type:        ev.Type,
				ProjectPath: projPath,
				Summary:     summarizeKernelEvent(projName, ev),
				SyntheticID: synID,
				Source:      "kernel",
			})
		}
	}
	return acts
}

// appendNewActivities appends incoming activities to existing, skipping any whose
// SyntheticID is already in the seenLRU. The seenLRU is the Aggregator's canonical
// seen-set — callers do not need to maintain their own. Returns at most maxActivities
// entries, sorted by time descending. Skips sort when no new items are added.
func appendNewActivities(existing, incoming []Activity, seenLRU map[string]struct{}, maxActivities int) []Activity {
	merged := make([]Activity, len(existing), len(existing)+len(incoming))
	copy(merged, existing)

	added := 0
	for _, a := range incoming {
		if a.SyntheticID != "" {
			if _, ok := seenLRU[a.SyntheticID]; ok {
				continue
			}
		}
		merged = append(merged, a)
		added++
	}

	// Only sort if new items were added
	if added > 0 {
		sort.Slice(merged, func(i, j int) bool {
			return merged[i].Time.After(merged[j].Time)
		})
	}

	if len(merged) > maxActivities {
		merged = merged[:maxActivities]
	}
	return merged
}

// parseEventTimestamp parses an IC event timestamp string.
func parseEventTimestamp(ts string) time.Time {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", ts); err == nil {
		return t
	}
	return time.Now()
}

// summarizeKernelEvent creates a human-readable summary of a kernel event.
func summarizeKernelEvent(projectName string, ev icdata.Event) string {
	ke := icdata.ParseKernelEvent(fmt.Sprintf("%s.%s", ev.Source, ev.Type))
	switch ke {
	case icdata.EventPhaseAdvance:
		return fmt.Sprintf("[%s] phase: %s → %s", projectName, ev.FromState, ev.ToState)
	case icdata.EventPhaseRollback:
		return fmt.Sprintf("[%s] rollback: %s → %s", projectName, ev.FromState, ev.ToState)
	case icdata.EventGatePassed:
		return fmt.Sprintf("[%s] gate passed: %s", projectName, ev.Reason)
	case icdata.EventGateFailed:
		return fmt.Sprintf("[%s] gate failed: %s", projectName, ev.Reason)
	case icdata.EventDispatchSpawned:
		return fmt.Sprintf("[%s] dispatch spawned: %s", projectName, ev.Reason)
	case icdata.EventDispatchCompleted:
		return fmt.Sprintf("[%s] dispatch completed", projectName)
	case icdata.EventDispatchFailed:
		return fmt.Sprintf("[%s] dispatch failed: %s", projectName, ev.Reason)
	default:
		return fmt.Sprintf("[%s] %s.%s", projectName, ev.Source, ev.Type)
	}
}

// computeKernelMetrics calculates aggregate metrics from kernel state.
func (a *Aggregator) computeKernelMetrics(ks *KernelState) {
	for _, runs := range ks.Runs {
		for _, r := range runs {
			us := icdata.UnifyStatus(r.Status)
			if us == icdata.StatusActive {
				ks.Metrics.ActiveRuns++
			}
		}
	}

	for _, dispatches := range ks.Dispatches {
		for _, d := range dispatches {
			us := icdata.UnifyStatus(d.Status)
			switch us {
			case icdata.StatusActive:
				ks.Metrics.ActiveDispatches++
			case icdata.StatusBlocked:
				ks.Metrics.BlockedAgents++
			}
			ks.Metrics.TotalTokensIn += int64(d.InTokens)
			ks.Metrics.TotalTokensOut += int64(d.OutTokens)
		}
	}
}
