package tui

import (
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

var (
	BaseStyle     = shared.BaseStyle
	PanelStyle    = shared.PanelStyle
	TitleStyle    = shared.TitleStyle
	SubtitleStyle = shared.SubtitleStyle
	LabelStyle    = shared.LabelStyle

	SelectedStyle   = shared.SelectedStyle
	UnselectedStyle = shared.UnselectedStyle

	HelpKeyStyle  = shared.HelpKeyStyle
	HelpDescStyle = shared.HelpDescStyle

	TabStyle       = shared.TabStyle
	ActiveTabStyle = shared.ActiveTabStyle

	StatusRunningStyle = shared.StatusRunning
	StatusWaitingStyle = shared.StatusWaiting
	StatusIdleStyle    = shared.StatusIdle
	StatusErrorStyle   = shared.StatusError

	// Use shared pane styles
	PaneFocusedStyle   = shared.PaneFocusedStyle
	PaneUnfocusedStyle = shared.PaneUnfocusedStyle
)

// StatusSymbol returns just the symbol for a status (re-exported from shared)
var StatusSymbol = shared.StatusSymbol
