package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

// RunListState holds the state for the kernel run list + detail view.
type RunListState struct {
	Runs         []runEntry
	SelectedIdx  int
	ShowAll      bool // false = active + recent 24h; true = full history
	ProjectPath  string
	DetailScroll int
}

type runEntry struct {
	Run       icdata.Run
	ProjPath  string
	ProjName  string
	Status    icdata.UnifiedStatus
	Duration  time.Duration
	PhaseIdx  int // index of current phase in Phases slice
}

// renderRunList renders the left-side run list pane.
func (m Model) renderRunList(rl *RunListState, width, height int) string {
	if len(rl.Runs) == 0 {
		return LabelStyle.Render("  No kernel runs")
	}

	var lines []string
	for i, e := range rl.Runs {
		if len(lines) >= height-1 {
			break
		}
		line := formatRunLine(e, width, i == rl.SelectedIdx)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderRunDetail renders the right-side detail pane for the selected run.
func (m Model) renderRunDetail(rl *RunListState, state aggregator.State, width int) string {
	if rl.SelectedIdx < 0 || rl.SelectedIdx >= len(rl.Runs) {
		return LabelStyle.Render("  Select a run")
	}

	entry := rl.Runs[rl.SelectedIdx]
	r := entry.Run
	var sections []string

	// Header
	statusSym := shared.UnifiedStatusSymbol(entry.Status)
	goal := r.Goal
	if len(goal) > width-20 {
		goal = goal[:width-23] + "..."
	}
	header := fmt.Sprintf("%s %s  %s  %s",
		statusSym,
		TitleStyle.Render(r.ID),
		SubtitleStyle.Render(r.Phase),
		goal,
	)
	sections = append(sections, header, "")

	// Phase progress
	if len(r.Phases) > 0 {
		phaseBar := renderPhaseProgress(r.Phases, r.Phase, width-4)
		sections = append(sections, "  "+phaseBar, "")
	}

	// Duration + complexity
	var meta []string
	if entry.Duration > 0 {
		meta = append(meta, fmt.Sprintf("Duration: %s", formatDuration(entry.Duration)))
	}
	if r.Complexity > 0 {
		meta = append(meta, fmt.Sprintf("Complexity: %d/5", r.Complexity))
	}
	if len(meta) > 0 {
		sections = append(sections, "  "+LabelStyle.Render(strings.Join(meta, "  •  ")), "")
	}

	// Dispatches for this run
	if state.Kernel != nil {
		dispatches := state.Kernel.Dispatches[entry.ProjPath]
		var runDispatches []icdata.Dispatch
		for _, d := range dispatches {
			if d.ScopeID != nil && *d.ScopeID == r.ID {
				runDispatches = append(runDispatches, d)
			}
		}
		if len(runDispatches) > 0 {
			sections = append(sections, SubtitleStyle.Render("  Dispatches"))
			for _, d := range runDispatches {
				us := icdata.UnifyStatus(d.Status)
				id := d.ID
				if len(id) > 8 {
					id = id[:8]
				}
				agent := d.DisplayName()
				if len(agent) > 20 {
					agent = agent[:20]
				}
				line := fmt.Sprintf("    %s %-8s %-20s %s",
					shared.UnifiedStatusSymbol(us),
					LabelStyle.Render(id),
					agent,
					us.String(),
				)
				sections = append(sections, line)
			}
			sections = append(sections, "")
		}

		// Events for this run
		events := state.Kernel.Events[entry.ProjPath]
		var runEvents []icdata.Event
		for _, ev := range events {
			if ev.RunID == r.ID {
				runEvents = append(runEvents, ev)
			}
		}
		if len(runEvents) > 0 {
			sections = append(sections, SubtitleStyle.Render("  Events"))
			for i, ev := range runEvents {
				if i >= 10 {
					sections = append(sections, LabelStyle.Render(fmt.Sprintf("    ... and %d more", len(runEvents)-10)))
					break
				}
				ts := parseTimestamp(ev.Timestamp)
				summary := fmt.Sprintf("%s.%s", ev.Source, ev.Type)
				if ev.FromState != "" && ev.ToState != "" {
					summary = fmt.Sprintf("%s → %s", ev.FromState, ev.ToState)
				} else if ev.Reason != "" {
					summary = ev.Reason
				}
				line := fmt.Sprintf("    %s %s",
					LabelStyle.Render(ts),
					summary,
				)
				sections = append(sections, line)
			}
			sections = append(sections, "")
		}
	}

	// Token summary
	tokens := computeRunTokens(state, entry.ProjPath, r.ID)
	if tokens.TotalTokens > 0 {
		sections = append(sections, SubtitleStyle.Render("  Tokens"))
		sections = append(sections, fmt.Sprintf("    in: %s / out: %s / total: %s",
			formatTokens(tokens.InputTokens),
			formatTokens(tokens.OutputTokens),
			formatTokens(tokens.TotalTokens),
		))
	}

	return strings.Join(sections, "\n")
}

// buildRunList creates the run entry list from aggregator state, filtering by project.
func buildRunList(state aggregator.State, projectPath string, showAll bool) []runEntry {
	if state.Kernel == nil {
		return nil
	}

	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)
	var entries []runEntry

	addRuns := func(projPath string, runs []icdata.Run) {
		for _, r := range runs {
			us := icdata.UnifyStatus(r.Status)
			dur := time.Duration(0)
			if r.UpdatedAt > 0 && r.CreatedAt > 0 {
				dur = time.Duration(r.UpdatedAt-r.CreatedAt) * time.Second
			}

			// Filter: active always shown; done/error only if within 24h
			if !showAll {
				if us == icdata.StatusDone || us == icdata.StatusErr {
					updated := time.Unix(r.UpdatedAt, 0)
					if updated.Before(cutoff) {
						continue
					}
				}
			}

			phaseIdx := 0
			for i, p := range r.Phases {
				if p == r.Phase {
					phaseIdx = i
					break
				}
			}

			entries = append(entries, runEntry{
				Run:      r,
				ProjPath: projPath,
				ProjName: lastSegment(projPath),
				Status:   us,
				Duration: dur,
				PhaseIdx: phaseIdx,
			})
		}
	}

	if projectPath != "" {
		if runs, ok := state.Kernel.Runs[projectPath]; ok {
			addRuns(projectPath, runs)
		}
	} else {
		for pp, runs := range state.Kernel.Runs {
			addRuns(pp, runs)
		}
	}

	// Sort: active first, then by updated desc
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Status != entries[j].Status {
			return entries[i].Status < entries[j].Status
		}
		return entries[i].Run.UpdatedAt > entries[j].Run.UpdatedAt
	})

	return entries
}

