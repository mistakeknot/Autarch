package tui

import "github.com/charmbracelet/lipgloss"

// Color palette (Tokyo Night inspired)
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
)

// Base styles
var (
	// Container styles
	BaseStyle = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorFg)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1)

	PaneFocusedStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(ColorPrimary)

	PaneUnfocusedStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorFgDim)

	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Status styles
	StatusRunning = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true)

	StatusWaiting = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	StatusIdle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StatusError = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	// List item styles
	SelectedStyle = lipgloss.NewStyle().
			Background(ColorBgLight).
			Foreground(ColorFg).
			Bold(true)

	UnselectedStyle = lipgloss.NewStyle().
			Foreground(ColorFgDim)

	// Badge styles
	BadgeStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorPrimary).
			Foreground(ColorBg)

	BadgeClaudeStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(lipgloss.Color("#e07353")).
				Foreground(ColorBg)

	BadgeCodexStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#00D4AA")).
			Foreground(ColorBg)

	BadgeAiderStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#14B8A6")).
			Foreground(ColorBg)

	BadgeCursorStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(lipgloss.Color("#0066FF")).
				Foreground(ColorBg)

	// Help styles
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Tab styles
	TabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorMuted)

	ActiveTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorPrimary).
			Bold(true).
			Underline(true)
)

// StatusIndicator returns a styled status indicator string
func StatusIndicator(status string) string {
	switch status {
	case "running":
		return StatusRunning.Render("● RUNNING")
	case "waiting":
		return StatusWaiting.Render("○ WAITING")
	case "idle":
		return StatusIdle.Render("◌ IDLE")
	case "error":
		return StatusError.Render("✗ ERROR")
	default:
		return StatusIdle.Render("? UNKNOWN")
	}
}

// AgentBadge returns a styled badge for an agent type
func AgentBadge(agentType string) string {
	switch agentType {
	case "claude":
		return BadgeClaudeStyle.Render("Claude")
	case "codex":
		return BadgeCodexStyle.Render("Codex")
	case "aider":
		return BadgeAiderStyle.Render("Aider")
	case "cursor":
		return BadgeCursorStyle.Render("Cursor")
	default:
		return BadgeStyle.Render(agentType)
	}
}
