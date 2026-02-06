package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSaveRevisionUsesFilesystemVersionAndDoesNotMutateInput(t *testing.T) {
	root := t.TempDir()
	spec := &Spec{
		ID:      "PRD-001",
		Title:   "Example",
		Summary: "Example summary",
		Version: 0,
	}

	rev1, err := SaveRevision(root, spec, "user", "manual", nil)
	if err != nil {
		t.Fatalf("save revision 1: %v", err)
	}
	if rev1.Version != 1 {
		t.Fatalf("expected version 1, got %d", rev1.Version)
	}
	if spec.Version != 0 {
		t.Fatalf("input spec version mutated: got %d, want 0", spec.Version)
	}

	rev2, err := SaveRevision(root, spec, "user", "manual", nil)
	if err != nil {
		t.Fatalf("save revision 2: %v", err)
	}
	if rev2.Version != 2 {
		t.Fatalf("expected version 2 from filesystem state, got %d", rev2.Version)
	}
	if spec.Version != 0 {
		t.Fatalf("input spec version mutated: got %d, want 0", spec.Version)
	}

	reloaded, err := LoadRevisionSpec(root, spec.ID, 2)
	if err != nil {
		t.Fatalf("load revision spec: %v", err)
	}
	if reloaded.Version != 2 {
		t.Fatalf("expected saved snapshot version 2, got %d", reloaded.Version)
	}
}

func TestSaveRevisionWriteFailureDoesNotMutateInput(t *testing.T) {
	root := t.TempDir()
	spec := &Spec{
		ID:      "PRD/001",
		Title:   "Invalid path id",
		Summary: "will fail to write",
		Version: 7,
	}

	_, err := SaveRevision(root, spec, "user", "manual", nil)
	if err == nil {
		t.Fatalf("expected save revision error")
	}
	if spec.Version != 7 {
		t.Fatalf("input spec version mutated on failure: got %d, want 7", spec.Version)
	}
}

func TestSaveRevisionConcurrentCallsProduceUniqueVersions(t *testing.T) {
	root := t.TempDir()
	const workers = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			spec := &Spec{
				ID:      "PRD-001",
				Title:   fmt.Sprintf("Spec %d", i),
				Summary: "Concurrent save",
				Version: 0,
			}

			rev, err := SaveRevision(root, spec, "arbiter", "manual", nil)
			if err != nil {
				errCh <- fmt.Errorf("worker %d save: %w", i, err)
				return
			}
			if spec.Version != 0 {
				errCh <- fmt.Errorf("worker %d mutated input version: got %d", i, spec.Version)
				return
			}

			snapPath := filepath.Join(historyDir(root), fmt.Sprintf("%s_v%d.yaml", spec.ID, rev.Version))
			if _, err := os.Stat(snapPath); err != nil {
				errCh <- fmt.Errorf("worker %d missing snapshot %q: %w", i, snapPath, err)
				return
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	revisions, err := LoadHistory(root, "PRD-001")
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(revisions) != workers {
		t.Fatalf("expected %d revisions, got %d", workers, len(revisions))
	}

	for i, rev := range revisions {
		expected := i + 1
		if rev.Version != expected {
			t.Fatalf("expected revision[%d] version %d, got %d", i, expected, rev.Version)
		}
	}
}