// formatRunLine renders a single run entry line.
func formatRunLine(e runEntry, width int, selected bool) string {
	id := e.Run.ID
	if len(id) > 8 {
		id = id[:8]
	}

	statusSym := shared.UnifiedStatusSymbol(e.Status)
	phase := e.Run.Phase
	if len(phase) > 12 {
		phase = phase[:12]
	}

	dur := "--"
	durStyle := LabelStyle
	if e.Duration > 0 && e.Status == icdata.StatusActive {
		dur = formatDuration(e.Duration)
		if e.Duration > 4*time.Hour {
			durStyle = lipgloss.NewStyle().Foreground(shared.ColorError)
		} else if e.Duration > 1*time.Hour {
			durStyle = lipgloss.NewStyle().Foreground(shared.ColorWarning)
		} else {
			durStyle = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		}
	}

	// Progress bar from phases
	progress := ""
	if len(e.Run.Phases) > 0 {
		progress = renderProgressBar(e.PhaseIdx+1, len(e.Run.Phases), 6)
	}

	// Goal fills remaining
	goal := e.Run.Goal
	fixedWidth := 8 + 1 + 1 + 12 + 1 + 6 + 1 + 6 + 3 // id + space + status + phase + dur + progress + gaps
	goalWidth := width - fixedWidth
	if goalWidth < 0 {
		goalWidth = 0
	}
	if len(goal) > goalWidth {
		if goalWidth > 3 {
			goal = goal[:goalWidth-3] + "..."
		} else {
			goal = ""
		}
	}

	line := fmt.Sprintf(" %s %-8s %-12s %s %s %s",
		statusSym,
		LabelStyle.Render(id),
		phase,
		durStyle.Render(fmt.Sprintf("%6s", dur)),
		progress,
		goal,
	)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1b26")).
			Foreground(lipgloss.Color("#c0caf5")).
			Bold(true).
			Render(line)
	}
	return line
}

