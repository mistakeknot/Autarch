// Package tui provides shared TUI styles and components for Autarch projects.
package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/pkg/tui/theme"
)

// Bridge variables: these map the old flat Color* constants to the semantic
// theme system. Existing code continues to work unchanged; new code should
// prefer theme.Current().Semantic() directly.
var (
	ColorPrimary   = lipgloss.Color("#7aa2f7") // theme.TokyoNight.Primary
	ColorSecondary = lipgloss.Color("#bb9af7") // theme.TokyoNight.Secondary
	ColorSuccess   = lipgloss.Color("#9ece6a") // theme.TokyoNight.Success
	ColorWarning   = lipgloss.Color("#e0af68") // theme.TokyoNight.Warning
	ColorError     = lipgloss.Color("#f7768e") // theme.TokyoNight.Error
	ColorInfo      = lipgloss.Color("#7dcfff") // theme.TokyoNight.Info (new)
	ColorMuted     = lipgloss.Color("#565f89") // theme.TokyoNight.Overlay
	ColorBg        = lipgloss.Color("#1a1b26") // theme.TokyoNight.Base
	ColorBgDark    = lipgloss.Color("#16161e") // theme.TokyoNight.Mantle
	ColorBgLight   = lipgloss.Color("#24283b") // theme.TokyoNight.Surface0
	ColorBgLighter = lipgloss.Color("#292e42") // theme.TokyoNight.Surface1
	ColorFg        = lipgloss.Color("#c0caf5") // theme.TokyoNight.Text
	ColorFgDim     = lipgloss.Color("#a9b1d6") // theme.TokyoNight.Subtext
	ColorBorder    = lipgloss.Color("#3b4261") // theme.TokyoNight.Surface2

	// Agent-specific colors
	ColorClaude = lipgloss.Color("#e07353") // theme.TokyoNight.Claude
	ColorCodex  = lipgloss.Color("#00D4AA") // theme.TokyoNight.Codex
	ColorAider  = lipgloss.Color("#14B8A6") // theme.TokyoNight.Aider
	ColorCursor = lipgloss.Color("#0066FF") // theme.TokyoNight.Cursor
)

// Ensure the theme package is linked (the bridge comments reference it).
var _ = theme.Default
