package homeask

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type hashGolden struct {
	AskFixture  string          `json:"ask_fixture"`
	FixtureFile string          `json:"fixture_file"`
	FixtureName string          `json:"fixture_name"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Thread      string          `json:"thread"`
	Generation  int             `json:"generation"`
	Ask         json.RawMessage `json:"ask"`
	Expected    map[string]any  `json:"expected"`
}

func hashGoldens(t *testing.T) []hashGolden {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/hash-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []hashGolden
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].AskFixture != "" {
			for _, v := range loadVectors(t).Asks {
				if v.Name == rows[i].AskFixture {
					rows[i].Ask = v.Input
					rows[i].Expected["normalized"] = v.Normalized
				}
			}
		}
		if rows[i].FixtureFile != "" {
			raw, err := os.ReadFile(cardFixtureDir + "/" + rows[i].FixtureFile)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Cases []cardCase `json:"cases"`
			}
			json.Unmarshal(raw, &doc)
			for _, c := range doc.Cases {
				if c.Name == rows[i].FixtureName {
					rows[i].Description = c.Description
				}
			}
			if rows[i].Description == "" {
				t.Fatal("fixture missing")
			}
		}
	}
	return rows
}
func goldenSHA(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	canon, err := CanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256([]byte(canon))
	return hex.EncodeToString(s[:])
}
func goldenHashes(t *testing.T, row hashGolden) map[string]any {
	t.Helper()
	var a Ask
	var c Card
	var err error
	var wire map[string]any
	if row.Description != "" {
		c, err = ParseCard(row.Description)
		if err != nil {
			t.Fatal(err)
		}
		wire = c.Ask
		a, err = c.ToV1(row.Thread)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		a, err = Parse(row.Ask)
		if err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(row.Ask, &wire)
	}
	blocks := []string{}
	for _, b := range c.Blocks {
		blocks = append(blocks, b.Ref)
	}
	blocks = dedupe(blocks)
	sort.Strings(blocks)
	// Filing inputs use decoded JSON values; canonicalize numeric spellings as JS does.
	rawWire, _ := json.Marshal(wire)
	json.Unmarshal(rawWire, &wire)
	req := CardRequest{Title: "Fixture", Blocks: blocks, Ask: wire, RootRun: c.RootRun, Pull: c.Pull != ""}
	if c.Move != nil {
		raw, _ := json.Marshal(c.Move)
		json.Unmarshal(raw, &req.Move)
	}
	h1, err := RequestIdentity(req)
	if err != nil {
		t.Fatal(err)
	}
	var h5 any
	if row.Description != "" {
		var norm map[string]any
		json.Unmarshal([]byte(NormalizedJSON(a)), &norm)
		delete(norm, "request_id")
		delete(norm, "supersedes")
		if a.Urgency == "blocker" {
			norm["urgency"] = "blocker"
		}
		node := map[string]any{"title": "Fixture", "blocks": blocks, "ask": norm, "root_run": c.RootRun, "request": c.Request, "asking_thread": row.Thread, "tasks_project": "fixture-project"}
		if c.Move != nil {
			node["move"] = c.Move
		}
		h5 = goldenSHA(t, node)
		if row.Generation > 1 {
			a.RequestID = "card:fixture:g" + string(rune('0'+row.Generation))
			a.Supersedes = "card-fixture-g" + string(rune('0'+row.Generation-1))
			raw, _ := json.Marshal(a)
			a, err = Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return map[string]any{"h1": h1, "identity": Identity(a), "revision": Revision(a), "semantic_digest": SemanticKey(a), "normalized": NormalizedJSON(a), "h5": h5}
}
func TestFrozenHashes(t *testing.T) {
	for _, row := range hashGoldens(t) {
		t.Run(row.Name, func(t *testing.T) {
			got := goldenHashes(t, row)
			if !reflect.DeepEqual(got, row.Expected) {
				for k, v := range got {
					if !reflect.DeepEqual(v, row.Expected[k]) {
						t.Errorf("%s: got %v want %v", k, v, row.Expected[k])
					}
				}
			}
		})
	}
}

// Rewrite only the wire JSON, preserving Request, root-run, move and prose.
func goldenMetadata(t *testing.T, description string, metadata map[string]any) string {
	t.Helper()
	re := strings.SplitN(description, "```home-ask\n", 2)
	end := strings.Index(re[1], "\n```")
	var wire map[string]any
	json.Unmarshal([]byte(re[1][:end]), &wire)
	for k, v := range metadata {
		wire[k] = v
	}
	raw, _ := json.Marshal(wire)
	return re[0] + "```home-ask\n" + string(raw) + re[1][end:]
}
