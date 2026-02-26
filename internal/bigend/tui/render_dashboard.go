package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

func (m Model) renderDashboard() string {
	state := m.agg.GetState()
	width := m.width

	// Stats row (cached)
	statsRow := m.dashCache.getOrRender(sectionStats, hashStats(state, width), func() string {
		return m.renderStatsRow(state, width)
	})

	sections := []string{statsRow, ""}

	// Active Runs (cached, kernel only)
	if state.Kernel != nil {
		runsSection := m.dashCache.getOrRender(sectionRuns, hashRuns(state.Kernel, width), func() string {
			return m.renderRunsSection(state.Kernel)
		})
		sections = append(sections, runsSection, "")
	}

	// Dispatches (cached, kernel only)
	if state.Kernel != nil {
		dispSection := m.dashCache.getOrRender(sectionDispatches, hashDispatches(state.Kernel, width), func() string {
			return m.renderDispatchesSection(state.Kernel)
		})
		if dispSection != "" {
			sections = append(sections, dispSection, "")
		}
	}

	// Recent Sessions (cached)
	sessSection := m.dashCache.getOrRender(sectionSessions, hashSessions(state.Sessions, 5), func() string {
		return m.renderRecentSessions(state.Sessions)
	})
	sections = append(sections, sessSection, "")

	// Registered Agents (cached)
	agentsSection := m.dashCache.getOrRender(sectionAgents, hashAgents(state.Agents, 5), func() string {
		return m.renderRecentAgents(state.Agents)
	})
	sections = append(sections, agentsSection, "")

	// Interspect Profiler (cached, only if any project has interspect data)
	interspectSection := m.dashCache.getOrRender(sectionInterspect, hashInterspect(state.Projects, width), func() string {
		return m.renderInterspectSection(state)
	})
	if interspectSection != "" {
		sections = append(sections, interspectSection, "")
	}

	// Cost Baseline (cached, kernel only)
	if state.Kernel != nil && state.Kernel.CostBaseline != nil {
		costSection := m.dashCache.getOrRender(sectionCostBaseline, hashCostBaseline(state.Kernel), func() string {
			return m.renderCostSection(state.Kernel.CostBaseline)
		})
		if costSection != "" {
			sections = append(sections, costSection, "")
		}
	}

	// Activity Feed (cached)
	if len(state.Activities) > 0 {
		actSection := m.dashCache.getOrRender(sectionActivity, hashActivities(state.Activities, 10), func() string {
			return m.renderActivityFeed(state.Activities)
		})
		sections = append(sections, actSection)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderStatsRow(state aggregator.State, width int) string {
	statsStyle := PanelStyle.Copy().Width(width/5 - 2)

	projectCount := len(state.Projects)
	projectLabel := "Projects"
	if state.Kernel != nil && len(state.Kernel.Metrics.KernelErrors) > 0 {
		kernelCount := 0
		for _, p := range state.Projects {
			if p.HasIntercore {
				kernelCount++
			}
		}
		okCount := kernelCount - len(state.Kernel.Metrics.KernelErrors)
		projectLabel = fmt.Sprintf("Projects (%d/%d)", okCount, kernelCount)
	}
	projectStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", projectCount)) + "\n" +
			LabelStyle.Render(projectLabel),
	)

	sessionStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Sessions))) + "\n" +
			LabelStyle.Render("Sessions"),
	)

	agentStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Agents))) + "\n" +
			LabelStyle.Render("Agents"),
	)

	var runsStats, dispatchStats string
	if state.Kernel != nil {
		km := state.Kernel.Metrics
		runsStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", km.ActiveRuns)) + "\n" +
				LabelStyle.Render("Active Runs"),
		)
		blockedStyle := LabelStyle
		if km.BlockedAgents > 0 {
			blockedStyle = StatusError
		}
		dispatchStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", km.ActiveDispatches)) + "\n" +
				blockedStyle.Render(fmt.Sprintf("Dispatches (%d blocked)", km.BlockedAgents)),
		)
	} else {
		activeCount := 0
		for _, s := range state.Sessions {
			if s.UnifiedState == icdata.StatusActive || s.UnifiedState == icdata.StatusWaiting {
				activeCount++
			}
		}
		runsStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", activeCount)) + "\n" +
				LabelStyle.Render("Active"),
		)
		dispatchStats = ""
	}

	statsItems := []string{projectStats, sessionStats, agentStats, runsStats}
	if dispatchStats != "" {
		statsItems = append(statsItems, dispatchStats)
	}
	if state.Kernel != nil {
		km := state.Kernel.Metrics
		totalTokens := km.TotalTokensIn + km.TotalTokensOut
		if totalTokens > 0 {
			tokenStats := statsStyle.Render(
				TitleStyle.Render(formatTokens(totalTokens)) + "\n" +
					LabelStyle.Render(fmt.Sprintf("%s in / %s out",
						formatTokens(km.TotalTokensIn), formatTokens(km.TotalTokensOut))),
			)
			statsItems = append(statsItems, tokenStats)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, statsItems...)
}

