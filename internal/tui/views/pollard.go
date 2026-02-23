package views

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// PollardView displays research insights with the unified shell layout.
type PollardView struct {
	client      *autarch.Client
	coordinator *research.Coordinator
	insights    []autarch.Insight
	selected    int
	width       int
	height      int
	loading     bool
	err         error

	// Progressive reveal state
	currentRunID   string
	hunterStatuses map[string]research.HunterStatus
	runActive      bool

	// Shell layout for unified 3-pane layout
	shell *pkgtui.ShellLayout
	// Chat panel for interactive input
	chatPanel *pkgtui.ChatPanel
	// Chat handler for Pollard-specific context
	chatHandler *PollardChatHandler
}

// NewPollardView creates a new Pollard view.
// Pass nil for coordinator if research integration is not needed.
func NewPollardView(client *autarch.Client, coordinator *research.Coordinator) *PollardView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Ask questions about this insight...")
	chatPanel.SetComposerHint("enter send  tab focus  ctrl+b sidebar")
	chatHandler := NewPollardChatHandler()
	chatPanel.SetHandler(chatHandler)

	return &PollardView{
		client:      client,
		coordinator: coordinator,
		shell:       pkgtui.NewShellLayout(),
		chatPanel:   chatPanel,
		chatHandler: chatHandler,
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *PollardView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
}

// SetAgentName sets the selected agent name (satisfies interface).
func (v *PollardView) SetAgentName(name string) {}

// SetChatSettings sets chat settings on the chat panel.
func (v *PollardView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatPanel.SetSettings(settings)
}

// ClearInput clears the chat composer (for ctrl+c soft cancel).
func (v *PollardView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*PollardView)(nil)

type insightsLoadedMsg struct {
	insights []autarch.Insight
	err      error
}

// Init implements View
func (v *PollardView) Init() tea.Cmd {
	return v.loadInsights()
}

func (v *PollardView) loadInsights() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return func() tea.Msg {
		insights, err := v.client.ListInsights("", "")
		return insightsLoadedMsg{insights: insights, err: err}
	}
}

