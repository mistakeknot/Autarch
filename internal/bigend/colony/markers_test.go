package colony

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".colony"), 0o755); err != nil {
		t.Fatalf("mkdir .colony: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}

	markers := detectMarkers(dir)
	if len(markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(markers))
	}
}
