package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/mistakeknot/autarch/pkg/signals"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// SignalsOverlay renders recent signals and events in a single-panel overlay.
type SignalsOverlay struct {
	visible    bool
	width      int
	height     int
	loaded     bool
	signals    []signals.Signal
	events     []*events.Event
	selected   int
	category   int // 0=signals, 1=events
	broker     *signals.Broker
	brokerSub  *signals.Subscription
	brokerDone chan struct{}
}

type signalsOverlayLoadedMsg struct {
	signals []signals.Signal
	events  []*events.Event
	err     error
}

type brokerOverlaySignalMsg struct {
	signal signals.Signal
}

// NewSignalsOverlay creates a new hidden signals overlay.
func NewSignalsOverlay() *SignalsOverlay {
	return &SignalsOverlay{}
}

// SetBroker configures the signal broker for push-based overlay updates.
func (o *SignalsOverlay) SetBroker(b *signals.Broker) {
	o.broker = b
}

// Visible returns whether the overlay is visible.
func (o *SignalsOverlay) Visible() bool {
	return o.visible
}

// Toggle flips visibility and triggers a data load when opening.
func (o *SignalsOverlay) Toggle() tea.Cmd {
	o.visible = !o.visible
	o.selected = 0
	if o.visible {
		o.loaded = false
		cmds := []tea.Cmd{o.loadData()}
		if o.broker != nil && o.brokerSub == nil {
			o.brokerDone = make(chan struct{})
			o.brokerSub = o.broker.Subscribe(nil)
			cmds = append(cmds, o.waitBrokerOverlaySignal())
		}
		return tea.Batch(cmds...)
	}
	// Closing — clean up subscription
	o.closeBrokerSub()
	return nil
}

// Close hides the overlay.
func (o *SignalsOverlay) Close() {
	o.visible = false
	o.closeBrokerSub()
}

// SetSize updates the terminal dimensions used for overlay sizing.
func (o *SignalsOverlay) SetSize(w, h int) {
	o.width = w
	o.height = h
}

// Update handles overlay-specific messages and key input.
func (o *SignalsOverlay) Update(msg tea.Msg) (consumed bool, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case signalsOverlayLoadedMsg:
		o.loaded = true
		if msg.err != nil {
			o.signals = nil
			o.events = nil
			o.selected = 0
			return true, nil
		}
		o.signals = msg.signals
		o.events = msg.events
		o.selected = clampOverlay(o.selected, 0, o.currentListLen()-1)
		return true, nil

	case brokerOverlaySignalMsg:
		if o.visible && o.category == 0 {
			o.signals = append([]signals.Signal{msg.signal}, o.signals...)
			o.selected = clampOverlay(o.selected, 0, o.currentListLen()-1)
		}
		if o.brokerSub != nil {
			return true, o.waitBrokerOverlaySignal()
		}
		return true, nil

	case tea.KeyMsg:
		if !o.visible {
			return false, nil
		}

		switch msg.String() {
		case "esc", "q":
			o.Close()
			return true, nil
		case "up", "k":
			o.selected = clampOverlay(o.selected-1, 0, o.currentListLen()-1)
			return true, nil
		case "down", "j":
			o.selected = clampOverlay(o.selected+1, 0, o.currentListLen()-1)
			return true, nil
		case "tab":
			o.category = (o.category + 1) % 2
			o.selected = clampOverlay(0, 0, o.currentListLen()-1)
			return true, nil
		case "ctrl+r":
			o.loaded = false
			o.selected = 0
			return true, o.loadData()
		default:
			return true, nil
		}
	}

	return false, nil
}

// View renders the overlay content box.
func (o *SignalsOverlay) View() string {
	if !o.visible {
		return ""
	}

	w := o.width
	h := o.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	boxW := clampOverlay(int(float64(w)*0.60), 40, w)
	boxH := clampOverlay(int(float64(h)*0.60), 12, h)

	activeTab := lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true)
	inactiveTab := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	var tabsLine string
	if o.category == 0 {
		tabsLine = activeTab.Render("[Signals]") + "  " + inactiveTab.Render("Events")
	} else {
		tabsLine = inactiveTab.Render("Signals") + "  " + activeTab.Render("[Events]")
	}

	contentWidth := boxW - 10 // account for border + padding
	if contentWidth < 20 {
		contentWidth = 20
	}

	items := o.renderItems(contentWidth)
	innerHeight := boxH - 4 // rough interior size after border/padding
	if innerHeight < 4 {
		innerHeight = 4
	}
	listHeight := innerHeight - 3 // title + spacer + footer
	if listHeight < 1 {
		listHeight = 1
	}
	if len(items) > listHeight {
		items = items[:listHeight]
	}

	footer := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Render("tab category  ↑/↓ navigate  ctrl+r refresh  esc close")
	footer = ansi.Truncate(footer, contentWidth, "…")

	lines := []string{tabsLine, ""}
	lines = append(lines, items...)
	for len(lines) < innerHeight-1 {
		lines = append(lines, "")
	}
	lines = append(lines, footer)

	boxStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 3).
		Width(boxW).
		Height(boxH)

	return boxStyle.Render(strings.Join(lines, "\n"))
}

