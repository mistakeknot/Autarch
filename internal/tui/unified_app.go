package tui

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// UnifiedApp is the main application shell that provides tabs, palette, log pane,
// and routes messages to the active dashboard view.
type UnifiedApp struct {
	client      *autarch.Client
	currentView View

	// Agent for AI generation
	codingAgent   *agent.Agent
	agentSelector *pkgtui.AgentSelector
	selectedAgent string

	// Dashboard state
	tabs      *TabBar
	dashViews []View
	palette   *Palette

	// UI state
	width       int
	height      int
	err         error
	showHelp    bool      // Help overlay visible
	lastCtrlC   time.Time // For double ctrl+c to quit
	keys        pkgtui.CommonKeys
	// Chat settings
	chatSettings     pkgtui.ChatSettings
	chatSettingsOpen bool
	chatSettingsView *pkgtui.ChatSettingsPanel

	// Log pane (always created, toggled with Ctrl+L)
	logPane          *pkgtui.LogPane
	logPaneVisible   bool // toggled by Ctrl+L or /logs
	logPaneAutoShown bool // true when auto-shown by scan (for auto-hide)

	// Initial tab to jump to when entering dashboard
	initialTab string

	// Skip onboarding and go directly to dashboard
	skipOnboarding bool

	// View factories (injected from main.go)
	createDashboardViews func(*autarch.Client) []View
}

// NewUnifiedApp creates a new unified application
func NewUnifiedApp(client *autarch.Client) *UnifiedApp {
	tabNames := []string{"Bigend", "Gurgeh", "Coldwine", "Pollard"}
	return &UnifiedApp{
		client:       client,
		tabs:         NewTabBar(tabNames),
		palette:      NewPalette(),
		logPane:      pkgtui.NewLogPane(),
		keys:         pkgtui.NewCommonKeys(),
		chatSettings: pkgtui.DefaultChatSettings(),
	}
}

// SetInlineMode enables inline mode (log pane visible by default).
func (a *UnifiedApp) SetInlineMode(enabled bool) {
	if enabled {
		a.logPaneVisible = true
	}
}

// LogPane returns the log pane.
func (a *UnifiedApp) LogPane() *pkgtui.LogPane {
	return a.logPane
}

// SetInitialTab sets the tab to jump to when entering dashboard mode.
// Valid names: bigend, gurgeh, coldwine, pollard (case-insensitive).
func (a *UnifiedApp) SetInitialTab(name string) {
	if name == "" {
		return
	}
	// Store for later - we'll apply it when dashViews are created
	a.initialTab = strings.ToLower(name)
}

// SetSkipOnboarding skips onboarding and enters dashboard mode immediately.
func (a *UnifiedApp) SetSkipOnboarding(skip bool) {
	a.skipOnboarding = skip
}

// SetDashboardViewFactory sets the factory for creating dashboard tab views.
func (a *UnifiedApp) SetDashboardViewFactory(factory func(*autarch.Client) []View) {
	a.createDashboardViews = factory
}

type agentSelectorSetter interface {
	SetAgentSelector(*pkgtui.AgentSelector)
}

type agentNameSetter interface {
	SetAgentName(string)
}

type chatSettingsSetter interface {
	SetChatSettings(pkgtui.ChatSettings)
}

type inputClearer interface {
	ClearInput()
}

type slashCommandHandler interface {
	HandleSlashCommand(command string, args []string) tea.Cmd
}

func (a *UnifiedApp) initAgentSelector() {
	if a.agentSelector != nil {
		return
	}

	projectRoot := ""
	if cwd, err := os.Getwd(); err == nil {
		projectRoot = cwd
	}

	options, err := LoadAgentOptions(projectRoot)
	if err != nil {
		return
	}

	options = filterSupportedAgentOptions(options)
	if len(options) == 0 {
		return
	}

	a.agentSelector = pkgtui.NewAgentSelector(options)
	if a.selectedAgent != "" {
		a.setSelectorIndex(a.selectedAgent)
		return
	}
	if a.codingAgent != nil {
		a.selectedAgent = string(a.codingAgent.Type)
		a.setSelectorIndex(a.selectedAgent)
		return
	}

	a.selectedAgent = options[0].Name
	if resolved, err := agent.DetectAgentByName(a.selectedAgent, exec.LookPath); err == nil {
		a.codingAgent = resolved
	}
}

