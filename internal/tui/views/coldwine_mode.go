package views

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// --- Mode switching ---

// switchMode toggles between Epics and Runs modes.
func (v *ColdwineView) switchMode() {
	if v.mode == ModeEpics {
		v.mode = ModeRuns
	} else {
		v.mode = ModeEpics
	}
}

// SetRunsMode sets mode to ModeRuns. Used by unified_app.go via the
// sprintModeActivator interface. Focus() handles the data load.
func (v *ColdwineView) SetRunsMode() {
	v.mode = ModeRuns
}

// --- Runs mode data loading ---

// loadRunsForMode fetches runs for Runs mode sidebar.
// Increments the generation counter so stale responses are rejected.
func (v *ColdwineView) loadRunsForMode() tea.Cmd {
	v.runsLoadSeq++
	return tea.Batch(
		LoadRuns(v.iclient, v.runsLoadSeq),
		v.loadEpicRuns(), // refresh epic associations for orphan detection
	)
}

// --- Runs mode key handling ---

// handleRunsKey handles keys when in Runs mode.
func (v *ColdwineView) handleRunsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		if v.selectedRun > 0 {
			v.selectedRun--
			if v.selectedRun < len(v.runs) {
				v.detailLoadSeq++
				return LoadRunDetail(v.iclient, v.runs[v.selectedRun].ID, v.detailLoadSeq)
			}
		}
	case "down", "j":
		if v.selectedRun < len(v.runs)-1 {
			v.selectedRun++
			v.detailLoadSeq++
			return LoadRunDetail(v.iclient, v.runs[v.selectedRun].ID, v.detailLoadSeq)
		}
	case "a":
		return v.advancePhase()
	case "c":
		return v.cancelRun()
	case "ctrl+r":
		return v.loadRunsForMode()
	}
	return nil
}

// --- Runs mode actions ---
// These derive the target run ID from v.runs[v.selectedRun] (TOCTOU-safe),
// NOT from a cached activeRun pointer.

// advancePhase advances the currently selected run's phase.
func (v *ColdwineView) advancePhase() tea.Cmd {
	if v.iclient == nil || v.selectedRun < 0 || v.selectedRun >= len(v.runs) {
		return nil
	}
	run := v.runs[v.selectedRun]
	runID := run.ID
	currentPhase := run.Phase
	ic := v.iclient
	cc := v.cclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Route through OS layer for gate enforcement when available.
		if cc != nil {
			pauseReason, advErr := cc.SprintAdvance(ctx, runID, currentPhase)
			if advErr == nil {
				if pauseReason != "" {
					return coldwineAdvancedMsg{result: &intercore.AdvanceResult{
						Advanced:   false,
						GateResult: "paused",
						Reason:     pauseReason,
					}}
				}
				// clavain-cli already advanced — read state, don't re-advance.
				updated, getErr := ic.RunStatus(ctx, runID)
				if getErr == nil {
					return coldwineAdvancedMsg{result: &intercore.AdvanceResult{
						Advanced:  true,
						FromPhase: currentPhase,
						ToPhase:   updated.Phase,
					}}
				}
			}
			// Fall through to direct ic on error
		}
		result, err := ic.RunAdvance(ctx, runID)
		return coldwineAdvancedMsg{result: result, err: err}
	}
}

// cancelRun cancels the currently selected run.
func (v *ColdwineView) cancelRun() tea.Cmd {
	if v.iclient == nil || v.selectedRun < 0 || v.selectedRun >= len(v.runs) {
		return nil
	}
	runID := v.runs[v.selectedRun].ID
	ic := v.iclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := ic.RunCancel(ctx, runID)
		return coldwineCancelledMsg{runID: runID, err: err}
	}
}

// toggleAutoAdvance toggles auto-advance for the selected run.
func (v *ColdwineView) toggleAutoAdvance() tea.Cmd {
	if v.iclient == nil || v.selectedRun < 0 || v.selectedRun >= len(v.runs) {
		return nil
	}
	run := v.runs[v.selectedRun]
	runID := run.ID
	newVal := !run.AutoAdvance
	ic := v.iclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := ic.RunSet(ctx, runID, intercore.SetAutoAdvance(newVal))
		return coldwineAutoAdvanceToggledMsg{runID: runID, enabled: newVal, err: err}
	}
}

