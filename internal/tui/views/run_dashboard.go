package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// RunDashboardView shows Intercore sprint run status with phase advancement.
// Reads from Intercore: runs, dispatches, budget, events, gates.
type RunDashboardView struct {
	client  *autarch.Client
	iclient *intercore.Client

	// Data
	runs       []intercore.Run
	activeRun  *intercore.Run
	dispatches []intercore.Dispatch
	budget     *intercore.BudgetResult
	events     []intercore.Event
	gate       *intercore.GateResult

	// Selection
	selectedRun int // index into runs

	// UI
	shell     *pkgtui.ShellLayout
	chatPanel *pkgtui.ChatPanel
	width     int
	height    int
	loading   bool
	statusMsg string // transient status message
}

// NewRunDashboardView creates a sprint run dashboard view.
func NewRunDashboardView(client *autarch.Client) *RunDashboardView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Sprint actions: advance, cancel, budget...")
	chatPanel.SetComposerHint("enter send  tab focus  ctrl+b sidebar")
	return &RunDashboardView{
		client:    client,
		shell:     pkgtui.NewShellLayout(),
		chatPanel: chatPanel,
	}
}

// SetIntercore sets the Intercore client for sprint operations.
func (v *RunDashboardView) SetIntercore(ic *intercore.Client) {
	v.iclient = ic
}

// --- Messages ---

type runDashRunsLoadedMsg struct {
	runs []intercore.Run
	err  error
}

type runDashDetailLoadedMsg struct {
	run        *intercore.Run
	dispatches []intercore.Dispatch
	budget     *intercore.BudgetResult
	events     []intercore.Event
	gate       *intercore.GateResult
}

type runDashAdvancedMsg struct {
	result *intercore.AdvanceResult
	err    error
}

type runDashCancelledMsg struct {
	runID string
	err   error
}

// --- View interface ---

func (v *RunDashboardView) Init() tea.Cmd {
	return v.loadRuns()
}

func (v *RunDashboardView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.shell.SetSize(msg.Width, msg.Height)
		v.chatPanel.SetSize(msg.Width/2, msg.Height)

	case tea.KeyMsg:
		if cmd := v.handleKey(msg); cmd != nil {
			return v, cmd
		}
		// Forward to chat panel when chat focused
		if v.shell.Focus() == pkgtui.FocusChat {
			var cmd tea.Cmd
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			return v, cmd
		}

	case runDashRunsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.runs = nil
			return v, nil
		}
		v.runs = msg.runs
		if len(v.runs) > 0 {
			v.selectedRun = 0
			return v, v.loadDetail(v.runs[0].ID)
		}

	case runDashDetailLoadedMsg:
		v.activeRun = msg.run
		v.dispatches = msg.dispatches
		v.budget = msg.budget
		v.events = msg.events
		v.gate = msg.gate

	case runDashAdvancedMsg:
		if msg.err != nil {
			v.statusMsg = fmt.Sprintf("Advance failed: %s", msg.err)
			return v, nil
		}
		if msg.result.Succeeded() {
			v.statusMsg = fmt.Sprintf("Advanced: %s → %s", msg.result.FromPhase, msg.result.ToPhase)
		} else {
			v.statusMsg = fmt.Sprintf("Gate blocked: %s (%s)", msg.result.GateResult, msg.result.Reason)
		}
		// Reload detail after advance
		if v.activeRun != nil {
			return v, v.loadDetail(v.activeRun.ID)
		}

	case runDashCancelledMsg:
		if msg.err != nil {
			v.statusMsg = fmt.Sprintf("Cancel failed: %s", msg.err)
			return v, nil
		}
		v.statusMsg = fmt.Sprintf("Cancelled run %s", msg.runID)
		return v, v.loadRuns()

	case tui.DispatchCompletedMsg:
		v.statusMsg = fmt.Sprintf("Dispatch %s %s", msg.Dispatch.ID, msg.Dispatch.Status)
		// Refresh detail if we're viewing the run this dispatch belongs to.
		if v.activeRun != nil && msg.Dispatch.RunID == v.activeRun.ID {
			return v, v.loadDetail(v.activeRun.ID)
		}
	}

	// Forward remaining messages to chat panel
	var cmd tea.Cmd
	v.chatPanel, cmd = v.chatPanel.Update(msg)
	cmds = append(cmds, cmd)

	return v, tea.Batch(cmds...)
}

