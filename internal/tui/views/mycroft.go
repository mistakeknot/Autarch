package views

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
	"github.com/mistakeknot/autarch/internal/mycroft/patrol"
	"github.com/mistakeknot/autarch/internal/mycroft/scheduler"
	"github.com/mistakeknot/autarch/internal/mycroft/tier"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// mycroftFleetMsg carries the result of a fleet state poll.
type mycroftFleetMsg struct {
	view mycroft.FleetView
	err  error
}

// mycroftTierMsg carries the current tier.
type mycroftTierMsg struct {
	tier mycroft.Tier
	err  error
}

// mycroftShadowsMsg carries recent shadow suggestions.
type mycroftShadowsMsg struct {
	entries []scheduler.DispatchEntry
	err     error
}

// mycroftTickMsg triggers a periodic fleet refresh.
type mycroftTickMsg struct{}

// MycroftsView displays fleet orchestrator state with the unified shell layout.
type MycroftsView struct {
	// Fleet state
	fleet       mycroft.FleetView
	currentTier mycroft.Tier
	shadows     []scheduler.DispatchEntry
	decisions   *escalate.DecisionQueue

	// Data sources
	source  mycroft.DataSource
	tierFSM *tier.FSM
	disp    *scheduler.Dispatcher

	// Selection
	selected      int
	selectedAgent string // ID of selected agent for detail view
	viewMode      mycroftViewMode

	// Layout
	width  int
	height int
	shell  *pkgtui.ShellLayout

	// Chat panel
	chatPanel   *pkgtui.ChatPanel
	chatHandler *MycroftChatHandler

	// State
	loading     bool
	lastRefresh time.Time
	errMsg      string
}

type mycroftViewMode int

const (
	mycroftViewFleet   mycroftViewMode = iota // Fleet overview
	mycroftViewAgent                          // Agent detail
	mycroftViewWork                           // Work queue
	mycroftViewShadows                        // Shadow suggestions
)

// NewMycroftsView creates a new Mycroft fleet view.
func NewMycroftsView() *MycroftsView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Commands: /pause, /resume, /shadows, /tier...")
	chatPanel.SetComposerHint("enter send  tab focus  ctrl+b sidebar")
	chatHandler := &MycroftChatHandler{}
	chatPanel.SetHandler(chatHandler)

	return &MycroftsView{
		decisions: escalate.NewDecisionQueue(),
		shell:     pkgtui.NewShellLayout(),
		chatPanel: chatPanel,
		chatHandler: chatHandler,
		viewMode:  mycroftViewFleet,
	}
}

// SetPatrolSource sets the fleet data source for polling.
func (v *MycroftsView) SetPatrolSource(src mycroft.DataSource) {
	v.source = src
}

// SetTierFSM sets the tier state machine for tier display.
func (v *MycroftsView) SetTierFSM(fsm *tier.FSM) {
	v.tierFSM = fsm
}

// SetDispatcher sets the dispatch logger for shadow/history queries.
func (v *MycroftsView) SetDispatcher(d *scheduler.Dispatcher) {
	v.disp = d
}

// SetAgentSelector sets the shared agent selector.
func (v *MycroftsView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
}

// SetAgentName sets the selected agent name (satisfies interface).
func (v *MycroftsView) SetAgentName(_ string) {}

// SetChatSettings sets chat settings on the chat panel.
func (v *MycroftsView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatPanel.SetSettings(settings)
}