func (a *UnifiedApp) setSelectorIndex(name string) {
	if a.agentSelector == nil {
		return
	}
	for i, opt := range a.agentSelector.Options {
		if strings.EqualFold(opt.Name, name) {
			a.agentSelector.Index = i
			return
		}
	}
}

func (a *UnifiedApp) attachAgentSelector(view View) {
	if view == nil {
		return
	}
	if a.agentSelector != nil {
		if setter, ok := view.(agentSelectorSetter); ok {
			setter.SetAgentSelector(a.agentSelector)
		}
	}
	a.attachAgentName(view)
	a.attachChatSettings(view)
}

func (a *UnifiedApp) attachAgentName(view View) {
	if view == nil || a.selectedAgent == "" {
		return
	}
	if setter, ok := view.(agentNameSetter); ok {
		setter.SetAgentName(a.selectedAgent)
	}
}

func (a *UnifiedApp) attachChatSettings(view View) {
	if view == nil {
		return
	}
	if setter, ok := view.(chatSettingsSetter); ok {
		setter.SetChatSettings(a.chatSettings)
	}
}

func filterSupportedAgentOptions(options []pkgtui.AgentOption) []pkgtui.AgentOption {
	if len(options) == 0 {
		return options
	}
	out := make([]pkgtui.AgentOption, 0, len(options))
	for _, opt := range options {
		switch strings.ToLower(opt.Name) {
		case "codex", "claude":
			out = append(out, opt)
		}
	}
	return out
}

// Init implements tea.Model
func (a *UnifiedApp) Init() tea.Cmd {
	if settings, err := LoadChatSettings(); err == nil {
		a.chatSettings = settings
	}
	a.chatSettingsView = pkgtui.NewChatSettingsPanel(a.chatSettings)

	// Detect coding agent
	detectedAgent, err := agent.DetectAgent()
	if err == nil {
		a.codingAgent = detectedAgent
		a.selectedAgent = string(detectedAgent.Type)
	}
	// Note: We don't error here - we'll handle missing agent when we need it
	a.initAgentSelector()

	// Populate palette with tab-switching commands (available in all modes)
	a.initPaletteCommands()

	// Always enter dashboard. If not skipping onboarding, the Gurgeh tab's
	// GurgehView.Init() will start the onboarding flow internally.
	return a.enterDashboard()
}

