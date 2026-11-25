package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/testutil"
)

func wedPT(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 11, 25, 10, 0, 0, 0, pacific)
}

func loadedOn() WeekdayHoldSetting {
	return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn, Source: "test"}
}

func TestWeeklyHoldSettingDefaultOnWednesday(t *testing.T) {
	testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "")
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	on := loadedOn()
	held, err := weeklyHold(wedPT(t), r, on, "GROK")
	if err != nil || !held {
		t.Fatalf("default on should hold Wednesday ordinary card: held=%v err=%v", held, err)
	}
	off, err := ParseWeekdayHoldValue("off")
	if err != nil {
		t.Fatal(err)
	}
	held, err = weeklyHold(wedPT(t), r, off, "GROK")
	if err != nil || held {
		t.Fatalf("off should not hold: held=%v err=%v", held, err)
	}
}

func TestWeeklyHoldExplicitOnAndEmptyEnv(t *testing.T) {
	testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "")
	s, err := LoadWeekdayHoldSetting("")
	if err != nil || !s.loaded || s.mode != weekdayHoldOn || s.Source != "default" {
		t.Fatalf("empty env, no file: %+v err=%v", s, err)
	}
	t.Setenv(EnvCardWeekdayHold, "on")
	s, err = LoadWeekdayHoldSetting("")
	if err != nil || s.mode != weekdayHoldOn || s.Source != "env" {
		t.Fatalf("explicit on: %+v err=%v", s, err)
	}
}

func TestWeeklyHoldWindowBoundaries(t *testing.T) {
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	on := loadedOn()
	off, _ := ParseWeekdayHoldValue("off")
	times := []struct {
		name string
		at   time.Time
		hold bool
	}{
		{"Thu 11:59", time.Date(2026, 11, 19, 11, 59, 0, 0, pacific), true},
		{"Thu 12:00", time.Date(2026, 11, 19, 12, 0, 0, 0, pacific), false},
		{"Mon 00:00", time.Date(2026, 11, 16, 0, 0, 0, 0, pacific), true},
		{"Wed 10:00", wedPT(t), true},
	}
	for _, tc := range times {
		t.Run(tc.name, func(t *testing.T) {
			got, err := weeklyHold(tc.at, r, on, "GROK")
			if err != nil || got != tc.hold {
				t.Fatalf("on: got=%v want=%v err=%v", got, tc.hold, err)
			}
			got, err = weeklyHold(tc.at, r, off, "GROK")
			if err != nil || got {
				t.Fatalf("off should never hold: got=%v err=%v", got, err)
			}
		})
	}
}

func TestWeeklyHoldOffProjectList(t *testing.T) {
	at := wedPT(t)
	grok := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	alph := CardRequest{Project: "ALPH", Title: "Q", Type: "other"}
	msq := CardRequest{Project: "MSQ", Title: "Q", Type: "other"}
	onlyGrok, err := ParseWeekdayHoldValue("off:GROK")
	if err != nil {
		t.Fatal(err)
	}
	held, err := weeklyHold(at, grok, onlyGrok, "GROK")
	if err != nil || held {
		t.Fatalf("off:GROK should not hold GROK: %v %v", held, err)
	}
	held, err = weeklyHold(at, alph, onlyGrok, "ALPH")
	if err != nil || !held {
		t.Fatalf("off:GROK should still hold ALPH: %v %v", held, err)
	}
	both, err := ParseWeekdayHoldValue("off:GROK,ALPH")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []CardRequest{grok, alph} {
		held, err = weeklyHold(at, r, both, r.Project)
		if err != nil || held {
			t.Fatalf("off:GROK,ALPH should not hold %s: %v %v", r.Project, held, err)
		}
	}
	held, err = weeklyHold(at, msq, both, "MSQ")
	if err != nil || !held {
		t.Fatalf("off:GROK,ALPH should hold MSQ: %v %v", held, err)
	}
}

func TestWeeklyHoldProjectNameAndIDMatch(t *testing.T) {
	at := wedPT(t)
	s, err := ParseWeekdayHoldValue("off:GROK")
	if err != nil {
		t.Fatal(err)
	}
	// Filed by name, resolved prefix GROK.
	r := CardRequest{Project: "Grok Bot", Title: "Q", Type: "other"}
	held, err := weeklyHold(at, r, s, "GROK")
	if err != nil || held {
		t.Fatalf("name + resolved prefix should match off:GROK: %v %v", held, err)
	}
	// Filed by id.
	r = CardRequest{Project: "proj_abc", Title: "Q", Type: "other"}
	held, err = weeklyHold(at, r, s, "GROK")
	if err != nil || held {
		t.Fatalf("id + resolved prefix should match: %v %v", held, err)
	}
	// Unresolved other project stays held.
	r = CardRequest{Project: "mystery", Title: "Q", Type: "other"}
	held, err = weeklyHold(at, r, s, "")
	if err != nil || !held {
		t.Fatalf("unresolved project should stay held: %v %v", held, err)
	}
}

