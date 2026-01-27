package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	sharedtui "github.com/mistakeknot/autarch/pkg/tui"
)

const (
	layoutBreakpointSingle  = 50
	layoutBreakpointStacked = 80
)

const (
	LayoutModeSingle  = "single"
	LayoutModeStacked = "stacked"
	LayoutModeDual    = "dual"
)

func layoutMode(width int) string {
	switch {
	case width < layoutBreakpointSingle:
		return LayoutModeSingle
	case width < layoutBreakpointStacked:
		return LayoutModeStacked
	default:
		return LayoutModeDual
	}
}

func renderFrame(header, body, footer string) string {
	return strings.Join([]string{header, body, footer}, "\n")
}

func renderSplitView(width int, left, right []string) string {
	if width < 100 {
		return strings.Join(left, "\n")
	}
	return joinColumns(left, right, 42)
}

func ensureExactHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	} else if len(lines) < n {
		for len(lines) < n {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func ensureExactWidth(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		lineWidth := visibleWidth(line)
		if lineWidth == width {
			result[i] = line
			continue
		}
		if lineWidth < width {
			result[i] = line + strings.Repeat(" ", width-lineWidth)
			continue
		}
		cut := width - 3
		if cut < 0 {
			cut = 0
		}
		truncated := line
		if cut < len(line) {
			truncated = line[:cut] + "..."
		}
		result[i] = padRight(truncated, width)
	}
	return strings.Join(result, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderDualColumnLayout(leftTitle, leftContent, rightTitle, rightContent string, width, height int) string {
	return renderDualColumnLayoutFocused(leftTitle, leftContent, rightTitle, rightContent, width, height, "LIST")
}

func renderDualColumnLayoutFocused(leftTitle, leftContent, rightTitle, rightContent string, width, height int, focus string) string {
	if height <= 0 {
		return ""
	}
	leftWidth := int(float64(width) * 0.35)
	rightWidth := width - leftWidth - 3
	if rightWidth < 1 {
		rightWidth = 1
	}
	panelTitleLines := 2
	panelContentHeight := height - panelTitleLines
	if panelContentHeight < 1 {
		panelContentHeight = 1
	}

	leftPanel := renderPanelTitle(leftTitle, leftWidth) + "\n" + ensureExactHeight(leftContent, panelContentHeight)
	rightPanel := renderPanelTitle(rightTitle, rightWidth) + "\n" + ensureExactHeight(rightContent, panelContentHeight)

	// Use focused/unfocused borders matching Vauxhall
	leftStyle := sharedtui.PaneUnfocusedStyle
	rightStyle := sharedtui.PaneUnfocusedStyle
	if focus == "LIST" {
		leftStyle = sharedtui.PaneFocusedStyle
	} else {
		rightStyle = sharedtui.PaneFocusedStyle
	}
	leftPanel = leftStyle.Width(leftWidth).Height(height).Render(leftPanel)
	rightPanel = rightStyle.Width(rightWidth).Height(height).Render(rightPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

func renderStackedLayout(listTitle, listContent, detailTitle, detailContent string, width, height int) string {
	if height <= 0 {
		return ""
	}
	listHeight := (height * 60) / 100
	previewHeight := height - listHeight - 1
	if listHeight < 3 {
		listHeight = 3
	}
	if previewHeight < 3 {
		previewHeight = 3
	}
	listPanel := renderPanelTitle(listTitle, width) + "\n" + ensureExactHeight(listContent, listHeight-2)
	previewPanel := renderPanelTitle(detailTitle, width) + "\n" + ensureExactHeight(detailContent, previewHeight-2)
	listPanel = stylePanel(listPanel, width, listHeight)
	previewPanel = stylePanel(previewPanel, width, previewHeight)
	return listPanel + "\n" + previewPanel
}

func renderSingleColumnLayout(listTitle, listContent string, width, height int) string {
	if height <= 0 {
		return ""
	}
	listPanel := renderPanelTitle(listTitle, width) + "\n" + ensureExactHeight(listContent, height-2)
	return stylePanel(listPanel, width, height)
}

func joinHorizontal(left, separator, right string, height int) string {
	if height <= 0 {
		return ""
	}
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	sepLines := strings.Split(separator, "\n")
	maxLines := height
	if len(leftLines) < maxLines {
		for len(leftLines) < maxLines {
			leftLines = append(leftLines, "")
		}
	}
	if len(rightLines) < maxLines {
		for len(rightLines) < maxLines {
			rightLines = append(rightLines, "")
		}
	}
	if len(sepLines) < maxLines {
		for len(sepLines) < maxLines {
			sepLines = append(sepLines, " │ ")
		}
	}
	var b strings.Builder
	for i := 0; i < maxLines; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(sepLines[i])
		b.WriteString(rightLines[i])
		if i < maxLines-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func stylePanel(content string, width, height int) string {
	style := sharedtui.PanelStyle.Copy()
	if width > 0 {
		style = style.Width(width)
	}
	if height > 0 {
		style = style.Height(height)
	}
	return style.Render(content)
}