// ClearInput clears the chat composer.
func (v *MycroftsView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// Compile-time interface assertions.
var _ pkgtui.SidebarProvider = (*MycroftsView)(nil)

// Init implements View.
func (v *MycroftsView) Init() tea.Cmd {
	return v.pollFleet()
}

// pollFleet fetches fleet state from the data source.
func (v *MycroftsView) pollFleet() tea.Cmd {
	if v.source == nil {
		return nil
	}
	return func() tea.Msg {
		view, err := v.source.FleetState()
		return mycroftFleetMsg{view: view, err: err}
	}
}

// pollTier fetches the current tier.
func (v *MycroftsView) pollTier() tea.Cmd {
	if v.tierFSM == nil {
		return nil
	}
	return func() tea.Msg {
		t, err := v.tierFSM.Current()
		return mycroftTierMsg{tier: t, err: err}
	}
}

// pollShadows fetches recent shadow suggestions.
func (v *MycroftsView) pollShadows() tea.Cmd {
	if v.disp == nil {
		return nil
	}
	return func() tea.Msg {
		entries, err := v.disp.ShadowDigest(20)
		return mycroftShadowsMsg{entries: entries, err: err}
	}
}

// scheduleRefresh sets up periodic fleet refresh.
func (v *MycroftsView) scheduleRefresh() tea.Cmd {
	return tea.Tick(10*time.Second, func(_ time.Time) tea.Msg {
		return mycroftTickMsg{}
	})
}

// Update implements View.
func (v *MycroftsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	// Forward non-key messages to chat panel first.
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

	case mycroftFleetMsg:
		v.loading = false
		v.lastRefresh = time.Now()
		if msg.err != nil {
			v.errMsg = msg.err.Error()
		} else {
			v.errMsg = ""
			v.fleet = msg.view
		}
		return v, nil

	case mycroftTierMsg:
		if msg.err == nil {
			v.currentTier = msg.tier
		}
		return v, nil

	case mycroftShadowsMsg:
		if msg.err == nil {
			v.shadows = msg.entries
		}
		return v, nil

	case mycroftTickMsg:
		return v, tea.Batch(v.pollFleet(), v.scheduleRefresh())

	case pkgtui.SidebarSelectMsg:
		v.handleSidebarSelect(msg.ItemID)
		return v, nil

	case tea.KeyMsg:
		// Let shell handle global keys first.
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		switch v.shell.Focus() {
		case pkgtui.FocusSidebar:
			// Navigation handled by shell/sidebar.
		case pkgtui.FocusDocument:
			switch {
			case key.Matches(msg, commonKeys.NavDown):
				v.moveSelection(1)
			case key.Matches(msg, commonKeys.NavUp):
				v.moveSelection(-1)
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, tea.Batch(v.pollFleet(), v.pollTier(), v.pollShadows())
			case msg.String() == "1":
				v.viewMode = mycroftViewFleet
			case msg.String() == "2":
				v.viewMode = mycroftViewWork
			case msg.String() == "3":
				v.viewMode = mycroftViewShadows
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

func (v *MycroftsView) handleSidebarSelect(id string) {
	switch {
	case id == "fleet-overview":
		v.viewMode = mycroftViewFleet
		v.selectedAgent = ""
	case id == "work-queue":
		v.viewMode = mycroftViewWork
	case id == "shadows":
		v.viewMode = mycroftViewShadows
	case strings.HasPrefix(id, "agent:"):
		v.viewMode = mycroftViewAgent
		v.selectedAgent = strings.TrimPrefix(id, "agent:")
	}
}

func (v *MycroftsView) moveSelection(delta int) {
	switch v.viewMode {
	case mycroftViewWork:
		v.selected += delta
		if v.selected < 0 {
			v.selected = 0
		}
		if v.selected >= len(v.fleet.Work) {
			v.selected = max(0, len(v.fleet.Work)-1)
		}
	case mycroftViewShadows:
		v.selected += delta
		if v.selected < 0 {
			v.selected = 0
		}
		if v.selected >= len(v.shadows) {
			v.selected = max(0, len(v.shadows)-1)
		}
	}
}

// View implements View.
func (v *MycroftsView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading fleet state...")
	}

	sidebarItems := v.SidebarItems()
	document := v.renderDocument()
	chat := v.chatPanel.View()

	return v.shell.Render(sidebarItems, document, chat)
}

// SidebarItems implements SidebarProvider.
func (v *MycroftsView) SidebarItems() []pkgtui.SidebarItem {
	var items []pkgtui.SidebarItem

	// Tier indicator.
	tierIcon := tierIcon(v.currentTier)
	items = append(items, pkgtui.SidebarItem{
		ID:    "fleet-overview",
		Label: fmt.Sprintf("Fleet %s", v.currentTier),
		Icon:  tierIcon,
	})

	// Decision badge.
	if v.decisions.Len() > 0 {
		badge := escalate.Badge(v.decisions.Len(), v.decisions.HighestSeverity())
		items = append(items, pkgtui.SidebarItem{
			ID:    "decisions",
			Label: badge,
			Icon:  "⚡",
		})
	}

	// Section: Agents.
	items = append(items, pkgtui.SidebarItem{
		ID:    "section-agents",
		Label: fmt.Sprintf("── Agents (%d)", len(v.fleet.Agents)),
		Icon:  "",
	})

	for _, agent := range v.fleet.Agents {
		icon := agentStatusIcon(agent.Status)
		label := agent.Name
		if agent.CurrentBead != "" {
			label += " → " + truncate(agent.CurrentBead, 10)
		}
		items = append(items, pkgtui.SidebarItem{
			ID:    "agent:" + agent.Name,
			Label: label,
			Icon:  icon,
		})
	}

	// Section: Work Queue.
	readyCount := 0
	for _, b := range v.fleet.Work {
		if b.DepsResolved && b.ClaimedBy == "" {
			readyCount++
		}
	}
	items = append(items, pkgtui.SidebarItem{
		ID:    "work-queue",
		Label: fmt.Sprintf("── Work (%d ready)", readyCount),
		Icon:  "",
	})

	// Section: Shadows.
	items = append(items, pkgtui.SidebarItem{
		ID:    "shadows",
		Label: fmt.Sprintf("── Shadows (%d)", len(v.shadows)),
		Icon:  "",
	})

	return items
}

// renderDocument renders the main content pane.
func (v *MycroftsView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	switch v.viewMode {
	case mycroftViewAgent:
		return v.renderAgentDetail(width)
	case mycroftViewWork:
		return v.renderWorkQueue(width)
	case mycroftViewShadows:
		return v.renderShadowDigest(width)
	default:
		return v.renderFleetOverview(width)
	}
}

func (v *MycroftsView) renderFleetOverview(width int) string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Fleet Overview"))
	lines = append(lines, "")

	// Tier status.
	tierStyle := tierStyle(v.currentTier)
	lines = append(lines, fmt.Sprintf("Tier: %s", tierStyle.Render(v.currentTier.String())))

	// Freshness.
	if !v.lastRefresh.IsZero() {
		ago := time.Since(v.lastRefresh).Truncate(time.Second)
		lines = append(lines, pkgtui.LabelStyle.Render(fmt.Sprintf("Last refresh: %s ago", ago)))
	}

	if v.errMsg != "" {
		lines = append(lines, "")
		lines = append(lines, pkgtui.StatusError.Render("Error: "+v.errMsg))
	}

	lines = append(lines, "")

	// Agent summary table.
	lines = append(lines, pkgtui.SubtitleStyle.Render("Agents"))
	lines = append(lines, "")

	if len(v.fleet.Agents) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No agents detected"))
	} else {
		// Header.
		header := fmt.Sprintf("  %-18s %-8s %-12s %-10s", "Name", "Status", "Runtime", "Bead")
		lines = append(lines, pkgtui.LabelStyle.Render(header))
		lines = append(lines, pkgtui.LabelStyle.Render(strings.Repeat("─", min(width-4, 60))))

		for _, agent := range v.fleet.Agents {
			icon := agentStatusIcon(agent.Status)
			bead := agent.CurrentBead
			if bead == "" {
				bead = "—"
			}
			row := fmt.Sprintf("%s %-18s %-8s %-12s %-10s",
				icon, truncate(agent.Name, 18), agent.Status,
				truncate(agent.Runtime, 12), truncate(bead, 10))
			lines = append(lines, row)
		}
	}

	lines = append(lines, "")

	// Work summary.
	lines = append(lines, pkgtui.SubtitleStyle.Render("Work Queue"))
	lines = append(lines, "")

	total := len(v.fleet.Work)
	ready := 0
	claimed := 0
	blocked := 0
	for _, b := range v.fleet.Work {
		switch {
		case b.ClaimedBy != "":
			claimed++
		case !b.DepsResolved:
			blocked++
		default:
			ready++
		}
	}
	lines = append(lines, fmt.Sprintf("  Total: %d  Ready: %d  Claimed: %d  Blocked: %d",
		total, ready, claimed, blocked))

	// Conflicts.
	if len(v.fleet.Conflicts) > 0 {
		lines = append(lines, "")
		lines = append(lines, pkgtui.StatusError.Render(
			fmt.Sprintf("⚠ %d file conflicts", len(v.fleet.Conflicts))))
	}

	lines = append(lines, "")
	lines = append(lines, pkgtui.LabelStyle.Render("1 Fleet  2 Work  3 Shadows  ctrl+r refresh"))

	return strings.Join(lines, "\n")
}

func (v *MycroftsView) renderAgentDetail(width int) string {
	var lines []string

	// Find agent.
	var agent *mycroft.AgentView
	for i := range v.fleet.Agents {
		if v.fleet.Agents[i].Name == v.selectedAgent {
			agent = &v.fleet.Agents[i]
			break
		}
	}
	if agent == nil {
		return pkgtui.LabelStyle.Render("Agent not found: " + v.selectedAgent)
	}

	lines = append(lines, pkgtui.TitleStyle.Render("Agent: "+agent.Name))
	lines = append(lines, "")

	// Status.
	statusStyle := agentStatusStyle(agent.Status)
	lines = append(lines, fmt.Sprintf("Status:    %s", statusStyle.Render(agent.Status)))
	lines = append(lines, fmt.Sprintf("Runtime:   %s", agent.Runtime))

	if agent.CurrentBead != "" {
		lines = append(lines, fmt.Sprintf("Working:   %s", agent.CurrentBead))
	}

	lines = append(lines, "")

	// Health.
	lines = append(lines, pkgtui.SubtitleStyle.Render("Health"))
	if agent.Health.IsHealthy {
		lines = append(lines, pkgtui.StatusRunning.Render("  ✓ Healthy"))
	} else {
		lines = append(lines, pkgtui.StatusError.Render("  ✗ Unhealthy"))
		if agent.Health.Details != "" {
			lines = append(lines, fmt.Sprintf("  %s", agent.Health.Details))
		}
	}
	if !agent.Health.LastSeen.IsZero() {
		ago := time.Since(agent.Health.LastSeen).Truncate(time.Second)
		lines = append(lines, pkgtui.LabelStyle.Render(fmt.Sprintf("  Last seen: %s ago", ago)))
	}

	lines = append(lines, "")

	// Cost.
	if agent.CostProfile.Model != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Cost"))
		lines = append(lines, fmt.Sprintf("  Model: %s", agent.CostProfile.Model))
		if agent.CostProfile.EstimatedPerBead > 0 {
			lines = append(lines, fmt.Sprintf("  Est/bead: $%.2f", agent.CostProfile.EstimatedPerBead))
		}
	}

	// Capabilities.
	if len(agent.Capabilities) > 0 {
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render("Capabilities"))
		for _, cap := range agent.Capabilities {
			lines = append(lines, fmt.Sprintf("  • %s", cap))
		}
	}

	// Reservations.
	if len(agent.Reservations) > 0 {
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render("File Reservations"))
		for _, res := range agent.Reservations {
			lines = append(lines, fmt.Sprintf("  📄 %s", res))
		}
	}

	return strings.Join(lines, "\n")
}