func TestWeekdayHoldEnvBeatsFileAndUnknownFails(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "")
	cfg := filepath.Join(dir, "autarch")
	if err := os.MkdirAll(cfg, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg, "needsmk.env")
	if err := os.WriteFile(path, []byte("AUTARCH_CARD_WEEKDAY_HOLD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadWeekdayHoldSetting(dir)
	if err != nil || s.mode != weekdayHoldOff || s.Source != "file" {
		t.Fatalf("file off: %+v err=%v", s, err)
	}
	t.Setenv(EnvCardWeekdayHold, "on")
	s, err = LoadWeekdayHoldSetting(dir)
	if err != nil || s.mode != weekdayHoldOn || s.Source != "env" {
		t.Fatalf("env should beat file: %+v err=%v", s, err)
	}
	t.Setenv(EnvCardWeekdayHold, "bogus")
	if _, err := LoadWeekdayHoldSetting(dir); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown value should fail closed: %v", err)
	}
	if _, err := ParseWeekdayHoldValue("maybe"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("maybe should be invalid: %v", err)
	}
}

func TestWeeklyHoldUnloadedErrors(t *testing.T) {
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	if _, err := weeklyHold(wedPT(t), r, WeekdayHoldSetting{}, "GROK"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unloaded setting should error: %v", err)
	}
}

func TestCheckSecretDeliveryOnAndOff(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "on")
	f := &CardFiler{Now: func() time.Time { return wedPT(t) }, ConfigDir: dir}
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other", Thread: "thr_23456789ab", Secret: map[string]any{"thread": "thr_23456789ab", "names": []string{"X"}, "path": "/p", "project": "g"}}
	if err := f.CheckSecretDelivery(r); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "weekly hold") {
		t.Fatalf("on + window should refuse --secret: %v", err)
	}
	t.Setenv(EnvCardWeekdayHold, "off")
	f = &CardFiler{Now: func() time.Time { return wedPT(t) }, ConfigDir: dir}
	if err := f.CheckSecretDelivery(r); err != nil {
		t.Fatalf("off should accept --secret: %v", err)
	}
}

func TestReleaseBadSettingChangesNothing(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "nope")
	stateDir := t.TempDir()
	f := &CardFiler{StateDir: stateDir, LockDir: t.TempDir(), ConfigDir: dir, Now: func() time.Time { return wedPT(t) }}
	path := filepath.Join(stateDir, "filings.json")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	orig := []byte(`{}`)
	if err := os.WriteFile(path, orig, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	_, err := f.Release(context.Background())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("release with bad setting should fail closed: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("filings.json changed: before=%s after=%s", before, after)
	}
}

func TestOffListReleaseUsesStoredProjectAndMap(t *testing.T) {
	at := wedPT(t)
	s, _ := ParseWeekdayHoldValue("off:GROK")
	m := map[string]string{"grok bot": "GROK", "proj_g": "GROK", "grok": "GROK", "alph": "ALPH"}
	heldGrok, err := weeklyHold(at, CardRequest{Project: "Grok Bot", Type: "other"}, s, prefixFromMap("Grok Bot", m))
	if err != nil || heldGrok {
		t.Fatalf("stored GROK-by-name should be released: %v %v", heldGrok, err)
	}
	heldAlph, err := weeklyHold(at, CardRequest{Project: "ALPH", Type: "other"}, s, prefixFromMap("ALPH", m))
	if err != nil || !heldAlph {
		t.Fatalf("ALPH should stay held: %v %v", heldAlph, err)
	}
}

func TestParseWeekdayHoldFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "needsmk.env")
	if err := os.WriteFile(p, []byte("# comment\n\nAUTARCH_CARD_WEEKDAY_HOLD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := readWeekdayHoldFile(p)
	if err != nil || !ok || raw != "off" {
		t.Fatalf("raw=%q ok=%v err=%v", raw, ok, err)
	}
}

func TestWeekdayHoldDecisionReason(t *testing.T) {
	testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "on")
	f := &CardFiler{Now: func() time.Time { return wedPT(t) }, ConfigDir: testutil.ConfigHome(t)}
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	held, reason, _, err := f.WeekdayHoldDecision(context.Background(), r, wedPT(t))
	if err != nil || !held || reason != "weekday-window" {
		t.Fatalf("on should hold with weekday-window: held=%v reason=%q err=%v", held, reason, err)
	}
	t.Setenv(EnvCardWeekdayHold, "off")
	f = &CardFiler{Now: func() time.Time { return wedPT(t) }, ConfigDir: testutil.ConfigHome(t)}
	held, reason, _, err = f.WeekdayHoldDecision(context.Background(), r, wedPT(t))
	if err != nil || held || reason != "" {
		t.Fatalf("off should not hold: held=%v reason=%q err=%v", held, reason, err)
	}
}

func TestCheckJSONShapeHelper(t *testing.T) {
	b, _ := json.Marshal(map[string]any{"checked": true, "weekday_hold": false, "weekday_hold_reason": "", "config_path": "/tmp/x"})
	if !strings.Contains(string(b), "weekday_hold") {
		t.Fatal(string(b))
	}
}
