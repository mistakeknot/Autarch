package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestExpectedOutcomeFilerCapability(t *testing.T) {
	for _, lane := range []string{"file", "update", "held release"} {
		for _, reply := range []struct {
			name   string
			result BBResult
		}{
			{"missing", BBResult{Stdout: []byte(`{"schema":1}`)}},
			{"unsupported", BBResult{Stdout: []byte(`{"schema":1,"expected_outcome":[2]}`)}},
			{"unreadable", BBResult{Stdout: []byte(`{"schema":1,"expected_outcome":"1"}`)}},
			{"failed", BBResult{Code: 1}},
		} {
			t.Run(lane+"/"+reply.name, func(t *testing.T) {
				ctx := context.Background()
				bb := newFake(t, registryOnCreate)
				f := bb.filer()
				f.StateDir = t.TempDir()
				req := CardRequest{Project: "P1", Title: "Fixture", Key: "fixture", Thread: "thr_fixture", Ask: map[string]any{
					"project": "example", "project_root": "/fixture", "question": "Proceed?",
					"options": []any{map[string]any{"id": "yes", "label": "Proceed"}, map[string]any{"id": "no", "label": "Wait"}},
				}}
				writes := func() int {
					return bb.count("tasks", "create") + bb.count("tasks", "update") + bb.count("tasks", "comment") + bb.count("tasks", "label", "create")
				}
				capability := okRes(map[string]any{"schema": 1, "expected_outcome": []int{1}})
				bb.fail = func(args []string) *BBResult {
					if args[0] == "home" && args[1] == "capabilities" {
						return &capability
					}
					return nil
				}
				var taskID string
				if lane == "update" {
					res, err := f.FileCard(ctx, req)
					if err != nil {
						t.Fatal(err)
					}
					taskID = res.ID
					req.Key = "fixture-edit"
				}
				req.Ask["review_by"] = "2026-01-01T00:00:00Z"
				if lane == "held release" {
					req.NonBlocking = true
					if res, err := f.FileCard(ctx, req); err != nil || !res.Queued {
						t.Fatalf("queue with capability: %+v %v", res, err)
					}
					f.Now = func() time.Time { return filingTestNow().Add(24 * time.Hour) }
				}
				beforeWrites, beforeReads := writes(), bb.count("home", "capabilities")
				capability = reply.result
				var err error
				switch lane {
				case "file":
					_, err = f.FileCard(ctx, req)
				case "update":
					_, err = f.FileOrUpdateCard(ctx, req, taskID)
				case "held release":
					_, _, err = f.releaseOne(ctx, req.Key)
				}
				if !errors.Is(err, ErrHomeDown) || writes() != beforeWrites || bb.count("home", "capabilities") != beforeReads+1 {
					t.Fatalf("want preflight refusal without writes, got %v, calls %v", err, bb.calls)
				}
			})
		}
	}
}

func TestExpectedOutcomeWriterUsesReaderValidation(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/expected-outcome-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name  string          `json:"name"`
		Wire  json.RawMessage `json:"wire"`
		Error bool            `json:"error"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row.Wire) == 0 || row.Name == "not scalar path" {
			continue // Checked before JSON decoding on the CLI.
		}
		t.Run(row.Name, func(t *testing.T) {
			var ask map[string]any
			if err := json.Unmarshal(row.Wire, &ask); err != nil {
				t.Fatal(err)
			}
			req := CardRequest{Title: "Fixture", Key: "fixture", Ask: ask}
			_, err := RequestIdentity(req)
			wantsRefusal := row.Error || row.Name == "historical merge" || row.Name == "historical close"
			if wantsRefusal != errors.Is(err, ErrInvalid) || !wantsRefusal && err != nil {
				t.Fatalf("invalid=%t, got %v", wantsRefusal, err)
			}
			if !wantsRefusal {
				if _, _, err := req.Description(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
