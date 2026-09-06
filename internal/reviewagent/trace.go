package reviewagent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

// One controller owns the source store and its shared projection database.
// Separate clients must not launch concurrent refresh writers either.
var traceRebuild sync.Mutex

func Trace(store *review.Store, r review.Request) review.Response {
	exe, _ := os.Executable()
	return traceFromRuntime(store, r, filepath.Dir(exe))
}

func traceFromRuntime(store *review.Store, r review.Request, runtimeDir string) review.Response {
	fail := func(err error) review.Response { return review.Response{Version: review.Version, Error: err.Error()} }
	if !traceRebuild.TryLock() {
		return fail(fmt.Errorf("review trace is already rebuilding; wait for the current refresh"))
	}
	defer traceRebuild.Unlock()
	state := store.Snapshot()
	kind := r.Text
	valid := false
	switch kind {
	case "feedback":
		v, ok := state.Feedback[r.Target]
		valid = ok && v.Project == r.Project && v.Revision == r.Revision
	case "proposal":
		v, ok := state.Proposals[r.Target]
		valid = ok && v.Project == r.Project && v.Revision == r.Revision
	case "execution":
		v, ok := state.Executions[r.Target]
		valid = ok && v.Project == r.Project
	case "session":
		v, ok := state.Sessions[r.Target]
		valid = ok && v.Project == r.Project
	}
	if !valid {
		return fail(fmt.Errorf("trace source is stale or outside selected project"))
	}
	lattice := filepath.Join(runtimeDir, "lattice")
	python := filepath.Join(lattice, ".venv", "bin", "python")
	if _, err := os.Stat(python); err != nil {
		return fail(fmt.Errorf("lattice feedback projection unavailable; rebuild the review pilot to install its local runtime"))
	}
	args := []string{"-m", "lattice.feedback", "--refresh-beads", "--records", filepath.Join(store.Dir(), "records"), "--database", filepath.Join(store.Dir(), "feedback.db"), "--project", r.Project, "--kind", kind, "--id", r.Target}
	if kind == "feedback" || kind == "proposal" {
		args = append(args, "--revision", strconv.Itoa(r.Revision))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Dir = lattice
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return fail(fmt.Errorf("feedback trace rebuild failed: %w: %s", err, stderr.String()))
	}
	return review.Response{Version: review.Version, Trace: data}
}
