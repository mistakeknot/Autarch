package specs

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadSummariesHybridID(t *testing.T) {
    dir := t.TempDir()
    spec1 := []byte("id: TAND-001\ntitle: Test One\nstatus: ready\n")
    spec2 := []byte("title: No ID\nstatus: draft\n")

    if err := os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), spec1, 0o644); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(dir, "TAND-002.yaml"), spec2, 0o644); err != nil {
        t.Fatal(err)
    }

    summaries, warnings := LoadSummaries(dir)
    if len(summaries) != 2 {
        t.Fatalf("expected 2 summaries, got %d", len(summaries))
    }
    if summaries[1].ID != "TAND-002" {
        t.Fatalf("expected filename fallback ID, got %s", summaries[1].ID)
    }
    if len(warnings) == 0 {
        t.Fatal("expected warnings for missing id")
    }
}