// Update implements tea.Model
func (a *UnifiedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Route log batches to pane (always active, accumulates regardless of visibility)
	if batch, ok := msg.(pkgtui.LogBatchMsg); ok {
		cmd := a.logPane.Update(batch)
		return a, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.tabs.SetWidth(msg.Width)
		a.palette.SetSize(msg.Width, msg.Height)

		// Always size the log pane (so it's ready when toggled visible)
		logPaneHeight := 0
		a.logPane.SetSize(msg.Width, 10)
		if a.logPaneVisible {
			logPaneHeight = 10
		}

		// Pass reduced size to current view (account for header + footer + log pane)
		if a.currentView != nil {
			headerHeight := 3
			footerHeight := 3
			contentMsg := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: msg.Height - headerHeight - footerHeight - logPaneHeight,
			}
			var cmd tea.Cmd
			a.currentView, cmd = a.currentView.Update(contentMsg)
			return a, cmd
		}
		return a, nil

	case pkgtui.SlashCommandMsg:
		// Handle global slash commands from chat input
		switch msg.Command {
		case "help", "h":
			a.showHelp = true
			return a, nil
		case "quit", "exit", "q":
			return a, tea.Quit
		case "settings", "config":
			a.openChatSettings()
			return a, nil
		case "model", "m":
			// Toggle model selector
			if a.agentSelector != nil {
				a.agentSelector.Open = !a.agentSelector.Open
			}
			return a, nil
		case "palette", "p":
			return a, a.palette.Show()
		case "refresh", "r":
			// Send refresh to current view
			if a.currentView != nil {
				var cmd tea.Cmd
				a.currentView, cmd = a.currentView.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
				return a, cmd
			}
			return a, nil
		case "back", "b":
			// Send escape to go back
			if a.currentView != nil {
				var cmd tea.Cmd
				a.currentView, cmd = a.currentView.Update(tea.KeyMsg{Type: tea.KeyEsc})
				return a, cmd
			}
			return a, nil
		// Tool-switching slash commands
		case "bigend", "big":
			return a, a.switchToTab(0)
		case "gurgeh", "gur":
			return a, a.switchToTab(1)
		case "coldwine", "cold":
			return a, a.switchToTab(2)
		case "pollard", "pol":
			return a, a.switchToTab(3)
		case "signals", "sig":
			// Signals overlay (Phase 3) - no-op for now
			return a, nil
		case "logs", "log", "l":
			a.logPaneVisible = !a.logPaneVisible
			a.logPaneAutoShown = false
			return a, a.sendWindowSize()
		}
		// Pass unknown commands to view-specific handler
		if handler, ok := a.currentView.(slashCommandHandler); ok {
			return a, handler.HandleSlashCommand(msg.Command, msg.Args)
		}
		// Unknown command - ignore silently
		return a, nil

	case tea.KeyMsg:
		if key.Matches(msg, a.keys.Quit) {
			now := time.Now()
			// Double ctrl+c within 500ms quits
			if now.Sub(a.lastCtrlC) < 500*time.Millisecond {
				return a, tea.Quit
			}
			// First ctrl+c: clear input and record time
			a.lastCtrlC = now
			// Try to clear the current view's chat input
			if clearer, ok := a.currentView.(inputClearer); ok {
				clearer.ClearInput()
			}
			return a, nil
		}
		// Handle help overlay first
		if a.showHelp {
			switch {
			case key.Matches(msg, a.keys.Help), key.Matches(msg, a.keys.Back):
				a.showHelp = false
			}
			return a, nil
		}

		// Handle palette if visible
		if a.palette.Visible() {
			var cmd tea.Cmd
			a.palette, cmd = a.palette.Update(msg)
			return a, cmd
		}
		if a.chatSettingsOpen {
			if msg.String() == "esc" {
				a.chatSettingsOpen = false
				return a, nil
			}
			if a.chatSettingsView != nil {
				if a.chatSettingsView.Update(msg) {
					a.chatSettings = a.chatSettingsView.Settings
					_ = SaveChatSettings(a.chatSettings)
					a.attachChatSettings(a.currentView)
				}
			}
			return a, nil
		}

		if key.Matches(msg, a.keys.Help) {
			a.showHelp = true
			return a, nil
		}

		switch msg.String() {

		case "ctrl+p":
			return a, a.palette.Show()

		case "ctrl+,":
			a.openChatSettings()
			return a, nil
		case "ctrl+l":
			a.logPaneVisible = !a.logPaneVisible
			a.logPaneAutoShown = false
			return a, a.sendWindowSize()
		}

		// Handle tab switching (works in both modes)
		switch {
		case msg.String() == "ctrl+left" || msg.String() == "ctrl+pgup":
			return a, a.switchToTab((a.tabs.Active() - 1 + len(a.tabs.TabNames())) % len(a.tabs.TabNames()))
		case msg.String() == "ctrl+right" || msg.String() == "ctrl+pgdown":
			return a, a.switchToTab((a.tabs.Active() + 1) % len(a.tabs.TabNames()))
		}

		// Pass unhandled keys to current view
		if a.currentView != nil {
			var cmd tea.Cmd
			a.currentView, cmd = a.currentView.Update(msg)
			return a, cmd
		}

	case pkgtui.AgentSelectedMsg:
		a.selectedAgent = msg.Name
		a.setSelectorIndex(msg.Name)
		if resolved, err := agent.DetectAgentByName(msg.Name, exec.LookPath); err == nil {
			a.codingAgent = resolved
		}
		a.attachAgentName(a.currentView)
		return a, nil

	case OnboardingCompleteMsg:
		// Onboarding finished inside GurgehView — no-op since we're already in dashboard mode.
		// The GurgehView internally sets showBrowser=true.
		return a, nil

	case logPaneAutoHideMsg:
		if a.logPaneAutoShown {
			a.logPaneVisible = false
			a.logPaneAutoShown = false
		}
		return a, nil

	case LogPaneAutoShowMsg:
		if !a.logPaneVisible {
			a.logPaneVisible = true
			a.logPaneAutoShown = true
		}
		return a, a.sendWindowSize()

	case LogPaneScheduleAutoHideMsg:
		if a.logPaneAutoShown {
			return a, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return logPaneAutoHideMsg{}
			})
		}
		return a, nil
	}

	// Pass to current view
	if a.currentView != nil {
		var cmd tea.Cmd
		a.currentView, cmd = a.currentView.Update(msg)
		return a, cmd
	}

	return a, nil
}

