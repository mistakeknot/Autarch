package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/icdata"
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

func (m Model) renderDashboard() string {
	state := m.agg.GetState()

	// Stats row
	statsStyle := PanelStyle.Copy().Width(m.width/5 - 2)

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

	// Kernel metrics stats
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
		// Count active sessions as before when no kernel
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
	statsRow := lipgloss.JoinHorizontal(lipgloss.Top, statsItems...)

	// Build sections
	sections := []string{statsRow, ""}

	// Active Runs section (kernel)
	if state.Kernel != nil {
		runsTitle := SubtitleStyle.Render("Active Runs")
		var runLines []string
		for projPath, runs := range state.Kernel.Runs {
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
			sections = append(sections, runsTitle, strings.Join(runLines, "\n"), "")
		} else {
			sections = append(sections, runsTitle, LabelStyle.Render("  No active runs"), "")
		}
	}

	// Dispatches section (kernel)
	if state.Kernel != nil {
		dispTitle := SubtitleStyle.Render("Dispatches")
		var dispLines []string
		// Collect all dispatches, sort active-first
		type dispEntry struct {
			projName string
			d        icdata.Dispatch
			us       icdata.UnifiedStatus
		}
		var entries []dispEntry
		for projPath, dispatches := range state.Kernel.Dispatches {
			pn := filepath.Base(projPath)
			for _, d := range dispatches {
				entries = append(entries, dispEntry{pn, d, icdata.UnifyStatus(d.Status)})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].us != entries[j].us {
				return entries[i].us < entries[j].us // Active(1) < Blocked(2) < Waiting(3) < Done(4)
			}
			return entries[i].d.CreatedAt > entries[j].d.CreatedAt // newest first within same status
		})
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
			sections = append(sections, dispTitle, strings.Join(dispLines, "\n"), "")
		}
	}

	// Recent sessions
	recentTitle := SubtitleStyle.Render("Recent Sessions")
	var recentSessions []string
	for i, s := range state.Sessions {
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
	sections = append(sections, recentTitle, strings.Join(recentSessions, "\n"), "")

	// Recent agents
	agentsTitle := SubtitleStyle.Render("Registered Agents")
	var recentAgents []string
	for i, a := range state.Agents {
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
	sections = append(sections, agentsTitle, strings.Join(recentAgents, "\n"), "")

	// Activity feed
	if len(state.Activities) > 0 {
		actTitle := SubtitleStyle.Render("Recent Activity")
		var actLines []string
		for i, a := range state.Activities {
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
		sections = append(sections, actTitle, strings.Join(actLines, "\n"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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
