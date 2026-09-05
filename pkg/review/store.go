package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu    sync.Mutex
	dir   string
	state State
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "records"), 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, state: State{Version: Version, Sessions: map[string]Session{}, Feedback: map[string]Feedback{}, Proposals: map[string]Proposal{}, Executions: map[string]Execution{}, Verdicts: map[string]Verdict{}, Receipts: map[string]Receipt{}}}
	files, err := filepath.Glob(filepath.Join(dir, "records", "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) > 0 {
		b, err := os.ReadFile(files[len(files)-1])
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &s.state); err != nil {
			return nil, fmt.Errorf("review records unreadable: %w", err)
		}
		if s.state.Version != Version {
			return nil, errors.New("unsupported review records version")
		}
	}
	return s, nil
}
func (s *Store) Snapshot() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state.Clone() }
func (s *Store) Dir() string     { return s.dir }
func (s *Store) Usage() int64 {
	var size int64
	_ = filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

// Immutable, fsynced revisions are the source of truth. Temporary files are
// ignored on recovery. In-memory state advances only after rename + directory
// sync, so an error never tells the caller their observation was saved.
func (s *Store) commit(next State) error {
	next.Revision = s.state.Revision + 1
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.dir, "records")
	f, err := os.CreateTemp(dir, ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%020d.json", next.Revision))
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	d.Close()
	if err != nil {
		return err
	}
	s.state = next
	return nil
}

func projectPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("project required")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("project must be absolute")
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project is not a directory")
	}
	return p, nil
}

func (s *Store) Apply(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(err error) Response { return Response{Version: Version, Error: err.Error()} }
	if req.Version != Version {
		return fail(errors.New("unsupported IPC version"))
	}
	if req.Method == "state" {
		state := s.state.Clone()
		return Response{Version: Version, State: &state, DataDir: s.dir}
	}
	if req.ID == "" {
		return fail(errors.New("request id required"))
	}
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if receipt, ok := s.state.Receipts[req.ID]; ok {
		if receipt.Hash != hash {
			return fail(errors.New("request id reused with different content"))
		}
		return Response{Version: Version, ID: receipt.ID}
	}
	next := s.state.Clone()
	id, err := s.apply(&next, req)
	if err != nil {
		return fail(err)
	}
	next.Receipts[req.ID] = Receipt{Hash: hash, ID: id}
	if err = s.commit(next); err != nil {
		return fail(fmt.Errorf("not saved: %w", err))
	}
	return Response{Version: Version, ID: id}
}

