package views

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/mistakeknot/autarch/pkg/intermute"
	"github.com/mistakeknot/autarch/pkg/signals"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

const (
	categorySignals  = "signals"
	categoryEvents   = "events"
	categoryConflicts = "conflicts"
)

// SignalsView renders signals + events + reconcile conflicts in a unified panel.
type SignalsView struct {
	client *autarch.Client
	shell  *pkgtui.ShellLayout

	width  int
	height int

	loading bool
	err     error

	category string

	signals  []signals.Signal
	events   []*events.Event
	conflicts []events.ReconcileConflict

	selected int

	projectPath string

	sourceFilter    int
	signalTypeFilter int
	eventTypeFilter  int
	severityFilter   int

	intermuteClient *intermute.Client
	intermuteEvents chan intermute.Event
	intermuteStatus string
}

// NewSignalsView creates a new Signals view.
func NewSignalsView(client *autarch.Client) *SignalsView {
	return &SignalsView{
		client:          client,
		shell:           pkgtui.NewShellLayout(),
		category:        categorySignals,
		intermuteEvents: make(chan intermute.Event, 32),
	}
}

// SetProjectContext sets an optional project path filter.
func (v *SignalsView) SetProjectContext(path string) {
	v.projectPath = path
}

type signalsLoadedMsg struct {
	signals   []signals.Signal
	events    []*events.Event
	conflicts []events.ReconcileConflict
	err       error
}

type intermuteReadyMsg struct {
	client  *intermute.Client
	offline bool
	err     error
}

type intermuteEventMsg struct {
	event intermute.Event
}

// Init implements View.
func (v *SignalsView) Init() tea.Cmd {
	return tea.Batch(
		v.loadData(),
		v.connectIntermute(),
	)
}

// Update implements View.
func (v *SignalsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case signalsLoadedMsg:
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			v.signals = msg.signals
			v.events = msg.events
			v.conflicts = msg.conflicts
			v.selected = clamp(v.selected, 0, v.currentListLen()-1)
		}
		return v, nil

	case intermuteReadyMsg:
		if msg.offline {
			v.intermuteStatus = "offline"
			return v, nil
		}
		if msg.err != nil {
			v.intermuteStatus = "error"
			return v, nil
		}
		v.intermuteClient = msg.client
		v.intermuteStatus = "live"
		return v, v.waitIntermuteEvent()

	case intermuteEventMsg:
		if v.intermuteStatus == "live" {
			return v, tea.Batch(v.loadData(), v.waitIntermuteEvent())
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		v.category = msg.ItemID
		v.selected = 0
		return v, nil

	case tea.KeyMsg:
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		switch v.shell.Focus() {
		case pkgtui.FocusDocument:
			switch {
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadData()
			case key.Matches(msg, commonKeys.NavDown):
				v.selected = clamp(v.selected+1, 0, v.currentListLen()-1)
			case key.Matches(msg, commonKeys.NavUp):
				v.selected = clamp(v.selected-1, 0, v.currentListLen()-1)
			case msg.Type == tea.KeyF3:
				v.sourceFilter = (v.sourceFilter + 1) % len(sourceFilters)
			case msg.Type == tea.KeyF4:
				v.cycleTypeFilter()
			case msg.Type == tea.KeyF5:
				v.severityFilter = (v.severityFilter + 1) % len(severityFilters)
			}
		}
	}

	return v, nil
}

// View implements View.
func (v *SignalsView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading signals...")
	}
	if v.err != nil {
		return tui.ErrorView(v.err)
	}

	sidebar := v.SidebarItems()
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.Render(sidebar, document, chat)
}

// SidebarItems implements SidebarProvider.
func (v *SignalsView) SidebarItems() []pkgtui.SidebarItem {
	return []pkgtui.SidebarItem{
		{ID: categorySignals, Label: fmt.Sprintf("Signals (%d)", len(v.filteredSignals())), Icon: "⚠"},
		{ID: categoryEvents, Label: fmt.Sprintf("Events (%d)", len(v.filteredEvents())), Icon: "•"},
		{ID: categoryConflicts, Label: fmt.Sprintf("Conflicts (%d)", len(v.conflicts)), Icon: "!"},
	}
}

