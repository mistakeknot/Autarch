package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"time"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

var defaultTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// BenchmarkHashBytes measures SHA256 hashing of YAML-sized payloads.
func BenchmarkHashBytes(b *testing.B) {
	sizes := []int{256, 1024, 4096}
	for _, sz := range sizes {
		data := make([]byte, sz)
		for i := range data {
			data[i] = byte(i % 256)
		}
		b.Run(fmt.Sprintf("bytes_%d", sz), func(b *testing.B) {
			b.SetBytes(int64(sz))
			for b.Loop() {
				sum := sha256.Sum256(data)
				hex.EncodeToString(sum[:])
			}
		})
	}
}

// BenchmarkYAMLDecode measures YAML decoding of spec and task documents.
func BenchmarkYAMLDecode(b *testing.B) {
	specYAML := []byte("id: \"PRD-001\"\ntitle: \"Benchmark Spec\"\ntype: \"feature\"\nstatus: \"draft\"\nversion: 1\ncreated_at: \"2026-03-19T00:00:00Z\"\n")
	taskYAML := []byte("id: \"TASK-001\"\nstory_id: \"STORY-001\"\ntitle: \"Benchmark Task\"\ndescription: \"A task for benchmarking\"\nstatus: \"in_progress\"\npriority: 1\nassignee: \"agent-1\"\nworktree_ref: \"wt-1\"\nsession_ref: \"sess-1\"\ncreated_at: \"2026-03-19T00:00:00Z\"\nupdated_at: \"2026-03-19T00:00:00Z\"\n")

	b.Run("spec_doc", func(b *testing.B) {
		for b.Loop() {
			var doc specDoc
			yamlsafe.Decode(specYAML, &doc)
		}
	})
	b.Run("task_doc", func(b *testing.B) {
		for b.Loop() {
			var doc taskDoc
			yamlsafe.Decode(taskYAML, &doc)
		}
	})
}

// BenchmarkMapTaskStatus measures the status mapping switch.
func BenchmarkMapTaskStatus(b *testing.B) {
	statuses := []string{"in_progress", "blocked", "done", "pending", "completed", "working", "todo", "unknown"}
	for b.Loop() {
		for _, s := range statuses {
			mapTaskStatus(s)
		}
	}
}

// BenchmarkParseTimeOr measures time parsing with fallback.
func BenchmarkParseTimeOr(b *testing.B) {
	b.Run("valid_rfc3339", func(b *testing.B) {
		for b.Loop() {
			parseTimeOr(defaultTime, "2026-03-19T12:00:00Z")
		}
	})
	b.Run("empty_fallback", func(b *testing.B) {
		for b.Loop() {
			parseTimeOr(defaultTime, "")
		}
	})
}

// BenchmarkReconcileSpecs benchmarks the full spec reconciliation path
// with a real SQLite store and synthetic YAML files on disk.
func BenchmarkReconcileSpecs(b *testing.B) {
	counts := []int{10, 50, 200}
	for _, n := range counts {
		b.Run(fmt.Sprintf("specs_%d", n), func(b *testing.B) {
			root := b.TempDir()
			specDir := filepath.Join(root, ".gurgeh", "specs")
			if err := os.MkdirAll(specDir, 0755); err != nil {
				b.Fatalf("mkdir: %v", err)
			}
			for i := 0; i < n; i++ {
				content := fmt.Sprintf("id: \"PRD-%04d\"\ntitle: \"Spec %d\"\nstatus: \"draft\"\nversion: 1\n", i, i)
				path := filepath.Join(specDir, fmt.Sprintf("PRD-%04d.yaml", i))
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					b.Fatalf("write spec %d: %v", i, err)
				}
			}

			dbPath := filepath.Join(root, "events.db")
			store, err := OpenStore(dbPath)
			if err != nil {
				b.Fatalf("open store: %v", err)
			}
			defer store.Close()

			ctx := context.Background()

			// First pass to populate cursors (so subsequent runs test the skip path)
			if _, err := ReconcileProject(ctx, root, store); err != nil {
				b.Fatalf("initial reconcile: %v", err)
			}

			b.ResetTimer()
			for b.Loop() {
				ReconcileProject(ctx, root, store)
			}
		})
	}
}

// BenchmarkReconcileTasks benchmarks the full task reconciliation path.
func BenchmarkReconcileTasks(b *testing.B) {
	counts := []int{10, 50, 200}
	for _, n := range counts {
		b.Run(fmt.Sprintf("tasks_%d", n), func(b *testing.B) {
			root := b.TempDir()
			tasksDir := filepath.Join(root, ".coldwine", "tasks")
			if err := os.MkdirAll(tasksDir, 0755); err != nil {
				b.Fatalf("mkdir: %v", err)
			}
			for i := 0; i < n; i++ {
				content := fmt.Sprintf("id: \"TASK-%04d\"\ntitle: \"Task %d\"\nstatus: \"pending\"\npriority: %d\n", i, i, i%5)
				path := filepath.Join(tasksDir, fmt.Sprintf("TASK-%04d.yaml", i))
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					b.Fatalf("write task %d: %v", i, err)
				}
			}

			dbPath := filepath.Join(root, "events.db")
			store, err := OpenStore(dbPath)
			if err != nil {
				b.Fatalf("open store: %v", err)
			}
			defer store.Close()

			ctx := context.Background()

			// First pass to populate cursors
			if _, err := ReconcileProject(ctx, root, store); err != nil {
				b.Fatalf("initial reconcile: %v", err)
			}

			b.ResetTimer()
			for b.Loop() {
				ReconcileProject(ctx, root, store)
			}
		})
	}
}

// BenchmarkReconcileFirstRun benchmarks the first reconciliation (no cursors yet).
// Uses traditional loop because b.Loop() does not support StopTimer/StartTimer.
func BenchmarkReconcileFirstRun(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		root := b.TempDir()
		specDir := filepath.Join(root, ".gurgeh", "specs")
		tasksDir := filepath.Join(root, ".coldwine", "tasks")
		os.MkdirAll(specDir, 0755)
		os.MkdirAll(tasksDir, 0755)
		for j := 0; j < 20; j++ {
			os.WriteFile(
				filepath.Join(specDir, fmt.Sprintf("PRD-%04d.yaml", j)),
				[]byte(fmt.Sprintf("id: \"PRD-%04d\"\ntitle: \"Spec %d\"\nstatus: \"draft\"\nversion: 1\n", j, j)),
				0644,
			)
			os.WriteFile(
				filepath.Join(tasksDir, fmt.Sprintf("TASK-%04d.yaml", j)),
				[]byte(fmt.Sprintf("id: \"TASK-%04d\"\ntitle: \"Task %d\"\nstatus: \"pending\"\n", j, j)),
				0644,
			)
		}

		dbPath := filepath.Join(root, "events.db")
		store, err := OpenStore(dbPath)
		if err != nil {
			b.Fatalf("open store: %v", err)
		}

		b.StartTimer()
		ReconcileProject(context.Background(), root, store)
		b.StopTimer()

		store.Close()
	}
}
