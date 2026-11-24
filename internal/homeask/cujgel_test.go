package homeask

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func cujgelExample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/cujgel/citadel-link.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Exercise the real card parser, including the fence and option membership checks.
func parseCujgelFixture(t *testing.T, raw string) (Card, error) {
	t.Helper()
	r := testReq()
	r.Ask["options"] = []any{map[string]any{"id": "option-a", "label": "A", "kind": "ruling-only"}, map[string]any{"id": "option-b", "label": "B", "kind": "ruling-only"}}
	desc, _, err := r.Description()
	if err != nil {
		t.Fatal(err)
	}
	return ParseCard(desc + "\n```cujgel/v1\n" + raw + "\n```\n")
}

func TestCujgelSchemaFixtures(t *testing.T) {
	for name, want := range map[string]string{
		"citadel-link.schema.json": "e406e255eaabc02a90746fd70f16e64e2110cd837eeacc63afc0aac6f3902122",
		"citadel-link.json":        "3fa3bdc3e8f79e895ba38b00aa94fcebae9ab9456a36d9f90617acc0e5ad2967",
	} {
		b, err := os.ReadFile(filepath.Join("testdata/cujgel", name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != want {
			t.Errorf("%s changed: %s", name, got)
		}
	}
}

func TestCujgelSchemaParity(t *testing.T) {
	var s map[string]any
	b, err := os.ReadFile("testdata/cujgel/citadel-link.schema.json")
	if err != nil || json.Unmarshal(b, &s) != nil {
		t.Fatal("cannot read vendored schema")
	}
	obj := func(m map[string]any, key string) map[string]any { return m[key].(map[string]any) }
	props := obj(s, "properties")
	defs := obj(s, "$defs")
	rev := obj(props, "comparison_rev")
	variants := obj(props, "variants")
	item := obj(variants, "items")
	// Read actual Go declarations, so schema parity fails even before the new validator exists.
	f, err := parser.ParseFile(token.NewFileSet(), "cujgel.go", nil, 0)
	if err != nil {
		t.Fatalf("Go validator unavailable for schema parity: %v", err)
	}
	decls := map[string]any{}
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || len(v.Values) != 1 {
			return true
		}
		if lit, ok := v.Values[0].(*ast.BasicLit); ok {
			if lit.Kind == token.STRING {
				decls[v.Names[0].Name], _ = strconv.Unquote(lit.Value)
			}
			if lit.Kind == token.INT {
				x, _ := strconv.Atoi(lit.Value)
				decls[v.Names[0].Name] = float64(x)
			}
		}
		if list, ok := v.Values[0].(*ast.CompositeLit); ok {
			var keys []any
			for _, e := range list.Elts {
				if lit, ok := e.(*ast.BasicLit); ok {
					x, _ := strconv.Unquote(lit.Value)
					keys = append(keys, x)
				}
			}
			decls[v.Names[0].Name] = keys
		}
		return true
	})
	pattern := func(m map[string]any) string { return strings.TrimSuffix(m["pattern"].(string), `(?![\s\S])`) + `\z` }
	for name, want := range map[string]any{
		"cujgelIDPattern": pattern(obj(defs, "id")), "cujgelHashPattern": pattern(obj(defs, "hash")),
		"cujgelOptionPattern": pattern(obj(obj(item, "properties"), "option_id")),
		"cujgelMinVariants":   variants["minItems"], "cujgelMaxVariants": variants["maxItems"],
		"cujgelMinRevision": obj(obj(rev, "properties"), "revision")["minimum"],
		"cujgelRequired":    s["required"], "cujgelRevRequired": rev["required"], "cujgelVariantRequired": item["required"],
	} {
		if !reflect.DeepEqual(decls[name], want) {
			t.Errorf("%s: Go=%v schema=%v", name, decls[name], want)
		}
	}
	for _, o := range []map[string]any{s, rev, item} {
		if o["type"] != "object" || o["additionalProperties"] != false || len(obj(o, "properties")) != len(o["required"].([]any)) {
			t.Error("closed required object shape changed")
		}
	}
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" || variants["type"] != "array" || obj(obj(rev, "properties"), "revision")["type"] != "integer" {
		t.Error("schema types changed")
	}
	for _, k := range []string{"project", "comparison_id", "question_id"} {
		if obj(props, k)["$ref"] != "#/$defs/id" {
			t.Errorf("%s reference changed", k)
		}
	}
	if obj(obj(item, "properties"), "variant_id")["$ref"] != "#/$defs/id" || obj(obj(rev, "properties"), "sha256")["$ref"] != "#/$defs/hash" {
		t.Error("nested reference changed")
	}
}

