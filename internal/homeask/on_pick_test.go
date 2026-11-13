package homeask

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestOnPickNumberSpellingAndTargetIdentity(t *testing.T) {
	for _, spelling := range []string{"1", "1.0", "1e0"} {
		t.Run(spelling, func(t *testing.T) {
			var ids []string
			for _, target := range []string{"thr_a", "thr_b"} {
				r := pickRequest()
				d := `{"schema":` + spelling + `,"target_thread":"` + target + `","reversible":true,"subject":{"kind":"none"},"ruling":{"bead":null,"under":[]}}`
				o := r.Ask["options"].([]any)[0].(map[string]any)
				o["on_pick"] = json.RawMessage(d)
				_, err := ValidateOnPick(json.RawMessage(d), Option{Label: "Blue", Instruction: "Use blue."}, CardValidationContext{Source: "card"})
				if (err == nil) != (spelling == "1") {
					t.Errorf("direct validation spelling %s: %v", spelling, err)
				}
				// Build the raw card without passing through the filing serializer.
				wireAsk := map[string]any{"schema": "home-ask/v2", "project": "sample", "project_root": "/sample", "question": "Colour?", "options": r.Ask["options"]}
				wire, err := json.Marshal(wireAsk)
				if err != nil {
					t.Fatal(err)
				}
				body := "Colour?\nRequest: sample sha256:0123456789abcdef\n```home-ask\n" + string(wire) + "\n```"
				validBody := strings.Replace(body, `"schema":`+spelling+`,`, `"schema":1,`, 1)
				validCard, err := ParseCard(validBody)
				if err != nil {
					t.Fatal(err)
				}
				validAsk, err := validCard.ToV1("thr_asker")
				if err != nil {
					t.Fatal(err)
				}
				stored := strings.Replace(NormalizedJSON(validAsk), `"schema":1,`, `"schema":`+spelling+`,`, 1)
				for _, escaped := range []bool{false, true} {
					candidate := stored
					if escaped {
						candidate = strings.ReplaceAll(strings.ReplaceAll(stored, `"on_pick"`, `"on_\u0070ick"`), `"schema":`+spelling, `"sch\u0065ma":`+spelling)
					}
					if _, err := parseAskSource([]byte(candidate), false, true); (err == nil) != (spelling == "1") {
						t.Errorf("stored spelling %s escaped=%t: %v", spelling, escaped, err)
					}
				}
				for _, escaped := range []bool{false, true} {
					candidate := body
					if escaped {
						candidate = strings.ReplaceAll(strings.ReplaceAll(body, `"on_pick"`, `"on_\u0070ick"`), `"schema":`+spelling, `"sch\u0065ma":`+spelling)
					}
					card, err := ParseCard(candidate)
					if spelling != "1" {
						if err == nil {
							ask, e := card.ToV1("thr_asker")
							if e == nil {
								ids = append(ids, ask.RequestID)
							}
							t.Errorf("accepted spelling %s escaped=%t", spelling, escaped)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						ask, err := card.ToV1("thr_asker")
						if err != nil {
							t.Fatal(err)
						}
						if !escaped {
							ids = append(ids, ask.RequestID)
						}
					}
				}
				if spelling != "1" {
					for _, update := range []bool{false, true} {
						calls := 0
						f := &CardFiler{Run: func(context.Context, []string, ...string) BBResult { calls++; return BBResult{Code: 1} }}
						if update {
							r.Ask["ask_key"] = "palette"
							_, err = f.FileOrUpdateCard(context.Background(), r, "")
						} else {
							_, err = f.FileCard(context.Background(), r)
						}
						if err == nil || calls != 0 {
							t.Errorf("filing update=%t: err=%v calls=%d", update, err, calls)
						}
					}
				}
			}
			if spelling == "1" && ids[0] == ids[1] {
				t.Fatal("different valid targets share request_id")
			}
			if spelling != "1" && len(ids) > 2 && ids[0] == ids[2] {
				t.Errorf("invalid spelling erased target: collision %s", ids[0])
			}
		})
	}
}

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

func TestOnPickPRNumberBoundaries(t *testing.T) {
	for _, tc := range []struct {
		spelling string
		valid    bool
	}{
		{"7", true}, {"9007199254740991", true}, {"7.0", false}, {"7e0", false},
		{"0", false}, {"-1", false}, {"9007199254740992", false},
	} {
		t.Run(tc.spelling, func(t *testing.T) {
			d := `{"schema":1,"target_thread":"thr_target","reversible":true,"subject":{"kind":"github-pr","repo":"example/sample","number":` + tc.spelling + `,"head":"` + strings.Repeat("a", 40) + `"},"ruling":{"bead":null,"under":[]}}`
			_, err := ValidateOnPick(json.RawMessage(d), Option{Label: "Blue", Instruction: "Use blue."}, CardValidationContext{Source: "card"})
			if (err == nil) != tc.valid {
				t.Fatalf("direct validation: %v", err)
			}
			raw := `{"schema":"home-ask/v2","project":"sample","project_root":"/sample","question":"Colour?","options":[{"id":"blue","label":"Blue","instruction":"Use blue.","on_pick":` + d + `},{"id":"wait","label":"Wait"}]}`
			for _, escaped := range []bool{false, true} {
				candidate := raw
				if escaped {
					candidate = strings.ReplaceAll(strings.ReplaceAll(raw, `"on_pick"`, `"on_\u0070ick"`), `"number"`, `"numb\u0065r"`)
				}
				body := "Colour?\nRequest: sample sha256:0123456789abcdef\n```home-ask\n" + candidate + "\n```"
				if _, err := ParseCard(body); (err == nil) != tc.valid {
					t.Fatalf("card escaped=%t: %v", escaped, err)
				}
			}
		})
	}
}

func TestOnPickCanonicalConversionCannotEraseDeclaration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("invalid internal declaration silently hashed")
		}
	}()
	optionNode(Option{OnPick: json.RawMessage(`{"schema":1.0}`)})
}
