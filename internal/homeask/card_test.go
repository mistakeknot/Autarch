package homeask

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The card vectors are shared with the TypeScript parser
// (integrations/bb-plugin-autarch/cards.ts): both sides read the same files.
const cardFixtureDir = "../../integrations/bb-plugin-autarch/__tests__/fixtures/cards"

type cardCase struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Expect      map[string]any `json:"expect"`
}

func loadCardFixtures(t *testing.T, kind string) map[string][]json.RawMessage {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.FromSlash(cardFixtureDir), "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no card fixtures: %v", err)
	}
	out := map[string][]json.RawMessage{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Kind  string            `json:"kind"`
			Cases []json.RawMessage `json:"cases"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if doc.Kind == kind {
			out[filepath.Base(f)] = doc.Cases
		}
	}
	return out
}

// generic round-trips v through JSON so it compares equal to the decoded fixture.
func generic(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCardVectors(t *testing.T) {
	files := loadCardFixtures(t, "card")
	total := 0
	for file, cases := range files {
		for _, raw := range cases {
			var c cardCase
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			total++
			t.Run(file+"/"+c.Name, func(t *testing.T) {
				card, err := ParseCard(c.Description)
				if want, bad := c.Expect["error"].(string); bad {
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("got err %v, want one containing %q", err, want)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, k := range []string{"question", "blocks", "request", "pull", "root_run"} {
					var got any
					switch k {
					case "question":
						got = card.Question
					case "blocks":
						got = card.Blocks
					case "request":
						got = card.Request
					case "pull":
						got = card.Pull
					case "root_run":
						got = card.RootRun
					}
					if !reflect.DeepEqual(generic(t, got), c.Expect[k]) {
						t.Errorf("%s: got %v want %v", k, generic(t, got), c.Expect[k])
					}
				}
				v1s, _ := c.Expect["v1"].([]any)
				for _, e := range v1s {
					want := e.(map[string]any)
					a, err := card.ToV1(want["thread"].(string))
					if err != nil {
						t.Fatalf("ToV1: %v", err)
					}
					if a.Asker != want["asker"] || a.Kind != "decide" || a.Supersedes != "" {
						t.Errorf("v1 shape: %+v", a)
					}
					// The output passes the unchanged rev-4 parser, and re-parsing is stable.
					raw, _ := json.Marshal(a)
					again, err := Parse(raw)
					if err != nil {
						t.Fatalf("rev-4 Parse refused toV1 output: %v", err)
					}
					if NormalizedJSON(again) != NormalizedJSON(a) {
						t.Errorf("toV1 output is not a Parse fixed point")
					}
					if Identity(a) != want["identity"] || Revision(a) != want["revision"] || SemanticKey(a) != want["semantic_key"] {
						t.Errorf("hashes: got %s %s %s want %v %v %v", Identity(a), Revision(a), SemanticKey(a), want["identity"], want["revision"], want["semantic_key"])
					}
				}
			})
		}
	}
	if total < 60 {
		t.Fatalf("only %d card vectors", total)
	}
}

func TestCardToV1PullIgnoresThread(t *testing.T) {
	c := Card{Pull: "mycroft", Ask: map[string]any{"project": "p", "project_root": "/r", "question": "q",
		"options": []any{map[string]any{"id": "a", "label": "A", "kind": "ruling-only"}, map[string]any{"id": "b", "label": "B", "kind": "ruling-only"}}}}
	a, err := c.ToV1("thr_x")
	if err != nil || a.Asker != "mycroft" || a.Thread != "" {
		t.Fatalf("got %+v %v", a, err)
	}
	if _, err := (Card{Ask: c.Ask}).ToV1(""); err == nil {
		t.Fatal("a thread ask with no thread must be refused")
	}
}

func TestAskingThreadVectors(t *testing.T) {
	files := loadCardFixtures(t, "asking-thread")
	n := 0
	for _, cases := range files {
		for _, raw := range cases {
			var c struct {
				Name     string    `json:"name"`
				Comments []Comment `json:"comments"`
				Expect   string    `json:"expect"`
			}
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			n++
			before := generic(t, c.Comments)
			if got := AskingThread(c.Comments); got != c.Expect {
				t.Errorf("%s: got %q want %q", c.Name, got, c.Expect)
			}
			if !reflect.DeepEqual(before, generic(t, c.Comments)) {
				t.Errorf("%s: AskingThread reordered its input", c.Name)
			}
		}
	}
	if n < 6 {
		t.Fatalf("only %d askingThread vectors", n)
	}
}