// renderProgressBar renders a unicode progress bar.
func renderProgressBar(current, total, width int) string {
	if total == 0 || width <= 0 {
		return ""
	}
	filled := (current * width) / total
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return LabelStyle.Render(bar)
}

// renderPhaseProgress renders a phase chain with the current phase highlighted.
func renderPhaseProgress(phases []string, current string, width int) string {
	if len(phases) == 0 {
		return ""
	}
	var parts []string
	for _, p := range phases {
		name := p
		if len(name) > 10 {
			name = name[:10]
		}
		if p == current {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render(name))
		} else {
			parts = append(parts, LabelStyle.Render(name))
		}
	}
	return strings.Join(parts, " → ")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

func parseTimestamp(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format("15:04:05")
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", ts); err == nil {
		return t.Format("15:04:05")
	}
	return ts
}

func lastSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// renderRunPaneLayout renders the four-pane layout: projects | run list | run detail | dashboard.
// On narrow screens (<100 cols), falls back to run list only.
func (m Model) renderRunPaneLayout(dashContent string) string {
	width := m.width
	h := m.height - 6
	gap := 2

	// Narrow fallback: run list only
	if width < 100 {
		runListContent := m.renderRunList(&m.runList, width, h)
		return runListContent
	}

	// Four-pane: projects (20%) | run list (25%) | run detail (30%) | dashboard (25%)
	projW := width / 5
	remaining := width - projW - gap
	runListW := remaining / 4
	runDetailW := remaining * 3 / 8
	dashW := remaining - runListW - runDetailW - gap*2

	if projW < 15 {
		projW = 15
	}
	if runListW < 20 {
		runListW = 20
	}

	state := m.agg.GetState()

	// Render panes with focus borders
	projStyle := PaneUnfocusedStyle
	runListStyle := PaneUnfocusedStyle
	runDetailStyle := PaneUnfocusedStyle
	dashStyle := PaneUnfocusedStyle

	switch m.activePane {
	case PaneProjects:
		projStyle = PaneFocusedStyle
	case PaneRunList:
		runListStyle = PaneFocusedStyle
	case PaneRunDetail:
		runDetailStyle = PaneFocusedStyle
	case PaneMain:
		dashStyle = PaneFocusedStyle
	}

	// Copy runList for rendering (avoid mutation)
	rl := m.runList

	projView := projStyle.Width(projW).Height(h).Render(m.projectsList.View())
	runListView := runListStyle.Width(runListW).Height(h).Render(
		SubtitleStyle.Render("Kernel Runs") + "\n\n" + m.renderRunList(&rl, runListW-2, h-3),
	)
	runDetailView := runDetailStyle.Width(runDetailW).Height(h).Render(m.renderRunDetail(&rl, state, runDetailW-2))
	dashView := dashStyle.Width(dashW).Height(h).Render(dashContent)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		projView, "  ",
		runListView, "  ",
		runDetailView, "  ",
		dashView,
	)
}

// computeRunTokens sums tokens from dispatches scoped to a run.
func computeRunTokens(state aggregator.State, projPath, runID string) icdata.TokenSummary {
	if state.Kernel == nil {
		return icdata.TokenSummary{}
	}
	dispatches := state.Kernel.Dispatches[projPath]
	var ts icdata.TokenSummary
	ts.RunID = runID
	for _, d := range dispatches {
		if d.ScopeID != nil && *d.ScopeID == runID {
			ts.InputTokens += int64(d.InTokens)
			ts.OutputTokens += int64(d.OutTokens)
			ts.TotalTokens += int64(d.InTokens + d.OutTokens)
		}
	}
	return ts
}
