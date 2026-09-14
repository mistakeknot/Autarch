package door

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type adapterRunner struct {
	responses map[string]string
	errAt     string
	calls     [][]string
}

func (r *adapterRunner) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	key := args[2]
	if key == r.errAt {
		return []byte("candidate unavailable"), errors.New("exit 1")
	}
	return []byte(r.responses[key]), nil
}

const candidateContract = `"binding_authority":"not_live","implementation_allowed":false,"coverage":"none","coverage_scope":"enforcement","authority_status":"available_read_only","history_status":"unavailable","attached_sessions":"unknown","classification_limit":"lexical evidence only"`

func TestWorkAdapterUnavailableByDefault(t *testing.T) {
	got := NewWorkAdapter(WorkAdapterConfig{}).Read(context.Background(), "/project")
	if got.State != "unavailable" || !strings.Contains(got.Error, "not configured") {
		t.Fatalf("candidate dependency was not unavailable by default: %+v", got)
	}
}

func TestWorkAdapterStrictQualifiedReadAndAmbiguity(t *testing.T) {
	r := &adapterRunner{responses: map[string]string{
		"discover": `{` + candidateContract + `,"command":"discover","result_status":"candidates_found","candidates":[{"tracker_uuid":"11111111-1111-4111-8111-111111111111","bead_id":"Sylveste-fuwn","title":"Workbench","beads_status":"in_progress","beads_assignee":"codex","repository_aliases":["autarch"],"evidence_score":9,"matched_fields":["title"],"classification":"exact_tracker_task_reference","reason":"exact qualified reference"},{"tracker_uuid":"22222222-2222-4222-8222-222222222222","bead_id":"Sylveste-fuwn","title":"Pinned routing incident","beads_status":"open","beads_assignee":"","repository_aliases":["other"],"evidence_score":4,"matched_fields":["title"],"classification":"ambiguous_shared_evidence_candidate","reason":"lexical only"}],"explanation":"candidates are evidence only"}`,
		"status":   `{` + candidateContract + `,"command":"status","result_status":"task_found","task":{"tracker_uuid":"11111111-1111-4111-8111-111111111111","bead_id":"Sylveste-fuwn","title":"Workbench","beads_status":"in_progress","beads_assignee":"codex","repository_aliases":["autarch"]},"explanation":"BEADS fields only"}`,
		"explain":  `{` + candidateContract + `,"command":"explain","result_status":"task_found","task":{"tracker_uuid":"11111111-1111-4111-8111-111111111111","bead_id":"Sylveste-fuwn","title":"Workbench","beads_status":"in_progress","beads_assignee":"codex","repository_aliases":["autarch"]},"explanation":"continuity with selected outcome"}`,
	}}
	a := NewWorkAdapter(WorkAdapterConfig{Binary: "/opt/clavain-cli", Registry: "/registry", Authority: "human", Runner: r})
	got := a.Read(context.Background(), "/project 11111111-1111-4111-8111-111111111111:Sylveste-fuwn")
	if got.State != "read" || len(got.Tasks) != 1 || got.Tasks[0].TrackerUUID != "11111111-1111-4111-8111-111111111111" || got.Tasks[0].BeadID != "Sylveste-fuwn" || got.Tasks[0].Assignee != "codex" || got.Tasks[0].Explanation == "" || len(got.Ambiguous) != 1 || got.Ambiguous[0].TrackerUUID != "22222222-2222-4222-8222-222222222222" || got.Ambiguous[0].BeadID != "Sylveste-fuwn" || got.Ambiguous[0].Assignee != "unassigned" {
		t.Fatalf("qualified evidence lost: %+v", got)
	}
	if len(r.calls) != 3 {
		t.Fatalf("expected discover/status/explain, got %+v", r.calls)
	}
	for _, want := range []string{"work discover", "--text /project 11111111-1111-4111-8111-111111111111:Sylveste-fuwn", "work status", "work explain", "--task 11111111-1111-4111-8111-111111111111:Sylveste-fuwn", "--registry /registry", "--authority human"} {
		if joined := strings.Join([]string{strings.Join(r.calls[0], " "), strings.Join(r.calls[1], " "), strings.Join(r.calls[2], " ")}, "\n"); !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q: %q", want, joined)
		}
	}
}

func TestWorkAdapterDoesNotInferQualifiedTaskFromAmbiguousMatch(t *testing.T) {
	r := &adapterRunner{responses: map[string]string{
		"discover": `{` + candidateContract + `,"command":"discover","result_status":"candidates_found","candidates":[{"tracker_uuid":"22222222-2222-4222-8222-222222222222","bead_id":"Sylveste-other","title":"Other","beads_status":"in_progress","beads_assignee":"","repository_aliases":["other"],"evidence_score":4,"matched_fields":["title"],"classification":"active_work_unranked","reason":"active only"}],"explanation":"evidence only"}`,
	}}
	got := NewWorkAdapter(WorkAdapterConfig{Binary: "/opt/clavain-cli", Registry: "/registry", Authority: "human", Runner: r}).Read(context.Background(), "/project")
	if got.State != "read" || len(got.Tasks) != 0 || len(got.Ambiguous) != 1 || len(r.calls) != 1 {
		t.Fatalf("ambiguous match became a qualified task: got=%+v calls=%+v", got, r.calls)
	}
}

func TestWorkAdapterPreservesMalformedNonzeroAndContractMismatch(t *testing.T) {
	base := WorkAdapterConfig{Binary: "/opt/clavain-cli", Registry: "/registry", Authority: "human"}
	for _, tc := range []struct {
		name   string
		runner *adapterRunner
		want   string
	}{
		{"malformed", &adapterRunner{responses: map[string]string{"discover": `{`}}, "malformed"},
		{"nonzero", &adapterRunner{responses: map[string]string{}, errAt: "discover"}, "candidate unavailable"},
		{"live-authority", &adapterRunner{responses: map[string]string{"discover": `{"command":"discover","binding_authority":"live","implementation_allowed":false,"coverage":"none","coverage_scope":"enforcement","authority_status":"available_read_only","result_status":"no_results","history_status":"unavailable","attached_sessions":"unknown","classification_limit":"lexical","explanation":"none"}`}}, "binding_authority"},
		{"implementation", &adapterRunner{responses: map[string]string{"discover": `{"command":"discover","binding_authority":"not_live","implementation_allowed":true,"coverage":"none","coverage_scope":"enforcement","authority_status":"available_read_only","result_status":"no_results","history_status":"unavailable","attached_sessions":"unknown","classification_limit":"lexical","explanation":"none"}`}}, "implementation_allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Runner = tc.runner
			got := NewWorkAdapter(cfg).Read(context.Background(), "/project")
			if got.State != "unavailable" || !strings.Contains(got.Error, tc.want) {
				t.Fatalf("%+v", got)
			}
		})
	}
}
