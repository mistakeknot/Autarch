package homeask

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOnPickRawBoundary(t *testing.T) {
	declaration := `{"schema":1,"target_thread":"thr_target","reversible":true,"subject":{"kind":"none"},"ruling":{"bead":null,"under":[]}}`
	for _, tc := range []struct{ name, text, extra string }{
		{"approval null", `"Use blue."`, `,"approval":null`},
		{"surrogate", `"Use \ud800."`, ""},
		{"line separator", `"force\u2028push"`, ""},
		{"paragraph separator", `"force\u2029push"`, ""},
		{"vertical tab", `"force\u000bpush"`, ""},
		{"BOM", `"force\ufeffpush"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := `{"schema":"home-ask/v2","project":"sample","project_root":"/sample","question":"Colour?","options":[{"id":"blue","label":"Blue","instruction":` + tc.text + `,"on_pick":` + declaration + tc.extra + `},{"id":"wait","label":"Wait"}]}`
			body := "Colour?\nRequest: sample sha256:0123456789abcdef\n```home-ask\n" + wire + "\n```"
			if _, err := ParseCard(body); err == nil {
				t.Fatal("accepted unsafe raw card")
			}
			if _, err := ParseCard(strings.ReplaceAll(body, `"on_pick"`, `"on_\u0070ick"`)); err == nil {
				t.Fatal("accepted escaped-key unsafe raw card")
			}
		})
	}
	for _, sep := range []string{"\t", "\n", "\r", "\v", "\f", " ", "\u00a0", "\u1680", "\u2000", "\u2028", "\u2029", "\u202f", "\u205f", "\u3000", "\ufeff"} {
		if _, err := ValidateOnPick(json.RawMessage(declaration), Option{Label: "Blue", Instruction: "force" + sep + "push"}, CardValidationContext{Source: "card"}); err == nil {
			t.Errorf("accepted separator %q", sep)
		}
	}
}

func TestOnPickRootRunExcludedAtParse(t *testing.T) {
	body := "Colour?\nRequest: sample sha256:0123456789abcdef\n```home-ask\n" + `{"schema":"home-ask/v2","project":"sample","project_root":"/sample","question":"Colour?","options":[{"id":"blue","label":"Blue","instruction":"Use blue.","on_pick":{"schema":1,"target_thread":"thr_target","reversible":true,"subject":{"kind":"none"},"ruling":{"bead":null,"under":[]}}},{"id":"wait","label":"Wait"}]}` + "\n```\n```root-run\nscript: /sample/check\nsha256: " + strings.Repeat("a", 64) + "\ntimeout: 30\nset: check\n```"
	if _, err := ParseCard(body); err == nil || !strings.Contains(err.Error(), "on_pick") {
		t.Fatalf("expected structural refusal, got %v", err)
	}
}

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