func (v *MycroftsView) renderWorkQueue(width int) string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Work Queue"))
	lines = append(lines, "")

	if len(v.fleet.Work) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No beads in work queue"))
		return strings.Join(lines, "\n")
	}

	// Header.
	header := fmt.Sprintf("  %-12s P  %-30s %-8s %-8s", "ID", "Title", "Complex", "Status")
	lines = append(lines, pkgtui.LabelStyle.Render(header))
	lines = append(lines, pkgtui.LabelStyle.Render(strings.Repeat("─", min(width-4, 70))))

	for i, bead := range v.fleet.Work {
		icon := priorityIcon(bead.Priority)
		status := "ready"
		if bead.ClaimedBy != "" {
			status = "claimed"
		} else if !bead.DepsResolved {
			status = "blocked"
		}

		statusStyle := workStatusStyle(status)
		row := fmt.Sprintf("%s %-12s %s  %-30s %-8s %s",
			selectionIndicator(i == v.selected),
			truncate(bead.ID, 12),
			icon,
			truncate(bead.Title, 30),
			truncate(bead.Complexity, 8),
			statusStyle.Render(status))

		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

func (v *MycroftsView) renderShadowDigest(width int) string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Shadow Suggestions"))
	lines = append(lines, pkgtui.LabelStyle.Render("What Mycroft would dispatch at higher tiers"))
	lines = append(lines, "")

	if len(v.shadows) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No shadow suggestions yet"))
		lines = append(lines, "")
		lines = append(lines, pkgtui.LabelStyle.Render("Run a patrol cycle to generate suggestions."))
		return strings.Join(lines, "\n")
	}

	for i, entry := range v.shadows {
		marker := selectionIndicator(i == v.selected)
		lines = append(lines, fmt.Sprintf("%s %s → %s",
			marker,
			pkgtui.SubtitleStyle.Render(entry.Agent),
			entry.Bead))
		if entry.Reason != "" {
			lines = append(lines, fmt.Sprintf("    %s", pkgtui.LabelStyle.Render(entry.Reason)))
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// Focus implements View.
func (v *MycroftsView) Focus() tea.Cmd {
	v.shell.SetFocus(pkgtui.FocusChat)
	return tea.Batch(
		v.chatPanel.Focus(),
		v.pollFleet(),
		v.pollTier(),
		v.pollShadows(),
		v.scheduleRefresh(),
	)
}

// Blur implements View.
func (v *MycroftsView) Blur() {
	v.chatPanel.CancelStream()
	v.chatPanel.Blur()
}

// Name implements View.
func (v *MycroftsView) Name() string {
	return "Mycroft"
}

// ShortHelp implements View.
func (v *MycroftsView) ShortHelp() string {
	badge := escalate.Badge(v.decisions.Len(), v.decisions.HighestSeverity())
	return fmt.Sprintf("1-3 views  ↑/↓ navigate  ctrl+r refresh  tab focus  %s", badge)
}

// Commands implements CommandProvider.
func (v *MycroftsView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "Refresh Fleet",
			Description: "Poll fleet state and work queue",
			Action: func() tea.Cmd {
				v.loading = true
				return tea.Batch(v.pollFleet(), v.pollTier(), v.pollShadows())
			},
		},
		{
			Name:        "Show Shadows",
			Description: "View what Mycroft would dispatch",
			Action: func() tea.Cmd {
				v.viewMode = mycroftViewShadows
				return v.pollShadows()
			},
		},
	}
}