func (v *RunDashboardView) View() string {
	sidebar := v.renderSidebar()
	document := v.renderDocument()
	chat := v.chatPanel.View()
	return v.shell.Render(sidebar, document, chat)
}

func (v *RunDashboardView) Focus() tea.Cmd {
	v.shell.SetFocus(pkgtui.FocusDocument)
	return tea.Batch(v.chatPanel.Focus(), v.loadRuns())
}

func (v *RunDashboardView) Blur() {
	v.chatPanel.CancelStream()
	v.chatPanel.Blur()
}

func (v *RunDashboardView) Name() string { return "Sprint" }
func (v *RunDashboardView) ShortHelp() string {
	return "↑/↓ select run  a advance  c cancel  ctrl+r refresh  tab focus  ctrl+b sidebar"
}

// --- CommandProvider ---

func (v *RunDashboardView) Commands() []pkgtui.Command {
	return []pkgtui.Command{
		{
			Name:        "Advance Phase",
			Description: "Advance the active sprint to the next phase",
			Action:      func() tea.Cmd { return v.advancePhase() },
		},
		{
			Name:        "Cancel Sprint",
			Description: "Cancel the active sprint run",
			Action:      func() tea.Cmd { return v.cancelRun() },
		},
		{
			Name:        "Refresh Sprints",
			Description: "Reload all sprint data",
			Action:      func() tea.Cmd { return v.loadRuns() },
		},
	}
}

// --- Key handling ---

func (v *RunDashboardView) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		if v.shell.Focus() == pkgtui.FocusDocument || v.shell.Focus() == pkgtui.FocusSidebar {
			if v.selectedRun > 0 {
				v.selectedRun--
				if v.selectedRun < len(v.runs) {
					return v.loadDetail(v.runs[v.selectedRun].ID)
				}
			}
		}
	case "down", "j":
		if v.shell.Focus() == pkgtui.FocusDocument || v.shell.Focus() == pkgtui.FocusSidebar {
			if v.selectedRun < len(v.runs)-1 {
				v.selectedRun++
				return v.loadDetail(v.runs[v.selectedRun].ID)
			}
		}
	case "a":
		if v.shell.Focus() != pkgtui.FocusChat {
			return v.advancePhase()
		}
	case "c":
		if v.shell.Focus() != pkgtui.FocusChat {
			return v.cancelRun()
		}
	case "ctrl+r":
		return v.loadRuns()
	case "tab":
		v.shell.NextFocus()
		return nil
	case "shift+tab":
		v.shell.PrevFocus()
		return nil
	case "ctrl+b":
		v.shell.ToggleSidebar()
		return nil
	}
	return nil
}

// --- Data loading ---

func (v *RunDashboardView) loadRuns() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	v.loading = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Active runs first, then recent inactive
		active, err := v.iclient.RunList(ctx, true)
		if err != nil {
			return runDashRunsLoadedMsg{err: err}
		}
		inactive, _ := v.iclient.RunList(ctx, false)
		runs := append(active, inactive...)
		return runDashRunsLoadedMsg{runs: runs}
	}
}

func (v *RunDashboardView) loadDetail(runID string) tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		run, _ := v.iclient.RunStatus(ctx, runID)
		dispatches, _ := v.iclient.DispatchList(ctx, false)
		budget, _ := v.iclient.RunBudget(ctx, runID)
		events, _ := v.iclient.RunEvents(ctx, runID)
		gate, _ := v.iclient.GateCheck(ctx, runID)

		// Filter dispatches to this run
		var runDispatches []intercore.Dispatch
		for _, d := range dispatches {
			if d.RunID == runID {
				runDispatches = append(runDispatches, d)
			}
		}

		return runDashDetailLoadedMsg{
			run:        run,
			dispatches: runDispatches,
			budget:     budget,
			events:     events,
			gate:       gate,
		}
	}
}

// --- Actions ---

func (v *RunDashboardView) advancePhase() tea.Cmd {
	if v.iclient == nil || v.activeRun == nil {
		return nil
	}
	runID := v.activeRun.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := v.iclient.RunAdvance(ctx, runID)
		return runDashAdvancedMsg{result: result, err: err}
	}
}

