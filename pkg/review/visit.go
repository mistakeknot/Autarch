package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

type VisitAnswer struct {
	ID         string    `json:"id"`
	QuestionID string    `json:"question_id"`
	Text       string    `json:"text"`
	Supersedes string    `json:"supersedes,omitempty"`
	At         time.Time `json:"at"`
}
type Visit struct {
	ID             string        `json:"id"`
	Project        string        `json:"project"`
	Outcome        string        `json:"outcome,omitempty"`
	View           string        `json:"view"`
	Layout         string        `json:"layout"`
	Pace           string        `json:"pace"`
	Density        string        `json:"density"`
	Selection      string        `json:"selection,omitempty"`
	Answers        []VisitAnswer `json:"answers"`
	TranscriptIDs  []string      `json:"transcript_ids"`
	OpenQuestionID string        `json:"open_question_id,omitempty"`
	RuntimeSession string        `json:"runtime_session,omitempty"`
	PreparationID  string        `json:"preparation_id,omitempty"`
	Status         string        `json:"status"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

func (st State) ProjectVisit(project string) (Visit, bool) {
	var latest Visit
	for _, v := range st.Visits {
		if v.Project == project && (latest.ID == "" || v.UpdatedAt.After(latest.UpdatedAt)) {
			latest = v
		}
	}
	return latest, latest.ID != ""
}

func applyVisit(st *State, r Request, project, id string, now time.Time) (string, error) {
	v, exists := st.ProjectVisit(project)
	if project == "" {
		return "", errors.New("visit requires a project")
	}
	if r.Method == "visit.open" {
		if exists {
			return v.ID, nil
		}
		v = Visit{ID: id, Project: project, Outcome: r.Text, View: "outcome", Layout: "purpose", Pace: "all", Density: "Cozy", Status: "exploring", UpdatedAt: now}
		st.Visits[v.ID] = v
		return v.ID, nil
	}
	if !exists || r.VisitID != v.ID {
		return "", errors.New("current project visit required")
	}
	switch r.Method {
	case "visit.view":
		if r.Visit == nil {
			return "", errors.New("view preferences required")
		}
		p := r.Visit
		if (p.View != "outcome" && p.View != "map") || (p.Layout != "purpose" && p.Layout != "theme") || (p.Density != "Cozy" && p.Density != "Compact") {
			return "", errors.New("invalid visit view preferences")
		}
		if p.Pace != "all" && p.Pace != "fast" && p.Pace != "steady" && p.Pace != "slow" && p.Pace != "unknown" {
			return "", errors.New("invalid pace filter")
		}
		v.View, v.Layout, v.Pace, v.Density, v.Selection = p.View, p.Layout, p.Pace, p.Density, p.Selection
	case "visit.outcome":
		if r.Text == "" {
			return "", errors.New("outcome required")
		}
		v.Outcome = r.Text
	case "visit.correct":
		if r.Text == "" {
			return "", errors.New("exact correction wording required")
		}
		var answer *VisitAnswer
		for i := range v.Answers {
			if v.Answers[i].Supersedes == r.Target {
				return "", errors.New("answer already corrected; correct its current successor")
			}
			if v.Answers[i].ID == r.Target {
				answer = &v.Answers[i]
			}
		}
		if answer == nil {
			return "", errors.New("answer missing from visit")
		}
		v.Answers = append(v.Answers, VisitAnswer{ID: r.ID, QuestionID: answer.QuestionID, Text: r.Text, Supersedes: r.Target, At: now})
		for key, p := range st.Proposals {
			if p.VisitID != v.ID || p.Status == "accepted" {
				continue
			}
			for _, a := range p.AnswerIDs {
				if a == r.Target {
					p.Status = "superseded"
					st.Proposals[key] = p
				}
			}
		}
		for i, q := range st.Questions {
			if q.VisitID == v.ID && q.Status == "pending" {
				st.Questions[i].Status = "superseded"
				st.Questions[i].Delivery = "pending"
			}
		}
		v.OpenQuestionID = ""
		st.Turns = append(st.Turns, Turn{ID: r.ID, Project: project, Kind: "user", Text: r.Text, At: now, Delivery: "pending"})
		v.TranscriptIDs = append(v.TranscriptIDs, r.ID)
	case "visit.runtime":
		if r.Text == "" {
			return "", errors.New("runtime identity required")
		}
		v.RuntimeSession = r.Text
		for i, q := range st.Questions {
			if q.VisitID != v.ID || q.Status != "pending" || q.RuntimeSession == r.Text {
				continue
			}
			st.Questions[i].Status = "expired"
			st.Questions[i].Delivery = "unavailable"
			q.PredecessorID = q.ID
			q.ID = NewID()
			q.RuntimeSession = r.Text
			q.Delivery = ""
			q.Answer = ""
			st.Questions = append(st.Questions, q)
			if q.Consequential {
				v.OpenQuestionID = q.ID
			}
		}
	default:
		return "", errors.New("unknown visit operation")
	}
	v.UpdatedAt = now
	st.Visits[v.ID] = v
	return v.ID, nil
}

// Mandatory context is exact retained source wording. Byte bounds are explicit,
// not a token estimate; exceeding them blocks the handoff instead of truncation.
func (st State) VisitContext(project string, maxBytes int) (string, error) {
	var accepted []Proposal
	for _, p := range st.Proposals {
		if p.Project == project && p.Status == "accepted" {
			accepted = append(accepted, p)
		}
	}
	sort.Slice(accepted, func(i, j int) bool { return accepted[i].ID < accepted[j].ID })
	v, _ := st.ProjectVisit(project)
	var pending []Question
	var preparations []map[string]any
	for _, p := range st.Preparations {
		if p.Project == project {
			preparations = append(preparations, map[string]any{
				"id": p.ID, "proposal_id": p.ProposalID, "proposal_revision": p.ProposalRevision,
				"status": p.Status, "reason": p.Reason, "budget_tokens": p.BudgetTokens,
				"request_sha256": digestBytes(p.Request), "receipt_sha256": digestBytes(p.Receipt),
				"artifact_reference": "preparation:" + p.ID,
			})
		}
	}
	sort.Slice(preparations, func(i, j int) bool { return preparations[i]["id"].(string) < preparations[j]["id"].(string) })
	for _, q := range st.Questions {
		if q.Project == project && q.Status == "pending" {
			pending = append(pending, q)
		}
	}
	payload := struct {
		Visit        Visit            `json:"visit"`
		Accepted     []Proposal       `json:"accepted_rulings"`
		Pending      []Question       `json:"pending_questions"`
		Preparations []map[string]any `json:"preparations"`
	}{v, accepted, pending, preparations}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if maxBytes <= 0 || len(data) > maxBytes {
		return "", fmt.Errorf("insufficient context budget: mandatory guidance needs %d bytes; budget is %d", len(data), maxBytes)
	}
	return string(data), nil
}

func validateVisitProposal(st *State, p Proposal) error {
	v, ok := st.Visits[p.VisitID]
	if !ok || v.Project != p.Project || len(p.AnswerIDs) == 0 {
		return errors.New("guidance synthesis must cite a visit and its exact answers")
	}
	for _, id := range p.AnswerIDs {
		found := false
		for _, a := range v.Answers {
			if a.Supersedes == id {
				return errors.New("synthesis cites a corrected answer")
			}
			if a.ID == id {
				found = true
			}
		}
		if !found {
			return errors.New("synthesis answer not present in visit")
		}
	}
	return nil
}
