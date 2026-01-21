package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestLockReserveAndRelease(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	reserve := LockCmd()
	out := bytes.NewBuffer(nil)
	reserve.SetOut(out)
	reserve.SetArgs([]string{"reserve", "--owner", "alice", "--exclusive", "a.go"})
	if err := reserve.Execute(); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	if !strings.Contains(out.String(), "granted") {
		t.Fatalf("expected grant output")
	}

	conflict := LockCmd()
	conflictOut := bytes.NewBuffer(nil)
	conflict.SetOut(conflictOut)
	conflict.SetArgs([]string{"reserve", "--owner", "bob", "--exclusive", "a.go"})
	if err := conflict.Execute(); err != nil {
		t.Fatalf("reserve conflict failed: %v", err)
	}
	if !strings.Contains(conflictOut.String(), "conflict") {
		t.Fatalf("expected conflict output")
	}

	release := LockCmd()
	releaseOut := bytes.NewBuffer(nil)
	release.SetOut(releaseOut)
	release.SetArgs([]string{"release", "--owner", "alice", "a.go"})
	if err := release.Execute(); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if !strings.Contains(releaseOut.String(), "released") {
		t.Fatalf("expected release output")
	}
}

func TestLockCommandUsage(t *testing.T) {
	if LockCmd().Use != "lock" {
		t.Fatalf("unexpected Use")
	}
}

func TestLockRenewAndForceRelease(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	reserve := LockCmd()
	reserveOut := bytes.NewBuffer(nil)
	reserve.SetOut(reserveOut)
	reserve.SetArgs([]string{"reserve", "--owner", "alice", "--ttl", "1h", "--json", "a.go"})
	if err := reserve.Execute(); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}

	var reservePayload struct {
		Granted []struct {
			ID        int64  `json:"ID"`
			Path      string `json:"Path"`
			ExpiresAt string `json:"ExpiresAt"`
		} `json:"granted"`
	}
	if err := json.Unmarshal(reserveOut.Bytes(), &reservePayload); err != nil {
		t.Fatalf("decode reserve json: %v", err)
	}
	if len(reservePayload.Granted) != 1 {
		t.Fatalf("expected 1 reservation")
	}
	original := reservePayload.Granted[0]
	originalExpires, err := time.Parse(time.RFC3339Nano, original.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires: %v", err)
	}

	renew := LockCmd()
	renewOut := bytes.NewBuffer(nil)
	renew.SetOut(renewOut)
	renew.SetArgs([]string{"renew", "--owner", "alice", "--extend", "1h", "--json", "a.go"})
	if err := renew.Execute(); err != nil {
		t.Fatalf("renew failed: %v", err)
	}

	var renewPayload struct {
		Renewed      int `json:"renewed"`
		Reservations []struct {
			ID        int64  `json:"id"`
			Path      string `json:"path"`
			OldExpiry string `json:"old_expires_ts"`
			NewExpiry string `json:"new_expires_ts"`
		} `json:"reservations"`
	}
	if err := json.Unmarshal(renewOut.Bytes(), &renewPayload); err != nil {
		t.Fatalf("decode renew json: %v", err)
	}
	if renewPayload.Renewed != 1 || len(renewPayload.Reservations) != 1 {
		t.Fatalf("expected 1 renewal")
	}
	renewed := renewPayload.Reservations[0]
	if renewed.ID != original.ID || renewed.Path != original.Path {
		t.Fatalf("unexpected renewal record")
	}
	newExpires, err := time.Parse(time.RFC3339Nano, renewed.NewExpiry)
	if err != nil {
		t.Fatalf("parse new expires: %v", err)
	}
	if !newExpires.After(originalExpires) {
		t.Fatalf("expected renewed expiry to be later")
	}

	force := LockCmd()
	forceOut := bytes.NewBuffer(nil)
	force.SetOut(forceOut)
	force.SetArgs([]string{"force-release", "--id", fmt.Sprintf("%d", original.ID), "--json"})
	if err := force.Execute(); err != nil {
		t.Fatalf("force release failed: %v", err)
	}

	db, closeDB, err := openStateDB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer closeDB()
	active, err := storage.ListActiveReservations(db, 10)
	if err != nil {
		t.Fatalf("list reservations: %v", err)
	}
	for _, res := range active {
		if res.ID == original.ID {
			t.Fatalf("expected reservation released")
		}
	}
}

func TestLockReleaseByID(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	reserve := LockCmd()
	reserveOut := bytes.NewBuffer(nil)
	reserve.SetOut(reserveOut)
	reserve.SetArgs([]string{"reserve", "--owner", "alice", "--json", "a.go"})
	if err := reserve.Execute(); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}

	var reservePayload struct {
		Granted []struct {
			ID   int64  `json:"ID"`
			Path string `json:"Path"`
		} `json:"granted"`
	}
	if err := json.Unmarshal(reserveOut.Bytes(), &reservePayload); err != nil {
		t.Fatalf("decode reserve json: %v", err)
	}
	if len(reservePayload.Granted) != 1 {
		t.Fatalf("expected 1 reservation")
	}
	reservationID := reservePayload.Granted[0].ID

	release := LockCmd()
	releaseOut := bytes.NewBuffer(nil)
	release.SetOut(releaseOut)
	release.SetArgs([]string{"release", "--owner", "alice", "--id", fmt.Sprintf("%d", reservationID), "--json"})
	if err := release.Execute(); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	var releasePayload struct {
		Released int `json:"released"`
	}
	if err := json.Unmarshal(releaseOut.Bytes(), &releasePayload); err != nil {
		t.Fatalf("decode release json: %v", err)
	}
	if releasePayload.Released != 1 {
		t.Fatalf("expected 1 release, got %d", releasePayload.Released)
	}

	db, closeDB, err := openStateDB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer closeDB()
	active, err := storage.ListActiveReservations(db, 10)
	if err != nil {
		t.Fatalf("list reservations: %v", err)
	}
	for _, res := range active {
		if res.ID == reservationID {
			t.Fatalf("expected reservation released")
		}
	}
}
