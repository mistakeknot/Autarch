package views

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/pkg/intercore"
)

// --- Shared message types for run operations ---

// RunsLoadedMsg carries the result of LoadRuns.
type RunsLoadedMsg struct {
	Runs []intercore.Run
	Err  error
	Seq  uint64 // generation counter — handler ignores msg.Seq < v.runsLoadSeq
}

// RunDetailLoadedMsg carries the result of LoadRunDetail.
type RunDetailLoadedMsg struct {
	Run        *intercore.Run
	Dispatches []intercore.Dispatch
	Budget     *intercore.BudgetResult
	Events     []intercore.Event
	Gate       *intercore.GateResult
	Err        error  // non-nil if any fetch failed; partial data still present
	Seq        uint64 // generation counter — handler ignores stale messages
}

// coldwineAdvancedMsg carries the result of a phase advance.
type coldwineAdvancedMsg struct {
	result *intercore.AdvanceResult
	err    error
}

// coldwineGateOverrideMsg carries the result of a gate override.
type coldwineGateOverrideMsg struct {
	runID  string
	reason string
	err    error
}

// coldwineCancelledMsg carries the result of a run cancellation.
type coldwineCancelledMsg struct {
	runID string
	err   error
}

// coldwineAutoAdvanceToggledMsg carries the result of toggling auto-advance.
type coldwineAutoAdvanceToggledMsg struct {
	runID   string
	enabled bool
	err     error
}

// coldwineModeChangeMsg requests a mode change from the command palette.
type coldwineModeChangeMsg struct {
	mode ColdwineMode
}

// layoutModeChangedMsg requests a layout mode change from the command palette.
type layoutModeChangedMsg struct {
	mode LayoutMode
}

// --- Shared action functions ---

// ShouldAutoAdvance checks if a completed dispatch should trigger phase advancement.
// Conditions: run has AutoAdvance enabled, dispatch succeeded (exit 0), run is active.
func ShouldAutoAdvance(run *intercore.Run, d intercore.Dispatch) bool {
	if run == nil || !run.AutoAdvance || !run.IsActive() {
		return false
	}
	return d.Status == "completed" && d.ExitCode != nil && *d.ExitCode == 0
}

// LoadRuns fetches active + inactive runs from Intercore.
// Returns a tea.Cmd that produces RunsLoadedMsg with the given seq number.
func LoadRuns(ic *intercore.Client, seq uint64) tea.Cmd {
	if ic == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		active, err := ic.RunList(ctx, true)
		if err != nil {
			return RunsLoadedMsg{Err: err, Seq: seq}
		}
		inactive, _ := ic.RunList(ctx, false)
		runs := append(active, inactive...)
		return RunsLoadedMsg{Runs: runs, Seq: seq}
	}
}

// LoadRunDetail fetches full run detail from Intercore in a single batch.
// Returns a tea.Cmd that produces RunDetailLoadedMsg with the given seq number.
// Errors are collected into Err but partial data is still returned.
func LoadRunDetail(ic *intercore.Client, runID string, seq uint64) tea.Cmd {
	if ic == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var firstErr error
		collectErr := func(err error) {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}

		run, err := ic.RunStatus(ctx, runID)
		collectErr(err)

		dispatches, err := ic.DispatchList(ctx, false)
		collectErr(err)

		budget, err := ic.RunBudget(ctx, runID)
		collectErr(err)

		events, err := ic.RunEvents(ctx, runID)
		collectErr(err)

		gate, err := ic.GateCheck(ctx, runID)
		collectErr(err)

		// Filter dispatches to this run
		var runDispatches []intercore.Dispatch
		for _, d := range dispatches {
			if d.RunID == runID {
				runDispatches = append(runDispatches, d)
			}
		}

		return RunDetailLoadedMsg{
			Run:        run,
			Dispatches: runDispatches,
			Budget:     budget,
			Events:     events,
			Gate:       gate,
			Err:        firstErr,
			Seq:        seq,
		}
	}
}
