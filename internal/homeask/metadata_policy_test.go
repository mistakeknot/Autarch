package homeask

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The read-only metadata policy must have one declaration, so enabling a writer
// cannot leave identity filtering and Description validation with separate lists.
func TestMetadataPolicySingleSource(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "cardfiler.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{"subject_key": 0, "signals": 0}
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			value, err := strconv.Unquote(lit.Value)
			if _, tracked := counts[value]; err == nil && tracked {
				counts[value]++
			}
		}
		return true
	})
	for key, count := range counts {
		if count != 1 {
			t.Errorf("%s policy declared %d times; want one shared source", key, count)
		}
	}
}

func TestMetadataPolicyIdentityAndDescription(t *testing.T) {
	metadata := subjectVectors(t)[1].Metadata
	for _, key := range readOnlyAskMetadata {
		for _, value := range []any{metadata[key], nil} {
			t.Run(fmt.Sprintf("%s/null=%v", key, value == nil), func(t *testing.T) {
				req := testReq()
				before, err := RequestIdentity(req)
				if err != nil {
					t.Fatal(err)
				}
				req.Ask[key] = value
				after, err := RequestIdentity(req)
				if err != nil || before != after {
					t.Fatalf("metadata changed identity: %s -> %s (%v)", before, after, err)
				}
				if got, ok := req.Ask[key]; !ok || !reflect.DeepEqual(got, value) {
					t.Fatal("RequestIdentity mutated input metadata")
				}
				_, _, err = req.Description()
				wantErrIs(t, err, ErrInvalid)
				if !strings.Contains(err.Error(), key+" is not writable yet") {
					t.Fatalf("Description returned %v", err)
				}
			})
		}
	}
}

func TestFilersRejectMetadataAtDescription(t *testing.T) {
	for _, lane := range []string{"exact", "update"} {
		for _, key := range []string{"subject_key", "signals"} {
			for _, value := range []any{subjectVectors(t)[1].Metadata[key], nil} {
				t.Run(fmt.Sprintf("%s/%s/null=%v", lane, key, value == nil), func(t *testing.T) {
					f := newFake(t, registryOnCreate)
					req := CardRequest{
						Project: "P1", Title: "Choose a color", Key: "color", Thread: "thr_a",
						Ask: map[string]any{
							"project": "autarch", "project_root": t.TempDir(), "question": "Which color?",
							"options": []any{
								map[string]any{"id": "blue", "label": "Blue", "kind": "ruling-only"},
								map[string]any{"id": "green", "label": "Green", "kind": "ruling-only"},
							},
						},
					}
					if lane == "update" {
						req.Ask["ask_key"] = "color"
					}
					if _, _, err := req.Description(); err != nil {
						t.Fatalf("baseline request invalid: %v", err)
					}
					req.Ask[key] = value
					var err error
					if lane == "exact" {
						_, err = f.filer().FileCard(context.Background(), req)
					} else {
						_, err = f.filer().FileOrUpdateCard(context.Background(), req, "")
					}
					wantErrIs(t, err, ErrInvalid)
					if !strings.Contains(err.Error(), key+" is not writable yet") {
						t.Fatalf("did not reach Description rejection: %v", err)
					}
					if len(f.calls) != 0 {
						t.Fatalf("contacted tasks before rejecting metadata: %v", f.calls)
					}
				})
			}
		}
	}
}