// Update implements View
func (v *PollardView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		v.chatPanel, cmd = v.chatPanel.Update(msg)
		if cmd != nil {
			return v, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.shell.SetSize(v.width, v.height)
		v.chatPanel.SetSize(v.shell.RightWidth(), v.shell.Height())
		return v, nil

	case insightsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.insights = msg.insights
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find insight by ID and select it
		for i, insight := range v.insights {
			if insight.ID == msg.ItemID {
				v.selected = i
				break
			}
		}
		return v, nil

	// Progressive research reveal messages
	case research.RunStartedMsg:
		v.currentRunID = msg.RunID
		v.runActive = true
		v.hunterStatuses = make(map[string]research.HunterStatus)
		for _, name := range msg.Hunters {
			v.hunterStatuses[name] = research.HunterStatus{
				Name:   name,
				Status: research.StatusPending,
			}
		}
		return v, nil

	case research.HunterStartedMsg:
		if msg.RunID != v.currentRunID {
			return v, nil
		}
		if hs, ok := v.hunterStatuses[msg.HunterName]; ok {
			hs.Status = research.StatusRunning
			hs.StartedAt = time.Now()
			v.hunterStatuses[msg.HunterName] = hs
		}
		return v, nil

	case research.HunterUpdateMsg:
		if msg.RunID != v.currentRunID {
			return v, nil
		}
		for _, f := range msg.Findings {
			v.addFinding(f)
		}
		return v, nil

	case research.HunterCompletedMsg:
		if msg.RunID != v.currentRunID {
			return v, nil
		}
		if hs, ok := v.hunterStatuses[msg.HunterName]; ok {
			hs.Status = research.StatusComplete
			hs.FinishedAt = time.Now()
			hs.Findings = msg.FindingCount
			v.hunterStatuses[msg.HunterName] = hs
		}
		return v, nil

	case research.HunterErrorMsg:
		if msg.RunID != v.currentRunID {
			return v, nil
		}
		if hs, ok := v.hunterStatuses[msg.HunterName]; ok {
			hs.Status = research.StatusError
			hs.FinishedAt = time.Now()
			hs.Error = msg.Error.Error()
			v.hunterStatuses[msg.HunterName] = hs
		}
		return v, nil

	case research.RunCompletedMsg:
		if msg.RunID != v.currentRunID {
			return v, nil
		}
		v.runActive = false
		return v, v.loadInsights()

	case tea.KeyMsg:
		// Let shell handle global keys first (Tab, Shift-Tab, Ctrl+B)
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		// Handle view-specific keys based on focus
		switch v.shell.Focus() {
		case pkgtui.FocusSidebar:
			// Navigation handled by shell/sidebar
		case pkgtui.FocusDocument:
			switch {
			case key.Matches(msg, commonKeys.NavDown):
				if v.selected < len(v.insights)-1 {
					v.selected++
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selected > 0 {
					v.selected--
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadInsights()
			}
		case pkgtui.FocusChat:
			if msg.Type == tea.KeyEnter {
				if slashCmd := v.chatPanel.SubmitInput(); slashCmd != nil {
					return v, slashCmd
				}
				return v, nil
			}
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			return v, cmd
		}
	}

	return v, nil
}

// View implements View
func (v *PollardView) View() string {
	if v.loading && !v.runActive {
		return pkgtui.LabelStyle.Render("Loading insights...")
	}

	if v.err != nil && !v.runActive {
		return tui.ErrorView(v.err)
	}

	sidebarItems := v.SidebarItems()
	document := v.renderDocument()
	chat := v.chatPanel.View()

	return v.shell.Render(sidebarItems, document, chat)
}

// SidebarItems implements SidebarProvider.
func (v *PollardView) SidebarItems() []pkgtui.SidebarItem {
	var items []pkgtui.SidebarItem

	// Show hunter status during active run (sorted for deterministic rendering)
	if v.runActive && len(v.hunterStatuses) > 0 {
		names := make([]string, 0, len(v.hunterStatuses))
		for name := range v.hunterStatuses {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			status := v.hunterStatuses[name]
			icon := hunterStatusIcon(status.Status)
			label := fmt.Sprintf("%s %s", icon, name)
			if status.Findings > 0 {
				label += fmt.Sprintf(" (%d)", status.Findings)
			}
			items = append(items, pkgtui.SidebarItem{
				ID:    "hunter:" + name,
				Label: label,
				Icon:  icon,
			})
		}
	}

	// Append insight items
	for _, insight := range v.insights {
		title := insight.Title
		if title == "" && len(insight.ID) >= 8 {
			title = insight.ID[:8]
		}
		items = append(items, pkgtui.SidebarItem{
			ID:    insight.ID,
			Label: title,
			Icon:  categoryIcon(insight.Category),
		})
	}

	return items
}

// categoryIcon returns an icon for the insight category.
func categoryIcon(category string) string {
	switch category {
	case "competitor":
		return "⚔"
	case "technology":
		return "⚙"
	case "market":
		return "📊"
	case "user":
		return "👤"
	default:
		return "•"
	}
}

// renderDocument renders the main document pane (insight details).
func (v *PollardView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	var lines []string

	// Run status header
	if v.runActive {
		running := 0
		complete := 0
		for _, hs := range v.hunterStatuses {
			switch hs.Status {
			case research.StatusRunning:
				running++
			case research.StatusComplete:
				complete++
			}
		}
		statusLine := fmt.Sprintf("Research: %d/%d hunters complete",
			complete, len(v.hunterStatuses))
		lines = append(lines, pkgtui.SubtitleStyle.Render(statusLine))
		lines = append(lines, "")
	}

	lines = append(lines, pkgtui.TitleStyle.Render("Insight Details"))
	lines = append(lines, "")

	if len(v.insights) == 0 {
		if v.runActive {
			lines = append(lines, pkgtui.LabelStyle.Render("Waiting for results..."))
		} else {
			lines = append(lines, pkgtui.LabelStyle.Render("No insights found"))
			lines = append(lines, "")
			lines = append(lines, pkgtui.LabelStyle.Render("Run Pollard hunters to gather research insights."))
		}
		return strings.Join(lines, "\n")
	}

	if v.selected >= len(v.insights) {
		lines = append(lines, pkgtui.LabelStyle.Render("No insight selected"))
		return strings.Join(lines, "\n")
	}

	i := v.insights[v.selected]

	lines = append(lines, fmt.Sprintf("Title: %s", i.Title))
	lines = append(lines, fmt.Sprintf("Category: %s  Source: %s", i.Category, i.Source))
	lines = append(lines, fmt.Sprintf("Score: %.2f", i.Score))
	lines = append(lines, "")

	if i.Body != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Summary"))
		wrapped := wordWrap(i.Body, width-4)
		lines = append(lines, wrapped...)
		lines = append(lines, "")
	}

	if i.URL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", i.URL))
	}

	if i.SpecID != "" {
		lines = append(lines, fmt.Sprintf("Linked Spec: %s", i.SpecID))
	}

	return strings.Join(lines, "\n")
}

