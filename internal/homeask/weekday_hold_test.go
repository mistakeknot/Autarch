package homeask

import (
	"context"
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
	held, err := weeklyHold(wedPT(t), r, on)
	if err != nil || !held {
		t.Fatalf("default on should hold Wednesday ordinary card: held=%v err=%v", held, err)
	}
	off, err := validateWeekdayHoldValue("off")
	if err != nil {
		t.Fatal(err)
	}
	held, err = weeklyHold(wedPT(t), r, off)
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

func TestWeekdayHoldEnvParsedBeforeConfigDir(t *testing.T) {
	t.Setenv(EnvCardWeekdayHold, "off")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("AppData", "")
	s, err := LoadWeekdayHoldSetting("")
	if err != nil || s.mode != weekdayHoldOff || s.Source != "env" {
		t.Fatalf("env should load without a config dir: %+v %v", s, err)
	}
}

func TestWeeklyHoldWindowBoundaries(t *testing.T) {
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	on := loadedOn()
	off, _ := validateWeekdayHoldValue("off")
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
			got, err := weeklyHold(tc.at, r, on)
			if err != nil || got != tc.hold {
				t.Fatalf("on: got=%v want=%v err=%v", got, tc.hold, err)
			}
			got, err = weeklyHold(tc.at, r, off)
			if err != nil || got {
				t.Fatalf("off should never hold: got=%v err=%v", got, err)
			}
		})
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
	if _, err := validateWeekdayHoldValue("maybe"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("maybe should be invalid: %v", err)
	}
	if _, err := validateWeekdayHoldValue("off:GROK"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("off:list should be invalid: %v", err)
	}
}

func TestWeekdayHoldFileUnknownKeyAndEmptyFailClosed(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "")
	cfg := filepath.Join(dir, "autarch")
	if err := os.MkdirAll(cfg, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg, "needsmk.env")
	if err := os.WriteFile(path, []byte("AUTARCH_CARD_WEEKDAY_HLOD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadWeekdayHoldSetting(dir)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), path+":1") || !strings.Contains(err.Error(), "AUTARCH_CARD_WEEKDAY_HLOD") {
		t.Fatalf("unknown key should name path and line: %v", err)
	}
	if _, err := CheckWeekdayHoldConfig(path); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), path+":1") {
		t.Fatalf("check-config should reject the unknown key: %v", err)
	}

	if err := os.WriteFile(path, []byte("# note\nAUTARCH_CARD_WEEKDAY_HOLD=\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadWeekdayHoldSetting(dir)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "empty") || !strings.Contains(err.Error(), path) {
		t.Fatalf("empty assignment should fail closed: %v", err)
	}
	if _, err := CheckWeekdayHoldConfig(path); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("check-config should reject an empty value: %v", err)
	}

	if err := os.WriteFile(path, []byte("# only a comment\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWeekdayHoldSetting(dir); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a present file without the key should not default to on: %v", err)
	}
}

func TestWeeklyHoldUnloadedErrors(t *testing.T) {
	r := CardRequest{Project: "GROK", Title: "Q", Type: "other"}
	if _, err := weeklyHold(wedPT(t), r, WeekdayHoldSetting{}); !errors.Is(err, ErrInvalid) {
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

func seedFilings(t *testing.T, stateDir string) (path string, orig []byte) {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(stateDir, "filings.json")
	orig = []byte(`{}`)
	if err := os.WriteFile(path, orig, 0600); err != nil {
		t.Fatal(err)
	}
	return path, orig
}

func TestReleaseBadSettingChangesNothing(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "nope")
	stateDir := t.TempDir()
	f := &CardFiler{StateDir: stateDir, LockDir: t.TempDir(), ConfigDir: dir, Now: func() time.Time { return wedPT(t) }}
	path, before := seedFilings(t, stateDir)
	_, err := f.Release(context.Background())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("release with bad setting should fail closed: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("filings.json changed: before=%s after=%s", before, after)
	}
}

func TestFileAndReleaseDetailedBadFileChangeNothing(t *testing.T) {
	dir := testutil.ConfigHome(t)
	t.Setenv(EnvCardWeekdayHold, "")
	cfg := filepath.Join(dir, "autarch")
	if err := os.MkdirAll(cfg, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg, "needsmk.env")
	if err := os.WriteFile(path, []byte("AUTARCH_CARD_WEEKDAY_HLOD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	filings, before := seedFilings(t, stateDir)
	f := &CardFiler{
		StateDir: stateDir, LockDir: t.TempDir(), ConfigDir: dir, TasksProject: "P1",
		Now: func() time.Time { return wedPT(t) },
		Run: func(context.Context, []string, ...string) BBResult {
			return BBResult{Code: 1, Stderr: []byte("unavailable")}
		},
	}
	if _, err := f.File(context.Background(), base()); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), path+":1") {
		t.Fatalf("File should fail closed on an unknown key: %v", err)
	}
	after, _ := os.ReadFile(filings)
	if string(before) != string(after) {
		t.Fatalf("File changed filings.json: before=%s after=%s", before, after)
	}

	if err := os.WriteFile(path, []byte("AUTARCH_CARD_WEEKDAY_HOLD=\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f = &CardFiler{StateDir: stateDir, LockDir: t.TempDir(), ConfigDir: dir, Now: func() time.Time { return wedPT(t) }}
	if _, err := f.ReleaseDetailed(context.Background()); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("ReleaseDetailed should fail closed on an empty value: %v", err)
	}
	after, _ = os.ReadFile(filings)
	if string(before) != string(after) {
		t.Fatalf("ReleaseDetailed changed filings.json: before=%s after=%s", before, after)
	}
}

func TestParseWeekdayHoldFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "needsmk.env")
	if err := os.WriteFile(p, []byte("# comment\n\nAUTARCH_CARD_WEEKDAY_HOLD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	raw, err := readWeekdayHoldFile(p)
	if err != nil || raw != "off" {
		t.Fatalf("raw=%q err=%v", raw, err)
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