func TestCujgelCardValidation(t *testing.T) {
	good := cujgelExample(t)
	c, err := parseCujgelFixture(t, good)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	var card map[string]any
	json.Unmarshal(b, &card)
	if card["cujgel"] == nil {
		t.Fatal("cujgel/v1 block was not parsed as typed metadata")
	}
	if _, err := parseCujgelFixture(t, good+strings.Repeat(" ", CujgelMaxBytes-len(good))); err != nil {
		t.Fatalf("exact size limit refused: %v", err)
	}
	r := requestWithCujgel(t)
	desc, id, err := r.Description()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCard(desc + "\n```cujgel/v1\n" + good + "\n```\n"); err == nil {
		t.Fatal("duplicate link block accepted")
	}
	plain := r
	plain.Cujgel = nil
	base, _ := RequestIdentity(plain)
	r.Cujgel.ComparisonRev.SHA256 = strings.Repeat("b", 64)
	changed, _ := RequestIdentity(r)
	if id == base || id == changed {
		t.Fatal("link is missing from request identity")
	}
	for _, tc := range []struct{ name, raw, field string }{
		{"unknown", strings.TrimSuffix(strings.TrimSpace(good), "}") + `,"url":"fixture"}`, "url"},
		{"path", strings.TrimSuffix(strings.TrimSpace(good), "}") + `,"path":"fixture"}`, "path"},
		{"duplicate", strings.Replace(good, `"project":`, `"project":"proj_other","project":`, 1), "project"},
		{"nested duplicate", strings.Replace(good, `"revision":`, `"revision":2,"revision":`, 1), "revision"},
		{"unknown revision", strings.Replace(good, `"revision":`, `"extra":0,"revision":`, 1), "extra"},
		{"unknown variant", strings.Replace(good, `"variant_id":`, `"extra":0,"variant_id":`, 1), "extra"},
		{"missing", strings.Replace(good, `"project": "proj_fixture",`, ``, 1), "project"},
		{"null", strings.Replace(good, `"proj_fixture"`, `null`, 1), "project"},
		{"type", strings.Replace(good, `"proj_fixture"`, `42`, 1), "project"},
		{"invalid id", strings.Replace(good, `comparison_fixture`, `comparison.fixture`, -1), "comparison_id"},
		{"id first", strings.Replace(good, `proj_fixture`, `_fixture`, 1), "project"},
		{"id too long", strings.Replace(good, `proj_fixture`, strings.Repeat("a", 129), 1), "project"},
		{"id newline", strings.Replace(good, `proj_fixture`, `proj_fixture\n`, 1), "project"},
		{"question mismatch", strings.Replace(good, `"question_id": "comparison_fixture"`, `"question_id":"other"`, 1), "question_id"},
		{"zero revision", strings.Replace(good, `"revision": 1`, `"revision":0`, 1), "revision"},
		{"fraction", strings.Replace(good, `"revision": 1`, `"revision":1.5`, 1), "revision"},
		{"nonfinite", strings.Replace(good, `"revision": 1`, `"revision":1e999`, 1), "revision"},
		{"negative", strings.Replace(good, `"revision": 1`, `"revision":-1`, 1), "revision"},
		{"hash uppercase", strings.Replace(good, strings.Repeat("a", 64), strings.Repeat("A", 64), 1), "sha256"},
		{"hash short", strings.Replace(good, strings.Repeat("a", 64), strings.Repeat("a", 63), 1), "sha256"},
		{"hash newline", strings.Replace(good, strings.Repeat("a", 64), strings.Repeat("a", 64)+`\n`, 1), "sha256"},
		{"duplicate variant", strings.Replace(good, `variant_b`, `variant_a`, 1), "variant_id"},
		{"foreign option", strings.Replace(good, `option-b`, `option-c`, 1), "option_id"},
		{"citadel option", strings.Replace(good, `option-b`, `option.b`, 1), "option_id"},
		{"option too long", strings.Replace(good, `option-b`, strings.Repeat("a", 33), 1), "option_id"},
		{"option newline", strings.Replace(good, `option-b`, `option-b\n`, 1), "option_id"},
		{"trailing JSON", good + `{}`, "cujgel"},
		{"utf8", strings.Replace(good, `proj_fixture`, string([]byte{0xff}), 1), "UTF-8"},
		{"over limit", good + strings.Repeat(" ", 16*1024), "16 KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCujgelFixture(t, tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("want refusal naming %s, got %v", tc.field, err)
			}
		})
	}
	for _, count := range []int{0, 1, 2, 3, 4, 5} {
		t.Run(fmt.Sprintf("variants_%d", count), func(t *testing.T) {
			var m map[string]any
			json.Unmarshal([]byte(good), &m)
			vs := []any{}
			for i := 0; i < count; i++ {
				vs = append(vs, map[string]any{"variant_id": fmt.Sprintf("V%d", i), "option_id": "option-a"})
			}
			m["variants"] = vs
			raw, _ := json.Marshal(m)
			_, err := parseCujgelFixture(t, string(raw))
			if (err == nil) != (count >= 2 && count <= 4) {
				t.Fatalf("count %d: %v", count, err)
			}
		})
	}
	for _, field := range []string{"project", "comparison_id", "question_id", "variant_id"} {
		raw := strings.Replace(good, `"`+field+`": "`, `"`+field+`": "A`, 1)
		if field == "comparison_id" {
			raw = strings.Replace(raw, `"question_id": "comparison_fixture"`, `"question_id":"Acomparison_fixture"`, 1)
		}
		if field == "question_id" {
			raw = strings.Replace(raw, `"comparison_id": "comparison_fixture"`, `"comparison_id":"Acomparison_fixture"`, 1)
		}
		if _, err := parseCujgelFixture(t, raw); err != nil {
			t.Errorf("uppercase %s refused: %v", field, err)
		}
	}
}

