package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderRunStatusBadge returns a styled status badge string.
// Accepts a plain status string — no domain type dependency.
func RenderRunStatusBadge(status string) string {
	var color lipgloss.Color
	switch status {
	case "active":
		color = ColorSuccess
	case "cancelled":
		color = ColorError
	case "completed":
		color = ColorInfo
	default:
		color = ColorMuted
	}
	return lipgloss.NewStyle().
		Foreground(ColorBg).
		Background(color).
		Padding(0, 1).
		Render(strings.ToUpper(status))
}

// FormatTokens formats a token count for display (e.g., "12.3K", "1.5M").
func FormatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
