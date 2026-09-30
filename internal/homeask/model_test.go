package homeask

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func base() Ask {
	return Ask{
		V: 1, Kind: "decide",
		Subject: "autarch/catch-up: collapse order",
		Project: "Autarch", ProjectRoot: "/home/mk/projects/Autarch",
		Asker: "thread", Thread: "thr_abc123",
		Question:       "Collapse routine catch-up items per project or per day?",
		Recommendation: "project",
		Options: []Option{
			{ID: "project", Label: "Collapse per project", Kind: "instruction", Reversible: true,
				Instruction: "On branch feat/bb-catchup, group per project, run npm test, commit locally, and report."},
			{ID: "day", Label: "Collapse per day", Kind: "instruction", Reversible: true,
				Instruction: "On branch feat/bb-catchup, group per day, run npm test, commit locally, and report."},
			{ID: "ask", Label: "Show me both first", Kind: "needs-context"},
		},
	}
}

func mustNorm(t *testing.T, a Ask) Ask {
	t.Helper()
	n, err := Normalize(a)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	return n
}

func wantErr(t *testing.T, a Ask, sub string) {
	t.Helper()
	_, err := Normalize(a)
	if err == nil {
		t.Fatalf("want error containing %q, got none", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("want error containing %q, got %q", sub, err.Error())
	}
}

func TestValidateDefaults(t *testing.T) {
	a := base()
	a.Options = []Option{
		{Label: "Collapse per project", Instruction: "do the project thing"},
		{Label: "Show me both first"},
	}
	a.Recommendation = "collapse-per-project"
	a.Question = "  Collapse   Routine items?  "
	n := mustNorm(t, a)
	if n.Options[0].Kind != "instruction" || n.Options[1].Kind != "needs-context" {
		t.Fatalf("kinds: %+v", n.Options)
	}
	if n.Options[0].ID != "collapse-per-project" || n.Options[1].ID != "show-me-both-first" {
		t.Fatalf("ids: %+v", n.Options)
	}
	if n.AskKey != "collapse routine items?" {
		t.Fatalf("ask_key default: %q", n.AskKey)
	}
	if n.RequestID != Identity(n) || n.RequestID == "" {
		t.Fatalf("request_id default %q vs identity %q", n.RequestID, Identity(n))
	}
	long := base()
	long.Question = strings.Repeat("x", 300)
	if got := len([]rune(mustNorm(t, long).AskKey)); got != 120 {
		t.Fatalf("ask_key length %d", got)
	}
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *Ask)
		want string
	}{
		{"bad version", func(a *Ask) { a.V = 2 }, "v must be 1"},
		{"bad kind", func(a *Ask) { a.Kind = "vote" }, "kind must be"},
		{"instruction on needs-context", func(a *Ask) { a.Options[2].Instruction = "x" }, "instruction is only allowed on an instruction option"},
		{"instruction kind without text", func(a *Ask) { a.Options[0].Instruction = "" }, "instruction option needs an instruction"},
		{"instruction too long", func(a *Ask) { a.Options[0].Instruction = strings.Repeat("a", 2001) }, "instruction must be 1-2000"},
		{"instruction NUL", func(a *Ask) { a.Options[0].Instruction = "a\x00b" }, "NUL"},
		{"instruction invalid utf8", func(a *Ask) { a.Options[0].Instruction = "a\xffb" }, "valid UTF-8"},
		{"duplicate ids", func(a *Ask) { a.Options[1].ID = "project" }, "duplicate option id"},
		{"bad id", func(a *Ask) { a.Options[1].ID = "Day!" }, "option id must match"},
		{"one option", func(a *Ask) { a.Options = a.Options[:1]; a.Recommendation = "" }, "2-6 options"},
		{"seven options", func(a *Ask) {
			a.Options = nil
			for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
				a.Options = append(a.Options, Option{ID: id, Label: id})
			}
			a.Recommendation = ""
		}, "2-6 options"},
		{"label too long", func(a *Ask) { a.Options[0].Label = strings.Repeat("l", 81) }, "label must be 1-80"},
		{"empty label", func(a *Ask) { a.Options[0].Label = "" }, "label must be 1-80"},
		{"recommendation not an option", func(a *Ask) { a.Recommendation = "nope" }, "recommendation must name an option"},
		{"request id chars", func(a *Ask) { a.RequestID = "bad id" }, "request_id must be"},
		{"request id long", func(a *Ask) { a.RequestID = strings.Repeat("r", 129) }, "request_id must be"},
		{"ask_key too long", func(a *Ask) { a.AskKey = strings.Repeat("k", 121) }, "ask_key must be 1-120"},
		{"subject too long", func(a *Ask) { a.Subject = strings.Repeat("s", 121) }, "subject must be 1-120"},
		{"asker", func(a *Ask) { a.Asker = "bob" }, "asker must be"},
		{"thread asker without thread", func(a *Ask) { a.Thread = "" }, "thread asker needs a thread"},
		{"mycroft with thread", func(a *Ask) { a.Asker = "mycroft" }, "mycroft asker takes no thread"},
		{"missing project", func(a *Ask) { a.Project = "" }, "project is required"},
		{"relative root", func(a *Ask) { a.ProjectRoot = "rel/path" }, "project_root must be an absolute path"},
		{"supersedes with mention_of", func(a *Ask) { a.Supersedes = "dec_1"; a.MentionOf = "dec_2" }, "supersedes and mention_of"},
		{"approval token in question", func(a *Ask) { a.Question = "ok? APPROVED-MERGE owner/repo#1 abc" }, "approval tokens are not accepted in Home text"},
		{"approval token in label", func(a *Ask) { a.Options[0].Label = "APPROVED-DEPLOY" }, "approval tokens are not accepted in Home text"},
		{"approval token in instruction", func(a *Ask) { a.Options[0].Instruction = "x\nAPPROVED-RELEASE y" }, "approval tokens are not accepted in Home text"},
		{"reversible with approval", func(a *Ask) {
			a.Options[0].Approval = &Approval{Kind: "merge", Target: "o/r#1", Identity: "abc"}
		}, "reversible is not allowed on an option with an approval"},
		{"mycroft instruction", func(a *Ask) {
			a.Asker, a.Thread = "mycroft", ""
			a.Options = []Option{{ID: "a", Label: "A", Kind: "ruling-only"}, {ID: "b", Label: "B", Kind: "instruction", Instruction: "x"}}
			a.Recommendation = ""
		}, "mycroft may offer only ruling-only"},
		{"mycroft reversible", func(a *Ask) {
			a.Asker, a.Thread = "mycroft", ""
			a.Options = []Option{{ID: "a", Label: "A", Kind: "ruling-only", Reversible: true}, {ID: "b", Label: "B", Kind: "ruling-only"}}
			a.Recommendation = ""
		}, "reversible is not allowed when asker is mycroft"},
		{"decide with steps", func(a *Ask) { a.Steps = []string{"x"} }, "decide takes no steps or machine"},
		{"approval kind", func(a *Ask) {
			a.Options[2].Approval = &Approval{Kind: "lunch", Target: "t", Identity: "i"}
		}, "approval kind must be"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := base()
			c.edit(&a)
			wantErr(t, a, c.want)
		})
	}
}

