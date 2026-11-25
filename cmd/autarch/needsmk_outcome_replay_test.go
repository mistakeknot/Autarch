package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestNeedsMkOutcomeNoOpUnderNewRequest(t *testing.T) {
	for _, variant := range []struct {
		name       string
		capability string
	}{
		{"reader absent", `{"schema":1}`},
		{"reader available", `{"schema":1,"expected_outcome":[1]}`},
	} {
		t.Run(variant.name, func(t *testing.T) {
			t.Setenv("BB_THREAD_ID", "thr_fixture")
			stateDir := t.TempDir()
			t.Setenv(homeask.EnvCardStateDir, stateDir)
			s := &stubBB{title: "Fixture", expectedOutcomeCapability: `{"schema":1,"expected_outcome":[1]}`}
			input := `{"project":"example","project_root":"/fixture","ask_key":"fixture-choice","question":"Proceed?","review_by":"2026-01-01T00:00:00Z","options":[{"id":"yes","label":"Proceed"},{"id":"no","label":"Wait"}]}`
			file := func(key string) homeask.CardResult {
				t.Helper()
				out, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Fixture",
					"--request", key, "--ask-file", writeAskBody(t, input))
				if err != nil {
					t.Fatal(err)
				}
				var result homeask.CardResult
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			first := file("fixture-initial")
			input = strings.Replace(input, "2026-01-01", "2026-01-02", 1)
			edit := file("fixture-edit")
			if !edit.Updated || edit.ID != first.ID || edit.Key != first.Key {
				t.Fatalf("expected an edit of the original card, got %+v", edit)
			}
			original := s.desc
			statePath := filepath.Join(stateDir, "filings.json")
			state, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}

			s.calls = nil
			s.expectedOutcomeCapability = variant.capability
			retry := file("fixture-retry")
			if !retry.Replay || retry.Updated || retry.ID != first.ID || retry.Key != first.Key {
				t.Fatalf("expected a replay of the original card, got %+v", retry)
			}
			for _, call := range s.calls {
				switch strings.Join(call[:min(len(call), 3)], " ") {
				case "home capabilities --json", "tasks create --project", "tasks update T1", "tasks comment T1", "tasks label create":
					t.Fatalf("replay made an unexpected call: %v", call)
				}
			}
			after, err := os.ReadFile(statePath)
			if err != nil || !bytes.Equal(state, after) || s.desc != original {
				t.Fatalf("replay changed the card or filing state: %v", err)
			}
		})
	}
}
