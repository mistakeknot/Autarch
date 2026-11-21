package homefooter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

// ExportOptions selects a read-only pick export.
type ExportOptions struct {
	DB   string
	Days int
	Now  func() time.Time
}

// ExportPicks writes one JSON object for each stored card pick by mk.
func ExportPicks(w io.Writer, o ExportOptions) error {
	if o.Days < 1 || o.Days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}
	if _, err := os.Stat(o.DB); err != nil {
		return fmt.Errorf("home database: %w", err)
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	end := now().UTC()
	q := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(5000)"}}
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: o.DB}).EscapedPath()+"?"+q.Encode())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.Query(`SELECT COALESCE(c.card_key,c.task_id),p.picked_at,c.state, p.option_id,COALESCE(c.title,''),d.kind,d.body_json,d.id,p.pick_id
 FROM picks p JOIN decisions d ON d.id=p.decision_id JOIN cards c ON c.task_id=d.task_id
 WHERE d.source='card' AND p."by"='mk' AND julianday(p.picked_at)>=julianday(?) AND julianday(p.picked_at)<=julianday(?)
 ORDER BY julianday(p.picked_at),p.pick_id`, end.AddDate(0, 0, -o.Days).Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("read card picks: %w", err)
	}
	defer rows.Close()
	enc := json.NewEncoder(w)
	for rows.Next() {
		var r pickExport
		var body string
		if err := rows.Scan(&r.ID, &r.Time, &r.State, &r.Pick, &r.Title, &r.Kind, &body, &r.DecisionID, &r.PickID); err != nil {
			return err
		}
		if err := r.enrich(body); err != nil {
			return fmt.Errorf("decision %s: %w", r.DecisionID, err)
		}
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

type questionRecommendation struct {
	ID               string  `json:"id"`
	RecommendationID *string `json:"recommendation_id"`
	Pick             *string `json:"pick"`
	Matched          *bool   `json:"matched_recommendation"`
}

type pickExport struct {
	ID               string                   `json:"id"`
	Time             string                   `json:"time"`
	State            string                   `json:"state"`
	Pick             string                   `json:"pick"`
	Title            string                   `json:"title"`
	RecommendationID *string                  `json:"recommendation_id"`
	Matched          *bool                    `json:"matched_recommendation"`
	Questions        []questionRecommendation `json:"question_recommendations"`
	Kind             string                   `json:"kind"`
	PRHead           *string                  `json:"pr_head"`
	Round            *int                     `json:"round"`
	AutoClose        any                      `json:"auto_close"`
	Revisions        *int                     `json:"revisions"`
	DecisionID       string                   `json:"decision_id"`
	PickID           string                   `json:"pick_id"`
}

func nonempty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func match(rec, pick *string) *bool {
	if rec == nil || pick == nil {
		return nil
	}
	v := *rec == *pick
	return &v
}

func (r *pickExport) enrich(raw string) error {
	var body struct {
		Recommendation string `json:"recommendation"`
		Options        []struct {
			ID       string `json:"id"`
			Approval *struct {
				Kind     string `json:"kind"`
				Identity string `json:"identity"`
			} `json:"approval"`
		} `json:"options"`
		Questions []struct {
			ID             string `json:"id"`
			Recommendation string `json:"recommendation"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return fmt.Errorf("read stored ask: %w", err)
	}
	r.RecommendationID = nonempty(body.Recommendation)
	r.Matched = match(r.RecommendationID, nonempty(r.Pick))
	for _, o := range body.Options {
		if o.ID == r.Pick && o.Approval != nil && o.Approval.Kind == "merge" && len(o.Approval.Identity) == 40 && strings.Trim(o.Approval.Identity, "0123456789abcdef") == "" {
			r.PRHead = nonempty(o.Approval.Identity)
		}
	}
	// Multi-pick reasons retain human-readable labels, not structured option IDs.
	// Export the recommendations without inferring picks from those labels.
	for _, q := range body.Questions {
		rec := nonempty(q.Recommendation)
		r.Questions = append(r.Questions, questionRecommendation{ID: q.ID, RecommendationID: rec})
	}
	return nil
}
