// Command escalation-check verifies the evidence behind an escalation and
// prints PASS, FAIL or UNKNOWN for each item as JSON. See internal/escalcheck.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/internal/escalcheck"
)

type multi []string

func (m *multi) String() string     { return fmt.Sprint(*m) }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr *os.File) int {
	var o escalcheck.Options
	var req multi
	var pretty bool
	fs := flag.NewFlagSet("escalation-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.Script, "script", "", "path of the script being handed over")
	fs.StringVar(&o.SHA, "sha", "", "sha256 the caller states for the script (64 hex)")
	fs.StringVar(&o.PR, "pr", "", "pull request URL, https://github.com/OWNER/REPO/pull/N")
	fs.StringVar(&o.Review, "review", "", "review receipt file ending in REVIEWED-* lines and a VERDICT line")
	fs.StringVar(&o.Ruling, "ruling", "", "ruling ID to look up in the ledger")
	fs.StringVar(&o.Rulings, "rulings", "", "rulings.jsonl (default $XDG_STATE_HOME/vizier/rulings.jsonl)")
	fs.Var(&req, "require-check", "name of a check that must be present and green (repeatable)")
	fs.BoolVar(&o.PostMerge, "post-merge", false, "expect the pull request to be MERGED instead of OPEN")
	fs.BoolVar(&o.Offline, "offline", false, "do not call gh; the PR items are UNKNOWN")
	fs.StringVar(&o.RehearseState, "rehearse-state", "", "handoff-rehearse state directory (default: the tool's own)")
	fs.BoolVar(&pretty, "pretty", false, "indent the JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "escalation-check: unexpected argument", fs.Arg(0))
		return 2
	}
	o.RequireChecks = req
	if err := o.Validate(); err != nil {
		fmt.Fprintln(stderr, "escalation-check:", err)
		return 2
	}
	rep := escalcheck.Run(context.Background(), o, escalcheck.DefaultEnv())
	enc := json.NewEncoder(stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintln(stderr, "escalation-check:", err)
		return 2
	}
	return escalcheck.ExitCode(rep.Overall)
}
