package tui

import tea "github.com/charmbracelet/bubbletea"

type Intent struct{}

type Screen interface {
	Update(tea.Msg, *SharedState) (Screen, Intent)
	View(*SharedState) string
	Title() string
}
