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
	wantCode(t, e.autarch(fileArgs(ask)...), 5, "already ruled", card)
	if len(e.st().Tasks) != 1 || e.st().Tasks[0].Desc != before {
		t.Fatal("refiling picked card changed board contents")
	}
	if len(e.callsWith("tasks", "update")) != 0 || len(e.callsWith("tasks", "create")) != 1 {
		t.Fatal("refiling picked card attempted a board write")
	}
}