func (s *Store) apply(st *State, r Request) (string, error) {
	now := time.Now().UTC()
	id := r.Target
	if id == "" {
		id = NewID()
	}
	project := ""
	if r.Project != "" {
		var err error
		project, err = projectPath(r.Project)
		if err != nil {
			return "", err
		}
	}
	same := func(p string) error {
		canonical, err := projectPath(p)
		if err != nil || project == "" || canonical != project {
			return errors.New("project mismatch")
		}
		return nil
	}
	switch r.Method {
	case "context":
		if r.Context == nil {
			return "", errors.New("context required")
		}
		st.Context = *r.Context
		st.Context.At = now
		return r.ID, nil
	case "session.save":
		if r.Session == nil || project == "" {
			return "", errors.New("session and project required")
		}
		v := *r.Session
		if v.ID == "" {
			v.ID = id
		}
		if old, ok := st.Sessions[v.ID]; ok {
			if err := same(old.Project); err != nil {
				return "", err
			}
			if v.Revision != old.Revision {
				return "", errors.New("stale session")
			}
			v.StartedAt = old.StartedAt
		} else {
			v.StartedAt = now
		}
		v.Project = project
		v.Revision++
		v.UpdatedAt = now
		st.Sessions[v.ID] = v
		return v.ID, nil
	case "feedback.save":
		v := Feedback{Text: r.Text}
		if r.Feedback != nil {
			v = *r.Feedback
		}
		if strings.TrimSpace(v.Text) == "" && len(v.Evidence) == 0 {
			return "", errors.New("feedback needs text or evidence")
		}
		if v.ID == "" {
			v.ID = id
		}
		if old, ok := st.Feedback[v.ID]; ok {
			if old.Project != project {
				return "", errors.New("project mismatch")
			}
			if v.Revision != old.Revision {
				return "", errors.New("stale feedback")
			}
			v.At = old.At
			v.OriginalText = old.OriginalText
			v.Evidence = mergeEvidence(old.Evidence, v.Evidence)
		} else {
			v.At = now
			v.OriginalText = v.Text
		}
		if v.SessionID != "" {
			session, ok := st.Sessions[v.SessionID]
			if !ok || session.Project != project {
				return "", errors.New("feedback session project mismatch")
			}
		}
		v.Project = project
		v.Revision++
		v.Analysis = "pending"
		if v.Context.At.IsZero() {
			v.Context = st.Context
		}
		if project == "" {
			v.SuggestedProject = st.Context.Project
			v.Analysis = "intake"
		}
		st.Feedback[v.ID] = v
		if v.Revision == 1 && v.SessionID != "" && len(v.Evidence) == 0 {
			st.Commands = append(st.Commands, CaptureCommand{ID: NewID(), Project: project, Method: "snapshot", Target: v.ID, Status: "pending"})
		}
		return v.ID, nil
	case "proposal.save":
		if r.Proposal == nil {
			return "", errors.New("proposal required")
		}
		v := *r.Proposal
		if err := same(v.Project); err != nil {
			return "", err
		}
		v.Project = project
		if v.ID == "" {
			v.ID = id
		}
		if old, ok := st.Proposals[v.ID]; ok {
			if err := same(old.Project); err != nil {
				return "", err
			}
			if old.Status == "accepted" {
				return "", errors.New("accepted proposals are immutable; propose a superseding change")
			}
			if v.Revision != old.Revision+1 {
				return "", errors.New("stale proposal")
			}
		}
		if v.Change == "" || v.Outcome == "" || len(v.Scope) == 0 || len(v.FeedbackIDs) == 0 || len(v.Checklist) == 0 {
			return "", errors.New("proposal requires outcome, change, scope, feedback and retest checklist")
		}
		if v.Priority < 0 || v.Priority > 4 || v.BudgetTokens < 0 {
			return "", errors.New("invalid priority or budget")
		}
		for _, f := range v.FeedbackIDs {
			note, ok := st.Feedback[f]
			if !ok || note.Project != project {
				return "", errors.New("proposal references feedback outside project")
			}
		}
		for _, g := range v.Guidance {
			if g.Text == "" || g.Scope == "" || g.Rationale == "" || g.BaseRevision == "" || filepath.IsAbs(g.Path) || strings.HasPrefix(filepath.Clean(g.Path), "..") {
				return "", errors.New("guidance requires relative canonical path, revision, text, scope and rationale")
			}
		}
		v.Status = "proposed"
		v.AcceptedAt = nil
		v.At = now
		if v.Revision == 0 {
			v.Revision = 1
		}
		st.Proposals[v.ID] = v
		return v.ID, nil
	case "proposal.accept":
		v, ok := st.Proposals[r.Target]
		if !ok {
			return "", errors.New("proposal missing")
		}
		if err := same(v.Project); err != nil {
			return "", err
		}
		if v.Revision != r.Revision {
			return "", errors.New("proposal changed; review the current revision")
		}
		if v.Status == "accepted" {
			return v.ID, nil
		}
		if v.Status != "proposed" {
			return "", errors.New("proposal is not awaiting acceptance")
		}
		v.Status = "accepted"
		v.AcceptedAt = &now
		st.Proposals[v.ID] = v
		status, reason := "queued", "Accepted scope awaits Clavain"
		if v.Priority > 2 || len(v.Dependencies) > 0 {
			status, reason = "deferred", "Priority or dependencies require scheduling"
		}
		st.Executions[v.ID] = Execution{ID: v.ID, Project: project, ProposalID: v.ID, ProposalRevision: v.Revision, Status: status, Reason: reason, UpdatedAt: now}
		return v.ID, nil
	case "proposal.reject":
		v, ok := st.Proposals[r.Target]
		if !ok || v.Status != "proposed" || v.Revision != r.Revision {
			return "", errors.New("proposal is stale or not proposed")
		}
		if err := same(v.Project); err != nil {
			return "", err
		}
		v.Status = "rejected"
		st.Proposals[v.ID] = v
		return v.ID, nil
	case "execution.save":
		if r.Execution == nil {
			return "", errors.New("execution required")
		}
		v := *r.Execution
		old, ok := st.Executions[v.ID]
		if !ok {
			return "", errors.New("execution requires accepted proposal")
		}
		if err := same(old.Project); err != nil {
			return "", err
		}
		if v.ProposalID != old.ProposalID || v.ProposalRevision != old.ProposalRevision || v.Project != old.Project {
			return "", errors.New("execution scope changed")
		}
		v.UpdatedAt = now
		st.Executions[v.ID] = v
		return v.ID, nil
	case "verdict.save":
		if r.Verdict == nil {
			return "", errors.New("verdict required")
		}
		v := *r.Verdict
		e, ok := st.Executions[v.ExecutionID]
		if !ok || e.Status != "ready_for_retest" || e.Build == "" || v.Build != e.Build {
			return "", errors.New("verdict requires the ready-for-retest build")
		}
		if err := same(e.Project); err != nil {
			return "", err
		}
		if v.Verdict != "pass" && v.Verdict != "fail" && v.Verdict != "inconclusive" {
			return "", errors.New("verdict must be pass, fail or inconclusive")
		}
		v.ID = id
		v.Project = project
		v.At = now
		st.Verdicts[id] = v
		return id, nil
	case "turn.save":
		if r.Turn == nil {
			return "", errors.New("turn required")
		}
		v := *r.Turn
		v.ID = id
		v.Project = project
		v.At = now
		st.Turns = append(st.Turns, v)
		return id, nil
	case "question.save":
		if r.Question == nil {
			return "", errors.New("question required")
		}
		v := *r.Question
		v.Project = project
		v.Status = "pending"
		for _, q := range st.Questions {
			if q.ID == v.ID && q.RuntimeSession == v.RuntimeSession {
				return "", errors.New("question already exists")
			}
		}
		st.Questions = append(st.Questions, v)
		return v.ID, nil
	case "question.answer", "question.cancel":
		for i, q := range st.Questions {
			if q.ID == r.Target && q.Project == project {
				if q.Status != "pending" {
					return "", errors.New("question no longer pending")
				}
				if r.Method == "question.cancel" {
					q.Status = "cancelled"
				} else {
					q.Status = "answered"
					q.Answer = r.Text
				}
				st.Questions[i] = q
				return q.ID, nil
			}
		}
		return "", errors.New("question missing")
	case "capture.command":
		switch r.Text {
		case "open", "pause", "resume", "stop", "snapshot", "voice", "play":
		default:
			return "", errors.New("unknown capture command")
		}
		st.Commands = append(st.Commands, CaptureCommand{ID: id, Project: project, Method: r.Text, Target: r.Target, Source: r.Source, Status: "pending"})
		return id, nil
	case "capture.ack":
		for i, c := range st.Commands {
			if c.ID == r.Target {
				c.Status = r.Status
				st.Commands[i] = c
				return c.ID, nil
			}
		}
		return "", errors.New("capture command missing")
	default:
		return "", fmt.Errorf("unknown method %q", r.Method)
	}
}
func mergeEvidence(old, added []Source) []Source {
	out := append([]Source(nil), old...)
	for _, a := range added {
		found := false
		for i, v := range out {
			if v.ID == a.ID {
				out[i] = a
				found = true
				break
			}
		}
		if !found {
			out = append(out, a)
		}
	}
	return out
}