func (m Model) renderRunsSection(kernel *aggregator.KernelState) string {
	runsTitle := SubtitleStyle.Render("Active Runs")
	var runLines []string
	for projPath, runs := range kernel.Runs {
		projName := filepath.Base(projPath)
		for _, r := range runs {
			if r.Status == "" || r.Status == "done" || r.Status == "cancelled" {
				continue
			}
			goal := r.Goal
			if len(goal) > 40 {
				goal = goal[:37] + "..."
			}
			id := r.ID
			if len(id) > 8 {
				id = id[:8]
			}
			line := fmt.Sprintf("  %s %s %s %s %s",
				shared.UnifiedStatusSymbol(shared.UnifyStatusForRender(r.Status)),
				LabelStyle.Render(id),
				projName,
				TitleStyle.Render(r.Phase),
				goal,
			)
			runLines = append(runLines, line)
		}
	}
	if len(runLines) > 0 {
		return runsTitle + "\n" + strings.Join(runLines, "\n")
	}
	return runsTitle + "\n" + LabelStyle.Render("  No active runs")
}

func (m Model) renderDispatchesSection(kernel *aggregator.KernelState) string {
	dispTitle := SubtitleStyle.Render("Dispatches")
	type dispEntry struct {
		projName string
		d        icdata.Dispatch
		us       icdata.UnifiedStatus
	}
	var entries []dispEntry
	for projPath, dispatches := range kernel.Dispatches {
		pn := filepath.Base(projPath)
		for _, d := range dispatches {
			entries = append(entries, dispEntry{pn, d, icdata.UnifyStatus(d.Status)})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].us != entries[j].us {
			return entries[i].us < entries[j].us
		}
		return entries[i].d.CreatedAt > entries[j].d.CreatedAt
	})
	var dispLines []string
	for i, e := range entries {
		if i >= 10 {
			break
		}
		id := e.d.ID
		if len(id) > 8 {
			id = id[:8]
		}
		agent := e.d.DisplayName()
		if len(agent) > 16 {
			agent = agent[:16]
		}
		line := fmt.Sprintf("  %s %-8s %-16s %s",
			shared.UnifiedStatusSymbol(e.us),
			LabelStyle.Render(id),
			agent,
			e.us.String(),
		)
		dispLines = append(dispLines, line)
	}
	if len(dispLines) > 0 {
		return dispTitle + "\n" + strings.Join(dispLines, "\n")
	}
	return ""
}

func (m Model) renderRecentSessions(sessions []aggregator.TmuxSession) string {
	recentTitle := SubtitleStyle.Render("Recent Sessions")
	var recentSessions []string
	for i, s := range sessions {
		if i >= 5 {
			break
		}
		name := s.Name
		if s.AgentName != "" {
			name = s.AgentName
		}
		line := fmt.Sprintf("  %s %s %s",
			shared.UnifiedStatusIndicator(s.UnifiedState),
			name,
			LabelStyle.Render(filepath.Base(s.ProjectPath)),
		)
		recentSessions = append(recentSessions, line)
	}
	if len(recentSessions) == 0 {
		recentSessions = append(recentSessions, LabelStyle.Render("  No sessions"))
	}
	return recentTitle + "\n" + strings.Join(recentSessions, "\n")
}

func (m Model) renderRecentAgents(agents []aggregator.Agent) string {
	agentsTitle := SubtitleStyle.Render("Registered Agents")
	var recentAgents []string
	for i, a := range agents {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("  %s %s • %s",
			AgentBadge(a.Program),
			a.Name,
			LabelStyle.Render(filepath.Base(a.ProjectPath)),
		)
		recentAgents = append(recentAgents, line)
	}
	if len(recentAgents) == 0 {
		recentAgents = append(recentAgents, LabelStyle.Render("  No agents registered"))
	}
	return agentsTitle + "\n" + strings.Join(recentAgents, "\n")
}