func (v *SignalsView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	header := v.renderHeader()
	lines := []string{header, ""}

	switch v.category {
	case categorySignals:
		lines = append(lines, v.renderSignals()...)
	case categoryConflicts:
		lines = append(lines, v.renderConflicts()...)
	default:
		lines = append(lines, v.renderEvents()...)
	}

	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (v *SignalsView) renderHeader() string {
	source := sourceFilters[v.sourceFilter]
	severity := severityFilters[v.severityFilter]
	typeFilter := v.currentTypeFilter()
	status := v.intermuteStatus
	if status == "" {
		status = "offline"
	}

	header := fmt.Sprintf("Source: %s · Type: %s · Severity: %s · Intermute: %s", source, typeFilter, severity, status)
	return lipgloss.NewStyle().Foreground(pkgtui.ColorMuted).Render(header)
}

func (v *SignalsView) renderSignals() []string {
	sigs := v.filteredSignals()
	if len(sigs) == 0 {
		return []string{pkgtui.LabelStyle.Render("No signals found.")}
	}

	lines := make([]string, 0, len(sigs))
	for i, sig := range sigs {
		line := fmt.Sprintf("%s  %s  %s  %s", formatTime(sig.CreatedAt), sig.Severity, sig.Type, titleOrDetail(sig))
		lines = append(lines, highlightLine(line, i == v.selected))
	}
	return lines
}

func (v *SignalsView) renderEvents() []string {
	evs := v.filteredEvents()
	if len(evs) == 0 {
		return []string{pkgtui.LabelStyle.Render("No events found.")}
	}

	lines := make([]string, 0, len(evs))
	for i, evt := range evs {
		line := fmt.Sprintf("%s  %s  %s", formatTime(evt.CreatedAt), evt.EventType, evt.EntityID)
		lines = append(lines, highlightLine(line, i == v.selected))
	}
	return lines
}

func (v *SignalsView) renderConflicts() []string {
	if len(v.conflicts) == 0 {
		return []string{pkgtui.LabelStyle.Render("No conflicts recorded.")}
	}
	lines := make([]string, 0, len(v.conflicts))
	for i, c := range v.conflicts {
		line := fmt.Sprintf("%s  %s/%s  %s", formatTime(c.CreatedAt), c.EntityType, c.EntityID, c.Reason)
		lines = append(lines, highlightLine(line, i == v.selected))
	}
	return lines
}

func (v *SignalsView) renderChat() string {
	return pkgtui.LabelStyle.Render("Signals and events are read-only.")
}

// Focus implements View.
func (v *SignalsView) Focus() tea.Cmd {
	return v.loadData()
}

// Blur implements View.
func (v *SignalsView) Blur() {}

// Name implements View.
func (v *SignalsView) Name() string {
	return "Signals"
}

// ShortHelp implements View.
func (v *SignalsView) ShortHelp() string {
	return "↑/↓ navigate  ctrl+r refresh  F3 source  F4 type  F5 severity  Tab focus  ctrl+b sidebar"
}

// Commands implements CommandProvider.
func (v *SignalsView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "Refresh signals/events",
			Description: "Reload signals and recent events",
			Action: func() tea.Cmd {
				return v.loadData()
			},
		},
	}
}

func (v *SignalsView) loadData() tea.Cmd {
	projectPath := v.projectPath
	return func() tea.Msg {
		store, err := events.OpenStore("")
		if err != nil {
			return signalsLoadedMsg{err: err}
		}
		defer store.Close()

		filter := events.NewEventFilter().WithLimit(200)
		evs, err := store.Query(filter)
		if err != nil {
			return signalsLoadedMsg{err: err}
		}

		if projectPath != "" {
			filtered := evs[:0]
			for _, evt := range evs {
				if evt.ProjectPath == projectPath {
					filtered = append(filtered, evt)
				}
			}
			evs = filtered
		}

		// newest first
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].CreatedAt.After(evs[j].CreatedAt) })

		var sigs []signals.Signal
		var otherEvents []*events.Event
		for _, evt := range evs {
			if evt.EventType == events.EventSignalRaised || evt.EventType == events.EventSignalDismissed {
				var sig signals.Signal
				if err := json.Unmarshal(evt.Payload, &sig); err == nil && sig.ID != "" {
					sigs = append(sigs, sig)
					continue
				}
				// Fallback to minimal signal from event metadata
				sigs = append(sigs, signals.Signal{
					ID:        evt.EntityID,
					Type:      signals.SignalType(evt.EventType),
					Source:    string(evt.SourceTool),
					Severity:  signals.SeverityWarning,
					Title:     fmt.Sprintf("%s", evt.EventType),
					Detail:    string(evt.Payload),
					CreatedAt: evt.CreatedAt,
				})
				continue
			}
			otherEvents = append(otherEvents, evt)
		}

		conflicts, err := store.ListConflicts(projectPath, 100)
		if err != nil {
			return signalsLoadedMsg{err: err}
		}

		return signalsLoadedMsg{signals: sigs, events: otherEvents, conflicts: conflicts}
	}
}

