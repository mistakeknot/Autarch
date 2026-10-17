package homeask

import (
	"strings"
	"testing"
)

const twinRevision = "c48f77fe103ee0817d037641b85b083c05d9ba3ed55767e1e75e3c5764cc8e1e"

const questionsAsk = `{"v":1,"kind":"decide","asker":"thread","thread":"thr_x","project":"p","project_root":"/tmp/p","question":"Pick the shape",
"questions":[
 {"id":"ui","prompt":"Terminal or web?","recommendation":"term","options":[{"id":"term","label":"Terminal","detail":"fast"},{"id":"web","label":"Web"}]},
 {"id":"pace","prompt":"Pace?","recommendation":"turn","options":[{"id":"turn","label":"Turn-based"},{"id":"live","label":"Real-time"}]}]}`

func TestQuestionsParse(t *testing.T) {
	a, err := Parse([]byte(questionsAsk))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Questions) != 2 || a.Questions[0].Options[0].Detail != "fast" {
		t.Fatalf("questions lost: %+v", a.Questions)
	}
	// The identity of an ask without questions is unchanged by the new field.
	plain := `{"v":1,"kind":"decide","asker":"thread","thread":"thr_x","project":"p","project_root":"/tmp/p","question":"Q?","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`
	if _, err := Parse([]byte(plain)); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionsRevisionDiffers(t *testing.T) {
	a, _ := Parse([]byte(questionsAsk))
	b, _ := Parse([]byte(strings.Replace(questionsAsk, `"recommendation":"turn"`, `"recommendation":"live"`, 1)))
	if Revision(a) == Revision(b) {
		t.Fatal("a different recommendation must change the revision")
	}
	// The TypeScript twin pins the same value (multi-pick.test.tsx).
	if got := Revision(a); got != twinRevision {
		t.Fatalf("revision %s, twin %s", got, twinRevision)
	}
}

func TestQuestionsRejected(t *testing.T) {
	cases := map[string]string{
		"dup id":    strings.Replace(questionsAsk, `"id":"pace"`, `"id":"ui"`, 1),
		"bad rec":   strings.Replace(questionsAsk, `"recommendation":"turn"`, `"recommendation":"nope"`, 1),
		"one opt":   strings.Replace(questionsAsk, `,{"id":"web","label":"Web"}`, ``, 1),
		"top recom": strings.Replace(questionsAsk, `"question":"Pick the shape",`, `"question":"Pick the shape","recommendation":"a",`, 1),
		"steps":     strings.Replace(strings.Replace(questionsAsk, `"kind":"decide"`, `"kind":"steps"`, 1), `"questions"`, `"steps":["x"],"questions"`, 1),
	}
	for name, raw := range cases {
		_, err := Parse([]byte(raw))
		if err == nil {
			t.Errorf("%s: want error", name)
		} else if strings.Contains(err.Error(), "project") {
			t.Errorf("%s: rejected for the wrong reason: %v", name, err)
		}
	}
}

func TestQuestionsCardV2(t *testing.T) {
	body := `{"schema":"home-ask/v2","project":"p","project_root":"/tmp/p","question":"Q","questions":[{"id":"ui","prompt":"P","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}]}`
	c := Card{Ask: nil}
	if err := parseWireAsk(body, &c); err != nil {
		t.Fatal(err)
	}
	a, err := c.ToV1("thr_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Questions) != 1 {
		t.Fatal("questions lost through ToV1")
	}
}