func TestCujgelRequiredFieldsAndTypes(t *testing.T) {
	good := cujgelExample(t)
	for _, path := range [][]string{
		{"project"}, {"comparison_id"}, {"question_id"}, {"comparison_rev"}, {"variants"},
		{"comparison_rev", "revision"}, {"comparison_rev", "sha256"},
		{"variants", "variant_id"}, {"variants", "option_id"},
	} {
		for _, mode := range []string{"missing", "null", "wrong type"} {
			t.Run(strings.Join(path, ".")+"/"+mode, func(t *testing.T) {
				var m map[string]any
				json.Unmarshal([]byte(good), &m)
				obj := m
				if path[0] == "comparison_rev" && len(path) == 2 {
					obj = m["comparison_rev"].(map[string]any)
				}
				if path[0] == "variants" && len(path) == 2 {
					obj = m["variants"].([]any)[0].(map[string]any)
				}
				key := path[len(path)-1]
				switch mode {
				case "missing":
					delete(obj, key)
				case "null":
					obj[key] = nil
				case "wrong type":
					obj[key] = true
				}
				b, _ := json.Marshal(m)
				_, err := parseCujgelFixture(t, string(b))
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("%s was not refused: %v", key, err)
				}
			})
		}
	}
}

func TestCujgelIntegralRevision(t *testing.T) {
	for _, n := range []string{"1.0", "1e1", "9007199254740993"} {
		raw := strings.Replace(cujgelExample(t), `"revision": 1`, `"revision":`+n, 1)
		link, err := ParseCujgel(raw)
		if err != nil || string(link.ComparisonRev.Revision) != n {
			t.Fatalf("integer %s was rounded or refused: %v", n, err)
		}
	}
	for _, n := range []string{`"1"`, "0.99999999999999999999", "1e-1000000000"} {
		if _, err := ParseCujgel(strings.Replace(cujgelExample(t), `"revision": 1`, `"revision":`+n, 1)); err == nil {
			t.Fatalf("accepted non-integer revision %s", n)
		}
	}
}

