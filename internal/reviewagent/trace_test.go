package reviewagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

func TestTraceRequestsCannotRunOverlappingProjectionWriters(t *testing.T) {
	store, _ := review.Open(t.TempDir())
	project := t.TempDir()
	r := store.Apply(review.Request{Version: review.Version, ID: "note", Method: "feedback.save", Project: project, Text: "Observation"})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	project = store.Snapshot().Feedback[r.ID].Project
	request := review.Request{Project: project, Target: r.ID, Text: "feedback", Revision: 1}
	runtimeDir := t.TempDir()
	python := filepath.Join(runtimeDir, "lattice", ".venv", "bin", "python")
	if err := os.MkdirAll(filepath.Dir(python), 0700); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if ! mkdir active 2>/dev/null; then echo "overlapping writer" >&2; exit 1; fi
touch started
tries=0
while [ ! -e release ] && [ "$tries" -lt 300 ]; do sleep 0.01; tries=$((tries+1)); done
printf '{"entities":[],"relationships":[]}'
`
	if err := os.WriteFile(python, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	first := make(chan review.Response, 1)
	go func() { first <- traceFromRuntime(store, request, runtimeDir) }()
	release := filepath.Join(runtimeDir, "lattice", "release")
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0600) })
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(runtimeDir, "lattice", "started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("projection did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	second := traceFromRuntime(store, request, runtimeDir)
	if !strings.Contains(second.Error, "already rebuilding") {
		t.Errorf("overlapping caller reached writer: %+v", second)
	}
	_ = os.WriteFile(release, nil, 0600)
	select {
	case response := <-first:
		if response.Error != "" || len(response.Trace) == 0 {
			t.Fatalf("first trace lost: %+v", response)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("projection did not finish")
	}
}
