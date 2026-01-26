package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SplitLayout renders a 2/3 + 1/3 horizontal split layout.
// Falls back to stacked layout for narrow terminals.
type SplitLayout struct {
	leftRatio float64 // Default 0.66 (2/3)
	width     int
	height    int
	minWidth  int // Minimum width before falling back to stacked
}

// NewSplitLayout creates a new split layout with the specified left ratio.
// The ratio determines how much of the width goes to the left pane.
// Use 0.66 for a 2/3 + 1/3 split.
func NewSplitLayout(leftRatio float64) *SplitLayout {
	if leftRatio <= 0 || leftRatio >= 1 {
		leftRatio = 0.66
	}
	return &SplitLayout{
		leftRatio: leftRatio,
		minWidth:  100, // Default breakpoint for stacked layout
	}
}

// SetSize sets the total dimensions available for the layout.
func (l *SplitLayout) SetSize(width, height int) {
	l.width = width
	l.height = height
}

// SetMinWidth sets the minimum width before falling back to stacked layout.
func (l *SplitLayout) SetMinWidth(minWidth int) {
	l.minWidth = minWidth
}

// LeftWidth returns the width available for the left pane.
func (l *SplitLayout) LeftWidth() int {
	if l.IsStacked() {
		return l.width
	}
	return int(float64(l.width) * l.leftRatio) - 2 // Account for separator
}

// RightWidth returns the width available for the right pane.
func (l *SplitLayout) RightWidth() int {
	if l.IsStacked() {
		return l.width
	}
	leftWidth := int(float64(l.width) * l.leftRatio)
	return l.width - leftWidth - 1 // Account for separator
}

// LeftHeight returns the height available for the left pane.
func (l *SplitLayout) LeftHeight() int {
	if l.IsStacked() {
		// In stacked mode, split height 60/40
		return int(float64(l.height) * 0.4)
	}
	return l.height
}

// RightHeight returns the height available for the right pane.
func (l *SplitLayout) RightHeight() int {
	if l.IsStacked() {
		// In stacked mode, give more to the right (chat) pane
		return l.height - l.LeftHeight() - 1 // -1 for separator
	}
	return l.height
}

// IsStacked returns true if the layout should fall back to stacked mode.
func (l *SplitLayout) IsStacked() bool {
	return l.width < l.minWidth
}

// Render combines left and right content into the split layout.
// Left and right should be pre-rendered strings.
func (l *SplitLayout) Render(left, right string) string {
	if l.height <= 0 || l.width <= 0 {
		return ""
	}

	if l.IsStacked() {
		return l.renderStacked(left, right)
	}
	return l.renderHorizontal(left, right)
}

// renderHorizontal renders side-by-side layout.
func (l *SplitLayout) renderHorizontal(left, right string) string {
	leftWidth := l.LeftWidth()
	rightWidth := l.RightWidth()

	// Ensure content fits width and height
	leftLines := ensureSize(left, leftWidth, l.height)
	rightLines := ensureSize(right, rightWidth, l.height)

	leftSplit := strings.Split(leftLines, "\n")
	rightSplit := strings.Split(rightLines, "\n")

	// Separator style
	sepStyle := lipgloss.NewStyle().
		Foreground(ColorBorder)
	sep := sepStyle.Render("│")

	// Join lines horizontally
	var result []string
	for i := 0; i < l.height; i++ {
		leftLine := ""
		rightLine := ""
		if i < len(leftSplit) {
			leftLine = leftSplit[i]
		}
		if i < len(rightSplit) {
			rightLine = rightSplit[i]
		}

		// Pad left line to exact width
		leftLine = padToWidth(leftLine, leftWidth)

		result = append(result, leftLine+" "+sep+" "+rightLine)
	}

	return strings.Join(result, "\n")
}

// renderStacked renders vertically stacked layout.
func (l *SplitLayout) renderStacked(left, right string) string {
	leftHeight := l.LeftHeight()
	rightHeight := l.RightHeight()

	// Ensure content fits
	leftContent := ensureSize(left, l.width, leftHeight)
	rightContent := ensureSize(right, l.width, rightHeight)

	// Separator
	sepStyle := lipgloss.NewStyle().
		Foreground(ColorBorder)
	separator := sepStyle.Render(strings.Repeat("─", l.width))

	return leftContent + "\n" + separator + "\n" + rightContent
}

// ensureSize pads or truncates content to fit exactly width x height.
func ensureSize(content string, width, height int) string {
	lines := strings.Split(content, "\n")

	// Adjust line count
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	// Adjust line widths
	for i, line := range lines {
		lines[i] = padToWidth(line, width)
	}

	return strings.Join(lines, "\n")
}

// padToWidth pads a line to exactly the specified width.
// Truncates if too long, pads with spaces if too short.
func padToWidth(line string, width int) string {
	runeLen := len([]rune(line))
	if runeLen == width {
		return line
	}
	if runeLen > width {
		// Truncate with ellipsis if possible
		runes := []rune(line)
		if width > 3 {
			return string(runes[:width-3]) + "..."
		}
		return string(runes[:width])
	}
	// Pad with spaces
	return line + strings.Repeat(" ", width-runeLen)
}

// RenderWithPanels is a convenience method that renders left and right
// content with panel-style borders.
func (l *SplitLayout) RenderWithPanels(leftTitle, leftContent, rightTitle, rightContent string) string {
	leftPanel := l.renderPanel(leftTitle, leftContent, l.LeftWidth(), l.LeftHeight())
	rightPanel := l.renderPanel(rightTitle, rightContent, l.RightWidth(), l.RightHeight())
	return l.Render(leftPanel, rightPanel)
}

// renderPanel renders content with a title in a panel style.
func (l *SplitLayout) renderPanel(title, content string, width, height int) string {
	if height <= 0 {
		return ""
	}

	var lines []string

	// Title line
	if title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Background(ColorBgDark).
			Padding(0, 1)

		// Render title with background
		titleLine := titleStyle.Render(title)
		lines = append(lines, padToWidth(titleLine, width))
		height-- // Reduce available height
	}

	// Content
	contentLines := strings.Split(content, "\n")
	for i := 0; i < height && i < len(contentLines); i++ {
		lines = append(lines, padToWidth(contentLines[i], width))
	}

	// Pad remaining height
	for len(lines) < height+1 { // +1 for title
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}
