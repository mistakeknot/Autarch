package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// advancePhaseMsg signals the orchestrator should advance to the next phase.
type advancePhaseMsg struct{}

// SprintView is the TUI component for the Arbiter Spec Sprint.
type SprintView struct {
	state        *arbiter.SprintState
	orchestrator *arbiter.Orchestrator
	width        int
	height       int
	focused      string // "draft" or "options"
	optionIndex  int
	showResearch bool // toggle research panel
	keys         pkgtui.CommonKeys
}

// NewSprintView creates a new SprintView for the given sprint state.
func NewSprintView(state *arbiter.SprintState) *SprintView {
	return &SprintView{
		state:        state,
		orchestrator: newOrchestratorWithScanner(state.ProjectPath),
		width:        80,
		height:       24,
		focused:      "draft",
		optionIndex:  0,
		keys:         pkgtui.NewCommonKeys(),
	}
}

// Init implements tea.Model.
func (v *SprintView) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (v *SprintView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.state == nil {
		return v, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			v.width = msg.Width
		}
		if msg.Height > 0 {
			v.height = msg.Height
		}
	case tea.KeyMsg:
		keyStr := msg.String()
		switch {
		case keyStr == "a" || keyStr == "A":
			v.orchestrator.AcceptDraft(v.state)
			return v, nil
		case keyStr == "e" || keyStr == "E":
			// Edit stub - future implementation
			return v, nil
		case len(v.keys.Sections) > 0 && key.Matches(msg, v.keys.Sections[0]):
			v.selectOption(0)
		case len(v.keys.Sections) > 1 && key.Matches(msg, v.keys.Sections[1]):
			v.selectOption(1)
		case len(v.keys.Sections) > 2 && key.Matches(msg, v.keys.Sections[2]):
			v.selectOption(2)
		case key.Matches(msg, v.keys.NavDown):
			section := v.currentSection()
			if section != nil && v.optionIndex < len(section.Options)-1 {
				v.optionIndex++
			}
		case key.Matches(msg, v.keys.NavUp):
			if v.optionIndex > 0 {
				v.optionIndex--
			}
		case keyStr == "R":
			v.showResearch = !v.showResearch
		case key.Matches(msg, v.keys.Quit), key.Matches(msg, v.keys.Back):
			return v, tea.Quit
		}
	}
	return v, nil
}

// View implements tea.Model.
func (v *SprintView) View() string {
	if v.state == nil {
		return ""
	}
	var b strings.Builder

	// Header: phase name + confidence
	b.WriteString(v.renderHeader())
	b.WriteString("\n\n")

	// Draft box
	b.WriteString(v.renderDraftBox())
	b.WriteString("\n\n")

	// Options
	b.WriteString(v.renderOptions())
	b.WriteString("\n")

	// Conflicts
	if len(v.state.Conflicts) > 0 {
		b.WriteString("\n")
		b.WriteString(v.renderConflicts())
		b.WriteString("\n")
	}

	// Research panel
	if v.showResearch {
		b.WriteString("\n")
		b.WriteString(v.renderResearchPanel())
		b.WriteString("\n")
	}

	// Help line
	b.WriteString("\n")
	b.WriteString(v.renderHelp())

	return b.String()
}

func (v *SprintView) currentSection() *arbiter.SectionDraft {
	if section, ok := v.state.Sections[v.state.Phase]; ok {
		return section
	}
	return nil
}

func (v *SprintView) selectOption(idx int) {
	section := v.currentSection()
	if section == nil || idx >= len(section.Options) {
		return
	}
	section.Content = section.Options[idx]
}

func (v *SprintView) renderHeader() string {
	phase := v.state.Phase.String()
	confidence := v.state.Confidence.Total() * 100

	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	confidenceStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSecondary)

	return headerStyle.Render(fmt.Sprintf("Sprint: %s", phase)) +
		"  " +
		confidenceStyle.Render(fmt.Sprintf("Confidence: %.0f%%", confidence))
}

func (v *SprintView) renderDraftBox() string {
	section := v.currentSection()
	if section == nil {
		return ""
	}

	icon := statusIcon(section.Status)
	content := section.Content
	if content == "" {
		content = "(pending)"
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorBorder).
		Padding(1, 2).
		Width(min(v.width-4, 76))

	return boxStyle.Render(fmt.Sprintf("%s %s", icon, content))
}

