package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestNeedsMkExpectedOutcomeAuthoring(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	var rows []struct {
		Name      string          `json:"name"`
		Wire      json.RawMessage `json:"wire"`
		Canonical string          `json:"canonical"`
	}
	raw, err := os.ReadFile("../../testdata/expected-outcome-vectors.json")
	if err != nil || json.Unmarshal(raw, &rows) != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var descriptions map[string]string
	raw, err = os.ReadFile("../../testdata/expected-outcome-writer.json")
	if err != nil || json.Unmarshal(raw, &descriptions) != nil {
		t.Fatalf("read descriptions: %v", err)
	}
	defaults := map[string]string{
		"merge": "86400", "close": "86400", "bead": "259200",
		"report": "172800", "file": "172800", "re-ask relative": "604800",
	}
	for _, row := range rows {
		seconds, defaulted := defaults[row.Name]
		if !defaulted && row.Name != "absent" && row.Name != "re-ask absolute" && row.Name != "review only" && row.Name != "not scalar path" {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			input := string(row.Wire)
			if defaulted {
				var wire map[string]any
				if err := json.Unmarshal(row.Wire, &wire); err != nil {
					t.Fatal(err)
				}
				outcome := wire["options"].([]any)[0].(map[string]any)["expected_outcome"].(map[string]any)
				delete(outcome, "by_when")
				raw, err := json.Marshal(wire)
				if err != nil {
					t.Fatal(err)
				}
				input = string(raw)
			}
			runner := &stubBB{expectedOutcomeCapability: `{"schema":1,"expected_outcome":[1]}`}
			out, err := runNeedsMk(t, runner, "file", "--project", "P1", "--title", "Fixture",
				"--blocks", "thread:thr_fixture", "--ask-file", writeAskBody(t, input))
			if row.Name == "not scalar path" {
				if err == nil || !strings.Contains(err.Error(), "Unicode scalar") || len(runner.calls) != 0 {
					t.Fatalf("want original-byte refusal with no calls, got %v, %v", err, runner.calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := descriptions[row.Name]; want != "" && runner.desc != want {
				t.Fatalf("filed description changed:\n%s\nwant:\n%s", runner.desc, want)
			}
			card, err := homeask.ParseCard(runner.desc)
			if err != nil {
				t.Fatal(err)
			}
			ask, err := card.ToV1("thr_fixture")
			if err != nil {
				t.Fatal(err)
			}
			if got := homeask.NormalizedJSON(ask); got != row.Canonical {
				t.Fatalf("filed ask:\n%s\nwant:\n%s", got, row.Canonical)
			}
			if defaulted && !strings.Contains(runner.desc, `"after_seconds":`+seconds) {
				t.Fatalf("filing did not materialize %s seconds: %s", seconds, runner.desc)
			}
			var result homeask.CardResult
			if err := json.Unmarshal([]byte(out), &result); err != nil || result.Key != card.Request.Key || result.CardKey != "P1-1" {
				t.Fatalf("filing receipt: %s (%v)", out, err)
			}
			capabilityRead := false
			for _, call := range runner.calls {
				if strings.Join(call, " ") == "home capabilities --json" {
					capabilityRead = true
				}
				if strings.Join(call[:2], " ") == "tasks create" && capabilityRead != (row.Name != "absent") {
					t.Fatalf("unexpected capability preflight before creation: %v", runner.calls)
				}
			}
			if row.Name == "absent" && capabilityRead {
				t.Fatal("plain card unexpectedly checked capabilities")
			}
		})
	}
}

func TestNeedsMkExpectedOutcomeCheckRefusesWithoutCapability(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	store := filepath.Join(t.TempDir(), "store")
	t.Setenv("AUTARCH_HOME_EVIDENCE", store)
	image := filepath.Join(t.TempDir(), "picture.gif")
	if err := os.WriteFile(image, []byte("GIF89a"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &stubBB{expectedOutcomeCapability: `{"schema":1}`}
	encodedImage, _ := json.Marshal(image)
	input := `{"project":"example","project_root":"/fixture","question":"Proceed?","review_by":"2026-01-01T00:00:00Z","options":[{"id":"yes","label":"Proceed"},{"id":"no","label":"Wait"}],"evidence":[{"caption":"Picture","image_path":` + string(encodedImage) + `}]}`
	_, err := runNeedsMk(t, s, "file", "--check", "--card-type", "design", "--project", "P1", "--title", "Fixture", "--ask-file", writeAskBody(t, input))
	if !errors.Is(err, homeask.ErrHomeDown) || !strings.Contains(err.Error(), "three-line opener") || s.card || s.commented {
		t.Fatalf("want capability and design errors without mutation, got %v, %v", err, s.calls)
	}
	if _, err := os.Stat(store); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evidence store changed during a refused check: %v", err)
	}
}
