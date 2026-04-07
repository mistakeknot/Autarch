package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// markdownRenderer returns a cached glamour renderer for the given width.
func (p *ChatPanel) markdownRenderer(width int) *glamour.TermRenderer {
	if p.mdRenderer != nil && p.mdWidth == width {
		return p.mdRenderer
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		return nil
	}
	p.mdRenderer = r
	p.mdWidth = width
	return r
}

// renderBuffer renders the live streaming buffer region.
func (p *ChatPanel) renderBuffer(width int) string {
	if p.buffer == nil {
		return ""
	}

	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	var parts []string

	// Render accumulated content if any.
	if p.buffer.Len() > 0 {
		text := p.buffer.String()
		if r := p.markdownRenderer(contentWidth); r != nil {
			rendered, err := r.Render(text)
			if err == nil {
				rendered = strings.TrimSpace(rendered)
				contentStyle := lipgloss.NewStyle().PaddingLeft(2)
				parts = append(parts, contentStyle.Render(rendered))
			} else {
				contentStyle := lipgloss.NewStyle().
					Foreground(ColorFg).
					PaddingLeft(2)
				wrapped := wrapText(text, contentWidth)
				parts = append(parts, contentStyle.Render(wrapped))
			}
		} else {
			contentStyle := lipgloss.NewStyle().
				Foreground(ColorFg).
				PaddingLeft(2)
			wrapped := wrapText(text, contentWidth)
			parts = append(parts, contentStyle.Render(wrapped))
		}
	}

	// Render status indicator.
	if p.streaming && p.status != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			PaddingLeft(2)
		parts = append(parts, statusStyle.Render(p.spinner.View()+" "+p.status))
	}

	return strings.Join(parts, "\n")
}

// renderMessageLines renders a single message into display lines.
// prevRole is the role of the preceding message (for GroupMessages suppression).
func (p *ChatPanel) renderMessageLines(msg ChatMessage, contentWidth int, prevRole string) []string {
	var lines []string
	roleLower := strings.ToLower(msg.Role)
	showRole := roleLower != "system"
	if p.settings.GroupMessages && roleLower == prevRole {
		showRole = false
	}

	// Role header (omit system labels)
	if showRole {
		roleStyle := p.roleStyle(msg.Role)
		lines = append(lines, roleStyle.Render(formatRole(msg.Role)+":"))
	}

	// Content rendering — agent messages get markdown, others get plain text
	if strings.ToLower(msg.Role) == "agent" {
		if r := p.markdownRenderer(contentWidth); r != nil {
			rendered, err := r.Render(msg.Content)
			if err == nil {
				rendered = strings.TrimSpace(rendered)
				contentStyle := lipgloss.NewStyle().PaddingLeft(2)
				lines = append(lines, contentStyle.Render(rendered))
			} else {
				contentStyle := lipgloss.NewStyle().
					Foreground(ColorFg).
					PaddingLeft(2)
				wrapped := wrapText(msg.Content, contentWidth)
				for _, line := range strings.Split(wrapped, "\n") {
					lines = append(lines, contentStyle.Render(line))
				}
			}
		} else {
			contentStyle := lipgloss.NewStyle().
				Foreground(ColorFg).
				PaddingLeft(2)
			wrapped := wrapText(msg.Content, contentWidth)
			for _, line := range strings.Split(wrapped, "\n") {
				lines = append(lines, contentStyle.Render(line))
			}
		}
	} else {
		contentStyle := lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(2)
		wrapped := wrapText(msg.Content, contentWidth)
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, contentStyle.Render(line))
		}
	}
	lines = append(lines, "") // Blank line between messages
	return lines
}

// rebuildHistoryCache re-renders all history messages into the line cache.
func (p *ChatPanel) rebuildHistoryCache(contentWidth int) {
	p.historyLines = make([][]string, len(p.history))
	prevRole := ""
	for i, msg := range p.history {
		p.historyLines[i] = p.renderMessageLines(msg, contentWidth, prevRole)
		prevRole = strings.ToLower(msg.Role)
	}
	p.historyWidth = contentWidth
}

// renderHistory renders the finalized chat history area from the line cache.
func (p *ChatPanel) renderHistory(height int) string {
	if height <= 0 {
		return ""
	}

	if len(p.history) == 0 && p.buffer == nil {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
		return emptyStyle.Render("No messages yet.")
	}

	contentWidth := p.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Rebuild cache if width changed or cache is stale.
	if len(p.historyLines) != len(p.history) || p.historyWidth != contentWidth {
		p.rebuildHistoryCache(contentWidth)
	}

	// Flatten cached lines.
	var lines []string
	for _, msgLines := range p.historyLines {
		lines = append(lines, msgLines...)
	}

	// Apply scrolling - show most recent messages that fit
	if len(lines) > height {
		start := len(lines) - height - p.scroll
		if start < 0 {
			start = 0
		}
		end := start + height
		if end > len(lines) {
			end = len(lines)
			start = end - height
			if start < 0 {
				start = 0
			}
		}
		lines = lines[start:end]
	}

	return strings.Join(lines, "\n")
}

// roleStyle returns the style for a given role.
func (p *ChatPanel) roleStyle(role string) lipgloss.Style {
	switch strings.ToLower(role) {
	case "user":
		return lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
	case "agent":
		return lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)
	case "system":
		return lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
	default:
		return lipgloss.NewStyle().
			Foreground(ColorFg)
	}
}

// formatRole formats the role name for display.
func formatRole(role string) string {
	r := strings.TrimSpace(role)
	if r == "" {
		return "Agent"
	}
	if len(r) == 1 {
		return strings.ToUpper(r)
	}
	return strings.ToUpper(r[:1]) + r[1:]
}

// ensureHeight pads or truncates content to exactly n lines.
func ensureHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// wrapText wraps text to the specified width.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		// Simple word wrap
		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var current string
		for _, word := range words {
			if current == "" {
				current = word
			} else if len(current)+1+len(word) <= width {
				current += " " + word
			} else {
				result = append(result, current)
				current = word
			}
		}
		if current != "" {
			result = append(result, current)
		}
	}
	return strings.Join(result, "\n")
}
