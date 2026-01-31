package planstatus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubGit struct{}

func (s stubGit) LastCommitDate(repoRoot, path string) (time.Time, bool) {
	return time.Time{}, false
}

func TestReportIncludesDerivedEvidence(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	planName := "2026-01-28-feat-coordination-api-foundation-plan.md"
	planPath := filepath.Join(plansDir, planName)
	if err := os.WriteFile(planPath, []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	derived := map[string][]string{
		planName: {"pkg/httpapi/envelope.go", "internal/pollard/server/server.go"},
	}

	report, err := GenerateReport(Options{
		RepoRoot:        tmp,
		IntermuteRoot:   "",
		Now:             time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		Git:             stubGit{},
		DerivedEvidence: derived,
	})
	if err != nil {
		t.Fatalf("GenerateReport error: %v", err)
	}
	if !strings.Contains(report, planName) {
		t.Fatalf("expected report to include plan name")
	}
	if !strings.Contains(report, "derived") {
		t.Fatalf("expected report to mark plan as derived")
	}
	if !strings.Contains(report, "pkg/httpapi/envelope.go") {
		t.Fatalf("expected report to include derived evidence path")
	}
}

func TestReportSkipsMissingPaths(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	planName := "2026-01-22-sample-plan.md"
	planPath := filepath.Join(plansDir, planName)
	planContent := "Create: `internal/missing/file.go`\n"
	if err := os.WriteFile(planPath, []byte(planContent), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	report, err := GenerateReport(Options{
		RepoRoot:        tmp,
		IntermuteRoot:   "",
		Now:             time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		Git:             stubGit{},
		DerivedEvidence: map[string][]string{},
	})
	if err != nil {
		t.Fatalf("GenerateReport error: %v", err)
	}
	expected := "| " + planName + " | none |"
	if !strings.Contains(report, expected) {
		t.Fatalf("expected missing path plan to be marked none; got report:\n%s", report)
	}
}

func TestReportSkipsStatusFile(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "STATUS.md"), []byte("status"), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
	planName := "2026-01-22-sample-plan.md"
	planPath := filepath.Join(plansDir, planName)
	if err := os.WriteFile(planPath, []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	report, err := GenerateReport(Options{
		RepoRoot:        tmp,
		IntermuteRoot:   "",
		Now:             time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		Git:             stubGit{},
		DerivedEvidence: map[string][]string{},
	})
	if err != nil {
		t.Fatalf("GenerateReport error: %v", err)
	}
	if strings.Contains(report, "STATUS.md") {
		t.Fatalf("expected report to exclude STATUS.md, got:\n%s", report)
	}
}