func (v *RunDashboardView) cancelRun() tea.Cmd {
	if v.iclient == nil || v.activeRun == nil {
		return nil
	}
	runID := v.activeRun.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := v.iclient.RunCancel(ctx, runID)
		return runDashCancelledMsg{runID: runID, err: err}
	}
}

// --- Rendering ---

func (v *RunDashboardView) renderSidebar() []pkgtui.SidebarItem {
	if v.iclient == nil {
		return []pkgtui.SidebarItem{{Label: "ic unavailable", Icon: "✗"}}
	}
	if v.loading {
		return []pkgtui.SidebarItem{{Label: "Loading...", Icon: "◐"}}
	}
	if len(v.runs) == 0 {
		return []pkgtui.SidebarItem{{Label: "No sprints", Icon: "○"}}
	}

	items := make([]pkgtui.SidebarItem, 0, len(v.runs))
	for i, r := range v.runs {
		icon := "○"
		switch r.Status {
		case "active":
			icon = "●"
		case "cancelled":
			icon = "✗"
		case "completed":
			icon = "✓"
		}

		label := r.Phase
		if label == "" {
			label = r.Status
		}
		idPrefix := r.ID
		if len(idPrefix) > 6 {
			idPrefix = idPrefix[:6]
		}
		label = idPrefix + " " + label
		if i == v.selectedRun {
			label = "▸ " + label
		}
		items = append(items, pkgtui.SidebarItem{
			ID:    r.ID,
			Label: label,
			Icon:  icon,
		})
	}
	return items
}

func (v *RunDashboardView) renderDocument() string {
	if v.iclient == nil {
		return v.renderUnavailable()
	}
	if v.activeRun == nil {
		if v.loading {
			return "  Loading sprint data..."
		}
		if len(v.runs) == 0 {
			return "  No sprint runs found.\n\n  Create a sprint from the Coldwine tab or via: ic run create"
		}
		return "  Select a sprint run."
	}

	var b strings.Builder
	r := v.activeRun

	// Header
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		titleStyle.Render("Sprint "+r.ID),
		renderRunStatusBadge(r.Status)))
	b.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render(r.Goal)))

	// Phase timeline
	b.WriteString(v.renderPhaseTimeline())
	b.WriteString("\n")

	// Budget
	b.WriteString(v.renderBudget())
	b.WriteString("\n")

	// Gate status
	b.WriteString(v.renderGateStatus())
	b.WriteString("\n")

	// Active dispatches
	b.WriteString(v.renderDispatches())
	b.WriteString("\n")

	// Recent events
	b.WriteString(v.renderEvents())

	// Status message
	if v.statusMsg != "" {
		b.WriteString("\n")
		warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
		b.WriteString(fmt.Sprintf("  %s\n", warnStyle.Render(v.statusMsg)))
	}

	return b.String()
}

func renderRunStatusBadge(status string) string {
	var color lipgloss.Color
	switch status {
	case "active":
		color = pkgtui.ColorSuccess
	case "cancelled":
		color = pkgtui.ColorError
	case "completed":
		color = pkgtui.ColorInfo
	default:
		color = pkgtui.ColorMuted
	}
	return lipgloss.NewStyle().
		Foreground(pkgtui.ColorBg).
		Background(color).
		Padding(0, 1).
		Render(strings.ToUpper(status))
}

func (v *RunDashboardView) renderPhaseTimeline() string {
	if v.activeRun == nil || len(v.activeRun.Phases) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Phase Timeline")))

	currentPhase := v.activeRun.Phase
	doneStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	activeStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true)
	pendingStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)

	pastCurrent := false
	for _, phase := range v.activeRun.Phases {
		var line string
		if phase == currentPhase {
			line = fmt.Sprintf("  ▸ %s  ◀ current", activeStyle.Render(phase))
			pastCurrent = true
		} else if !pastCurrent {
			line = fmt.Sprintf("  ✓ %s", doneStyle.Render(phase))
		} else {
			line = fmt.Sprintf("  ○ %s", pendingStyle.Render(phase))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (v *RunDashboardView) renderBudget() string {
	if v.budget == nil {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Token Budget")))

	used := v.budget.TokensUsed
	total := v.budget.TokenBudget
	if total == 0 {
		b.WriteString("  No budget set\n")
		return b.String()
	}

	pct := float64(used) / float64(total) * 100
	barWidth := 30
	filled := int(float64(barWidth) * float64(used) / float64(total))
	if filled > barWidth {
		filled = barWidth
	}

	var barColor lipgloss.Color
	switch {
	case v.budget.Exceeded:
		barColor = pkgtui.ColorError
	case pct > 80:
		barColor = pkgtui.ColorWarning
	default:
		barColor = pkgtui.ColorSuccess
	}

	barFilled := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled))
	barEmpty := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted).Render(strings.Repeat("░", barWidth-filled))

	b.WriteString(fmt.Sprintf("  %s%s  %s / %s (%.0f%%)\n",
		barFilled, barEmpty,
		formatTokens(used), formatTokens(total), pct))

	if v.budget.Exceeded {
		warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorError)
		b.WriteString(fmt.Sprintf("  %s\n", warnStyle.Render("BUDGET EXCEEDED")))
	}

	return b.String()
}