// --- Helpers ---

func tierIcon(t mycroft.Tier) string {
	switch t {
	case mycroft.T0:
		return "👁"
	case mycroft.T1:
		return "💡"
	case mycroft.T2:
		return "⚡"
	case mycroft.T3:
		return "🚀"
	default:
		return "?"
	}
}

func tierStyle(t mycroft.Tier) lipgloss.Style {
	switch t {
	case mycroft.T0:
		return pkgtui.LabelStyle
	case mycroft.T1:
		return lipgloss.NewStyle().Foreground(pkgtui.ColorInfo)
	case mycroft.T2:
		return lipgloss.NewStyle().Foreground(pkgtui.ColorWarning).Bold(true)
	case mycroft.T3:
		return lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess).Bold(true)
	default:
		return pkgtui.LabelStyle
	}
}

func agentStatusIcon(status string) string {
	switch status {
	case "active":
		return "●"
	case "idle":
		return "○"
	case "stuck":
		return "⊘"
	case "crashed":
		return "✗"
	default:
		return "?"
	}
}

func agentStatusStyle(status string) lipgloss.Style {
	switch status {
	case "active":
		return pkgtui.StatusRunning
	case "idle":
		return pkgtui.StatusIdle
	case "stuck":
		return pkgtui.StatusWaiting
	case "crashed":
		return pkgtui.StatusError
	default:
		return pkgtui.LabelStyle
	}
}

