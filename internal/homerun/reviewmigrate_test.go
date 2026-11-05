//go:build unix

package homerun

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const oldReviewSchema = `
CREATE TABLE review_refusals(
  seq INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, reason TEXT NOT NULL, detail TEXT NOT NULL,
  envelope_digest TEXT NOT NULL, card_id TEXT NOT NULL, op TEXT NOT NULL, nonce TEXT NOT NULL, review_sha256 TEXT NOT NULL);
CREATE TRIGGER review_refusals_no_update BEFORE UPDATE ON review_refusals BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
CREATE TRIGGER review_refusals_no_delete BEFORE DELETE ON review_refusals BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
`

// A store created with the first review_refusals shape is migrated in place: rows are aggregated by
// (envelope digest, reason) and the new refusals keep working.
func TestReviewRefusalsMigratesOldShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(oldReviewSchema); err != nil {
		t.Fatal(err)
	}
	dA, dB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	ins := `INSERT INTO review_refusals(at,reason,detail,envelope_digest,card_id,op,nonce,review_sha256) VALUES(?,?,?,?,?,?,?,?)`
	for _, r := range []struct {
		at     int64
		reason string
		detail string
		digest string
		nonce  string
	}{
		{100, "receipt-missing", "first", dA, "n1"},
		{200, "receipt-missing", "second", dA, "n1"},
		{300, "receipt-refused", "other reason", dA, "n1"},
		{150, "receipt-missing", "b", dB, "n2"},
		{400, "receipt-missing", "third", dA, "n1"},
	} {
		if _, err := db.Exec(ins, r.at, r.reason, r.detail, r.digest, "AUT-1", "check", r.nonce, strings.Repeat("c", 64)); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(path, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rs, err := s.ReviewRefusals(context.Background(), 0, 100)
	if err != nil || len(rs) != 3 {
		t.Fatalf("want 3 aggregated rows: %v %+v", err, rs)
	}
	var a *ReviewRefusal
	for i := range rs {
		if rs[i].EnvelopeDigest == dA && rs[i].Reason == "receipt-missing" {
			a = &rs[i]
		}
	}
	if a == nil || a.Count != 3 || a.FirstAt.Unix() != 100 || a.LastAt.Unix() != 400 || a.Detail != "third" || a.Nonce != "n1" {
		t.Fatalf("bad aggregate %+v", a)
	}
	// the new triggers are in force: delete is refused, and a new refusal is recorded
	if _, err := s.db.Exec(`DELETE FROM review_refusals`); err == nil {
		t.Fatal("delete must be refused")
	}
	if _, err := s.db.Exec(`UPDATE review_refusals SET count=count+5`); err == nil {
		t.Fatal("arbitrary count update must be refused")
	}
	e := &Envelope{CardID: "AUT-2", Op: "check", Nonce: "n3", ReviewSHA256: strings.Repeat("d", 64)}
	err = s.recordRefusal(context.Background(), ConsumeRequest{Env: e, EnvelopeDigest: strings.Repeat("e", 64), Now: time.Unix(1000, 0)}, &ReviewError{Reason: ReviewMissing, Detail: "x"})
	if err == nil {
		t.Fatal("recordRefusal returns the refusal")
	}
	rs, _ = s.ReviewRefusals(context.Background(), 0, 100)
	if len(rs) != 4 {
		t.Fatalf("new refusal not recorded: %+v", rs)
	}
	// reopening is a no-op
	s.Close()
	s2, err := Open(path, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if rs, _ := s2.ReviewRefusals(context.Background(), 0, 100); len(rs) != 4 {
		t.Fatalf("reopen changed rows: %+v", rs)
	}
}
