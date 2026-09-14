package door

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type WorkCandidate struct {
	TrackerUUID    string
	BeadID         string
	Title          string
	Status         string
	Assignee       string
	Classification string
	Reason         string
	MatchedFields  []string
	EvidenceScore  int
	Explanation    string
}

type WorkDiscovery struct {
	State       string
	Error       string
	Explanation string
	Tasks       []WorkCandidate
	Ambiguous   []WorkCandidate
	ObservedAt  time.Time
}

type WorkRunner interface {
	Run(context.Context, io.Reader, ...string) ([]byte, error)
}

type WorkAdapterConfig struct {
	Binary, Registry, Authority string
	Runner                      WorkRunner
}

func WorkAdapterConfigFromEnv() WorkAdapterConfig {
	return WorkAdapterConfig{
		Binary:    os.Getenv("AUTARCH_WORK_BINARY"),
		Registry:  os.Getenv("AUTARCH_WORK_REGISTRY"),
		Authority: os.Getenv("AUTARCH_WORK_AUTHORITY"),
	}
}

type WorkAdapter struct{ config WorkAdapterConfig }

func NewWorkAdapter(config WorkAdapterConfig) WorkAdapter { return WorkAdapter{config: config} }

type workExecRunner struct{}

func (workExecRunner) Run(ctx context.Context, _ io.Reader, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("binary required")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = 250 * time.Millisecond
	var stdout limitedProductOutput
	stdout.limit = 1 << 20
	var stderr limitedProductOutput
	stderr.limit = 8 << 10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if stdout.overflow {
		return nil, errors.New("candidate output exceeds 1 MiB")
	}
	if stderr.overflow {
		return nil, errors.New("candidate error output exceeds 8 KiB")
	}
	if err != nil {
		return []byte(strings.TrimSpace(stderr.String())), err
	}
	return stdout.Bytes(), nil
}

type candidateTask struct {
	TrackerUUID string   `json:"tracker_uuid"`
	BeadID      string   `json:"bead_id"`
	Title       string   `json:"title"`
	Status      string   `json:"beads_status"`
	Assignee    string   `json:"beads_assignee"`
	Aliases     []string `json:"repository_aliases"`
}

type candidateMatch struct {
	TrackerUUID    string   `json:"tracker_uuid"`
	BeadID         string   `json:"bead_id"`
	Title          string   `json:"title"`
	Status         string   `json:"beads_status"`
	Assignee       string   `json:"beads_assignee"`
	Aliases        []string `json:"repository_aliases"`
	EvidenceScore  int      `json:"evidence_score"`
	MatchedFields  []string `json:"matched_fields"`
	Classification string   `json:"classification"`
	Reason         string   `json:"reason"`
}

type candidateEnvelope struct {
	Command               string           `json:"command"`
	BindingAuthority      string           `json:"binding_authority"`
	ImplementationAllowed bool             `json:"implementation_allowed"`
	Coverage              string           `json:"coverage"`
	CoverageScope         string           `json:"coverage_scope"`
	AuthorityStatus       string           `json:"authority_status"`
	ResultStatus          string           `json:"result_status"`
	HistoryStatus         string           `json:"history_status"`
	AttachedSessions      string           `json:"attached_sessions"`
	ClassificationLimit   string           `json:"classification_limit"`
	Task                  *candidateTask   `json:"task,omitempty"`
	Candidates            []candidateMatch `json:"candidates,omitempty"`
	Explanation           string           `json:"explanation,omitempty"`
}

func decodeCandidate(phase string, data []byte) (candidateEnvelope, error) {
	var envelope candidateEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, fmt.Errorf("%s returned malformed JSON: %w", phase, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return envelope, fmt.Errorf("%s returned trailing or malformed JSON", phase)
	}
	if envelope.Command != phase {
		return envelope, fmt.Errorf("%s returned command %q", phase, envelope.Command)
	}
	if envelope.BindingAuthority != "not_live" {
		return envelope, fmt.Errorf("%s contract binding_authority must be not_live", phase)
	}
	if envelope.ImplementationAllowed {
		return envelope, fmt.Errorf("%s contract implementation_allowed must be false", phase)
	}
	if envelope.Coverage != "none" || envelope.CoverageScope != "enforcement" {
		return envelope, fmt.Errorf("%s contract coverage must be none for enforcement", phase)
	}
	if envelope.AuthorityStatus != "available_read_only" || envelope.HistoryStatus != "unavailable" || envelope.AttachedSessions != "unknown" {
		return envelope, fmt.Errorf("%s returned unsupported authority, history, or session status", phase)
	}
	if strings.TrimSpace(envelope.ClassificationLimit) == "" {
		return envelope, fmt.Errorf("%s omitted its classification limit", phase)
	}
	return envelope, nil
}

var workUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateCandidateTask(tracker, bead string) error {
	if !workUUID.MatchString(tracker) || strings.TrimSpace(bead) == "" || strings.TrimSpace(bead) != bead {
		return errors.New("candidate task requires an exact tracker UUID and Bead ID")
	}
	return nil
}

func displayCandidateAssignee(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unassigned"
	}
	return value
}

func (a WorkAdapter) invoke(ctx context.Context, runner WorkRunner, phase string, extra ...string) (candidateEnvelope, error) {
	args := []string{a.config.Binary, "work", phase, "--json", "--registry", a.config.Registry, "--authority", a.config.Authority}
	args = append(args, extra...)
	out, err := runner.Run(ctx, nil, args...)
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return candidateEnvelope{}, fmt.Errorf("%s unavailable: %s", phase, detail)
	}
	return decodeCandidate(phase, out)
}

// Read invokes only the reviewed read-only verbs. Evidence is passed as data,
// never through a shell. The adapter is inert until all three explicit values
// are configured; mk-ag2s.25's source candidate is not imported or discovered.
func (a WorkAdapter) Read(ctx context.Context, evidence string) WorkDiscovery {
	cfg := a.config
	if cfg.Binary == "" || cfg.Registry == "" || cfg.Authority == "" {
		return WorkDiscovery{State: "unavailable", Error: "clavain work adapter not configured; reviewed candidate is not qualified or installed"}
	}
	if !filepath.IsAbs(cfg.Binary) || !filepath.IsAbs(cfg.Registry) || strings.ContainsAny(cfg.Authority, "\x00\n\r") {
		return WorkDiscovery{State: "unavailable", Error: "clavain work adapter requires absolute binary/registry and explicit authority"}
	}
	runner := cfg.Runner
	if runner == nil {
		runner = workExecRunner{}
	}
	discover, err := a.invoke(ctx, runner, "discover", "--text", evidence)
	if err != nil {
		return WorkDiscovery{State: "unavailable", Error: err.Error()}
	}
	if discover.ResultStatus != "candidates_found" && discover.ResultStatus != "no_results" {
		return WorkDiscovery{State: "unavailable", Error: "discover returned an unknown result status"}
	}
	result := WorkDiscovery{State: "read", Explanation: discover.Explanation, ObservedAt: time.Now().UTC()}
	var exact []candidateMatch
	for _, match := range discover.Candidates {
		if err := validateCandidateTask(match.TrackerUUID, match.BeadID); err != nil {
			return WorkDiscovery{State: "unavailable", Error: "discover: " + err.Error()}
		}
		candidate := WorkCandidate{
			TrackerUUID: match.TrackerUUID, BeadID: match.BeadID, Title: match.Title,
			Status: match.Status, Assignee: displayCandidateAssignee(match.Assignee),
			Classification: match.Classification, Reason: match.Reason,
			MatchedFields: append([]string(nil), match.MatchedFields...), EvidenceScore: match.EvidenceScore,
		}
		if match.Classification == "exact_tracker_task_reference" {
			exact = append(exact, match)
		} else {
			result.Ambiguous = append(result.Ambiguous, candidate)
		}
	}
	if len(exact) > 16 {
		return WorkDiscovery{State: "unavailable", Error: "discover returned more than 16 exact task references"}
	}
	for _, match := range exact {
		qualified := match.TrackerUUID + ":" + match.BeadID
		status, err := a.invoke(ctx, runner, "status", "--task", qualified)
		if err != nil {
			return WorkDiscovery{State: "unavailable", Error: err.Error()}
		}
		explain, err := a.invoke(ctx, runner, "explain", "--task", qualified)
		if err != nil {
			return WorkDiscovery{State: "unavailable", Error: err.Error()}
		}
		if status.ResultStatus != "task_found" || explain.ResultStatus != "task_found" || status.Task == nil || explain.Task == nil {
			return WorkDiscovery{State: "unavailable", Error: "status or explain did not return the qualified task"}
		}
		for _, observed := range []*candidateTask{status.Task, explain.Task} {
			if observed.TrackerUUID != match.TrackerUUID || observed.BeadID != match.BeadID || displayCandidateAssignee(observed.Assignee) != displayCandidateAssignee(match.Assignee) {
				return WorkDiscovery{State: "unavailable", Error: "status or explain changed the qualified task identity or assignee"}
			}
		}
		result.Tasks = append(result.Tasks, WorkCandidate{
			TrackerUUID: match.TrackerUUID, BeadID: match.BeadID, Title: status.Task.Title,
			Status: status.Task.Status, Assignee: displayCandidateAssignee(status.Task.Assignee),
			Classification: match.Classification, Reason: match.Reason,
			MatchedFields: append([]string(nil), match.MatchedFields...), EvidenceScore: match.EvidenceScore,
			Explanation: explain.Explanation,
		})
	}
	return result
}