// logPaneAutoHideMsg is sent after a timer to auto-hide the log pane.
type logPaneAutoHideMsg struct{}

func (a *UnifiedApp) blurCurrentView() {
	if a.currentView != nil {
		a.currentView.Blur()
	}
}

func (a *UnifiedApp) enterDashboard() tea.Cmd {
	a.blurCurrentView()

	// Create dashboard views
	if a.createDashboardViews != nil {
		a.dashViews = a.createDashboardViews(a.client)
		if len(a.dashViews) > 0 {
			for _, v := range a.dashViews {
				a.attachAgentSelector(v)
			}
			a.updateCommands()

			// Determine initial tab index
			initialIdx := 0
			if a.initialTab != "" {
				for i, v := range a.dashViews {
					if strings.ToLower(v.Name()) == a.initialTab {
						initialIdx = i
						break
					}
				}
			}
			a.tabs.SetActive(initialIdx)
			a.currentView = a.dashViews[initialIdx]

			// Initialize all views
			var cmds []tea.Cmd
			for _, v := range a.dashViews {
				cmds = append(cmds, v.Init())
			}
			cmds = append(cmds, a.currentView.Focus())
			cmds = append(cmds, a.sendWindowSize())
			return tea.Batch(cmds...)
		}
	}
	return nil
}

// initPaletteCommands sets up palette commands available in all modes (including onboarding).
func (a *UnifiedApp) initPaletteCommands() {
	var cmds []Command
	for i, name := range a.tabs.TabNames() {
		idx := i
		cmds = append(cmds, Command{
			Name:        "Switch to " + name,
			Description: fmt.Sprintf("View %s", strings.ToLower(name)),
			Action:      func() tea.Cmd { return a.switchToTab(idx) },
		})
	}
	cmds = append(cmds, Command{
		Name:        "Switch model",
		Description: "Toggle model selector",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyF2}
			}
		},
	})
	cmds = append(cmds, Command{
		Name:        "Chat settings",
		Description: "Configure chat panel",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}}
			}
		},
	})
	a.palette.SetCommands(cmds)
}

func (a *UnifiedApp) updateCommands() {
	var cmds []Command

	cmds = append(cmds, Command{
		Name:        "Switch model",
		Description: "Toggle model selector",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyF2}
			}
		},
	})
	cmds = append(cmds, Command{
		Name:        "Chat settings",
		Description: "Configure chat panel",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}}
			}
		},
	})
	for i, v := range a.dashViews {
		idx := i
		name := v.Name()
		desc := fmt.Sprintf("View %s", strings.ToLower(name))
		cmds = append(cmds, Command{
			Name:        "Switch to " + name,
			Description: desc,
			Action:      func() tea.Cmd { return a.switchToTab(idx) },
		})
		if provider, ok := v.(CommandProvider); ok {
			cmds = append(cmds, provider.Commands()...)
		}
	}

	a.palette.SetCommands(cmds)
}

func (a *UnifiedApp) openChatSettings() {
	a.chatSettingsOpen = true
	if a.chatSettingsView == nil {
		a.chatSettingsView = pkgtui.NewChatSettingsPanel(a.chatSettings)
	} else {
		a.chatSettingsView.Settings = a.chatSettings
	}
}