func (v *RunDashboardView) renderGateStatus() string {
	if v.gate == nil {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Gate Status")))

	var gateIcon, gateLabel string
	var gateColor lipgloss.Color
	if v.gate.Passed() {
		gateIcon = "✓"
		gateLabel = "PASS"
		gateColor = pkgtui.ColorSuccess
	} else {
		gateIcon = "✗"
		gateLabel = "BLOCKED"
		gateColor = pkgtui.ColorError
	}

	gateStyle := lipgloss.NewStyle().Foreground(gateColor)
	transition := fmt.Sprintf("%s → %s", v.gate.FromPhase, v.gate.ToPhase)
	b.WriteString(fmt.Sprintf("  %s %s  %s  (tier: %s)\n",
		gateStyle.Render(gateIcon),
		gateStyle.Render(gateLabel),
		transition,
		v.gate.Tier))

	if v.gate.Evidence != nil {
		for _, cond := range v.gate.Evidence.Conditions {
			icon := "✓"
			color := pkgtui.ColorSuccess
			if cond.Result != "pass" {
				icon = "✗"
				color = pkgtui.ColorError
			}
			condStyle := lipgloss.NewStyle().Foreground(color)
			detail := cond.Check
			if cond.Detail != "" {
				detail += ": " + cond.Detail
			}
			b.WriteString(fmt.Sprintf("    %s %s\n", condStyle.Render(icon), detail))
		}
	}

	return b.String()
}

func (v *RunDashboardView) renderDispatches() string {
	if len(v.dispatches) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Dispatches")))

	for _, d := range v.dispatches {
		icon := "○"
		var color lipgloss.Color
		switch d.Status {
		case "running":
			icon = "●"
			color = pkgtui.ColorPrimary
		case "completed":
			icon = "✓"
			color = pkgtui.ColorSuccess
		case "failed":
			icon = "✗"
			color = pkgtui.ColorError
		default:
			color = pkgtui.ColorMuted
		}

		dispStyle := lipgloss.NewStyle().Foreground(color)
		agent := d.Agent
		if agent == "" {
			agent = d.Type
		}
		idPrefix := d.ID
		if len(idPrefix) > 8 {
			idPrefix = idPrefix[:8]
		}
		line := fmt.Sprintf("  %s %s  %s  %s",
			dispStyle.Render(icon),
			idPrefix,
			agent,
			d.Status)
		if d.ExitCode != nil {
			line += fmt.Sprintf("  (exit %d)", *d.ExitCode)
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

func (v *RunDashboardView) renderEvents() string {
	if len(v.events) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Recent Events")))

	// Show last 8 events
	start := 0
	if len(v.events) > 8 {
		start = len(v.events) - 8
	}
	dimStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
	for _, ev := range v.events[start:] {
		ts := ev.EventTime().Format("15:04:05")
		detail := ev.Type
		if ev.FromState != "" && ev.ToState != "" {
			detail += fmt.Sprintf(" %s→%s", ev.FromState, ev.ToState)
		}
		if ev.Reason != "" {
			detail += " (" + ev.Reason + ")"
		}
		b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
			dimStyle.Render(ts),
			dimStyle.Render(ev.Source),
			detail))
	}

	return b.String()
}

func (v *RunDashboardView) renderUnavailable() string {
	warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
	return fmt.Sprintf("  %s\n\n  The Intercore kernel (ic) is not available.\n  Install it or check that it's in your PATH.",
		warnStyle.Render("Intercore Unavailable"))
}

// --- Helpers ---

func formatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