func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	words := strings.Fields(text)
	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > width {
			if current.Len() > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return lines
}

// addFinding converts a research.Finding to an autarch.Insight and inserts
// it into v.insights sorted by score descending.
func (v *PollardView) addFinding(f research.Finding) {
	insight := autarch.Insight{
		ID:       f.ID,
		Title:    f.Title,
		Body:     f.Summary,
		Source:   f.Source,
		Category: f.SourceType,
		Score:    f.Relevance,
	}
	// Insert sorted by score descending
	idx := sort.Search(len(v.insights), func(i int) bool {
		return v.insights[i].Score < insight.Score
	})
	v.insights = append(v.insights, autarch.Insight{})
	copy(v.insights[idx+1:], v.insights[idx:])
	v.insights[idx] = insight
}

func hunterStatusIcon(s research.Status) string {
	switch s {
	case research.StatusRunning:
		return "↻"
	case research.StatusComplete:
		return "✓"
	case research.StatusError:
		return "✗"
	case research.StatusPending:
		return "○"
	default:
		return "?"
	}
}

// Focus implements View
func (v *PollardView) Focus() tea.Cmd {
	v.shell.SetFocus(pkgtui.FocusChat)
	return tea.Batch(v.chatPanel.Focus(), v.loadInsights())
}

// Blur implements View
func (v *PollardView) Blur() {
	v.chatPanel.CancelStream()
	v.chatPanel.Blur()
}

// Name implements View
func (v *PollardView) Name() string {
	return "Pollard"
}

// ShortHelp implements View
func (v *PollardView) ShortHelp() string {
	help := "↑/↓ navigate  ctrl+r refresh  ctrl+g model  tab focus  ctrl+b sidebar"
	if v.runActive {
		help = "↻ research active  " + help
	}
	return help
}

// Commands implements CommandProvider
func (v *PollardView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "Run Research",
			Description: "Execute Pollard hunters",
			Action: func() tea.Cmd {
				if v.coordinator == nil {
					return nil
				}
				return func() tea.Msg {
					hunterNames := []string{"competitor-tracker", "hackernews-trendwatcher", "github-scout"}
					_, err := v.coordinator.StartRun(
						context.Background(),
						"default",
						hunterNames,
						nil,
					)
					if err != nil {
						return insightsLoadedMsg{err: err}
					}
					return nil
				}
			},
		},
		{
			Name:        "Link Insight",
			Description: "Link insight to a spec",
			Action: func() tea.Cmd {
				return nil
			},
		},
	}
}
