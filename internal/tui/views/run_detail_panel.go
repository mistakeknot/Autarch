package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// RunDetailPanel renders sprint run detail: phase timeline, budget bar,
// gate conditions, dispatches list, event log. Embeddable in any parent view.
//
// Methods are named Render/CompactRender (NOT View/CompactView) to avoid
// collision with the pkgtui.View interface.
type RunDetailPanel struct {
	run        *intercore.Run
	dispatches []intercore.Dispatch
	budget     *intercore.BudgetResult
	events     []intercore.Event
	gate       *intercore.GateResult
	width      int
	height     int
	statusMsg  string
	maxEvents  int // 0 = default (8), configurable for compact mode
}

// NewRunDetailPanel creates an empty run detail panel.
func NewRunDetailPanel() *RunDetailPanel {
	return &RunDetailPanel{}
}

// SetData replaces all run data at once.
func (p *RunDetailPanel) SetData(run *intercore.Run, dispatches []intercore.Dispatch, budget *intercore.BudgetResult, events []intercore.Event, gate *intercore.GateResult) {
	p.run = run
	p.dispatches = dispatches
	p.budget = budget
	p.events = events
	p.gate = gate
}

// SetSize sets the available rendering dimensions.
func (p *RunDetailPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetMaxEvents configures the maximum events to show (0 = default 8).
func (p *RunDetailPanel) SetMaxEvents(n int) {
	p.maxEvents = n
}

// SetStatusMsg sets a transient status message displayed at the bottom.
func (p *RunDetailPanel) SetStatusMsg(msg string) {
	p.statusMsg = msg
}

// Render returns the full detail view: header, phase timeline, budget,
// gate, dispatches, events, and status message.
func (p *RunDetailPanel) Render() string {
	if p.run == nil {
		return p.renderEmpty()
	}

	// Loading state: run set but no detail data yet
	if p.dispatches == nil && p.budget == nil && p.events == nil && p.gate == nil {
		return p.renderLoading()
	}

	var b strings.Builder

	b.WriteString(p.renderHeader())
	b.WriteString(p.renderPhaseTimeline())
	if s := p.renderBudget(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}
	if s := p.renderGateStatus(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}
	if s := p.renderDispatches(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}
	if s := p.renderEvents(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}

	if p.statusMsg != "" {
		b.WriteString("\n")
		warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
		b.WriteString(fmt.Sprintf("  %s\n", warnStyle.Render(p.statusMsg)))
	}

	return b.String()
}

// CompactRender returns a compact view: header, phase timeline, budget,
// and gate status only. No dispatches or events.
func (p *RunDetailPanel) CompactRender() string {
	if p.run == nil {
		return p.renderEmpty()
	}

	if p.dispatches == nil && p.budget == nil && p.events == nil && p.gate == nil {
		return p.renderLoading()
	}

	var b strings.Builder
	b.WriteString(p.renderHeader())
	b.WriteString(p.renderPhaseTimeline())
	if s := p.renderBudget(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}
	if s := p.renderGateStatus(); s != "" {
		b.WriteString("\n")
		b.WriteString(s)
	}
	return b.String()
}

func (p *RunDetailPanel) renderEmpty() string {
	return "  Select a sprint run."
}

func (p *RunDetailPanel) renderLoading() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		titleStyle.Render("Sprint "+p.run.ID),
		pkgtui.RenderRunStatusBadge(p.run.Status)))
	b.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render(p.run.Goal)))
	b.WriteString("  Loading sprint detail...\n")
	return b.String()
}

func (p *RunDetailPanel) renderHeader() string {
	r := p.run
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
	autoAdvBadge := ""
	if r.AutoAdvance {
		autoAdvBadge = "  " + lipgloss.NewStyle().
			Foreground(pkgtui.ColorBg).
			Background(pkgtui.ColorInfo).
			Padding(0, 1).
			Render("AUTO")
	}
	return fmt.Sprintf("  %s  %s%s\n  %s\n\n",
		titleStyle.Render("Sprint "+r.ID),
		pkgtui.RenderRunStatusBadge(r.Status),
		autoAdvBadge,
		dimStyle.Render(r.Goal))
}

func (p *RunDetailPanel) renderPhaseTimeline() string {
	if p.run == nil || len(p.run.Phases) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Phase Timeline")))

	currentPhase := p.run.Phase
	doneStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	activeStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true)
	pendingStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)

	pastCurrent := false
	for _, phase := range p.run.Phases {
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

func (p *RunDetailPanel) renderBudget() string {
	if p.budget == nil {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Token Budget")))

	used := p.budget.TokensUsed
	total := p.budget.TokenBudget
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
	case p.budget.Exceeded:
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
		pkgtui.FormatTokens(used), pkgtui.FormatTokens(total), pct))

	if p.budget.Exceeded {
		warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorError)
		b.WriteString(fmt.Sprintf("  %s\n", warnStyle.Render("BUDGET EXCEEDED")))
	}

	return b.String()
}

func (p *RunDetailPanel) renderGateStatus() string {
	if p.gate == nil {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Gate Status")))

	var gateIcon, gateLabel string
	var gateColor lipgloss.Color
	if p.gate.Passed() {
		gateIcon = "✓"
		gateLabel = "PASS"
		gateColor = pkgtui.ColorSuccess
	} else {
		gateIcon = "✗"
		gateLabel = "BLOCKED"
		gateColor = pkgtui.ColorError
	}

	gateStyle := lipgloss.NewStyle().Foreground(gateColor)
	transition := fmt.Sprintf("%s → %s", p.gate.FromPhase, p.gate.ToPhase)
	b.WriteString(fmt.Sprintf("  %s %s  %s  (tier: %s)\n",
		gateStyle.Render(gateIcon),
		gateStyle.Render(gateLabel),
		transition,
		p.gate.Tier))

	if p.gate.Evidence != nil {
		for _, cond := range p.gate.Evidence.Conditions {
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

func (p *RunDetailPanel) renderDispatches() string {
	if len(p.dispatches) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Dispatches")))

	for _, d := range p.dispatches {
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

func (p *RunDetailPanel) renderEvents() string {
	if len(p.events) == 0 {
		return ""
	}

	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(pkgtui.ColorFg)
	b.WriteString(fmt.Sprintf("  %s\n", sectionStyle.Render("Recent Events")))

	maxEvents := 8
	if p.maxEvents > 0 {
		maxEvents = p.maxEvents
	}

	start := 0
	if len(p.events) > maxEvents {
		start = len(p.events) - maxEvents
	}
	dimStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
	for _, ev := range p.events[start:] {
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

func (p *RunDetailPanel) renderUnavailable() string {
	warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
	return fmt.Sprintf("  %s\n\n  The Intercore kernel (ic) is not available.\n  Install it or check that it's in your PATH.",
		warnStyle.Render("Intercore Unavailable"))
}

// renderRunSidebarItems builds sidebar items from a list of runs.
func renderRunSidebarItems(runs []intercore.Run, selectedIdx int) []pkgtui.SidebarItem {
	if len(runs) == 0 {
		return []pkgtui.SidebarItem{{Label: "No sprints", Icon: "○"}}
	}

	items := make([]pkgtui.SidebarItem, 0, len(runs))
	for i, r := range runs {
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
		if i == selectedIdx {
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