func TestValidateKinds(t *testing.T) {
	s := base()
	s.Kind, s.Options, s.Recommendation = "steps", nil, ""
	s.Steps = []string{"Turn on Tailscale", "Approve the token"}
	mustNorm(t, s)
	s.Options = []Option{{Label: "x"}, {Label: "y"}}
	wantErr(t, s, "steps takes no options")
	s.Options = nil
	s.Steps = nil
	wantErr(t, s, "steps needs 1-20 steps")

	m := base()
	m.Kind, m.Options, m.Recommendation = "machine", nil, ""
	m.Machine = &Machine{Class: "dns", Detail: "zone is stale"}
	mustNorm(t, m)
	m.Machine = nil
	wantErr(t, m, "machine needs a machine block")
	m.Machine = &Machine{Class: "DNS!", Detail: "x"}
	wantErr(t, m, "machine class must match")
	m.Machine = &Machine{Class: "dns", Detail: "x", OwnerThread: "thr_1"}
	m.Steps = []string{"x"}
	wantErr(t, m, "machine takes no steps")
}

func TestValidateApprovalRefusals(t *testing.T) {
	// A reversible mark on an approval option is the only refusal listed for reversible; ruling-only options may carry approval.
	a := base()
	a.Options[2].Approval = &Approval{Kind: "merge", Target: "o/r#1", Identity: "abc123"}
	mustNorm(t, a)
}