func (v *SprintView) renderOptions() string {
	var lines []string

	acceptStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	editStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
	optStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)

	lines = append(lines, acceptStyle.Render("[a] Accept")+"  "+editStyle.Render("[e] Edit"))

	section := v.currentSection()
	if section != nil && len(section.Options) > 0 {
		lines = append(lines, "")
		mutedStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
		lines = append(lines, mutedStyle.Render("Alternatives:"))
		for i, opt := range section.Options {
			prefix := fmt.Sprintf("[%d] ", i+1)
			lines = append(lines, optStyle.Render(prefix+opt))
		}
	}

	return strings.Join(lines, "\n")
}

func (v *SprintView) renderConflicts() string {
	conflictStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorError).
		Padding(0, 1).
		Width(min(v.width-4, 76))

	var lines []string
	for _, c := range v.state.Conflicts {
		icon := "🟡"
		if c.Severity == arbiter.SeverityBlocker {
			icon = "🔴"
		}
		lines = append(lines, fmt.Sprintf("%s %s", icon, c.Message))
	}

	return conflictStyle.Render(strings.Join(lines, "\n"))
}

func (v *SprintView) renderHelp() string {
	helpStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	return helpStyle.Render("a accept  e edit  r research  1-3 alternatives  j/k navigate  q quit")
}

func (v *SprintView) renderResearchPanel() string {
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorSecondary).
		Padding(0, 1).
		Width(min(v.width-4, 76))

	titleStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSecondary).
		Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render("Research Findings"))

	// Deep scan status
	switch v.state.DeepScan.Status {
	case arbiter.DeepScanRunning:
		lines = append(lines, "🔄 Deep scan in progress...")
	case arbiter.DeepScanComplete:
		lines = append(lines, "✅ Deep scan complete")
	case arbiter.DeepScanFailed:
		lines = append(lines, fmt.Sprintf("❌ Deep scan failed: %s", v.state.DeepScan.Error))
	}

	// Quick scan results (GitHub + HN)
	if v.state.ResearchCtx != nil {
		ctx := v.state.ResearchCtx
		if len(ctx.GitHubHits) > 0 {
			lines = append(lines, titleStyle.Render("GitHub"))
			for _, gh := range ctx.GitHubHits {
				lines = append(lines, fmt.Sprintf("  ★ %d  %s — %s", gh.Stars, gh.Name, gh.Description))
			}
		}
		if len(ctx.HNHits) > 0 {
			lines = append(lines, titleStyle.Render("HackerNews"))
			for _, hn := range ctx.HNHits {
				lines = append(lines, fmt.Sprintf("  ▲ %d  %s — %s", hn.Points, hn.Title, hn.Theme))
			}
		}
		if ctx.Summary != "" {
			lines = append(lines, "")
			lines = append(lines, ctx.Summary)
		}
	}

	// Intermute findings
	if len(v.state.Findings) > 0 {
		lines = append(lines, titleStyle.Render("Insights"))
		for i, f := range v.state.Findings {
			if i >= 8 {
				mutedStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
				lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ... and %d more", len(v.state.Findings)-8)))
				break
			}
			relevance := fmt.Sprintf("%.0f%%", f.Relevance*100)
			tagStr := ""
			if len(f.Tags) > 0 {
				tagStr = " [" + strings.Join(f.Tags, ", ") + "]"
			}
			lines = append(lines, fmt.Sprintf("  %s %s (%s)%s", f.SourceType, f.Title, relevance, tagStr))
		}
	}

	if v.state.ResearchCtx == nil && len(v.state.Findings) == 0 {
		mutedStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
		lines = append(lines, mutedStyle.Render("No research findings yet"))
	}

	return panelStyle.Render(strings.Join(lines, "\n"))
}

func statusIcon(status arbiter.DraftStatus) string {
	switch status {
	case arbiter.DraftProposed:
		return "📝"
	case arbiter.DraftAccepted:
		return "✅"
	case arbiter.DraftNeedsRevision:
		return "✏️"
	default:
		return "⏳"
	}
}