// tryAutoAdvance attempts to auto-advance the run associated with a dispatch.
// Uses the actual AdvanceResult.FromPhase from the server, not a captured closure phase.
func (v *ColdwineView) tryAutoAdvance(runID string) tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	ic := v.iclient
	cc := v.cclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Gate check first — avoid noisy errors on failure.
		gate, err := ic.GateCheck(ctx, runID)
		if err != nil || gate == nil || !gate.Passed() {
			return coldwineAdvancedMsg{
				result: &intercore.AdvanceResult{
					Advanced:   false,
					GateResult: "blocked",
					Reason:     "auto-advance: gate not ready",
				},
			}
		}

		// Route through OS layer when available.
		if cc != nil {
			// Need current phase for clavain-cli — get from status.
			run, statusErr := ic.RunStatus(ctx, runID)
			if statusErr == nil {
				pauseReason, advErr := cc.SprintAdvance(ctx, runID, run.Phase)
				if advErr == nil {
					if pauseReason != "" {
						return coldwineAdvancedMsg{result: &intercore.AdvanceResult{
							Advanced:   false,
							GateResult: "paused",
							Reason:     pauseReason,
						}}
					}
					updated, getErr := ic.RunStatus(ctx, runID)
					if getErr == nil {
						return coldwineAdvancedMsg{result: &intercore.AdvanceResult{
							Advanced:  true,
							FromPhase: run.Phase,
							ToPhase:   updated.Phase,
						}}
					}
				}
			}
			// Fall through to direct ic on error
		}

		result, err := ic.RunAdvance(ctx, runID)
		return coldwineAdvancedMsg{result: result, err: err}
	}
}

// shouldAutoAdvanceForRun checks if a run should auto-advance after a dispatch.
func (v *ColdwineView) shouldAutoAdvanceForRun(d intercore.Dispatch) (*intercore.Run, bool) {
	// Check runs list first (Runs mode data)
	for _, r := range v.runs {
		if r.ID == d.RunID {
			if ShouldAutoAdvance(&r, d) {
				return &r, true
			}
			return nil, false
		}
	}
	// Check epicRuns (Epics mode data)
	for _, r := range v.epicRuns {
		if r != nil && r.ID == d.RunID {
			if ShouldAutoAdvance(r, d) {
				return r, true
			}
			return nil, false
		}
	}
	return nil, false
}

// --- Runs mode rendering ---

// runsModeSidebarItems returns sidebar items for Runs mode.
func (v *ColdwineView) runsModeSidebarItems() []pkgtui.SidebarItem {
	if v.iclient == nil {
		return []pkgtui.SidebarItem{{Label: "ic unavailable", Icon: "✗"}}
	}
	return renderRunSidebarItems(v.runs, v.selectedRun)
}

// runsModeDocument returns the document content for Runs mode.
func (v *ColdwineView) runsModeDocument() string {
	if v.iclient == nil {
		return v.runsRunDetail.renderUnavailable()
	}
	return v.runsRunDetail.Render()
}

// epicsSidebarItems returns sidebar items for Epics mode.
func (v *ColdwineView) epicsSidebarItems() []pkgtui.SidebarItem {
	if len(v.epics) == 0 {
		return nil
	}

	items := make([]pkgtui.SidebarItem, 0, len(v.epics)+1) // +1 for potential orphans
	for _, epic := range v.epics {
		title := epic.Title
		if title == "" && len(epic.ID) >= 8 {
			title = epic.ID[:8]
		}
		items = append(items, pkgtui.SidebarItem{
			ID:    epic.ID,
			Label: title,
			Icon:  epicStatusIcon(epic.Status),
		})
	}

	// Orphan runs pseudo-entry
	if len(v.orphanRuns) > 0 {
		items = append(items, pkgtui.SidebarItem{
			ID:    "__unscoped_sprints",
			Label: fmt.Sprintf("Unscoped (%d)", len(v.orphanRuns)),
			Icon:  "◇",
		})
	}

	return items
}

// modeIcon returns the icon for a mode toggle sidebar entry.
func modeIcon(current, target ColdwineMode) string {
	if current == target {
		return "●"
	}
	return "○"
}

// computeOrphanRuns identifies runs not associated with any epic.
// Must be called from both RunsLoadedMsg and epicRunsLoadedMsg handlers.
func (v *ColdwineView) computeOrphanRuns() {
	if v.runs == nil || v.epicRuns == nil {
		v.orphanRuns = nil
		return
	}
	associated := make(map[string]bool)
	for _, run := range v.epicRuns {
		if run != nil {
			associated[run.ID] = true
		}
	}
	v.orphanRuns = nil
	for _, r := range v.runs {
		if !associated[r.ID] {
			v.orphanRuns = append(v.orphanRuns, r)
		}
	}
}

