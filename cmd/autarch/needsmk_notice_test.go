package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNoticeDismissalUnknownRequestIsInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(homeask.EnvCardStateDir, dir)
	cmd := needsMkCmd()
	cmd.SetArgs([]string{"dismiss-owner-notice", "unknown"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); !errors.Is(err, homeask.ErrInvalid) {
		t.Fatalf("unknown request should be invalid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "filings.json")); !os.IsNotExist(err) {
		t.Fatalf("unknown request wrote filing state: %v", err)
	}
}

func TestNoticeDismissalDoesNotCancelRecoveredOrRetired(t *testing.T) {
	for _, kind := range []string{"recovered", "retired", "held"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(homeask.EnvCardStateDir, dir)
			bb := &stubBB{}
			f := &homeask.CardFiler{StateDir: dir, LockDir: t.TempDir(), Run: bb.run}
			rawAsk, e := os.ReadFile(writeAsk(t))
			if e != nil {
				t.Fatal(e)
			}
			var ask map[string]any
			if e = json.Unmarshal(rawAsk, &ask); e != nil {
				t.Fatal(e)
			}
			if _, e = f.FileCard(context.Background(), homeask.CardRequest{Project: "P1", Title: "Choose", Key: "example", Thread: "thr_x", Ask: ask, NonBlocking: true}); e != nil {
				t.Fatal(e)
			}
			base, e := os.ReadFile(filepath.Join(dir, "filings.json"))
			if e != nil {
				t.Fatal(e)
			}
			var entries map[string]map[string]any
			if e = json.Unmarshal(base, &entries); e != nil {
				t.Fatal(e)
			}
			entry := entries["example"]
			entry["owner_attention"] = "owner handoff done"
			entry["held"] = kind == "held"
			if kind != "held" {
				entry["attempted"] = true
			}
			if kind == "retired" {
				entry["refunded"] = true
			}
			if kind == "recovered" {
				entry["delivered_card"] = "example-card"
			}
			if kind == "retired" {
				entry["terminal_reason"] = "absent"
			}
			raw, e := json.Marshal(map[string]any{"example": entry})
			if e != nil {
				t.Fatal(e)
			}
			p := filepath.Join(dir, "filings.json")
			if e = os.WriteFile(p, raw, 0600); e != nil {
				t.Fatal(e)
			}
			want := map[string]any{}
			for k, v := range entry {
				if k != "owner_attention" {
					want[k] = v
				}
			}
			for i := 0; i < 2; i++ {
				root := &cobra.Command{Use: "autarch"}
				root.AddCommand(needsMkCmd())
				root.SetArgs([]string{"needs-mk", "dismiss-owner-notice", "example"})
				root.SetOut(&bytes.Buffer{})
				root.SetErr(&bytes.Buffer{})
				if e = root.Execute(); e != nil {
					t.Fatalf("notice cannot be dismissed: %v", e)
				}
			}
			after, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			var state map[string]map[string]any
			if e = json.Unmarshal(after, &state); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(state["example"], want) {
				t.Fatalf("dismissal changed request/delivery/budget fields: %v want %v", state["example"], want)
			}
			var footer bytes.Buffer
			reader := &homeask.CardFiler{StateDir: dir}
			if e = reader.PrintReleaseOwnerAttention(&footer); e != nil || footer.Len() != 0 {
				t.Fatal("dismissed notice still appears", footer.String(), e)
			}
		})
	}
}
func noticeFooterDB(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "home.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE settings_kv(key TEXT,value TEXT); CREATE TABLE cards(task_id TEXT,card_key TEXT,title TEXT,state TEXT,deleted_at TEXT); CREATE TABLE picks(decision_id TEXT,option_id TEXT,picked_at TEXT,reason TEXT,"by" TEXT); CREATE TABLE obligations(kind TEXT,state TEXT,recipient TEXT,decision_id TEXT,voided_at TEXT,last_error TEXT);`)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestNoticeFooterStateFaultOnlyWarns(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(homeask.EnvCardStateDir, dir)
			p := filepath.Join(dir, "filings.json")
			if kind == "corrupt" {
				if e := os.WriteFile(p, []byte("{broken"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "directory" {
				if e := os.Mkdir(p, 0700); e != nil {
					t.Fatal(e)
				}
			}
			cmd := needsMkFooterCmd()
			cmd.SetArgs([]string{"--db", noticeFooterDB(t), "--cursor", filepath.Join(t.TempDir(), "cursor"), "--peek"})
			var out, warn bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&warn)
			if e := cmd.Execute(); e != nil {
				t.Fatalf("notice read broke primary footer: %v", e)
			}
			if !strings.Contains(out.String(), "To decide (Home):") {
				t.Fatal(out.String())
			}
			if (warn.Len() > 0) != (kind != "missing") {
				t.Fatalf("warning missing or unexpected: %q", warn.String())
			}
			if kind == "missing" {
				if _, e := os.Stat(p); !os.IsNotExist(e) {
					t.Fatal("missing state file created", e)
				}
			}
			if kind == "corrupt" {
				b, e := os.ReadFile(p)
				if e != nil || string(b) != "{broken" {
					t.Fatal("corrupt state changed", string(b), e)
				}
			}
		})
	}
}
func TestNoticeAlreadyRuledReleaseExitZero(t *testing.T) {
	f, bb := releaseNotesFixture(t)
	raw, e := os.ReadFile(writeAsk(t))
	if e != nil {
		t.Fatal(e)
	}
	var ask map[string]any
	if e = json.Unmarshal(raw, &ask); e != nil {
		t.Fatal(e)
	}
	first, e := f.FileCard(context.Background(), homeask.CardRequest{Project: "P1", Title: "Choose", Key: "base", Thread: "thr_x", Ask: ask})
	if e != nil {
		t.Fatal(e)
	}
	ask["ask_key"] = "same-decision"
	r := homeask.CardRequest{Project: "P1", Title: "Choose", Key: "superseding", Thread: "thr_x", Ask: ask, NonBlocking: true}
	if _, e = f.FileOrUpdateCard(context.Background(), r, first.ID); e != nil {
		t.Fatal(e)
	}
	f.Run = func(ctx context.Context, env []string, a ...string) homeask.BBResult {
		if len(a) > 3 && a[0] == "home" && a[1] == "get" && a[2] == "--card" {
			b, _ := json.Marshal(homeask.CardView{TaskID: first.ID, ProjectID: "P1", State: "ruled", DecisionState: "picked"})
			return homeask.BBResult{Stdout: b}
		}
		return bb.run(ctx, env, a...)
	}
	cmd := needsMkReleaseCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if e = cmd.Execute(); e != nil {
		t.Fatalf("harmless ruled request failed timer: %v", e)
	}
	if !strings.Contains(out.String(), "superseding") {
		t.Fatal("terminal result not reported", out.String())
	}
}
