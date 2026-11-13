package homeask

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGroupOnlyUpdateNoop(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "same"
	r.Ask["group"] = map[string]any{"key": "pair", "order": 1}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	first := f.tasks[0].Desc
	r.Ask["group"] = map[string]any{"key": "pair", "order": 2}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	if f.tasks[0].Desc != first || f.count("tasks", "update") != 0 {
		t.Fatal("group-only update was not a no-op")
	}
}

func TestGroupVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/group-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name, Raw string
		Valid     bool
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			raw, _ := json.Marshal(base())
			var m map[string]json.RawMessage
			json.Unmarshal(raw, &m)
			m["group"] = json.RawMessage(v.Raw)
			raw, _ = json.Marshal(m)
			a, err := Parse(raw)
			if (err == nil) != v.Valid {
				t.Fatalf("parse valid=%v: %v", v.Valid, err)
			}
			if v.Valid {
				b, _ := json.Marshal(a)
				if !strings.Contains(string(b), `"group"`) {
					t.Fatal("parser dropped group")
				}
			}
			req := testReq()
			var group any
			json.Unmarshal([]byte(v.Raw), &group)
			req.Ask["group"] = group
			desc, _, err := req.Description()
			if (err == nil) != v.Valid {
				t.Fatalf("filing valid=%v: %v", v.Valid, err)
			}
			if v.Valid {
				c, err := ParseCard(desc)
				if err != nil || c.Ask["group"] == nil {
					t.Fatal("card lost group", err)
				}
			}
		})
	}
}

func TestGroupHashesUnchanged(t *testing.T) {
	for _, c := range loadVectors(t).Asks {
		var m map[string]json.RawMessage
		json.Unmarshal(c.Input, &m)
		m["group"] = json.RawMessage(`{"key":"merge-pair","order":2}`)
		raw, _ := json.Marshal(m)
		a, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if Identity(a) != c.Identity || Revision(a) != c.Revision || SemanticKey(a) != c.SemanticKey || NormalizedJSON(a) != c.Normalized {
			t.Fatal("group changed pinned hashes", c.Name)
		}
	}
	req := testReq()
	first, err := RequestIdentity(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Ask["group"] = map[string]any{"key": "merge-pair", "order": 2}
	second, err := RequestIdentity(req)
	if err != nil || first != second {
		t.Fatal("group changed RequestIdentity", err)
	}
}
