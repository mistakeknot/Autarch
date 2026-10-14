// Package escalcheck decides, in code, the evidence checks an escalation needs:
// a script's digest, lint and rehearsal, a pull request's head and checks, a
// review receipt that binds that head or digest, and a ruling in an append-only
// ledger. It judges nothing that needs a person or a model (coverage of the
// action, review quality, design). PASS is evidence, never authority.
package escalcheck

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Status is the outcome of one item or of the whole report.
type Status string

const (
	Pass    Status = "PASS"
	Fail    Status = "FAIL"
	Unknown Status = "UNKNOWN"
)

// Item is one check's result.
type Item struct {
	ID     string         `json:"id"`
	Status Status         `json:"status"`
	Detail string         `json:"detail,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

// Options are the caller's inputs. Empty means not requested.
type Options struct {
	Script        string
	SHA           string
	PR            string
	Review        string
	Ruling        string
	Rulings       string
	RequireChecks []string
	PostMerge     bool
	Offline       bool
	RehearseState string
}

// Report is the JSON the CLI prints.
type Report struct {
	Schema  string         `json:"schema"`
	At      string         `json:"at"`
	Inputs  map[string]any `json:"inputs"`
	Items   []Item         `json:"items"`
	Overall Status         `json:"overall"`
	Summary map[string]int `json:"summary"`
}

// Env is everything the checks reach outside the process through, so tests can
// replace it.
type Env struct {
	Runner  Runner
	Now     func() time.Time
	Environ func() []string
	TempDir string
	// OnRead, when set, is called with each evidence file's path just before it
	// is read, so tests can check the order of reads.
	OnRead func(path string)
}

func (e Env) read(path string, max int64) ([]byte, os.FileInfo, error) {
	if e.OnRead != nil {
		e.OnRead(path)
	}
	return readRegular(path, max)
}

// DefaultEnv is the real environment.
func DefaultEnv() Env {
	return Env{Runner: ExecRunner{}, Now: time.Now, Environ: os.Environ}
}

var (
	hex64   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	prURLRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})/pull/([1-9][0-9]{0,8})$`)
	idRe    = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	nameRe  = regexp.MustCompile(`^[A-Za-z0-9 _.:/()\[\]@+-]{1,200}$`)
)

// UsageError is a bad combination of inputs; no items run.
type UsageError string

func (e UsageError) Error() string { return string(e) }

// Validate checks the dependency matrix before anything runs.
func (o Options) Validate() error {
	if o.Script == "" && o.PR == "" && o.Review == "" && o.Ruling == "" {
		return UsageError("give at least one of --script, --pr, --review, --ruling")
	}
	if o.Script != "" && o.SHA == "" {
		return UsageError("--script needs --sha: the digest being vouched for must come from the caller")
	}
	if o.SHA != "" {
		if o.Script == "" {
			return UsageError("--sha needs --script")
		}
		if !hex64.MatchString(strings.ToLower(o.SHA)) {
			return UsageError("--sha must be 64 hex digits")
		}
	}
	if o.Review != "" && o.PR == "" && o.Script == "" {
		return UsageError("--review needs --pr or --script: a review is checked against what it claims to bind")
	}
	if (o.PostMerge || len(o.RequireChecks) > 0) && o.PR == "" {
		return UsageError("--post-merge and --require-check need --pr")
	}
	if o.RehearseState != "" && o.Script == "" {
		return UsageError("--rehearse-state needs --script")
	}
	if o.Rulings != "" && o.Ruling == "" {
		return UsageError("--rulings needs --ruling")
	}
	if o.Ruling != "" && !idRe.MatchString(o.Ruling) {
		return UsageError("--ruling is not a plain ruling ID")
	}
	for _, n := range o.RequireChecks {
		if !nameRe.MatchString(n) {
			return UsageError(fmt.Sprintf("--require-check %q has characters a check name cannot", n))
		}
	}
	return nil
}

// Run performs every requested check. The caller has already called Validate.
func Run(ctx context.Context, o Options, env Env) Report {
	r := Report{Schema: "escalation-check/v1", At: env.Now().UTC().Format(time.RFC3339), Inputs: map[string]any{}}
	set := func(k string, v any) { r.Inputs[k] = v }
	if o.Script != "" {
		set("script", o.Script)
		set("sha", strings.ToLower(o.SHA))
	}
	if o.PR != "" {
		set("pr", o.PR)
		set("post_merge", o.PostMerge)
		set("offline", o.Offline)
		if len(o.RequireChecks) > 0 {
			set("require_checks", o.RequireChecks)
		}
	}
	if o.Review != "" {
		set("review", o.Review)
	}
	if o.Ruling != "" {
		set("ruling", o.Ruling)
	}

	// Order: script, first PR read, ruling, review file, final PR read. The
	// final read comes after every other evidence read; the review is bound to
	// its head.
	var scriptItems, prItems, reviewItems, rulingItems []Item
	var scriptDigest string
	if o.Script != "" {
		scriptItems, scriptDigest = checkScript(ctx, o, env)
	}
	var run *prRun
	if o.PR != "" {
		run = startPR(ctx, o, env)
	}
	if o.Ruling != "" {
		rulingItems = checkRuling(o, env)
	}
	var rv *reviewDoc
	if o.Review != "" {
		rv = readReview(o, env)
	}
	var pr *prEvidence
	if run != nil {
		prItems, pr = run.finish(ctx)
	}
	if rv != nil {
		reviewItems = rv.items(o, pr, scriptDigest)
	}
	r.Items = append(append(append(append(r.Items, scriptItems...), prItems...), reviewItems...), rulingItems...)
	r.Overall, r.Summary = roll(r.Items)
	return r
}

func roll(items []Item) (Status, map[string]int) {
	sum := map[string]int{"pass": 0, "fail": 0, "unknown": 0}
	for _, it := range items {
		switch it.Status {
		case Pass:
			sum["pass"]++
		case Fail:
			sum["fail"]++
		default:
			sum["unknown"]++
		}
	}
	switch {
	case sum["fail"] > 0:
		return Fail, sum
	case sum["unknown"] > 0 || len(items) == 0:
		return Unknown, sum
	}
	return Pass, sum
}

// ExitCode maps the overall status: 0 PASS, 1 FAIL, 3 UNKNOWN (2 is usage).
func ExitCode(s Status) int {
	switch s {
	case Pass:
		return 0
	case Fail:
		return 1
	}
	return 3
}

func item(id string, st Status, format string, a ...any) Item {
	return Item{ID: id, Status: st, Detail: fmt.Sprintf(format, a...)}
}
