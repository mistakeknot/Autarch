package homeask

import (
	"strings"
	"testing"
)

func TestOnPickCardPreservesDeclarationAndRejectsV1(t *testing.T) {
	body := "Which colour?\nBlocks: thread:thr_target\nRequest: sample sha256:0123456789abcdef\n```home-ask\n" + `{"schema":"home-ask/v2","project":"sample","project_root":"/sample","question":"Which colour?","options":[{"id":"blue","label":"Blue","kind":"instruction","instruction":"Use blue 🟦.","reversible":true,"on_pick":{"schema":1,"target_thread":"thr_target","reversible":false,"subject":{"kind":"none"},"ruling":{"bead":null,"under":[]}}},{"id":"wait","label":"Wait","kind":"ruling-only"}]}` + "\n```"
	c, err := ParseCard(body)
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.ToV1("thr_asker")
	if err != nil {
		t.Fatal(err)
	}
	raw := NormalizedJSON(a)
	if !strings.Contains(raw, `"on_pick"`) {
		t.Fatal("declaration lost")
	}
	if !a.Options[0].Reversible || a.Options[0].Instruction != "Use blue 🟦." {
		t.Fatal("delegation or relay text changed")
	}
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("public v1 accepted card declaration")
	}
	if _, err := parseExistingAsk([]byte(raw)); err == nil {
		t.Fatal("retained v1 accepted card declaration")
	}
}
