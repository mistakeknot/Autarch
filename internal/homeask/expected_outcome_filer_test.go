package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func outcomeFilingFixture(t *testing.T) (*memBB, *CardFiler, CardRequest, *BBResult) {
	t.Helper()
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.StateDir = t.TempDir()
	capability := BBResult{Stdout: []byte(`{"schema":1,"expected_outcome":[1]}`)}
	bb.fail = func(args []string) *BBResult {
		if strings.Join(args, " ") == "home capabilities --json" {
			return &capability
		}
		if len(args) >= 4 && strings.Join(args[:3], " ") == "home get --card" {
			result := okRes(CardView{TaskID: args[3], ProjectID: "P1", State: "open"})
			return &result
		}
		return nil
	}
	req := CardRequest{Project: "P1", Title: "Fixture", Key: "fixture", Thread: "thr_fixture", Ask: map[string]any{
		"project": "example", "project_root": "/fixture", "question": "Proceed?",
		"options": []any{map[string]any{"id": "yes", "label": "Proceed"}, map[string]any{"id": "no", "label": "Wait"}},
	}}
	return bb, f, req, &capability
}

func TestExpectedOutcomeFilerCapability(t *testing.T) {
	for _, lane := range []string{"file", "update", "held release"} {
		for _, reply := range []struct {
			name   string
			result BBResult
		}{
			{"missing", BBResult{Stdout: []byte(`{"schema":1}`)}},
			{"missing schema", BBResult{Stdout: []byte(`{"expected_outcome":[1]}`)}},
			{"wrong schema", BBResult{Stdout: []byte(`{"schema":2,"expected_outcome":[1]}`)}},
			{"unsupported", BBResult{Stdout: []byte(`{"schema":1,"expected_outcome":[2]}`)}},
			{"unreadable", BBResult{Stdout: []byte(`{"schema":1,"expected_outcome":"1"}`)}},
			{"failed", BBResult{Code: 1}},
		} {
			t.Run(lane+"/"+reply.name, func(t *testing.T) {
				ctx := context.Background()
				bb, f, req, capability := outcomeFilingFixture(t)
				writes := func() int {
					return bb.count("tasks", "create") + bb.count("tasks", "update") + bb.count("tasks", "comment") + bb.count("tasks", "label", "create")
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
				*capability = reply.result
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

func TestExpectedOutcomeFilerReplayWithoutCapability(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.Ask["options"].([]any)[0].(map[string]any)["expected_outcome"] = map[string]any{
		"schema": 1, "kind": "file-exists", "target": map[string]any{"path": "/fixture/result"},
		"by_when": map[string]any{"after_seconds": 172800},
	}
	first, err := f.FileCard(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	before := bb.count("home", "capabilities")
	replayed, err := f.FileCard(ctx, req)
	if err != nil || !replayed.Replay || replayed.ID != first.ID || bb.count("home", "capabilities") != before {
		t.Fatalf("filed card replay must skip the capability probe: %+v, %v, calls %v", replayed, err, bb.calls)
	}
	req.Key = "fixture-fresh"
	if _, err := f.FileCard(ctx, req); !errors.Is(err, ErrHomeDown) {
		t.Fatalf("fresh request must refuse missing capability: %v", err)
	}
	if bb.count("tasks", "create") != 1 || bb.count("tasks", "comment") != 1 || bb.count("home", "capabilities") != before+1 {
		t.Fatalf("unexpected calls for fresh request: %v", bb.calls)
	}
}

func TestExpectedOutcomeHeldReleaseSkipsCapabilityDuringWeeklyHold(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	f.Now = func() time.Time { return time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC) }
	req.Ask["review_by"] = "2026-11-20T00:00:00Z"
	if result, err := f.FileCard(ctx, req); err != nil || !result.Queued {
		t.Fatalf("want weekly hold: %+v, %v", result, err)
	}
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	before := bb.count("home", "capabilities")
	if result, _, err := f.releaseOne(ctx, req.Key); err != nil || result.ID != "" || bb.count("home", "capabilities") != before {
		t.Fatalf("release must skip the held request quietly: %+v, %v, calls %v", result, err, bb.calls)
	}
}

func TestExpectedOutcomeHeldReleaseRecoversDeliveredCardWithoutCapability(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.Ask["review_by"] = "2026-11-20T00:00:00Z"
	req.NonBlocking = true
	if result, err := f.FileCard(ctx, req); err != nil || !result.Queued {
		t.Fatalf("want held request: %+v, %v", result, err)
	}
	read := bb.fail
	bb.fail = func(args []string) *BBResult {
		if strings.Join(args, " ") == "home get --card T1 --json" {
			return badRes(1, "fixture index unavailable")
		}
		return read(args)
	}
	f.ReadbackTimeout = 10 * time.Millisecond
	f.ReadbackInterval = time.Millisecond
	if _, _, err := f.releaseOne(ctx, req.Key); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("want post-create uncertainty: %v", err)
	}
	bb.fail = read
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	before := bb.count("home", "capabilities")
	got, _, err := f.releaseOne(ctx, req.Key)
	if err != nil || got.ID != "T1" || !got.Replay || bb.count("home", "capabilities") != before ||
		bb.count("tasks", "create") != 1 || bb.count("tasks", "comment") != 1 {
		t.Fatalf("want existing card recovered without a probe or duplicate: %+v, %v, calls %v", got, err, bb.calls)
	}
}

func TestExpectedOutcomeLibraryPreflightJoinsFailures(t *testing.T) {
	_, f, req, capability := outcomeFilingFixture(t)
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	req.Type = "design"
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	err := f.Preflight(context.Background(), req)
	if !errors.Is(err, ErrHomeDown) || !strings.Contains(err.Error(), "three-line opener") {
		t.Fatalf("want capability and design errors, got %v", err)
	}
}

func TestExpectedOutcomeWriterPreservesTypedOptionSlice(t *testing.T) {
	_, _, req, _ := outcomeFilingFixture(t)
	yes := Option{ID: "yes", Label: "Proceed", Kind: "ruling-only", ExpectedOutcome: json.RawMessage(
		`{"schema":1,"kind":"file-exists","target":{"path":"/fixture/result"},"by_when":{"after_seconds":172800}}`,
	)}
	options := []any{yes, Option{ID: "no", Label: "Wait", Kind: "ruling-only"}}
	req.Ask["options"] = options
	if _, _, err := req.Description(); err != nil {
		t.Fatal(err)
	}
	if got, ok := options[0].(Option); !ok || string(got.ExpectedOutcome) != string(yes.ExpectedOutcome) {
		t.Fatalf("caller option changed: %#v", options[0])
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
