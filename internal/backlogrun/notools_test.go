package backlogrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireNoToolsRefusesAWrapperWithoutIt(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.sh")
	os.WriteFile(old, []byte("case $1 in --dry-run) ;; esac\n"), 0o700)
	if err := (Router{Dispatch: old}).RequireNoTools(); err == nil || !strings.Contains(err.Error(), "--no-tools") {
		t.Fatalf("an old wrapper must be refused, got %v", err)
	}
	cur := filepath.Join(dir, "cur.sh")
	os.WriteFile(cur, []byte("case $1 in\n    --no-tools)\n      NO_TOOLS=true ;;\nesac\n"), 0o700)
	if err := (Router{Dispatch: cur}).RequireNoTools(); err != nil {
		t.Fatalf("a wrapper with --no-tools must pass: %v", err)
	}
	if err := (Router{Dispatch: filepath.Join(dir, "missing.sh")}).RequireNoTools(); err == nil {
		t.Fatal("a missing wrapper must be refused")
	}
}

func TestActualSeatUsesOnlyANewRecordAndNeverKeepsMixedFields(t *testing.T) {
	run := func(out string) Runner {
		return func(ctx context.Context, name string, args ...string) ([]byte, error) { return []byte(out), nil }
	}
	seat := Seat{Role: "coordination", ProfileRef: "coordination-sonnet", Backend: "claude", Model: "claude-sonnet-5-5", Effort: "medium"}
	got := Router{Run: run("gpt-6.1-sol\n")}.actualSeat(context.Background(), seat, "4")
	if got.Model != "gpt-6.1-sol" || got.Backend != "unrecorded" || got.Effort != "unrecorded" || !strings.Contains(got.ProfileRef, "fell back") {
		t.Fatalf("fallback seat = %+v", got)
	}
	if got := (Router{Run: run("")}).actualSeat(context.Background(), seat, "4"); !strings.Contains(got.ProfileRef, "unverified") {
		t.Fatalf("no new record must be marked unverified: %+v", got)
	}
	if got := (Router{Run: run("claude-sonnet-5-5")}).actualSeat(context.Background(), seat, ""); !strings.Contains(got.ProfileRef, "unverified") {
		t.Fatalf("no baseline must be marked unverified: %+v", got)
	}
	if got := (Router{Run: run("claude-sonnet-5-5")}).actualSeat(context.Background(), seat, "4"); got.ProfileRef != "coordination-sonnet" {
		t.Fatalf("a matching record leaves the seat: %+v", got)
	}
}
