package door

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// CheckerName is what the dotfiles installers put on PATH. Resolution goes
// through PATH for the same reason the Zed extension's does: the installers
// are the deployment contract, and a hardcoded home-relative path would rot
// the day that contract moves.
const CheckerName = "card-check.py"

// checkerEnv overrides checker resolution; tests point it at a stub so the
// suite exercises this package's transcription, not the checker's rules
// (those have their own suite in dotfiles).
const checkerEnv = "AUTARCH_CARD_CHECK"

// perCheckTimeout bounds one checker run. The checker reads one file; if it
// is stuck for 10 seconds something is wrong enough to report as Unchecked.
const perCheckTimeout = 10 * time.Second

// ResolveChecker finds card-check.py, honoring the test override.
func ResolveChecker() (string, error) {
	if p := os.Getenv(checkerEnv); p != "" {
		return p, nil
	}
	p, err := exec.LookPath(CheckerName)
	if err != nil {
		return "", fmt.Errorf("%s not on PATH -- deploy it with dotfiles install-macos.sh (or install-server.sh)", CheckerName)
	}
	return p, nil
}

// payload is the checker's --json envelope. Verdict and strength are both
// always present in the contract precisely so no consumer infers one from
// the other; parsing them as required fields keeps that property here.
type payload struct {
	Verdict  string   `json:"verdict"`
	Code     int      `json:"code"`
	Card     string   `json:"card"`
	Reason   string   `json:"reason"`
	Strength Strength `json:"strength"`
}

// CheckOne runs the checker for a single project root and transcribes the
// result onto the project. Any failure to obtain a parseable verdict leaves
// the project Unchecked with Err set -- never Absent, which is a verdict
// only the checker may issue.
func CheckOne(ctx context.Context, checker string, p Project) Project {
	ctx, cancel := context.WithTimeout(ctx, perCheckTimeout)
	defer cancel()

	// All four verdicts arrive as JSON on stdout with exit codes 0-3, so the
	// exit code is not an error signal here; an unparseable stdout is.
	out, _ := exec.CommandContext(ctx, checker, p.Root, "--json").Output()

	var pl payload
	if err := json.Unmarshal(out, &pl); err != nil {
		p.Verdict = VerdictUnchecked
		p.Err = fmt.Errorf("checker gave no verdict for %s: %w", p.Name, err)
		if ctx.Err() != nil {
			p.Err = fmt.Errorf("checker timed out on %s", p.Name)
		}
		return p
	}
	switch pl.Verdict {
	case string(VerdictConfirmed), string(VerdictProvisional),
		string(VerdictInvalid), string(VerdictAbsent):
		p.Verdict = Verdict(pl.Verdict)
	default:
		p.Verdict = VerdictUnchecked
		p.Err = fmt.Errorf("checker returned unknown verdict %q for %s", pl.Verdict, p.Name)
		return p
	}
	p.Strength = pl.Strength
	p.Reason = pl.Reason
	p.Err = nil
	return p
}

// CheckAll runs the checker across the estate with bounded parallelism and
// streams each finished project to onResult (called from worker goroutines;
// the Bubble Tea model receives them via a channel, so ordering is by
// completion, not by index). Blocks until every project has been reported.
func CheckAll(ctx context.Context, checker string, projects []Project, onResult func(int, Project)) {
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range projects {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			onResult(i, CheckOne(ctx, checker, projects[i]))
		}(i)
	}
	wg.Wait()
}
