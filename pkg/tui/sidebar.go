package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	// SidebarWidth is the fixed width when expanded
	SidebarWidth = 20
	// MaxLabelWidth is the max label length before truncation (SidebarWidth - padding - icon)
	MaxLabelWidth = 17
)

// SidebarItem represents a single item in the sidebar navigation.
type SidebarItem struct {
	ID    string
	Label string
	Icon  string // e.g., "●", "◐", "✓"
}

// Sidebar provides collapsible navigation for the unified shell.
type Sidebar struct {
	items     []SidebarItem
	selected  int
	collapsed bool
	width     int // Fixed 20 chars when expanded, 0 when collapsed
	height    int
	focused   bool
}

// NewSidebar creates a new sidebar with default settings.
func NewSidebar() *Sidebar {
	return &Sidebar{
		width: SidebarWidth,
	}
}

// Toggle switches between collapsed and expanded states.
func (s *Sidebar) Toggle() {
	s.collapsed = !s.collapsed
}

// SetItems updates the sidebar items.
func (s *Sidebar) SetItems(items []SidebarItem) {
	s.items = items
	// Clamp selection to valid range
	if s.selected >= len(items) {
		s.selected = max(0, len(items)-1)
	}
}

// Selected returns the currently selected item, if any.
func (s *Sidebar) Selected() (SidebarItem, bool) {
	if len(s.items) == 0 || s.selected < 0 || s.selected >= len(s.items) {
		return SidebarItem{}, false
	}
	return s.items[s.selected], true
}

// IsCollapsed returns whether the sidebar is collapsed.
func (s *Sidebar) IsCollapsed() bool {
	return s.collapsed
}

// SetSize sets the available dimensions for the sidebar.
func (s *Sidebar) SetSize(width, height int) {
	if s.collapsed {
		s.width = 0
	} else {
		s.width = SidebarWidth
	}
	s.height = height
}

// Width returns the current width (0 if collapsed).
func (s *Sidebar) Width() int {
	if s.collapsed {
		return 0
	}
	return SidebarWidth
}

// Update handles keyboard input for navigation.
func (s *Sidebar) Update(msg tea.Msg) (*Sidebar, tea.Cmd) {
	if !s.focused || s.collapsed {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if s.selected < len(s.items)-1 {
				s.selected++
			}
		case "k", "up":
			if s.selected > 0 {
				s.selected--
			}
		case "enter":
			if item, ok := s.Selected(); ok {
				return s, func() tea.Msg {
					return SidebarSelectMsg{ItemID: item.ID}
				}
			}
		}
	}

	return s, nil
}

// View renders the sidebar.
func (s *Sidebar) View() string {
	if s.collapsed || s.width == 0 {
		return ""
	}

	// Border style based on focus
	borderColor := ColorBorder
	if s.focused {
		borderColor = ColorPrimary
	}

	borderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(SidebarWidth - 2). // Account for border
		Height(s.height - 2)     // Account for border

	// Empty state
	if len(s.items) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
		content := emptyStyle.Render("No items yet")
		return borderStyle.Render(content)
	}

	// Render items
	var lines []string
	for i, item := range s.items {
		line := s.renderItem(item, i == s.selected)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return borderStyle.Render(content)
}

// renderItem renders a single sidebar item.
func (s *Sidebar) renderItem(item SidebarItem, selected bool) string {
	// Icon with space
	icon := item.Icon
	if icon == "" {
		icon = "•"
	}

	// Truncate label if needed
	label := item.Label
	if ansi.StringWidth(label) > MaxLabelWidth {
		label = ansi.Truncate(label, MaxLabelWidth-1, "") + "…"
	}

	line := icon + " " + label

	if selected {
		style := lipgloss.NewStyle().
			Background(ColorBgLighter).
			Foreground(ColorPrimary).
			Bold(true)
		return style.Render(padToWidth(line, SidebarWidth-4)) // -4 for border + padding
	}

	style := lipgloss.NewStyle().
		Foreground(ColorFg)
	return style.Render(line)
}

// Focus marks the sidebar as focused.
func (s *Sidebar) Focus() tea.Cmd {
	s.focused = true
	return nil
}

// Blur marks the sidebar as not focused.
func (s *Sidebar) Blur() {
	s.focused = false
}

// IsFocused returns whether the sidebar is focused.
func (s *Sidebar) IsFocused() bool {
	return s.focused
}

// SidebarSelectMsg is sent when an item is selected via Enter.
type SidebarSelectMsg struct {
	ItemID string
}

// SidebarProvider is an optional interface for views that provide sidebar items.
type SidebarProvider interface {
	SidebarItems() []SidebarItem
}
