package views

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// SprintView provides a chat-driven spec flow through all 8 PRD phases.
// It owns the Orchestrator directly — no intermediate controller layer.
type SprintView struct {
	orch *arbiter.Orchestrator

	// Layout
	chatPanel *pkgtui.ChatPanel
	docPanel  *SprintDocPanel
	sidebar   *PhaseSidebar
	shell     *pkgtui.ShellLayout

	// Dimensions
	width  int
	height int

	// Streaming
	responseCh <-chan string
	cancelChat context.CancelFunc

	// Agent selector
	agentSelector *pkgtui.AgentSelector

	// Callbacks
	onBack func() tea.Cmd

	keys pkgtui.CommonKeys
}

// NewSprintView creates a new sprint view. Call StartSprint or StartSprintWithScan
// to begin.
func NewSprintView(projectPath string) *SprintView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Chat about the current phase...")
	chatPanel.SetComposerHint("enter send  a accept  e revise  d details  esc back")

	v := &SprintView{
		orch:      arbiter.NewOrchestrator(projectPath),
		chatPanel: chatPanel,
		docPanel:  NewSprintDocPanel(),
		sidebar:   NewPhaseSidebar(),
		shell:     pkgtui.NewShellLayout(),
		keys:      pkgtui.NewCommonKeys(),
	}
	return v
}

// SetCallbacks sets navigation callbacks.
func (v *SprintView) SetCallbacks(onBack func() tea.Cmd) {
	v.onBack = onBack
}

// SetAgentSelector sets the shared agent selector.
func (v *SprintView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// StartSprint starts a new sprint and returns the initial command.
func (v *SprintView) StartSprint(userInput string) tea.Cmd {
	return func() tea.Msg {
		_, err := v.orch.Start(context.Background(), userInput)
		if err != nil {
			return tui.GenerationErrorMsg{What: "sprint", Error: err}
		}
		state := v.orch.State()
		return tui.SprintDraftUpdatedMsg{
			Phase:   state.Phase.String(),
			Content: state.Sections[state.Phase].Content,
		}
	}
}

// StartSprintWithScan starts a sprint seeded with scan artifacts.
func (v *SprintView) StartSprintWithScan(userInput string, artifacts *scan.Artifacts) tea.Cmd {
	return func() tea.Msg {
		_, err := v.orch.StartWithScan(context.Background(), userInput, artifacts)
		if err != nil {
			return tui.GenerationErrorMsg{What: "sprint", Error: err}
		}
		state := v.orch.State()
		return tui.SprintDraftUpdatedMsg{
			Phase:   state.Phase.String(),
			Content: state.Sections[state.Phase].Content,
		}
	}
}

// Init implements View.
func (v *SprintView) Init() tea.Cmd {
	return v.chatPanel.Focus()
}

// Update implements View.
func (v *SprintView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.shell.SetSize(v.width, v.height)
		split := v.shell.SplitLayout()
		v.docPanel.SetSize(split.LeftWidth(), split.LeftHeight())
		v.chatPanel.SetSize(split.RightWidth(), split.RightHeight())
		return v, nil

	case tui.SprintDraftUpdatedMsg:
		v.chatPanel.AddMessage("system", fmt.Sprintf("Draft for %s is ready. Review in the left panel.", msg.Phase))
		v.chatPanel.AddMessage("system", "Type feedback, press 'a' to accept, or 'e' to revise.")
		v.syncDocPanel()
		return v, nil

	case tui.SprintPhaseAdvancedMsg:
		v.chatPanel.AddMessage("system", fmt.Sprintf("Advanced to %s phase.", msg.Phase))
		v.syncDocPanel()
		return v, nil

	case tui.SprintConflictMsg:
		for _, m := range msg.Messages {
			v.chatPanel.AddMessage("system", "⚠ Conflict: "+m)
		}
		v.syncDocPanel()
		return v, nil

	case tui.SprintStreamLineMsg:
		v.chatPanel.AddMessage("agent", msg.Content)
		v.syncDocPanel()
		if v.responseCh != nil {
			return v, v.waitForResponse()
		}
		return v, nil

	case tui.SprintStreamDoneMsg:
		v.responseCh = nil
		v.syncDocPanel()
		return v, nil

	case tui.GenerationErrorMsg:
		v.chatPanel.AddMessage("system", "Error: "+msg.Error.Error())
		return v, nil

	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			if v.chatPanel.Focused() {
				v.chatPanel.ScrollUp()
			} else {
				v.docPanel.ScrollUp()
			}
			return v, nil
		case tea.MouseWheelDown:
			if v.chatPanel.Focused() {
				v.chatPanel.ScrollDown()
			} else {
				v.docPanel.ScrollDown()
			}
			return v, nil
		}

	case tea.KeyMsg:
		// Agent selector first
		if v.agentSelector != nil {
			selectorMsg, selectorCmd := v.agentSelector.Update(msg)
			if selectorMsg != nil {
				return v, tea.Batch(selectorCmd, func() tea.Msg { return selectorMsg })
			}
			if v.agentSelector.Open || msg.Type == tea.KeyF2 {
				return v, selectorCmd
			}
		}

		// Shell handles Tab/Ctrl+B
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		// View-specific keys
		if v.chatPanel.Focused() {
			switch {
			case msg.Type == tea.KeyEnter:
				return v, v.handleChatSubmit()
			case msg.Type == tea.KeyEscape:
				if v.onBack != nil {
					v.cancelStreaming()
					return v, v.onBack()
				}
				return v, nil
			case msg.String() == "a" && v.chatPanel.Value() == "":
				return v, v.handleAccept()
			case msg.String() == "e" && v.chatPanel.Value() == "":
				v.chatPanel.SetValue("Edit: ")
				return v, nil
			case msg.String() == "d" && v.chatPanel.Value() == "":
				v.docPanel.ToggleDetails()
				v.syncDocPanel()
				return v, nil
			default:
				v.chatPanel, cmd = v.chatPanel.Update(msg)
				return v, cmd
			}
		}

		// Non-focused keys
		switch {
		case key.Matches(msg, v.keys.Back):
			if v.onBack != nil {
				v.cancelStreaming()
				return v, v.onBack()
			}
		}
	}

	return v, nil
}

