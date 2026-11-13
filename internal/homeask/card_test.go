package homeask

import (
	"context"
	"encoding/json"
	"errors"
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

func TestUrgencyOnlyBlocker(t *testing.T) {
	a := Ask{V: 1, Kind: "decide", Project: "p", ProjectRoot: "/r", Asker: "thread", Thread: "thr_x", Question: "Q?", Urgency: "blocker", Stopped: "Release waits", Options: []Option{{ID: "a", Label: "A", Kind: "ruling-only"}, {ID: "b", Label: "B", Kind: "ruling-only"}}}
	n, err := Normalize(a)
	if err != nil || n.Urgency != "blocker" {
		t.Fatalf("blocker refused: %v", err)
	}
	// urgency never moves a hash: the same ask without it has the same identity, revision and normalized JSON
	b := a
	b.Urgency = ""
	b.Stopped = ""
	if Identity(a) != Identity(b) || Revision(a) != Revision(b) || NormalizedJSON(a) != NormalizedJSON(b) {
		t.Fatal("urgency changed a hash")
	}
	a.Urgency = "soon"
	if _, err := Normalize(a); err == nil {
		t.Fatal("urgency soon accepted")
	}
}

func TestStoppedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/blocker-stopped.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string
		Fields      map[string]any
		Valid       bool
		ReaderValid bool `json:"reader_valid"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			m := map[string]any{"schema": "home-ask/v2", "project": "p", "project_root": "/r", "question": "Q?", "options": []any{map[string]any{"id": "a", "label": "A", "kind": "ruling-only"}, map[string]any{"id": "b", "label": "B", "kind": "ruling-only"}}}
			for k, v := range c.Fields {
				m[k] = v
			}
			b, _ := json.Marshal(m)
			_, err := ParseCard("Blocks: bead:example\nRequest: example sha256:0123456789abcdef\n\n```home-ask\n" + string(b) + "\n```\n")
			if (err == nil) != c.ReaderValid {
				t.Fatalf("reader valid=%v err=%v", c.ReaderValid, err)
			}
			delete(m, "schema")
			m["v"] = 1
			m["kind"] = "decide"
			m["asker"] = "thread"
			m["thread"] = "t"
			b, _ = json.Marshal(m)
			_, err = Parse(b)
			if (err == nil) != c.Valid {
				t.Fatalf("v1 valid=%v err=%v", c.Valid, err)
			}

		})
	}
}

// Design choice in bead mk-ddtv5.7.9: stopped participates in filer H1, as urgency does.
// The normalized ask hashes remain independent of this delivery metadata.
func TestStoppedRequestIdentity(t *testing.T) {
	legacy := testReq()
	h, err := RequestIdentity(legacy)
	if err != nil || h != "d978b44d835e92ce" {
		t.Fatalf("legacy H1 changed: %q %v", h, err)
	}
	a, b := testReq(), testReq()
	a.Ask["urgency"] = "blocker"
	a.Ask["stopped"] = "Release waits"
	b.Ask["urgency"] = "blocker"
	b.Ask["stopped"] = "Tests wait"
	x, err := RequestIdentity(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := RequestIdentity(b)
	if err != nil {
		t.Fatal(err)
	}
	if x == y {
		t.Fatal("stopped must affect H1")
	}
	av, err := (Card{Ask: a.Ask}).ToV1("thr_a")
	if err != nil {
		t.Fatal(err)
	}
	bv, err := (Card{Ask: b.Ask}).ToV1("thr_a")
	if err != nil {
		t.Fatal(err)
	}
	if Revision(av) != Revision(bv) || SemanticKey(av) != SemanticKey(bv) || NormalizedJSON(av) != NormalizedJSON(bv) {
		t.Fatal("stopped affected normalized hashes")
	}
}

func TestNewCardFilerRequiresStopped(t *testing.T) {
	for _, lane := range []string{"exact", "update"} {
		t.Run(lane, func(t *testing.T) {
			f := newFake(t, registryOnCreate)
			req := testReq()
			req.Ask["urgency"] = "blocker"
			var err error
			if lane == "exact" {
				_, err = f.filer().FileCard(context.Background(), req)
			} else {
				req.Ask["ask_key"] = "release"
				_, err = f.filer().FileOrUpdateCard(context.Background(), req, "")
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "stopped") {
				t.Fatalf("want named missing field: %v", err)
			}
			if len(f.calls) != 0 {
				t.Fatalf("contacted Home: %v", f.calls)
			}
		})
	}
}