func (a *UnifiedApp) switchDashboardTab(idx int) tea.Cmd {
	if idx < 0 || idx >= len(a.dashViews) {
		return nil
	}

	oldActive := a.tabs.Active()
	if oldActive == idx {
		return nil
	}

	// Blur old view
	if oldActive < len(a.dashViews) {
		a.dashViews[oldActive].Blur()
	}

	a.tabs.SetActive(idx)
	a.currentView = a.dashViews[idx]
	return a.currentView.Focus()
}

// switchToTab switches the active dashboard tab.
func (a *UnifiedApp) switchToTab(idx int) tea.Cmd {
	return a.switchDashboardTab(idx)
}

func (a *UnifiedApp) sendWindowSize() tea.Cmd {
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: a.width, Height: a.height}
	}
}

// View implements tea.Model
func (a *UnifiedApp) View() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	// Calculate heights
	// Header: Padding(1,3) = 2 rows padding + 1 row tabs = 3
	headerHeight := 3
	footerHeight := 3 // Footer with padding
	logPaneHeight := 0
	if a.logPaneVisible {
		logPaneHeight = 10
	}
	contentHeight := a.height - headerHeight - footerHeight - logPaneHeight

	// Header area: tabs
	header := a.tabs.View()
	headerStyle := pkgtui.HeaderStyle.
		Width(a.width).
		Height(headerHeight)
	headerRendered := headerStyle.Render(header)

	// Content area
	var content string
	if a.currentView != nil {
		content = a.currentView.View()
	}

	// Apply content styling with padding (use ColorBgDark for uniform background)
	contentStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgDark).
		Foreground(pkgtui.ColorFg).
		Padding(1, 3).
		Width(a.width).
		Height(contentHeight)

	contentRendered := contentStyle.Render(content)

	// Log pane (visible when toggled or auto-shown)
	var logPaneRendered string
	if a.logPaneVisible {
		logPaneRendered = a.logPane.View()
	}

	// Footer
	footerStyle := pkgtui.FooterStyle.
		Width(a.width).
		Height(footerHeight)
	footerRendered := footerStyle.Render(a.renderFooterContent())

	// Join all sections vertically
	sections := []string{headerRendered, contentRendered}
	if logPaneRendered != "" {
		sections = append(sections, logPaneRendered)
	}
	sections = append(sections, footerRendered)
	result := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Overlay palette if visible
	if a.palette.Visible() {
		return a.overlay(result, a.palette.View())
	}

	if a.chatSettingsOpen && a.chatSettingsView != nil {
		return a.overlay(result, a.chatSettingsView.View())
	}

	// Overlay help if visible
	if a.showHelp {
		return a.overlay(result, a.renderHelpOverlay())
	}

	return result
}

func (a *UnifiedApp) renderFooterContent() string {
	help := ""
	if a.currentView != nil {
		help = a.currentView.ShortHelp()
	}
	help += "  │  /big /gur /cold /pol  ctrl+l logs  ctrl+p palette  ctrl+, settings  /help  ctrl+c×2 quit"
	return help
}
// renderHelpOverlay renders the full keybinding help overlay
func (a *UnifiedApp) renderHelpOverlay() string {
	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true).
		MarginBottom(1)

	viewName := "Help"
	if a.currentView != nil {
		viewName = a.currentView.Name() + " Help"
	}
	lines = append(lines, titleStyle.Render(viewName))
	lines = append(lines, "")

	// Get full help from view if it supports it
	var bindings []HelpBinding
	if provider, ok := a.currentView.(FullHelpProvider); ok {
		bindings = provider.FullHelp()
	} else {
		// Fall back to generic help from ShortHelp
		bindings = a.defaultHelpBindings()
	}

	// Render bindings
	keyStyle := pkgtui.HelpKeyStyle.Width(12)
	descStyle := pkgtui.HelpDescStyle

	for _, b := range bindings {
		line := keyStyle.Render(b.Key) + " " + descStyle.Render(b.Description)
		lines = append(lines, line)
	}

	// Global keys section
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("Global"))

	globalBindings := []HelpBinding{
		{Key: "?", Description: "Show this help"},
		{Key: "ctrl+c", Description: "Quit"},
		{Key: "/big /gur etc.", Description: "Switch tabs (Bigend/Gurgeh/Coldwine/Pollard)"},
		{Key: "ctrl+left/right", Description: "Cycle tabs"},
		{Key: "ctrl+p", Description: "Command palette"},
		{Key: "ctrl+g", Description: "Agent selector"},
		{Key: "/bigend, etc.", Description: "Switch to tool by name"},
		{Key: "ctrl+l", Description: "Toggle log pane"},
	}
	for _, b := range globalBindings {
		line := keyStyle.Render(b.Key) + " " + descStyle.Render(b.Description)
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, pkgtui.LabelStyle.Render("Press ? or Esc to close"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 3).
		Width(50)

	return boxStyle.Render(content)
}