func priorityIcon(p int) string {
	switch p {
	case 0:
		return "🔴"
	case 1:
		return "🟠"
	case 2:
		return "🟡"
	case 3:
		return "🟢"
	default:
		return "⚪"
	}
}

func workStatusStyle(status string) lipgloss.Style {
	switch status {
	case "ready":
		return pkgtui.StatusRunning
	case "claimed":
		return pkgtui.StatusWaiting
	case "blocked":
		return pkgtui.StatusError
	default:
		return pkgtui.LabelStyle
	}
}

func selectionIndicator(selected bool) string {
	if selected {
		return "▸"
	}
	return " "
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// MycroftChatHandler handles chat input for the Mycroft view.
// Slash commands are routed through the unified app's handler;
// freeform messages get a simple response about Mycroft status.
type MycroftChatHandler struct {
	mu              sync.RWMutex
	continueSession bool
	sessionID       string
}

// HandleMessage implements ChatHandler.
func (h *MycroftChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
	out := make(chan pkgtui.StreamMsg, 4)
	go func() {
		defer close(out)
		response := "Mycroft chat is not yet connected to an LLM. Use slash commands (/myc, /pause, /resume) or the command palette (Ctrl+P) to interact with the fleet orchestrator."
		select {
		case out <- pkgtui.TextDelta{Text: response}:
		case <-ctx.Done():
			return
		}
		select {
		case out <- pkgtui.StreamDone{FinishReason: "stop"}:
		case <-ctx.Done():
		}
	}()
	return out, nil
}

// SetContinue implements MultiTurnHandler.
func (h *MycroftChatHandler) SetContinue(cont bool, sessionID string) {
	h.mu.Lock()
	h.continueSession = cont
	h.sessionID = sessionID
	h.mu.Unlock()
}

// ResetSession implements MultiTurnHandler.
func (h *MycroftChatHandler) ResetSession() {
	h.mu.Lock()
	h.continueSession = false
	h.sessionID = ""
	h.mu.Unlock()
}

// Ensure patrol.PatrolSource satisfies the DataSource interface used by the view.
var _ mycroft.DataSource = (*patrol.PatrolSource)(nil)
