package reviewagent

import (
	"context"
	"time"

	"github.com/mistakeknot/autarch/pkg/clavain"
	"github.com/mistakeknot/autarch/pkg/review"
)

// RunExecution is independent of model investigation and the TUI. Only frozen,
// human-accepted records can cross this boundary into Clavain.
func RunExecution(ctx context.Context, store *review.Store, bin string) {
	tick := time.NewTicker(4 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		state := store.Snapshot()
		for _, execution := range state.Executions {
			if execution.Status == "ready_for_retest" || execution.Status == "failed" {
				continue
			}
			p, ok := state.Proposals[execution.ProposalID]
			if !ok || p.Status != "accepted" || p.Revision != execution.ProposalRevision {
				continue
			}
			options := []clavain.Option{clavain.WithProjectDir(p.Project)}
			if bin != "" {
				options = append(options, clavain.WithBinPath(bin))
			}
			client, err := clavain.New(options...)
			var next review.Execution
			if err == nil {
				call, cancel := context.WithTimeout(ctx, 15*time.Second)
				next, err = client.Review(call, p)
				cancel()
			}
			if err != nil {
				next = execution
				next.Status = "blocked"
				next.Reason = err.Error()
			}
			if next.Status == execution.Status && next.WorkID == execution.WorkID && next.RunID == execution.RunID && next.Model == execution.Model && next.Reason == execution.Reason {
				continue
			}
			store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "execution.save", Project: p.Project, Execution: &next})
		}
	}
}
