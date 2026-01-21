package commands

import (
    "testing"

    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
)

func TestSummariesToCounts(t *testing.T) {
    counts := summariesToCounts([]specs.SpecSummary{
        {Status: "ready"},
        {Status: "draft"},
        {Status: ""},
    })
    if counts["ready"] != 1 || counts["draft"] != 1 || counts["unknown"] != 1 {
        t.Fatalf("unexpected counts: %v", counts)
    }
}
