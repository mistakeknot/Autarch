package homeask

// Reader-only v2 metadata. These claims confer no authority and stay outside v1
// asks and all identity projections.
import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"
)

type SubjectKey struct {
	Type      string  `json:"type"`
	Repo      string  `json:"repo,omitempty"`
	Number    int64   `json:"number,omitempty"`
	Head      *string `json:"head,omitempty"`
	Round     *int64  `json:"round,omitempty"`
	ScriptSHA string  `json:"script_sha,omitempty"`
}

// SignalEvidence retains JSON values without interpreting gate evidence.
type SignalEvidence struct {
	Known  bool   `json:"known"`
	Value  any    `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Signals struct {
	Schema     int                       `json:"schema"`
	Kind       string                    `json:"kind"`
	Approval   string                    `json:"approval"`
	Evidence   map[string]SignalEvidence `json:"evidence"`
	Conditions map[string]string         `json:"conditions"`
	Failed     []string                  `json:"failed"`
	CheckedAt  string                    `json:"checked_at"`
}

var signalItems = []string{"base", "default_branch", "head_start", "head_end", "visibility", "checks", "files", "receipt", "review_text", "coverage", "open_cards", "rule_history"}
var mergeConditions = []string{"base.default", "head.stable", "repo.private", "review.provenance", "review.head", "review.bound", "review.verdict", "review.tests", "checks.green", "coverage.entry", "ask.recommendation", "keep.paths", "keep.complete", "subject.single", "subject.unvoided"}
var roundConditions = []string{"head.stable", "repo.private", "review.provenance", "review.head", "review.bound", "keep.paths", "keep.complete", "subject.single", "subject.unvoided", "round.trigger", "round.history", "round.new", "round.fewer", "round.scope", "ask.recommendation", "coverage.entry"}
var conditionReads = map[string][]string{
	"base.default": {"base", "default_branch"}, "head.stable": {"head_start", "head_end"}, "repo.private": {"visibility"},
	"review.provenance": {"receipt"}, "review.head": {"receipt", "head_end"}, "review.bound": {"receipt", "review_text"}, "review.verdict": {"receipt"}, "review.tests": {"receipt"},
	"checks.green": {"checks"}, "coverage.entry": {"coverage"}, "ask.recommendation": {}, "keep.paths": {"files"}, "keep.complete": {"files"}, "subject.single": {"open_cards"}, "subject.unvoided": {"rule_history"},
	"round.trigger": {"receipt"}, "round.history": {"receipt"}, "round.new": {"receipt"}, "round.fewer": {"receipt"}, "round.scope": {"receipt", "files"},
}

func metadataObject(raw any, keys []string, what string) (map[string]any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", what)
	}
	if err := acExactKeys(m, keys, what); err != nil {
		return nil, err
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return nil, fmt.Errorf("%s missing %s", what, k)
		}
	}
	return m, nil
}
func metadataInteger(raw any, min, max float64) (int64, bool) {
	n, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	return int64(f), err == nil && f >= min && f <= max && math.Trunc(f) == f
}
func parseSubjectKey(raw any) (*SubjectKey, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("subject_key must be an object")
	}
	typ, _ := m["type"].(string)
	s := &SubjectKey{Type: typ}
	switch typ {
	case "pr":
		if _, err := metadataObject(raw, []string{"type", "repo", "number", "head", "round"}, subjectKeyMetadata); err != nil {
			return nil, err
		}
		s.Repo, _ = m["repo"].(string)
		if !acRepoRe.MatchString(s.Repo) {
			return nil, errors.New("subject_key repo must be owner/name")
		}
		n, ok := metadataInteger(m["number"], 1, 999999999)
		if !ok {
			return nil, errors.New("subject_key number must be a pull request number")
		}
		s.Number = n
		if m["head"] != nil {
			head, ok := m["head"].(string)
			if !ok || !acHeadRe.MatchString(head) {
				return nil, errors.New("subject_key head must be null or 40 lowercase hex characters")
			}
			s.Head = &head
		}
		if m["round"] != nil {
			n, ok := metadataInteger(m["round"], 1, 9007199254740991)
			if !ok {
				return nil, errors.New("subject_key round must be null or a positive safe integer")
			}
			s.Round = &n
		}
	case "script":
		if _, err := metadataObject(raw, []string{"type", "script_sha"}, subjectKeyMetadata); err != nil {
			return nil, err
		}
		s.ScriptSHA, _ = m["script_sha"].(string)
		if !rootSHARe.MatchString(s.ScriptSHA) {
			return nil, errors.New("subject_key script_sha must be 64 lowercase hex characters")
		}
	default:
		return nil, errors.New("subject_key type must be pr or script")
	}
	return s, nil
}
func parseSignals(raw any) (*Signals, error) {
	m, err := metadataObject(raw, []string{"schema", "kind", "approval", "evidence", "conditions", "failed", "checked_at"}, signalsMetadata)
	if err != nil {
		return nil, err
	}
	if n, ok := metadataInteger(m["schema"], 1, 1); !ok || n != 1 {
		return nil, errors.New("signals schema must be 1")
	}
	kind, _ := m["kind"].(string)
	ids := mergeConditions
	if kind == "round" {
		ids = roundConditions
	} else if kind != "merge" {
		return nil, errors.New("signals kind must be merge or round")
	}
	approval, ok := m["approval"].(string)
	if !ok || approval == "" {
		return nil, errors.New("signals approval must be a nonempty string")
	}
	checked, ok := m["checked_at"].(string)
	if !ok || checked == "" {
		return nil, errors.New("signals checked_at must be a nonempty string")
	}
	ev, err := metadataObject(m["evidence"], signalItems, "signals evidence")
	if err != nil {
		return nil, err
	}
	s := &Signals{Schema: 1, Kind: kind, Approval: approval, CheckedAt: checked, Evidence: map[string]SignalEvidence{}, Conditions: map[string]string{}, Failed: []string{}}
	for _, item := range signalItems {
		e, ok := ev[item].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("signals evidence %s must be an object", item)
		}
		known, ok := e["known"].(bool)
		if !ok {
			return nil, errors.New("signals evidence known must be boolean")
		}
		keys := []string{"known", "reason"}
		if known {
			keys = []string{"known", "value"}
		}
		if _, err := metadataObject(e, keys, "signals evidence "+item); err != nil {
			return nil, err
		}
		entry := SignalEvidence{Known: known}
		if known {
			entry.Value = e["value"]
		} else {
			reason, ok := e["reason"].(string)
			if !ok || utf8.RuneCountInString(reason) > 300 {
				return nil, errors.New("signals reason must be a string of at most 300 characters")
			}
			entry.Reason = reason
		}
		s.Evidence[item] = entry
	}
	cond, err := metadataObject(m["conditions"], ids, "signals conditions")
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		outcome, ok := cond[id].(string)
		if !ok || (outcome != "pass" && outcome != "fail" && outcome != "unknown") {
			return nil, errors.New("signals condition must be pass, fail or unknown")
		}
		s.Conditions[id] = outcome
		if outcome == "pass" {
			for _, item := range conditionReads[id] {
				if !s.Evidence[item].Known {
					return nil, fmt.Errorf("signals pass %s reads unknown %s", id, item)
				}
			}
		} else {
			s.Failed = append(s.Failed, id)
		}
	}
	failed, ok := m["failed"].([]any)
	if !ok || len(failed) != len(s.Failed) {
		return nil, errors.New("signals failed must list exactly the non-pass conditions in order")
	}
	for i, v := range failed {
		if v != s.Failed[i] {
			return nil, errors.New("signals failed must list exactly the non-pass conditions in order")
		}
	}
	return s, nil
}

// MarshalJSON preserves the tagged union, including explicit null PR metadata.
func (s SubjectKey) MarshalJSON() ([]byte, error) {
	if s.Type == "script" {
		return json.Marshal(map[string]any{"type": s.Type, "script_sha": s.ScriptSHA})
	}
	return json.Marshal(map[string]any{"type": s.Type, "repo": s.Repo, "number": s.Number, "head": s.Head, "round": s.Round})
}
func (e SignalEvidence) MarshalJSON() ([]byte, error) {
	if e.Known {
		return json.Marshal(map[string]any{"known": true, "value": e.Value})
	}
	return json.Marshal(map[string]any{"known": false, "reason": e.Reason})
}
