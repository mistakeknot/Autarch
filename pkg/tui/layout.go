package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// LayoutMode represents the current layout mode based on terminal width
type LayoutMode int

const (
	// LayoutModeSingle shows only the primary pane (< 50 chars)
	LayoutModeSingle LayoutMode = iota
	// LayoutModeStacked shows panes stacked vertically (50-80 chars)
	LayoutModeStacked
	// LayoutModeDual shows panes side-by-side (> 80 chars)
	LayoutModeDual
)

// Layout breakpoints
const (
	BreakpointSingle  = 50  // Below this: single column
	BreakpointStacked = 80  // Below this: stacked, above: dual
)

// GetLayoutMode returns the appropriate layout mode for the given width
func GetLayoutMode(width int) LayoutMode {
	if width < BreakpointSingle {
		return LayoutModeSingle
	}
	if width < BreakpointStacked {
		return LayoutModeStacked
	}
	return LayoutModeDual
}

// LayoutConfig holds configuration for responsive layouts
type LayoutConfig struct {
	Width       int
	Height      int
	LeftRatio   float64 // Ratio of width for left pane in dual mode (0.0-1.0)
	TopRatio    float64 // Ratio of height for top pane in stacked mode (0.0-1.0)
	GapWidth    int     // Gap between panes
	BorderWidth int     // Width consumed by borders
}

// DefaultLayoutConfig returns sensible defaults
func DefaultLayoutConfig(width, height int) LayoutConfig {
	return LayoutConfig{
		Width:       width,
		Height:      height,
		LeftRatio:   0.5,
		TopRatio:    0.6,
		GapWidth:    2,
		BorderWidth: 2,
	}
}

// RenderDualPane renders two panes side by side
func RenderDualPane(cfg LayoutConfig, leftTitle, leftContent, rightTitle, rightContent string, leftFocused bool) string {
	// Calculate pane widths
	availableWidth := cfg.Width - cfg.GapWidth
	leftWidth := int(float64(availableWidth) * cfg.LeftRatio)
	rightWidth := availableWidth - leftWidth

	// Account for borders
	leftInnerWidth := leftWidth - cfg.BorderWidth
	rightInnerWidth := rightWidth - cfg.BorderWidth

	if leftInnerWidth < 10 {
		leftInnerWidth = 10
	}
	if rightInnerWidth < 10 {
		rightInnerWidth = 10
	}

	// Select styles based on focus
	leftStyle := PaneUnfocusedStyle
	rightStyle := PaneUnfocusedStyle
	if leftFocused {
		leftStyle = PaneFocusedStyle
	} else {
		rightStyle = PaneFocusedStyle
	}

	// Build left pane
	leftLines := []string{TitleStyle.Render(leftTitle)}
	leftLines = append(leftLines, strings.Split(leftContent, "\n")...)
	leftPane := leftStyle.Width(leftWidth).Render(strings.Join(leftLines, "\n"))

	// Build right pane
	rightLines := []string{TitleStyle.Render(rightTitle)}
	rightLines = append(rightLines, strings.Split(rightContent, "\n")...)
	rightPane := rightStyle.Width(rightWidth).Render(strings.Join(rightLines, "\n"))

	// Join horizontally with gap
	gap := strings.Repeat(" ", cfg.GapWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, gap, rightPane)
}

// RenderStackedPane renders two panes stacked vertically
func RenderStackedPane(cfg LayoutConfig, topTitle, topContent, bottomTitle, bottomContent string, topFocused bool) string {
	// Calculate pane heights
	topHeight := int(float64(cfg.Height) * cfg.TopRatio)
	bottomHeight := cfg.Height - topHeight - 1 // -1 for gap

	if topHeight < 3 {
		topHeight = 3
	}
	if bottomHeight < 3 {
		bottomHeight = 3
	}

	// Select styles based on focus
	topStyle := PaneUnfocusedStyle
	bottomStyle := PaneUnfocusedStyle
	if topFocused {
		topStyle = PaneFocusedStyle
	} else {
		bottomStyle = PaneFocusedStyle
	}

	// Build top pane
	topLines := []string{TitleStyle.Render(topTitle)}
	topLines = append(topLines, strings.Split(topContent, "\n")...)
	topPane := topStyle.Width(cfg.Width - cfg.BorderWidth).Height(topHeight).Render(strings.Join(topLines, "\n"))

	// Build bottom pane
	bottomLines := []string{TitleStyle.Render(bottomTitle)}
	bottomLines = append(bottomLines, strings.Split(bottomContent, "\n")...)
	bottomPane := bottomStyle.Width(cfg.Width - cfg.BorderWidth).Height(bottomHeight).Render(strings.Join(bottomLines, "\n"))

	// Join vertically
	return lipgloss.JoinVertical(lipgloss.Left, topPane, bottomPane)
}

// RenderSinglePane renders a single pane
func RenderSinglePane(cfg LayoutConfig, title, content string) string {
	lines := []string{TitleStyle.Render(title)}
	lines = append(lines, strings.Split(content, "\n")...)
	return PaneFocusedStyle.Width(cfg.Width - cfg.BorderWidth).Render(strings.Join(lines, "\n"))
}

// RenderResponsive automatically chooses the best layout for the given width
func RenderResponsive(cfg LayoutConfig, leftTitle, leftContent, rightTitle, rightContent string, leftFocused bool) string {
	mode := GetLayoutMode(cfg.Width)

	switch mode {
	case LayoutModeSingle:
		// Show only the focused pane
		if leftFocused {
			return RenderSinglePane(cfg, leftTitle, leftContent)
		}
		return RenderSinglePane(cfg, rightTitle, rightContent)

	case LayoutModeStacked:
		return RenderStackedPane(cfg, leftTitle, leftContent, rightTitle, rightContent, leftFocused)

	default: // LayoutModeDual
		return RenderDualPane(cfg, leftTitle, leftContent, rightTitle, rightContent, leftFocused)
	}
}

// PadToHeight pads content with empty lines to fill the given height
func PadToHeight(content string, height int) string {
	if height <= 0 {
		return content
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// TruncateWidth truncates each line to the given width
func TruncateWidth(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > width {
			if width <= 3 {
				lines[i] = line[:width]
			} else {
				lines[i] = line[:width-3] + "..."
			}
		}
	}
	return strings.Join(lines, "\n")
}

// WrapText wraps text to the given width
func WrapText(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}

	var lines []string
	for len(text) > width {
		// Find last space before width
		idx := strings.LastIndex(text[:width], " ")
		if idx <= 0 {
			idx = width
		}
		lines = append(lines, text[:idx])
		text = strings.TrimSpace(text[idx:])
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

// CenterText centers text within the given width
func CenterText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}

// Box renders content in a box with a title
func Box(title, content string, width int, style lipgloss.Style) string {
	lines := []string{TitleStyle.Render(title)}
	lines = append(lines, strings.Split(content, "\n")...)
	return style.Width(width).Render(strings.Join(lines, "\n"))
}
