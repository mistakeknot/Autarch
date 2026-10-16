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

func TestActualSeatReportsTheRecordedModel(t *testing.T) {
	r := Router{Repo: "/x", Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("gpt-6.1-sol\n"), nil
	}}
	got := r.actualSeat(context.Background(), Seat{ProfileRef: "coordination-sonnet", Model: "claude-sonnet-5-5"})
	if got.Model != "gpt-6.1-sol" || !strings.Contains(got.ProfileRef, "fell back") {
		t.Fatalf("seat = %+v", got)
	}
}
