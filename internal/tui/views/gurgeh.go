package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// gurgehViewAgentSelectorSetter is a local interface for views that accept an agent selector.
type gurgehViewAgentSelectorSetter interface {
	SetAgentSelector(*pkgtui.AgentSelector)
}

// GurgehView displays specs (PRDs) with the unified shell layout.
// It also acts as a container: when onboarding config is provided, it delegates
// to GurgehOnboardingView until onboarding completes, then shows the spec browser.
type GurgehView struct {
	client        *autarch.Client
	specs         []autarch.Spec
	selected      int
	pendingSpecID string
	width         int
	height        int
	loading       bool
	err           error

	// Shell layout for unified 3-pane layout
	shell *pkgtui.ShellLayout
	// Chat panel for interactive input
	chatPanel *pkgtui.ChatPanel

	// Onboarding sub-view (nil when no config provided, or after onboarding completes)
	onboarding *GurgehOnboardingView
	// If true, show spec browser; else show onboarding
	showBrowser bool
}

// NewGurgehView creates a new Gurgeh view.
// If cfg is non-nil, onboarding is shown first; otherwise the spec browser is shown immediately.
func NewGurgehView(client *autarch.Client, cfg *tui.GurgehConfig) *GurgehView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Ask questions about this spec...")
	chatPanel.SetComposerHint("enter send  tab focus  ctrl+b sidebar")

	v := &GurgehView{
		client:    client,
		shell:     pkgtui.NewShellLayout(),
		chatPanel: chatPanel,
	}
	if cfg != nil {
		cfg.Client = client
		v.onboarding = NewGurgehOnboardingView(*cfg)
	} else {
		v.showBrowser = true // No config = skip onboarding
	}
	return v
}

// SetAgentSelector sets the shared agent selector.
func (v *GurgehView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
	if v.onboarding != nil {
		v.onboarding.SetAgentSelector(selector)
	}
}

// SetAgentName sets the selected agent name on the onboarding sub-view.
func (v *GurgehView) SetAgentName(name string) {
	if v.onboarding != nil {
		v.onboarding.SetAgentName(name)
	}
}

// SetChatSettings sets chat settings on the onboarding sub-view and browser chat panel.
func (v *GurgehView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatPanel.SetSettings(settings)
	if v.onboarding != nil {
		v.onboarding.SetChatSettings(settings)
	}
}

// ClearInput clears the chat composer (for ctrl+c soft cancel).
func (v *GurgehView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*GurgehView)(nil)

type specsLoadedMsg struct {
	specs []autarch.Spec
	err   error
}

// Init implements View
func (v *GurgehView) Init() tea.Cmd {
	if !v.showBrowser && v.onboarding != nil {
		return v.onboarding.Init()
	}
	return v.loadSpecs()
}

func (v *GurgehView) loadSpecs() tea.Cmd {
	return func() tea.Msg {
		specs, err := v.client.ListSpecs("")
		return specsLoadedMsg{specs: specs, err: err}
	}
}