func (m Model) renderActivityFeed(activities []aggregator.Activity) string {
	actTitle := SubtitleStyle.Render("Recent Activity")
	var actLines []string
	for i, a := range activities {
		if i >= 10 {
			break
		}
		prefix := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("[T]")
		switch a.Source {
		case "kernel":
			prefix = lipgloss.NewStyle().Foreground(shared.ColorPrimary).Render("[K]")
		case "intermute":
			prefix = lipgloss.NewStyle().Foreground(shared.ColorSuccess).Render("[M]")
		}
		ts := LabelStyle.Render(a.Time.Format("15:04:05"))
		line := fmt.Sprintf("  %s %s %s", ts, prefix, a.Summary)
		actLines = append(actLines, line)
	}
	return actTitle + "\n" + strings.Join(actLines, "\n")
}

func (m Model) renderInterspectSection(state aggregator.State) string {
	title := SubtitleStyle.Render("Interspect Profiler")
	var lines []string
	for _, p := range state.Projects {
		if p.InterspectStats == nil {
			continue
		}
		s := p.InterspectStats
		line := fmt.Sprintf("  %s  %s events  %s sessions  %s dispatches  %s advances  %s blocks",
			lipgloss.NewStyle().Foreground(shared.ColorSecondary).Render(p.Name),
			TitleStyle.Render(fmt.Sprintf("%d", s.TotalEvents)),
			LabelStyle.Render(fmt.Sprintf("%d", s.Sessions)),
			LabelStyle.Render(fmt.Sprintf("%d", s.Dispatches)),
			LabelStyle.Render(fmt.Sprintf("%d", s.Advances)),
			blockCountStyle(s.Blocks).Render(fmt.Sprintf("%d", s.Blocks)),
		)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return title + "\n" + strings.Join(lines, "\n")
}

func (m Model) renderCostSection(cb *icdata.CostBaseline) string {
	title := SubtitleStyle.Render(fmt.Sprintf("Cost Baseline (last %dd)", cb.Period.Days))
	var lines []string

	// Row 1: shipped beads + coverage
	coverage := 0
	if cb.ShippedBeads > 0 && cb.Stats.Count > 0 {
		coverage = cb.Stats.Count * 100 / cb.ShippedBeads
		if coverage > 100 {
			coverage = 100
		}
	}
	lines = append(lines, fmt.Sprintf("  Shipped: %s beads    Coverage: %s",
		TitleStyle.Render(fmt.Sprintf("%d", cb.ShippedBeads)),
		TitleStyle.Render(fmt.Sprintf("%d%%", coverage)),
	))

	if cb.Stats.Count == 0 {
		lines = append(lines, LabelStyle.Render("  Token tagging in progress. Data populates as sprints complete."))
		return title + "\n" + strings.Join(lines, "\n")
	}

	// Row 2: percentiles
	lines = append(lines, fmt.Sprintf("  p50: %s    p90: %s    p95: %s    mean: %s",
		TitleStyle.Render(formatTokensCompact(cb.Stats.P50)),
		LabelStyle.Render(formatTokensCompact(cb.Stats.P90)),
		LabelStyle.Render(formatTokensCompact(cb.Stats.P95)),
		LabelStyle.Render(formatTokensCompact(cb.Stats.Mean)),
	))

	// Row 3: total tokens with input/output split
	lines = append(lines, fmt.Sprintf("  Total: %s tokens (input: %s, output: %s)",
		TitleStyle.Render(formatTokensCompact(cb.Stats.Total)),
		LabelStyle.Render(formatTokensCompact(cb.Stats.InputTotal)),
		LabelStyle.Render(formatTokensCompact(cb.Stats.OutputTotal)),
	))

	return title + "\n" + strings.Join(lines, "\n")
}

// formatTokensCompact formats a token count compactly (e.g., 142.3k, 9.9M).
func formatTokensCompact(n int64) string {
	if n == 0 {
		return "--"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

// blockCountStyle returns a warning style if blocks > 0.
func blockCountStyle(blocks int) lipgloss.Style {
	if blocks > 0 {
		return lipgloss.NewStyle().Foreground(shared.ColorWarning)
	}
	return LabelStyle
}

// formatTokens formats a token count with comma separators (e.g., 12,450).
func formatTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d", n/1_000_000, (n%1_000_000)/1000, n%1000)
}
