package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/scheduler"
)

type fakeWorker struct {
	err    error
	detail string
}

func (f fakeWorker) Spawn(string, mycroft.BeadView, string) (string, error) { return "thr_w", f.err }
func (f fakeWorker) Detail(string) string                                   { return f.detail }

func logRows(t *testing.T, path string) (outcome, reason, action, bead string, n int) {
	t.Helper()
	db, err := mycroft.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT COUNT(*) FROM dispatch_log").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > 0 {
		if err := db.QueryRow("SELECT COALESCE(outcome,''), COALESCE(reason,''), action, bead FROM dispatch_log").Scan(&outcome, &reason, &action, &bead); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func TestApproveSpawnLogsTheDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.db")
	db, err := mycroft.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	disp := scheduler.NewDispatcher(db, nil, "demarch")
	bead := mycroft.BeadView{ID: "mk-1", Card: "AUTA-1"}
	if err := approveSpawn(disp, fakeWorker{detail: "thread thr_w bead=mk-1 card=AUTA-1 role=routine-execution"}, bead); err != nil {
		t.Fatal(err)
	}
	db.Close()
	outcome, reason, action, b, n := logRows(t, path)
	if n != 1 || outcome != "accepted" || action != "suggest" || b != "mk-1" || !strings.Contains(reason, "card=AUTA-1") {
		t.Fatalf("log row: n=%d outcome=%q action=%q bead=%q reason=%q", n, outcome, action, b, reason)
	}
}

func TestApproveSpawnLogsAFailedStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.db")
	db, err := mycroft.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	disp := scheduler.NewDispatcher(db, nil, "demarch")
	err = approveSpawn(disp, fakeWorker{err: errors.New("attach failed (thread thr_w stopped)")}, mycroft.BeadView{ID: "mk-2", Card: "AUTA-2"})
	if err == nil {
		t.Fatal("a failed start must return its error")
	}
	db.Close()
	outcome, reason, _, _, n := logRows(t, path)
	if n != 1 || outcome != "failure" || !strings.Contains(reason, "attach failed") {
		t.Fatalf("log row: n=%d outcome=%q reason=%q", n, outcome, reason)
	}
}

func TestApproveSpawnReportsALogWriteFailureAfterStart(t *testing.T) {
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	disp := scheduler.NewDispatcher(db, nil, "demarch")
	db.Close() // writes now fail
	err = approveSpawn(disp, fakeWorker{}, mycroft.BeadView{ID: "mk-3", Card: "AUTA-3"})
	if err == nil || !strings.Contains(err.Error(), "thr_w is running") {
		t.Fatalf("want an error naming the running thread, got %v", err)
	}
}