// --- Runs mode Update() message handlers ---

// handleRunsLoadedMsg handles the result of LoadRuns.
func (v *ColdwineView) handleRunsLoadedMsg(msg RunsLoadedMsg) (tui.View, tea.Cmd) {
	if msg.Seq < v.runsLoadSeq {
		return v, nil // stale — ignore
	}
	if msg.Err != nil {
		v.runs = nil
		return v, nil
	}
	v.runs = msg.Runs
	// Clamp selectedRun
	if v.selectedRun >= len(v.runs) {
		v.selectedRun = max(0, len(v.runs)-1)
	}
	v.computeOrphanRuns()
	// Load detail for the selected run
	if len(v.runs) > 0 && v.selectedRun < len(v.runs) {
		v.detailLoadSeq++
		return v, LoadRunDetail(v.iclient, v.runs[v.selectedRun].ID, v.detailLoadSeq)
	}
	return v, nil
}

// handleRunDetailLoadedMsg handles the result of LoadRunDetail.
func (v *ColdwineView) handleRunDetailLoadedMsg(msg RunDetailLoadedMsg) (tui.View, tea.Cmd) {
	if msg.Seq < v.detailLoadSeq {
		return v, nil // stale — ignore
	}
	// Route to the correct panel based on context.
	// If we're in Runs mode, update runsRunDetail.
	// Also update epicsRunDetail if the run matches the selected epic.
	v.runsRunDetail.SetData(msg.Run, msg.Dispatches, msg.Budget, msg.Events, msg.Gate)

	// Also update the epics panel if it matches
	if msg.Run != nil && v.selected >= 0 && v.selected < len(v.epics) {
		epicID := v.epics[v.selected].ID
		if run, ok := v.epicRuns[epicID]; ok && run != nil && run.ID == msg.Run.ID {
			v.epicsRunDetail.SetData(msg.Run, msg.Dispatches, msg.Budget, msg.Events, msg.Gate)
		}
	}
	return v, nil
}

// handleColdwineAdvancedMsg handles phase advance results.
func (v *ColdwineView) handleColdwineAdvancedMsg(msg coldwineAdvancedMsg) (tui.View, tea.Cmd) {
	if msg.err != nil {
		v.statusMsg = fmt.Sprintf("Advance failed: %s", msg.err)
		return v, nil
	}
	if msg.result.Succeeded() {
		v.statusMsg = fmt.Sprintf("Advanced: %s → %s", msg.result.FromPhase, msg.result.ToPhase)
	} else {
		v.statusMsg = fmt.Sprintf("Gate blocked: %s (%s)", msg.result.GateResult, msg.result.Reason)
	}
	v.runsRunDetail.SetStatusMsg(v.statusMsg)
	// Reload detail
	if v.selectedRun >= 0 && v.selectedRun < len(v.runs) {
		v.detailLoadSeq++
		return v, LoadRunDetail(v.iclient, v.runs[v.selectedRun].ID, v.detailLoadSeq)
	}
	return v, nil
}

// handleColdwineCancelledMsg handles run cancellation results.
func (v *ColdwineView) handleColdwineCancelledMsg(msg coldwineCancelledMsg) (tui.View, tea.Cmd) {
	if msg.err != nil {
		v.statusMsg = fmt.Sprintf("Cancel failed: %s", msg.err)
		return v, nil
	}
	v.statusMsg = fmt.Sprintf("Cancelled run %s", msg.runID)
	return v, v.loadRunsForMode()
}

// handleAutoAdvanceToggledMsg handles auto-advance toggle results.
func (v *ColdwineView) handleAutoAdvanceToggledMsg(msg coldwineAutoAdvanceToggledMsg) (tui.View, tea.Cmd) {
	if msg.err != nil {
		v.statusMsg = fmt.Sprintf("Toggle auto-advance failed: %s", msg.err)
		return v, nil
	}
	label := "disabled"
	if msg.enabled {
		label = "enabled"
	}
	v.statusMsg = fmt.Sprintf("Auto-advance %s for %s", label, msg.runID)
	// Update local state
	for i := range v.runs {
		if v.runs[i].ID == msg.runID {
			v.runs[i].AutoAdvance = msg.enabled
			break
		}
	}
	return v, nil
}
