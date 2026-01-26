// Package tui provides shared TUI styles and components for Autarch projects.
package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Composer is a multi-line text input component with Tokyo Night styling.
// It wraps bubbles/textarea with consistent theming and keyboard hints.
type Composer struct {
	textarea textarea.Model
	title    string
	hint     string // Keyboard shortcuts hint
	width    int
	height   int // Total height including borders and hint
	focused  bool
}

// NewComposer creates a new Composer with the specified content height (lines).
// The total height will include borders and the hint line.
func NewComposer(contentHeight int) *Composer {
	if contentHeight < 1 {
		contentHeight = 4
	}

	ta := textarea.New()
	ta.Placeholder = "Type your response..."
	ta.CharLimit = 2000
	// Don't set a default width here - width will be set via SetSize()
	// Setting a width now would cause it to persist even after SetSize() in some cases
	ta.SetHeight(contentHeight)
	ta.ShowLineNumbers = false

	// Style the textarea to match Tokyo Night theme
	// NO Background() - inherit terminal background for clean look
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		Foreground(ColorFg)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // No background highlight
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().
		Foreground(ColorMuted)
	ta.FocusedStyle.Text = lipgloss.NewStyle().
		Foreground(ColorFg)
	ta.BlurredStyle = ta.FocusedStyle

	// Cursor style
	ta.Cursor.Style = lipgloss.NewStyle().
		Foreground(ColorBg).
		Background(ColorPrimary)

	return &Composer{
		textarea: ta,
		hint:     "enter: send  ctrl+j: newline",
		height:   contentHeight + 4, // +2 for border, +1 for title, +1 for hint
	}
}

// SetTitle sets the title displayed above the input area.
func (c *Composer) SetTitle(title string) {
	c.title = title
}

// SetHint sets the keyboard shortcuts hint displayed below the input.
func (c *Composer) SetHint(hint string) {
	c.hint = hint
}

// SetPlaceholder sets the placeholder text shown when empty.
func (c *Composer) SetPlaceholder(placeholder string) {
	c.textarea.Placeholder = placeholder
}

// SetSize sets the width and height of the composer.
// Height includes the border, title line, and hint line.
func (c *Composer) SetSize(width, height int) {
	c.width = width
	c.height = height

	// Calculate textarea dimensions accounting for border and decorations
	textareaWidth := width - 4 // Account for border padding
	if textareaWidth < 10 {
		textareaWidth = 10
	}

	// Height: total - border (2) - title (1 if present) - hint (1)
	decorations := 2 // border
	if c.title != "" {
		decorations++
	}
	if c.hint != "" {
		decorations++
	}
	textareaHeight := height - decorations
	if textareaHeight < 1 {
		textareaHeight = 1
	}

	c.textarea.SetWidth(textareaWidth)
	c.textarea.SetHeight(textareaHeight)
}

// Update handles tea.Msg for the composer.
func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	return c, cmd
}

// View renders the composer with border, title, and hint.
func (c *Composer) View() string {
	width := c.width
	if width < 10 {
		width = 40 // Minimum reasonable width
	}

	// Calculate the inner width for content (accounting for border and padding)
	// Border = 2 (left + right), Padding = 2 (1 on each side)
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Ensure textarea width matches the calculated inner width
	// This handles the case where SetSize() hasn't been called yet
	c.textarea.SetWidth(innerWidth)

	// Build content sections
	var content string

	// Title if present
	if c.title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
		content = titleStyle.Render(c.title) + "\n"
	}

	// Textarea content
	content += c.textarea.View()

	// Border style based on focus
	borderColor := ColorMuted
	if c.focused {
		borderColor = ColorPrimary
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
	// Removed: .Width(width - 2).MaxWidth(width - 2)
	// Let lipgloss auto-size to content - the textarea is already sized via SetSize()

	boxedContent := boxStyle.Render(content)

	// Add hint below if present
	if c.hint != "" {
		hintStyle := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
		return boxedContent + "\n" + hintStyle.Render(c.hint)
	}

	return boxedContent
}

// Value returns the current text content.
func (c *Composer) Value() string {
	return c.textarea.Value()
}

// SetValue sets the text content.
func (c *Composer) SetValue(s string) {
	c.textarea.SetValue(s)
}

// Reset clears the text content.
func (c *Composer) Reset() {
	c.textarea.Reset()
}

// Focus focuses the composer and returns the blink command.
func (c *Composer) Focus() tea.Cmd {
	c.focused = true
	return c.textarea.Focus()
}

// Blur removes focus from the composer.
func (c *Composer) Blur() {
	c.focused = false
	c.textarea.Blur()
}

// Focused returns whether the composer is focused.
func (c *Composer) Focused() bool {
	return c.focused
}

// CursorPosition returns the current cursor position (line, column).
func (c *Composer) CursorPosition() (int, int) {
	return c.textarea.Line(), c.textarea.LineInfo().ColumnOffset
}
