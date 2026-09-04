package door

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVisitRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "last-visit")
	at := time.Date(2026, 9, 1, 22, 14, 0, 0, time.FixedZone("PDT", -7*3600))
	if err := SaveVisit(path, at); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadLastVisit(path)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !got.Equal(at) {
		t.Fatalf("got %v, want %v", got, at)
	}
}

func TestVisitAbsentIsFirstVisit(t *testing.T) {
	_, ok, err := LoadLastVisit(filepath.Join(t.TempDir(), "nope"))
	if ok || err != nil {
		t.Fatalf("absent stamp must be ok=false with no error, got ok=%v err=%v", ok, err)
	}
}

func TestVisitMalformedIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-visit")
	if err := os.WriteFile(path, []byte("yesterday\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLastVisit(path); err == nil {
		t.Fatal("a stamp that exists but does not parse must be an error, not a first visit")
	}
}

func TestWindow(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	stamp := filepath.Join(t.TempDir(), "last-visit")

	since, source, err := Window("", stamp, now)
	if err != nil || source != "first visit" || !since.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("no stamp: want first visit at -24h, got %v %q err=%v", since, source, err)
	}

	if err := SaveVisit(stamp, now.Add(-30*time.Hour)); err != nil {
		t.Fatal(err)
	}
	since, source, err = Window("", stamp, now)
	if err != nil || source != "last visit" || !since.Equal(now.Add(-30*time.Hour)) {
		t.Fatalf("stamp: want last visit at -30h, got %v %q err=%v", since, source, err)
	}

	since, source, err = Window("36h", stamp, now)
	if err != nil || source != "--since 36h" || !since.Equal(now.Add(-36*time.Hour)) {
		t.Fatalf("override: got %v %q err=%v", since, source, err)
	}
	if since, _, err = Window("3d", stamp, now); err != nil || !since.Equal(now.Add(-72*time.Hour)) {
		t.Fatalf("3d: got %v err=%v", since, err)
	}
	if since, _, err = Window("2026-09-01T00:00:00Z", stamp, now); err != nil || !since.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("RFC3339: got %v err=%v", since, err)
	}
	if since, _, err = Window("bogus", stamp, now); err == nil || !since.IsZero() {
		t.Fatalf("a bad override must be fatal (zero time + error), got %v err=%v", since, err)
	}

	if err := os.WriteFile(stamp, []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	since, source, err = Window("", stamp, now)
	if err == nil || source != "first visit" || !since.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("unreadable stamp: want first-visit window AND the error, got %v %q err=%v", since, source, err)
	}
}
