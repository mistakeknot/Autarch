package reviewagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredGuidanceNeverTruncatesRulingAfterOldExcerptLimit(t *testing.T) {
	project, _ := filepath.EvalSymlinks(t.TempDir())
	text := strings.Repeat("Historical context.\n", 1800) + "MANDATORY END RULING: preserve every correction exactly.\n"
	if err := os.WriteFile(filepath.Join(project, "PHILOSOPHY.md"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := RequiredGuidance(context.Background(), project, 100000)
	if err != nil || !strings.Contains(result, text) {
		t.Fatal("canonical ruling was truncated", err)
	}
	if _, err = RequiredGuidance(context.Background(), project, 1000); err == nil {
		t.Fatal("insufficient mandatory allowance silently accepted")
	}
}
