package orchcompat

import (
	"strings"
	"testing"
)

func TestFileAlreadyPickedCardExitsFive(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(map[string]any{"ask_key": "picked-card"}))
	first := e.autarch(fileArgs(ask)...)
	wantCode(t, first, 0)
	card := parseFiled(t, first).Card
	e.update(func(s *fbState) {
		s.Fail = []fbFail{{Prefix: "home get --card", Times: 1, Stdout: fbJSON(map[string]any{"task_id": card, "project_id": "P1", "state": "open", "decision_state": "picked"})}}
	})
	before := e.st().Tasks[0].Desc
	setupCalls := len(e.calls())
	for _, call := range e.calls()[:setupCalls] {
		if len(call.Args) >= 2 && call.Args[0] == "tasks" && call.Args[1] == "update" {
			t.Errorf("first filing attempted a board update: %q", call.Args)
		}
	}
	wantCode(t, e.autarch(fileArgs(ask)...), 5, "already ruled", card)
	if len(e.st().Tasks) != 1 || e.st().Tasks[0].Desc != before {
		t.Fatal("refiling picked card changed board contents")
	}
	for _, call := range e.calls()[setupCalls:] {
		args := call.Args
		// Allow known reads explicitly, so writes under any command are caught.
		if len(args) >= 2 {
			switch strings.Join(args[:2], " ") {
			case "tasks list", "tasks show", "thread show", "thread list", "home binding":
				continue
			}
		}
		if len(args) >= 3 {
			switch strings.Join(args[:3], " ") {
			case "tasks label list", "tasks project list", "home get --request", "home get --card":
				continue
			}
		}
		t.Errorf("refiling picked card attempted a board write: %q", args)
	}
}
