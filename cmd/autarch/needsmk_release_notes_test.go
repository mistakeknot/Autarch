package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseNotesFixture(t *testing.T) (*homeask.CardFiler, *stubBB) {
	t.Helper()
	bb := &stubBB{}
	f := &homeask.CardFiler{Now: filingTestNow, Run: bb.run, StateDir: t.TempDir(), LockDir: t.TempDir()}
	raw, e := os.ReadFile(writeAsk(t))
	if e != nil {
		t.Fatal(e)
	}
	var ask map[string]any
	if e = json.Unmarshal(raw, &ask); e != nil {
		t.Fatal(e)
	}
	r := homeask.CardRequest{Project: "P1", Title: "Choose", Ask: ask, Key: "held", Thread: "thr_x", NonBlocking: true}
	if _, e = f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	old, check := newCardFiler, checkFilingPreconditions
	t.Cleanup(func() { newCardFiler = old; checkFilingPreconditions = check })
	newCardFiler = func() *homeask.CardFiler { return f }
	checkFilingPreconditions = func(string, string) error { return nil }
	return f, bb
}
func TestReleaseNotesBatchFaultsFail(t *testing.T) {
	for _, kind := range []string{"entry-lock", "home-down", "outcome-unknown"} {
		t.Run(kind, func(t *testing.T) {
			f, bb := releaseNotesFixture(t)
			switch kind {
			case "entry-lock":
				if e := os.Remove(filepath.Join(f.StateDir, "delivery-held.lock")); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(filepath.Join(f.StateDir, "delivery-held.lock"), 0700); e != nil {
					t.Fatal(e)
				}
			case "home-down":
				f.Run = func(context.Context, []string, ...string) homeask.BBResult {
					return homeask.BBResult{Code: 1, Stderr: []byte("service unavailable")}
				}
			case "outcome-unknown":
				f.Run = func(ctx context.Context, env []string, a ...string) homeask.BBResult {
					if len(a) > 1 && a[0] == "tasks" && a[1] == "comment" {
						return homeask.BBResult{Code: 1, Stderr: []byte("reply lost")}
					}
					return bb.run(ctx, env, a...)
				}
			}
			cmd := needsMkReleaseCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			if e := cmd.Execute(); e == nil {
				t.Fatal("systemic failure exited zero", kind)
			}
		})
	}
}
func TestReleaseNotesFooterShowsOwnerAttention(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(homeask.EnvCardStateDir, dir)
	state := `{"held":{"request":{"Owner":"thr_archived"},"held":true,"owner_attention":"owner thr_archived: cancel-held held, then file with a new request key and live owner"}}`
	if e := os.WriteFile(filepath.Join(dir, "filings.json"), []byte(state), 0600); e != nil {
		t.Fatal(e)
	}
	dbpath := filepath.Join(t.TempDir(), "home.db")
	db, e := sql.Open("sqlite", dbpath)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE settings_kv(key TEXT,value TEXT); CREATE TABLE cards(task_id TEXT,card_key TEXT,title TEXT,state TEXT,deleted_at TEXT); CREATE TABLE picks(decision_id TEXT,option_id TEXT,picked_at TEXT,reason TEXT,"by" TEXT); CREATE TABLE obligations(kind TEXT,state TEXT,recipient TEXT,decision_id TEXT,voided_at TEXT,last_error TEXT);`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	cmd := needsMkFooterCmd()
	cmd.SetArgs([]string{"--db", dbpath, "--cursor", filepath.Join(t.TempDir(), "cursor"), "--peek"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if e = cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "thr_archived") || !strings.Contains(out.String(), "cancel-held held") {
		t.Fatal("footer lost owner attention", out.String())
	}
	after, e := os.ReadFile(filepath.Join(dir, "filings.json"))
	if e != nil || string(after) != state {
		t.Fatal("peek footer mutated filing state", e)
	}
}
