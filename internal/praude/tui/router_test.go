package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type dummyScreen struct {
	title string
}

func (d dummyScreen) Update(msg tea.Msg, state *SharedState) (Screen, Intent) {
	return d, Intent{}
}

func (d dummyScreen) View(state *SharedState) string {
	return d.title
}

func (d dummyScreen) Title() string {
	return d.title
}

func TestRouterDispatchesToActiveScreen(t *testing.T) {
	list := dummyScreen{title: "List"}
	help := dummyScreen{title: "Help"}
	r := NewRouter(map[string]Screen{"list": list, "help": help}, "list")
	if r.ActiveName() != "list" {
		t.Fatalf("expected list")
	}
	r.Switch("help")
	if r.ActiveName() != "help" {
		t.Fatalf("expected help")
	}
}
