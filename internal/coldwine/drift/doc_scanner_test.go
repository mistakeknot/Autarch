package drift

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDocActionItems(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "solutions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(docs, "plans"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	solution := `---
title: "Fix"
date_resolved:
---
# Solution
- [ ] follow up implementation
`
	if err := os.WriteFile(filepath.Join(docs, "solutions", "fix.md"), []byte(solution), 0o644); err != nil {
		t.Fatalf("write solution: %v", err)
	}

	plan := `## Open Questions
- should we add retries?

## The Critical Issues
- missing timeout policy
`
	if err := os.WriteFile(filepath.Join(docs, "plans", "plan.md"), []byte(plan), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	items, err := ScanDocActionItems(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(items) < 4 {
		t.Fatalf("expected multiple action items, got %d", len(items))
	}

	hasKind := func(kind string) bool {
		for _, it := range items {
			if it.Kind == kind {
				return true
			}
		}
		return false
	}

	for _, kind := range []string{"solution_unresolved", "checkbox", "open_question", "critical_section"} {
		if !hasKind(kind) {
			t.Fatalf("expected kind %q in extracted items", kind)
		}
	}
}