func TestValidateUnknownField(t *testing.T) {
	if _, err := Parse([]byte(`{"v":1,"kind":"steps","single_use":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := Parse([]byte(`{"v":1,"kind":"decide","options":[{"id":"a","label":"A","approval":{"kind":"merge","target":"t","identity":"i","single_use":true}}]}`)); err == nil {
		t.Fatal("unknown approval field accepted")
	}
}

func TestIdentityAndRevisionScope(t *testing.T) {
	a := mustNorm(t, base())
	id0, rev0 := Identity(a), Revision(a)
	if len(id0) != 64 || len(rev0) != 64 {
		t.Fatalf("hash lengths %d %d", len(id0), len(rev0))
	}
	for _, edit := range []func(a *Ask){
		func(a *Ask) { a.Project = "Other" },
		func(a *Ask) { a.ProjectRoot = "/tmp/other" },
		func(a *Ask) { a.Thread = "thr_zzz" },
		func(a *Ask) {
			a.Asker, a.Thread = "mycroft", ""
			a.Options = []Option{{ID: "a", Label: "A", Kind: "ruling-only"}, {ID: "b", Label: "B", Kind: "ruling-only"}}
			a.Recommendation = ""
		},
	} {
		b := base()
		edit(&b)
		b = mustNorm(t, b)
		if Identity(b) == id0 {
			t.Fatal("identity did not change with scope")
		}
		if Revision(b) == rev0 {
			t.Fatal("revision did not change with scope")
		}
	}
	// request_id never affects identity
	b := base()
	b.RequestID = "req_custom"
	if Identity(mustNorm(t, b)) != id0 {
		t.Fatal("request_id changed identity")
	}
	// ask_key and subject change identity but not revision
	c := base()
	c.AskKey = "different key"
	c = mustNorm(t, c)
	if Identity(c) == id0 || Revision(c) != rev0 {
		t.Fatal("ask_key: identity must change, revision must not")
	}
	// option instruction changes revision
	d := base()
	d.Options[0].Instruction += " extra"
	if Revision(mustNorm(t, d)) == rev0 {
		t.Fatal("instruction did not change revision")
	}
	// supersedes changes revision
	e := base()
	e.Supersedes = "dec_1"
	if Revision(mustNorm(t, e)) == rev0 {
		t.Fatal("supersedes did not change revision")
	}
}

func TestIdentityKeyOrderAndNFC(t *testing.T) {
	a := `{"v":1,"kind":"decide","project":"P","project_root":"/r","asker":"thread","thread":"t1","question":"Café?","options":[{"label":"A"},{"label":"B"}]}`
	b := `{"options":[{"label":"A"},{"label":"B"}],"question":"Café?","thread":"t1","asker":"thread","project_root":"/r","project":"P","kind":"decide","v":1}`
	pa, err := Parse([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	pb, err := Parse([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	if Identity(pa) != Identity(pb) || Revision(pa) != Revision(pb) || pa.RequestID != pb.RequestID {
		t.Fatal("key order or NFC changed a hash")
	}
}

func TestSemanticKey(t *testing.T) {
	a := mustNorm(t, base())
	k0 := SemanticKey(a)
	if len(k0) != 64 {
		t.Fatalf("key %q", k0)
	}
	same := base()
	same.AskKey = "another"
	same.RequestID = "req_x"
	if SemanticKey(mustNorm(t, same)) != k0 {
		t.Fatal("ask_key or request_id changed the semantic key")
	}
	other := base()
	other.Thread = "thr_other"
	if SemanticKey(mustNorm(t, other)) != k0 {
		t.Fatal("asker thread changed the semantic key (scope is compared separately)")
	}
	diffs := map[string]func(a *Ask){
		"kind":           func(a *Ask) { a.Kind = "steps"; a.Options, a.Recommendation, a.Steps = nil, "", []string{"do it"} },
		"subject":        func(a *Ask) { a.Subject = "autarch/catch-up: other" },
		"question":       func(a *Ask) { a.Question = "Something else entirely?" },
		"label":          func(a *Ask) { a.Options[0].Label = "Collapse per project!" },
		"instruction":    func(a *Ask) { a.Options[0].Instruction += "." },
		"reversible":     func(a *Ask) { a.Options[0].Reversible = false },
		"approval":       func(a *Ask) { a.Options[2].Approval = &Approval{Kind: "merge", Target: "o/r#1", Identity: "sha1"} },
		"recommendation": func(a *Ask) { a.Recommendation = "day" },
	}
	for name, edit := range diffs {
		b := base()
		edit(&b)
		if SemanticKey(mustNorm(t, b)) == k0 {
			t.Fatalf("%s did not change the semantic key", name)
		}
	}
	// approval SHA specifically
	p1, p2 := base(), base()
	p1.Options[2].Approval = &Approval{Kind: "merge", Target: "o/r#1", Identity: "sha1"}
	p2.Options[2].Approval = &Approval{Kind: "merge", Target: "o/r#1", Identity: "sha2"}
	if SemanticKey(mustNorm(t, p1)) == SemanticKey(mustNorm(t, p2)) {
		t.Fatal("approval SHA did not change the semantic key")
	}
	yn := func(q string) Ask {
		return Ask{V: 1, Kind: "decide", Subject: "autarch: remove feature", Project: "P", ProjectRoot: "/r", Asker: "thread", Thread: "t",
			Question: q, Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}}
	}
	if SemanticKey(mustNorm(t, yn("Remove feature A?"))) == SemanticKey(mustNorm(t, yn("Remove feature B?"))) {
		t.Fatal("feature A and B collided")
	}
	mach := func(detail string) Ask {
		return Ask{V: 1, Kind: "machine", Subject: "blocker", Project: "P", ProjectRoot: "/r", Asker: "thread", Thread: "t",
			Question: "blocked", Machine: &Machine{Class: "dns", Detail: detail}}
	}
	if SemanticKey(mustNorm(t, mach("zone a"))) == SemanticKey(mustNorm(t, mach("zone b"))) {
		t.Fatal("machine detail did not change the key")
	}
	ns := base()
	ns.Subject = ""
	if SemanticKey(mustNorm(t, ns)) != "" {
		t.Fatal("ask without subject got a semantic key")
	}
}

type vectorFile struct {
	Canonical []struct {
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
		Expected string          `json:"expected"`
	} `json:"canonical"`
	Asks []struct {
		Name        string          `json:"name"`
		Input       json.RawMessage `json:"input"`
		Normalized  string          `json:"normalized"`
		Identity    string          `json:"identity"`
		Revision    string          `json:"revision"`
		SemanticKey string          `json:"semantic_key"`
	} `json:"asks"`
	Invalid []struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
		Error string          `json:"error"`
	} `json:"invalid"`
}

const vectorsPath = "../../schema/home-ask/v1/vectors.json"

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(vectorsPath))
	if err != nil {
		t.Fatal(err)
	}
	var v vectorFile
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVectors(t *testing.T) {
	v := loadVectors(t)
	if len(v.Canonical) < 6 || len(v.Asks) < 6 || len(v.Invalid) < 4 {
		t.Fatalf("vector file too small: %d %d %d", len(v.Canonical), len(v.Asks), len(v.Invalid))
	}
	for _, c := range v.Canonical {
		got, err := CanonicalJSON(c.Input)
		if err != nil || got != c.Expected {
			t.Errorf("canonical %s: got %q err %v want %q", c.Name, got, err, c.Expected)
		}
	}
	for _, c := range v.Asks {
		a, err := Parse(c.Input)
		if err != nil {
			t.Errorf("ask %s: %v", c.Name, err)
			continue
		}
		if got := NormalizedJSON(a); got != c.Normalized {
			t.Errorf("ask %s normalized:\n got %s\nwant %s", c.Name, got, c.Normalized)
		}
		if Identity(a) != c.Identity || Revision(a) != c.Revision || SemanticKey(a) != c.SemanticKey {
			t.Errorf("ask %s hashes: got %s %s %s", c.Name, Identity(a), Revision(a), SemanticKey(a))
		}
	}
	for _, c := range v.Invalid {
		_, err := Parse(c.Input)
		if err == nil || !strings.Contains(err.Error(), c.Error) {
			t.Errorf("invalid %s: got %v want containing %q", c.Name, err, c.Error)
		}
	}
}

func TestSchemaFieldsMatch(t *testing.T) {
	raw, err := os.ReadFile("../../schema/home-ask/v1/ask.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	want := []string{"v", "kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread",
		"question", "recommendation", "supersedes", "mention_of", "options", "steps", "machine"}
	if len(s.Properties) != len(want) {
		t.Fatalf("schema has %d properties, want %d", len(s.Properties), len(want))
	}
	for _, k := range want {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("schema lacks %s", k)
		}
	}
}
