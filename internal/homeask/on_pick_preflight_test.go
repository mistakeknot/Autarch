package homeask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnPickTargetAndHeadRefusalBeforeMutation(t *testing.T) {
	for _, problem := range []string{"missing-title", "mismatched-thread", "archived", "unbound", "unlisted", "head-unavailable", "head-mismatch", "valid-none", "valid-pr"} {
		t.Run(problem, func(t *testing.T) {
			req := pickRequest()
			if strings.Contains(problem, "head") || problem == "valid-pr" {
				opt := req.Ask["options"].([]any)[0].(map[string]any)
				opt["on_pick"].(map[string]any)["subject"] = map[string]any{"kind": "github-pr", "repo": "sample/repo", "number": 7, "head": strings.Repeat("a", 40)}
				dir := t.TempDir()
				script := "#!/bin/sh\nprintf '%s' '{\"state\":\"OPEN\",\"headRefOid\":\"" + strings.Repeat("a", 40) + "\"}'\n"
				if problem == "head-unavailable" {
					script = "#!/bin/sh\nexit 1\n"
				}
				if problem == "head-mismatch" {
					script = strings.ReplaceAll(script, strings.Repeat("a", 40), strings.Repeat("b", 40))
				}
				if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
			}
			if problem == "unlisted" {
				req.Ask["options"].([]any)[0].(map[string]any)["on_pick"].(map[string]any)["target_thread"] = "thr_other"
			}
			f := &CardFiler{Run: func(_ context.Context, _ []string, args ...string) BBResult {
				var answer any
				switch strings.Join(args[:min(2, len(args))], " ") {
				case "home capabilities":
					answer = map[string]any{"schema": 1, "on_pick": []int{1}, "target_lookup": []int{1}}
				case "tasks project":
					answer = map[string]any{"projects": []any{map[string]any{"id": "P1"}}}
				case "home binding":
					state := "confirmed"
					if problem == "unbound" {
						state = "suggested"
					}
					answer = map[string]any{"state": state, "home_project": "autarch"}
				case "home target":
					m := map[string]any{"schema": 1, "thread_id": argFlag(args, "--thread"), "title": "Palette", "project_id": "bb-project", "project_name": "Design", "archived": false, "deleted": false, "observed_at": "2026-01-01T00:00:00Z"}
					if problem == "missing-title" {
						delete(m, "title")
					}
					if problem == "mismatched-thread" {
						m["thread_id"] = "thr_wrong"
					}
					if problem == "archived" {
						m["archived"] = true
					}
					answer = m
				default:
					t.Fatalf("unexpected call: %v", args)
				}
				raw, _ := json.Marshal(answer)
				return BBResult{Stdout: raw}
			}}
			err := f.PreflightOnPick(context.Background(), req)
			if strings.HasPrefix(problem, "valid-") {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid preflight")
			}
		})
	}
}

func pickRequest() CardRequest {
	r := testReq()
	r.Ask["options"] = []any{map[string]any{"id": "yes", "label": "Blue", "kind": "instruction", "instruction": "Use blue.", "on_pick": map[string]any{"schema": 1, "target_thread": "thr_a", "reversible": true, "subject": map[string]any{"kind": "none"}, "ruling": map[string]any{"bead": nil, "under": []any{}}}}, map[string]any{"id": "no", "label": "Wait", "kind": "ruling-only"}}
	return r
}

func TestOnPickPreflightRefusesBeforeTasksMutation(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "update"}[update], func(t *testing.T) {
			r := pickRequest()
			if update {
				r.Ask["ask_key"] = "palette"
			}
			f := newFake(t, registryAbsent)
			filer := f.filer()
			f.fail = func(args []string) *BBResult {
				if strings.Join(args[:min(2, len(args))], " ") == "home capabilities" {
					return badRes(1, "unsupported")
				}
				return nil
			}
			var err error
			if update {
				_, err = filer.FileOrUpdateCard(context.Background(), r, "")
			} else {
				_, err = filer.FileCard(context.Background(), r)
			}
			if err == nil || !strings.Contains(err.Error(), "capabilities") {
				t.Fatalf("expected capabilities refusal: %v", err)
			}
			for _, c := range f.calls {
				if c[0] == "tasks" && (c[1] == "create" || c[1] == "update" || c[1] == "comment" || c[1] == "label") {
					t.Fatalf("mutation before preflight: %v", c)
				}
			}
		})
	}
}