// Update implements View
func (v *GurgehView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	// --- Onboarding delegation ---
	if !v.showBrowser && v.onboarding != nil {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			// Pass to BOTH onboarding and spec browser so browser is sized when we switch
			v.width = msg.Width - 6
			v.height = msg.Height - 4 - 2
			v.shell.SetSize(v.width, v.height)
			_, cmd := v.onboarding.Update(msg)
			return v, cmd

		case tui.OnboardingCompleteMsg:
			// Onboarding finished — switch to spec browser
			v.showBrowser = true
			v.pendingSpecID = msg.SpecID
			// Return the message so UnifiedApp can call enterDashboard
			return v, tea.Batch(
				v.loadSpecs(),
				func() tea.Msg { return msg },
			)
		}

		// Default pass-through: all other messages go to onboarding.
		// CRITICAL: This ensures SprintConflictMsg, SprintStreamLineMsg, etc.
		// reach the SprintView inside onboarding.
		_, cmd := v.onboarding.Update(msg)
		return v, cmd
	}

	// --- Spec browser mode ---
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.shell.SetSize(v.width, v.height)
		v.chatPanel.SetSize(v.shell.RightWidth(), v.shell.Height())
		return v, nil

	case specsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.specs = msg.specs
			if v.pendingSpecID != "" {
				for i, s := range v.specs {
					if s.ID == v.pendingSpecID {
						v.selected = i
						break
					}
				}
				v.pendingSpecID = ""
			}
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find spec by ID and select it
		for i, s := range v.specs {
			if s.ID == msg.ItemID {
				v.selected = i
				break
			}
		}
		return v, nil

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
				if v.selected < len(v.specs)-1 {
					v.selected++
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selected > 0 {
					v.selected--
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadSpecs()
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
func (v *GurgehView) View() string {
	if !v.showBrowser && v.onboarding != nil {
		return v.onboarding.View()
	}

	if v.loading {
		return pkgtui.LabelStyle.Render("Loading specs...")
	}

	if v.err != nil {
		return tui.ErrorView(v.err)
	}

	// Render using shell layout
	sidebarItems := v.SidebarItems()
	document := v.renderDocument()
	chat := v.chatPanel.View()

	return v.shell.Render(sidebarItems, document, chat)
}

// SidebarItems implements SidebarProvider.
func (v *GurgehView) SidebarItems() []pkgtui.SidebarItem {
	if len(v.specs) == 0 {
		return nil
	}

	items := make([]pkgtui.SidebarItem, len(v.specs))
	for i, s := range v.specs {
		title := s.Title
		if title == "" && len(s.ID) >= 8 {
			title = s.ID[:8]
		}

		items[i] = pkgtui.SidebarItem{
			ID:    s.ID,
			Label: title,
			Icon:  statusIcon(s.Status),
		}
	}
	return items
}

// statusIcon returns an icon for the spec status.
func statusIcon(status autarch.SpecStatus) string {
	switch status {
	case autarch.SpecStatusDraft:
		return "◐"
	case autarch.SpecStatusResearch:
		return "◑"
	case autarch.SpecStatusValidated:
		return "✓"
	case autarch.SpecStatusArchived:
		return "○"
	default:
		return "•"
	}
}

// renderDocument renders the main document pane (spec details).
func (v *GurgehView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Spec Details"))
	lines = append(lines, "")

	if len(v.specs) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No specs found"))
		lines = append(lines, "")
		lines = append(lines, pkgtui.LabelStyle.Render("Use the command palette (ctrl+p) to create a new spec."))
		return strings.Join(lines, "\n")
	}

	if v.selected >= len(v.specs) {
		lines = append(lines, pkgtui.LabelStyle.Render("No spec selected"))
		return strings.Join(lines, "\n")
	}

	s := v.specs[v.selected]

	lines = append(lines, fmt.Sprintf("Title: %s", s.Title))
	lines = append(lines, fmt.Sprintf("Status: %s", s.Status))
	lines = append(lines, fmt.Sprintf("Project: %s", s.Project))
	lines = append(lines, "")

	if s.Vision != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Vision"))
		lines = append(lines, s.Vision)
		lines = append(lines, "")
	}

	if s.Problem != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Problem"))
		lines = append(lines, s.Problem)
		lines = append(lines, "")
	}

	if s.Users != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Users"))
		lines = append(lines, s.Users)
	}

	return strings.Join(lines, "\n")
}

// Focus implements View
func (v *GurgehView) Focus() tea.Cmd {
	if !v.showBrowser && v.onboarding != nil {
		return v.onboarding.Focus()
	}
	v.shell.SetFocus(pkgtui.FocusChat)
	return tea.Batch(v.chatPanel.Focus(), v.loadSpecs())
}

// Blur implements View
func (v *GurgehView) Blur() {
	v.chatPanel.Blur()
	if v.onboarding != nil {
		v.onboarding.Blur()
	}
}

// Name implements View
func (v *GurgehView) Name() string {
	return "Gurgeh"
}

// ShortHelp implements View
func (v *GurgehView) ShortHelp() string {
	if !v.showBrowser && v.onboarding != nil {
		return v.onboarding.ShortHelp()
	}
	return "↑/↓ navigate  ctrl+r refresh  ctrl+g model  tab focus  ctrl+b sidebar"
}

// Commands implements CommandProvider
func (v *GurgehView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Spec",
			Description: "Create a new specification",
			Action: func() tea.Cmd {
				// TODO: implement
				return nil
			},
		},
		{
			Name:        "Refresh Specs",
			Description: "Reload spec list",
			Action: func() tea.Cmd {
				return v.loadSpecs()
			},
		},
	}
}
