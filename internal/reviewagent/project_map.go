package reviewagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mistakeknot/autarch/internal/door"
	"github.com/mistakeknot/autarch/pkg/review"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func ProjectMap(store *review.Store, r review.Request) review.Response {
	fail := func(err error) review.Response { return review.Response{Version: review.Version, Error: err.Error()} }
	canonical, err := filepath.EvalSymlinks(r.Project)
	if err != nil || canonical != r.Project {
		return fail(fmt.Errorf("canonical selected project required"))
	}
	if _, ok := store.Snapshot().ProjectVisit(r.Project); !ok {
		return fail(fmt.Errorf("unknown project visit"))
	}
	if r.Method == "project.overview" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		brief := door.ReadProductBrief(ctx, r.Project, nil)
		data, _ := json.Marshal(map[string]string{"overview": door.BuildOnboardingBrief(brief)})
		return review.Response{Version: review.Version, Trace: data}
	}
	exe, _ := os.Executable()
	lattice := filepath.Join(filepath.Dir(exe), "lattice")
	python := filepath.Join(lattice, ".venv", "bin", "python")
	if _, err = os.Stat(python); err != nil {
		return fail(fmt.Errorf("Lattice project projection runtime unavailable"))
	}
	args := []string{"-m", "lattice.feedback", "--records", filepath.Join(store.Dir(), "records"), "--database", filepath.Join(store.Dir(), "feedback.db"), "--project", r.Project}
	if r.Method == "project.rebuild" {
		if !traceRebuild.TryLock() {
			return fail(fmt.Errorf("projection rebuild already in progress"))
		}
		defer traceRebuild.Unlock()
	} else if r.Method == "project.map" {
		args = append(args, "--query", "project_map", "--max-nodes", "120", "--max-edges", "200")
	} else {
		return fail(fmt.Errorf("unknown project query"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Dir = lattice
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fail(fmt.Errorf("project projection unavailable: %w: %s", err, stderr.String()))
	}
	return review.Response{Version: review.Version, Trace: out}
}
