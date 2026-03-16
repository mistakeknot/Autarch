package theme

import (
	"github.com/charmbracelet/lipgloss"
	masaqtheme "github.com/mistakeknot/Masaq/theme"
)

// SyncToMasaq maps the current Autarch theme's colors to Masaq's global
// theme so that Masaq components (tabbar, viewport, spinner, etc.) render
// with the correct palette. Call this once at startup and again on theme change.
func SyncToMasaq() {
	t := Current()
	masaqtheme.SetCurrent(masaqtheme.NewTheme("autarch", masaqtheme.SemanticColors{
		Primary:    pair(t.Primary),
		Secondary:  pair(t.Secondary),
		Success:    pair(t.Success),
		Warning:    pair(t.Warning),
		Error:      pair(t.Error),
		Info:       pair(t.Info),
		Active:     pair(t.Sky),
		Muted:      pair(t.Overlay),
		Bg:         pair(t.Base),
		BgDark:     pair(t.Mantle),
		BgLight:    pair(t.Surface0),
		Fg:         pair(t.Text),
		FgDim:      pair(t.Subtext),
		Border:     pair(t.Surface2),
		DiffAdd:    pair(t.Green),
		DiffRemove: pair(t.Red),
		DiffContext: pair(t.Surface1),
	}))
}

// pair wraps a lipgloss.Color into a Masaq ColorPair. Since Autarch's Theme
// struct uses flat lipgloss.Color (no dark/light split), we use the same
// value for both modes. Autarch handles light/dark at the theme selection
// level (CatppuccinLatte vs TokyoNight), not at the color pair level.
func pair(c lipgloss.Color) masaqtheme.ColorPair {
	s := string(c)
	return masaqtheme.ColorPair{Dark: s, Light: s}
}