// View implements View.
func (v *SprintView) View() string {
	state := v.orch.State()
	sidebarItems := v.sidebar.Items(state)
	return v.shell.Render(sidebarItems, v.docPanel.View(), v.chatPanel.View())
}

// Focus implements View.
func (v *SprintView) Focus() tea.Cmd {
	return v.chatPanel.Focus()
}

// Blur implements View.
func (v *SprintView) Blur() {
	v.cancelStreaming()
	v.chatPanel.Blur()
}

// Name implements View.
func (v *SprintView) Name() string {
	return "Sprint"
}

// ShortHelp implements View.
func (v *SprintView) ShortHelp() string {
	return "enter send  a accept  e revise  d details  F2 model  esc back"
}

// SidebarItems returns the current phase sidebar items.
func (v *SprintView) SidebarItems() []pkgtui.SidebarItem {
	return v.sidebar.Items(v.orch.State())
}

// --- internal helpers ---

func (v *SprintView) syncDocPanel() {
	v.docPanel.Update(v.orch.State())
}

func (v *SprintView) cancelStreaming() {
	if v.cancelChat != nil {
		v.cancelChat()
		v.cancelChat = nil
	}
	v.responseCh = nil
}

func (v *SprintView) handleChatSubmit() tea.Cmd {
	msg := v.chatPanel.Value()
	if msg == "" {
		return nil
	}
	v.chatPanel.ClearComposer()
	v.chatPanel.AddMessage("user", msg)

	// Cancel any in-progress streaming
	v.cancelStreaming()

	ctx, cancel := context.WithCancel(context.Background())
	v.cancelChat = cancel
	v.responseCh = v.orch.ProcessChatMessage(ctx, msg)

	return v.waitForResponse()
}

func (v *SprintView) waitForResponse() tea.Cmd {
	ch := v.responseCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return tui.SprintStreamDoneMsg{}
		}
		return tui.SprintStreamLineMsg{Content: line}
	}
}

func (v *SprintView) handleAccept() tea.Cmd {
	v.chatPanel.AddMessage("user", "Accept draft")
	return func() tea.Msg {
		err := v.orch.ChatAcceptDraft(context.Background())
		if err != nil {
			if arbiter.IsBlockerError(err) {
				return tui.SprintConflictMsg{
					Phase:    v.orch.State().Phase.String(),
					Messages: []string{err.Error()},
				}
			}
			return tui.GenerationErrorMsg{What: "accept", Error: err}
		}
		state := v.orch.State()
		return tui.SprintPhaseAdvancedMsg{Phase: state.Phase.String()}
	}
}
