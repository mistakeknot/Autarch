// Package tui provides shared TUI styles and components for Vauxpraudemonium projects.
package tui

import "github.com/charmbracelet/lipgloss"

// Tokyo Night inspired color palette - shared across all projects
var (
	ColorPrimary   = lipgloss.Color("#7aa2f7") // Blue
	ColorSecondary = lipgloss.Color("#bb9af7") // Purple
	ColorSuccess   = lipgloss.Color("#9ece6a") // Green
	ColorWarning   = lipgloss.Color("#e0af68") // Yellow
	ColorError     = lipgloss.Color("#f7768e") // Red
	ColorMuted     = lipgloss.Color("#565f89") // Gray
	ColorBg        = lipgloss.Color("#1a1b26") // Dark background
	ColorBgLight   = lipgloss.Color("#24283b") // Lighter background
	ColorFg        = lipgloss.Color("#c0caf5") // Foreground
	ColorFgDim     = lipgloss.Color("#a9b1d6") // Dimmed foreground

	// Agent-specific colors
	ColorClaude = lipgloss.Color("#e07353") // Orange/coral for Claude
	ColorCodex  = lipgloss.Color("#00D4AA") // Teal for Codex
	ColorAider  = lipgloss.Color("#14B8A6") // Cyan-teal for Aider
	ColorCursor = lipgloss.Color("#0066FF") // Blue for Cursor
)
