package tui

import (
	"github.com/mistakeknot/Masaq/tabbar"
)

// TabBar renders a horizontal tab bar using the shared Masaq tabbar component.
type TabBar struct {
	model tabbar.Model
	names []string
}

// NewTabBar creates a new tab bar from the given tab names.
func NewTabBar(tabs []string) *TabBar {
	masaqTabs := make([]tabbar.Tab, len(tabs))
	for i, name := range tabs {
		masaqTabs[i] = tabbar.Tab{Label: name}
	}
	return &TabBar{
		model: tabbar.New(masaqTabs),
		names: tabs,
	}
}

// SetActive sets the active tab.
func (t *TabBar) SetActive(index int) {
	t.model.SetActive(index)
}

// Active returns the active tab index.
func (t *TabBar) Active() int {
	return t.model.Active()
}

// SetWidth sets the tab bar width.
func (t *TabBar) SetWidth(width int) {
	// Masaq tabbar gets width via tea.WindowSizeMsg, but we can
	// also trigger it by storing and using it in View.
	// For now, the underlying model handles truncation via its width field.
}

// Next moves to the next tab.
func (t *TabBar) Next() {
	n := len(t.names)
	if n > 0 {
		t.model.SetActive((t.model.Active() + 1) % n)
	}
}

// Prev moves to the previous tab.
func (t *TabBar) Prev() {
	n := len(t.names)
	if n > 0 {
		t.model.SetActive((t.model.Active() - 1 + n) % n)
	}
}

// View renders the tab bar.
func (t *TabBar) View() string {
	return t.model.View()
}

// TabNames returns the list of tab names.
func (t *TabBar) TabNames() []string {
	return t.names
}
