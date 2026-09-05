package door

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"gopkg.in/yaml.v3"
)

type Density int

const (
	DensityCozy Density = iota
	DensityCompact
)

func (d Density) String() string {
	if d == DensityCompact {
		return "Compact"
	}
	return "Cozy"
}

func DefaultDisplayPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".autarch", "display.yaml")
}

// WithDisplay opts the CLI into a local preference file. Tests and embedded
// models without an explicit path change density only for that instance.
func (m Model) WithDisplay(path string) Model {
	m.displayPath = path
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m
	}
	var prefs map[string]any
	if err == nil {
		err = yaml.Unmarshal(data, &prefs)
	}
	if err != nil {
		m.status = "Display preference unreadable: " + err.Error()
		return m
	}
	switch prefs["density"] {
	case "compact":
		m.density = DensityCompact
	case "cozy", nil:
	default:
		m.status = "Unknown display density; using Cozy for this visit"
	}
	return m
}

func saveDensity(path string, density Density) error {
	if path == "" {
		return nil
	}
	prefs := make(map[string]any)
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &prefs); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if prefs == nil {
		prefs = make(map[string]any)
	}
	prefs["density"] = strings.ToLower(density.String())
	data, err = yaml.Marshal(prefs)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".display-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (m Model) setDensity(d Density) (tea.Model, tea.Cmd) {
	m.density = d
	m.status = d.String() + " view"
	if err := saveDensity(m.displayPath, d); err != nil {
		m.status += " for this visit; could not save preference: " + err.Error()
	} else if m.displayPath != "" {
		m.status += " · saved for next time"
	}
	return m, nil
}

var rangeDurations = []time.Duration{0, 24 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}
var rangeNames = []string{"Since last visit", "Last 24 hours", "Last 3 days", "Last 7 days", "Last 30 days"}

func (m Model) rangeName() string {
	if m.rangeIndex > 0 {
		return rangeNames[m.rangeIndex]
	}
	switch m.briefing.SinceSource {
	case "first visit":
		return "Last 24 hours · first visit"
	case "last visit":
		return "Since last visit"
	default:
		return "Opening window"
	}
}

func (m Model) chooseRange(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(rangeDurations) {
		return m, nil
	}
	if m.moveRemaining > 0 || len(m.movements) < len(m.projects) {
		m.pendingRange = &i
		m.status = "Time range will change when the current read finishes"
		return m, nil
	}
	m.rangeIndex, m.catchupOffset, m.widen = i, 0, -1
	m.window, m.windowSource = m.briefing.Since, m.briefing.SinceSource
	if i > 0 {
		m.window, m.windowSource = m.now().Add(-rangeDurations[i]), rangeNames[i]
	}
	m.resetMovements()
	m.status = "Showing " + m.rangeName()
	if len(m.projects) == 0 {
		return m, nil
	}
	return m, tea.Batch(m.startMovements(), m.waitForMovement())
}

func (m Model) menuItems() []string {
	switch m.menu {
	case "density":
		return []string{"Cozy — spacious project summaries", "Compact — more projects at a glance"}
	case "range":
		items := append([]string(nil), rangeNames...)
		if m.briefing.SinceSource != "last visit" {
			items[0] = "Opening window — " + m.briefing.SinceSource
		}
		return items
	}
	return nil
}

func (m Model) menuWindow() (int, int) {
	count := max(1, (m.dashboardRoom()-1)/2)
	start := max(0, m.menuSelection-count+1)
	return start, count
}

func (m Model) handleMenuKey(key string) (tea.Model, tea.Cmd) {
	items := m.menuItems()
	switch key {
	case "q", "ctrl+c":
		return m, m.quit()
	case "esc", "?":
		m.menu = ""
		return m, nil
	case "j", "down":
		m.menuSelection++
	case "k", "up":
		m.menuSelection--
	case "home", "g":
		m.menuSelection = 0
	case "end", "G":
		m.menuSelection = len(items) - 1
	case "enter", " ":
		menu := m.menu
		m.menu = ""
		if menu == "density" {
			return m.setDensity(Density(m.menuSelection))
		}
		if menu == "range" {
			return m.chooseRange(m.menuSelection)
		}
		return m, nil
	}
	if m.menu == "help" {
		m.menuSelection = max(0, m.menuSelection)
	} else {
		m.menuSelection = max(0, min(m.menuSelection, len(items)-1))
	}
	return m, nil
}

func (m Model) displayMenuView() string {
	title := "View options"
	intro := "Choose the spacing that feels right. You can switch at any time."
	if m.menu == "range" {
		title, intro = "Time range", "Choose how far back to look. No command-line arguments needed."
	}
	lines := []string{intro, ""}
	if m.menu == "help" {
		title = "A quick guide to Autarch"
		lines = []string{"Open Autarch and catch up at your own pace.", "",
			"1 Catch-up   Recent changes and reported outcomes",
			"2 Questions  Read the question and its supporting context",
			"3 Projects   Choose a project; i opens its product HUD",
			"4 Threads    Inspect every terminal session", "",
			"d  Switch Cozy / Compact     v  Choose a view",
			"w  Choose a time range       r  Refresh the snapshot",
			"↑↓ Scroll or select          Esc  Back     q  Quit", "",
			"Enter opens a question's evidence first. Enter again opens its session.",
			"Saved questions are history. Use s in their evidence to resume them.",
			"Product HUD: 1–5 sections, o source, Esc back.", "",
			"Your view preference is remembered. Your last visit sets the next catch-up window."}
	} else {
		start, count := m.menuWindow()
		items := m.menuItems()
		for i := start; i < len(items) && i < start+count; i++ {
			item := items[i]
			mark := "  ○ "
			if i == m.menuSelection {
				mark = "› ● "
			}
			line := mark + item
			if i == m.menuSelection {
				line = styleSelected.Render(line)
			}
			lines = append(lines, line, "")
		}
	}
	keys := "↑↓ choose · Enter apply · Esc back"
	if m.menu == "help" {
		var wrapped []string
		for _, line := range lines {
			wrapped = append(wrapped, strings.Split(ansi.Wrap(line, m.dashboardContentWidth(), ""), "\n")...)
		}
		lines = wrapped
		start := max(0, min(m.menuSelection, len(lines)-m.dashboardRoom()))
		lines = lines[start:]
		keys = "↑↓ scroll · Esc back"
	}
	return m.dashboardFrame(title, lines, keys)
}

func (m Model) handleDisplayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.menu != "" {
		// Body starts on row 4; the two-line intro precedes the options.
		start, _ := m.menuWindow()
		i := start + (msg.Y-6)/2
		if msg.Y >= 6 && msg.Y < 4+m.dashboardRoom() && (msg.Y-6)%2 == 0 && msg.X > 0 && msg.X < m.lineWidth()-1 && i < len(m.menuItems()) {
			m.menuSelection = i
			return m.handleMenuKey("enter")
		}
		return m, nil
	}
	for _, b := range m.dashboardButtons() {
		if msg.Y == b.y && msg.X >= b.x && msg.X < b.x+b.width {
			return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(b.key)})
		}
	}
	return m, nil
}

func (m Model) displayStatus() string {
	if m.pendingRange != nil {
		return "Reading… your time range is queued"
	}
	return fmt.Sprintf("%s · snapshot since %s", m.rangeName(), m.window.Local().Format("Mon 2 Jan 15:04"))
}
