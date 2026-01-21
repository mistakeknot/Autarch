package brief

import (
	"strings"
	"testing"
)

func TestBriefIncludesSummary(t *testing.T) {
	b := Compose(Input{ID: "PRD-001", Title: "T", Summary: "S"})
	if b == "" || !strings.Contains(b, "Summary:") || !strings.Contains(b, "S") {
		t.Fatalf("expected summary in brief")
	}
}
