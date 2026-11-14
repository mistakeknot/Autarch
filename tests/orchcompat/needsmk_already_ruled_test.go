package orchcompat

import "testing"

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
	wantCode(t, e.autarch(fileArgs(ask)...), 5, "already ruled", card)
	if len(e.st().Tasks) != 1 || e.st().Tasks[0].Desc != before {
		t.Fatal("refiling picked card changed board contents")
	}
	for _, call := range e.calls()[setupCalls:] {
		args := call.Args
		if len(args) < 2 || args[0] != "tasks" {
			continue
		}
		// Only known board reads are allowed after setup. This also catches
		// comment, label create, and any future board write verbs.
		if args[1] == "list" || args[1] == "show" {
			continue
		}
		if len(args) >= 3 && (args[1] == "label" || args[1] == "project") && args[2] == "list" {
			continue
		}
		t.Errorf("refiling picked card attempted a board write: %q", args)
	}
}