func (v *SignalsView) connectIntermute() tea.Cmd {
	return func() tea.Msg {
		client, err := intermute.NewClient(nil)
		if err != nil {
			return intermuteReadyMsg{err: err}
		}
		if !client.Available() {
			return intermuteReadyMsg{offline: true}
		}
		client.On("*", func(evt intermute.Event) {
			select {
			case v.intermuteEvents <- evt:
			default:
			}
		})
		if err := client.Connect(context.Background()); err != nil {
			return intermuteReadyMsg{err: err}
		}
		return intermuteReadyMsg{client: client}
	}
}

func (v *SignalsView) waitIntermuteEvent() tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-v.intermuteEvents
		if !ok {
			return nil
		}
		return intermuteEventMsg{event: evt}
	}
}

func (v *SignalsView) filteredSignals() []signals.Signal {
	source := sourceFilters[v.sourceFilter]
	typeFilter := signalTypeFilters[v.signalTypeFilter]
	severity := severityFilters[v.severityFilter]

	var filtered []signals.Signal
	for _, sig := range v.signals {
		if source != "all" && sig.Source != source {
			continue
		}
		if typeFilter != "all" && string(sig.Type) != typeFilter {
			continue
		}
		if severity != "all" && string(sig.Severity) != severity {
			continue
		}
		filtered = append(filtered, sig)
	}
	return filtered
}

func (v *SignalsView) filteredEvents() []*events.Event {
	source := sourceFilters[v.sourceFilter]
	typeFilter := eventTypeFilters[v.eventTypeFilter]

	var filtered []*events.Event
	for _, evt := range v.events {
		if source != "all" && string(evt.SourceTool) != source {
			continue
		}
		if typeFilter != "all" && string(evt.EventType) != typeFilter {
			continue
		}
		filtered = append(filtered, evt)
	}
	return filtered
}

func (v *SignalsView) currentTypeFilter() string {
	if v.category == categorySignals {
		return signalTypeFilters[v.signalTypeFilter]
	}
	return eventTypeFilters[v.eventTypeFilter]
}

func (v *SignalsView) cycleTypeFilter() {
	if v.category == categorySignals {
		v.signalTypeFilter = (v.signalTypeFilter + 1) % len(signalTypeFilters)
		return
	}
	v.eventTypeFilter = (v.eventTypeFilter + 1) % len(eventTypeFilters)
}

func (v *SignalsView) currentListLen() int {
	switch v.category {
	case categorySignals:
		return len(v.filteredSignals())
	case categoryConflicts:
		return len(v.conflicts)
	default:
		return len(v.filteredEvents())
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	return t.Format("Jan02 15:04")
}

func titleOrDetail(sig signals.Signal) string {
	if sig.Title != "" {
		return sig.Title
	}
	if sig.Detail != "" {
		return sig.Detail
	}
	return sig.ID
}

func highlightLine(line string, selected bool) string {
	if !selected {
		return line
	}
	return lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true).Render("› " + line)
}

func clamp(val, minVal, maxVal int) int {
	if maxVal < minVal {
		return minVal
	}
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

var sourceFilters = []string{"all", "gurgeh", "coldwine", "pollard", "bigend"}

var severityFilters = []string{"all", "info", "warning", "critical"}

var signalTypeFilters = []string{
	"all",
	string(signals.SignalCompetitorShipped),
	string(signals.SignalResearchInvalidation),
	string(signals.SignalAssumptionDecayed),
	string(signals.SignalHypothesisStale),
	string(signals.SignalSpecHealthLow),
	string(signals.SignalExecutionDrift),
	string(signals.SignalVisionDrift),
}

var eventTypeFilters = []string{
	"all",
	string(events.EventSpecRevised),
	string(events.EventTaskCreated),
	string(events.EventTaskStarted),
	string(events.EventTaskBlocked),
	string(events.EventTaskCompleted),
	string(events.EventRunStarted),
	string(events.EventRunWaiting),
	string(events.EventRunCompleted),
	string(events.EventRunFailed),
	string(events.EventRunArtifactAdded),
	string(events.EventOutcomeRecorded),
}

// Ensure interface compliance.
var _ pkgtui.SidebarProvider = (*SignalsView)(nil)
var _ tui.CommandProvider = (*SignalsView)(nil)