func requestWithCujgel(t *testing.T) CardRequest {
	t.Helper()
	r := testReq()
	// JSON keeps this test buildable against the pre-feature CardRequest.
	if err := json.Unmarshal([]byte(`{"Cujgel":`+cujgelExample(t)+`}`), &r); err != nil {
		t.Fatal(err)
	}
	r.Ask["options"] = []any{map[string]any{"id": "option-a", "label": "A", "kind": "ruling-only"}, map[string]any{"id": "option-b", "label": "B", "kind": "ruling-only"}}
	return r
}

func TestCujgelFileCardProjectBinding(t *testing.T) {
	for _, linked := range []string{"proj_fixture", "proj_other", ""} {
		t.Run("linked_"+linked, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			r := requestWithCujgel(t)
			f.StateDir = filepath.Join(t.TempDir(), "not-created")
			bb.fail = func(args []string) *BBResult {
				if strings.Join(args[:min(3, len(args))], " ") == "tasks project list" {
					x := okRes(map[string]any{"projects": []any{map[string]any{"id": "P1", "name": "Alpha", "prefix": "ONE", "linkedBbProjectId": linked}}})
					return &x
				}
				return nil
			}
			_, err := f.FileCard(context.Background(), r)
			if linked == "proj_fixture" {
				if err != nil || len(bb.tasks) != 1 || !strings.Contains(bb.tasks[0].Desc, "```cujgel/v1\n") {
					t.Fatalf("linked card failed: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "project") || len(bb.tasks) != 0 {
				t.Fatalf("binding not refused before filing: %v", err)
			}
			if _, e := os.Stat(f.StateDir); !os.IsNotExist(e) {
				t.Fatalf("binding refusal wrote state: %v", e)
			}
		})
	}
}

func TestCujgelFileCardResolvesTargetBoard(t *testing.T) {
	t.Setenv("BB_PROJECT_ID", "proj_other")
	for _, board := range []string{"P1", "one", "Alpha", "missing", ""} {
		t.Run("board_"+board, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			r := requestWithCujgel(t)
			r.Project = board
			f.StateDir = filepath.Join(t.TempDir(), "not-created")
			bb.fail = func(args []string) *BBResult {
				if strings.Join(args[:min(3, len(args))], " ") == "tasks project list" {
					x := okRes(map[string]any{"projects": []any{map[string]any{"id": "P1", "name": "Alpha", "prefix": "ONE", "linkedBbProjectId": "proj_fixture"}}})
					return &x
				}
				return nil
			}
			_, err := f.FileCard(context.Background(), r)
			if board == "missing" || board == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("unknown target accepted: %v", err)
				}
				if _, e := os.Stat(f.StateDir); !os.IsNotExist(e) {
					t.Fatalf("refusal wrote state: %v", e)
				}
				return
			}
			if err != nil {
				t.Fatalf("explicit board link lost to ambient project: %v", err)
			}
		})
	}
}

func TestCujgelFileCardRejectsSecret(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	r := requestWithCujgel(t)
	r.Secret = fakeSecretMetadata()
	f.StateDir = filepath.Join(t.TempDir(), "not-created")
	if _, err := f.FileCard(context.Background(), r); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "cannot be combined with --secret") {
		t.Fatalf("combination accepted: %v", err)
	}
	if _, err := os.Stat(f.StateDir); !os.IsNotExist(err) {
		t.Fatalf("combination refusal wrote state: %v", err)
	}
	if len(bb.tasks) != 0 {
		t.Fatal("combination wrote a card")
	}
}
