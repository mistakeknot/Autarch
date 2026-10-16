package tui

import "github.com/charmbracelet/lipgloss"

// Base styles - shared across all projects
var (
	// Container styles
	BaseStyle = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorFg)

	// Full screen container - fills entire terminal with background
	FullScreenStyle = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorFg)

	// Content container with padding
	ContentStyle = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorFg).
			Padding(1, 3)

	// Card/Panel style with border and background
	CardStyle = lipgloss.NewStyle().
			Background(ColorBgLight).
			Foreground(ColorFg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(1, 2)

	// Focused card
	CardFocusedStyle = lipgloss.NewStyle().
				Background(ColorBgLight).
				Foreground(ColorFg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(1, 2)

	// Header bar style
	HeaderStyle = lipgloss.NewStyle().
			Background(ColorBgDark).
			Foreground(ColorFg).
			Padding(1, 3).
			Bold(true)

	// Footer bar style
	FooterStyle = lipgloss.NewStyle().
			Background(ColorBgDark).
			Foreground(ColorMuted).
			Padding(1, 3)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1)

	// Pane focus styles - for two-pane layouts
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

	// Badge base style
	BadgeStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorPrimary).
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

// Status badge styles for PRD/spec status indicators
var (
	StatusDraftBadge = lipgloss.NewStyle().
				Foreground(ColorWarning)

	StatusActiveBadge = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	StatusDoneBadge = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StatusArchivedBadge = lipgloss.NewStyle().
				Foreground(ColorMuted)
)

// Agent badge styles
var (
	BadgeClaudeStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(ColorClaude).
				Foreground(ColorBg)

	BadgeCodexStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorCodex).
			Foreground(ColorBg)

	BadgeAiderStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorAider).
			Foreground(ColorBg)

	BadgeCursorStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(ColorCursor).
				Foreground(ColorBg)
)