func (o *SignalsOverlay) waitBrokerOverlaySignal() tea.Cmd {
	sub := o.brokerSub   // capture at call time
	done := o.brokerDone // capture at call time
	if sub == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case sig, ok := <-sub.Chan():
			if !ok {
				return nil
			}
			return brokerOverlaySignalMsg{signal: sig}
		case <-done:
			return nil
		}
	}
}

func (o *SignalsOverlay) closeBrokerSub() {
	if o.brokerDone != nil {
		close(o.brokerDone)
		o.brokerDone = nil
	}
	if o.brokerSub != nil {
		o.brokerSub.Close()
		o.brokerSub = nil
	}
}

func (o *SignalsOverlay) renderItems(contentWidth int) []string {
	if !o.loaded {
		return []string{pkgtui.LabelStyle.Render("Loading...")}
	}

	if o.category == 0 {
		if len(o.signals) == 0 {
			return []string{pkgtui.LabelStyle.Render("No signals found.")}
		}

		lines := make([]string, 0, len(o.signals))
		for i, sig := range o.signals {
			ts := "--"
			if !sig.CreatedAt.IsZero() {
				ts = sig.CreatedAt.Format("Jan02 15:04")
			}
			source := sig.Source
			if source == "" {
				source = "-"
			}
			title := sig.Title
			if title == "" {
				title = sig.Detail
			}
			if title == "" {
				title = sig.ID
			}
			if title == "" {
				title = string(sig.Type)
			}

			base := fmt.Sprintf("%s %s  %-10s  %s", signalSeverityIcon(sig.Severity), ts, source, title)
			prefix := "  "
			if i == o.selected {
				prefix = "› "
			}
			line := ansi.Truncate(prefix+base, contentWidth, "…")
			if i == o.selected {
				line = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
		return lines
	}

	if len(o.events) == 0 {
		return []string{pkgtui.LabelStyle.Render("No events found.")}
	}

	lines := make([]string, 0, len(o.events))
	for i, evt := range o.events {
		ts := "--"
		if !evt.CreatedAt.IsZero() {
			ts = evt.CreatedAt.Format("Jan02 15:04")
		}
		source := string(evt.SourceTool)
		if source == "" {
			source = "-"
		}
		title := string(evt.EventType)
		if evt.EntityID != "" {
			title = fmt.Sprintf("%s: %s", evt.EventType, evt.EntityID)
		}

		base := fmt.Sprintf("%s %s  %-10s  %s", "  ", ts, source, title)
		prefix := "  "
		if i == o.selected {
			prefix = "› "
		}
		line := ansi.Truncate(prefix+base, contentWidth, "…")
		if i == o.selected {
			line = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true).Render(line)
		}
		lines = append(lines, line)
	}

	return lines
}

func (o *SignalsOverlay) currentListLen() int {
	if o.category == 0 {
		return len(o.signals)
	}
	return len(o.events)
}

func (o *SignalsOverlay) loadData() tea.Cmd {
	return func() tea.Msg {
		store, err := events.OpenStore("")
		if err != nil {
			return signalsOverlayLoadedMsg{err: err}
		}
		defer store.Close()

		evs, err := store.Query(events.NewEventFilter().WithLimit(100))
		if err != nil {
			return signalsOverlayLoadedMsg{err: err}
		}

		sort.SliceStable(evs, func(i, j int) bool {
			return evs[i].CreatedAt.After(evs[j].CreatedAt)
		})

		var sigs []signals.Signal
		var other []*events.Event
		for _, evt := range evs {
			if evt.EventType == events.EventSignalRaised || evt.EventType == events.EventSignalDismissed {
				var sig signals.Signal
				if err := json.Unmarshal(evt.Payload, &sig); err == nil {
					if sig.CreatedAt.IsZero() {
						sig.CreatedAt = evt.CreatedAt
					}
					if sig.Source == "" {
						sig.Source = string(evt.SourceTool)
					}
					if sig.Title == "" {
						sig.Title = string(sig.Type)
					}
					sigs = append(sigs, sig)
					continue
				}
				sigs = append(sigs, signals.Signal{
					ID:        evt.EntityID,
					Type:      signals.SignalType(evt.EventType),
					Source:    string(evt.SourceTool),
					Severity:  signals.SeverityWarning,
					Title:     string(evt.EventType),
					Detail:    string(evt.Payload),
					CreatedAt: evt.CreatedAt,
				})
				continue
			}
			other = append(other, evt)
		}

		return signalsOverlayLoadedMsg{
			signals: sigs,
			events:  other,
		}
	}
}

func signalSeverityIcon(s signals.Severity) string {
	switch s {
	case signals.SeverityCritical:
		return "!!"
	case signals.SeverityWarning:
		return "! "
	default:
		return "  "
	}
}

func clampOverlay(val, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}