// defaultHelpBindings returns generic navigation help
func (a *UnifiedApp) defaultHelpBindings() []HelpBinding {
	return []HelpBinding{
		{Key: "up/down", Description: "Navigate"},
		{Key: "enter", Description: "Select/expand"},
		{Key: "esc", Description: "Back/cancel"},
	}
}

func (a *UnifiedApp) overlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	startRow := (a.height - len(overlayLines)) / 4
	startCol := (a.width - lipgloss.Width(overlayLines[0])) / 2

	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	for i, line := range overlayLines {
		row := startRow + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = insertAt(baseLines[row], startCol, line)
	}

	return strings.Join(baseLines, "\n")
}

func insertAt(base string, col int, overlay string) string {
	baseWidth := ansi.StringWidth(base)
	overlayWidth := lipgloss.Width(overlay)

	var result strings.Builder

	// Left portion: ANSI-aware truncation to visual column
	if col > 0 {
		if baseWidth >= col {
			result.WriteString(ansi.Truncate(base, col, ""))
		} else {
			result.WriteString(base)
			for i := baseWidth; i < col; i++ {
				result.WriteByte(' ')
			}
		}
	}

	result.WriteString(overlay)

	// Right portion: skip past overlay width using ANSI-aware left truncation
	end := col + overlayWidth
	if baseWidth > end {
		result.WriteString(ansi.TruncateLeft(base, end, ""))
	}

	return result.String()
}

// RunOpts configures TUI execution options.
type RunOpts struct {
	InlineMode  bool   // Inline mode preserves scrollback and shows log pane
	InitialTool string // Jump directly to this tool tab (bigend, signals, gurgeh, coldwine, pollard)
}

// ErrorView shows an error state
func ErrorView(err error) string {
	return fmt.Sprintf("%s\n\n%s",
		pkgtui.StatusError.Render("Error"),
		pkgtui.LabelStyle.Render(err.Error()),
	)
}

// EmptyView shows an empty state
func EmptyView(message string) string {
	return pkgtui.LabelStyle.Render(message)
}

// Run starts the unified TUI application with configurable options.
func Run(client *autarch.Client, app *UnifiedApp, opts RunOpts) error {
	app.SetInlineMode(opts.InlineMode)
	app.SetInitialTab(opts.InitialTool)

	var progOpts []tea.ProgramOption
	if !opts.InlineMode {
		progOpts = append(progOpts, tea.WithAltScreen())
	}
	progOpts = append(progOpts, tea.WithMouseCellMotion())

	p := tea.NewProgram(app, progOpts...)

	// Always create log handler so slog messages route to the log pane
	handler := pkgtui.NewLogHandler(slog.LevelDebug)
	handler.SetProgram(p)
	slog.SetDefault(slog.New(handler))

	_, err := p.Run()

	// Cleanup
	handler.Close()

	// Dump logs for scrollback (inline mode only — alt-screen is gone)
	if opts.InlineMode {
		fmt.Println("\n--- Log History ---")
		for _, e := range app.LogPane().Entries() {
			fmt.Printf("[%s] %s: %s\n", e.Time.Format("15:04:05"), e.Level, e.Message)
		}
	}

	return err
}
