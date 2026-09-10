package reviewagent

import (
	"context"
	"github.com/mistakeknot/autarch/pkg/clavain"
	"github.com/mistakeknot/autarch/pkg/review"
	"time"
)

func RunPreparation(ctx context.Context, store *review.Store, bin string) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		for _, p := range store.Snapshot().Preparations {
			if p.Status == "reviewed" || p.Status == "blocked" {
				continue
			}
			options := []clavain.Option{clavain.WithProjectDir(p.Project)}
			if bin != "" {
				options = append(options, clavain.WithBinPath(bin))
			}
			client, err := clavain.New(options...)
			if err != nil {
				_ = store.RecordPreparation(p.ID, nil, err)
				continue
			}
			call, cancel := context.WithTimeout(ctx, 45*time.Second)
			operation := "status"
			if p.Status == "requested" {
				_, err = client.Prepare(call, "ratify", p.Request)
				operation = "submit"
			}
			if err != nil {
				cancel()
				_ = store.RecordPreparation(p.ID, nil, err)
				continue
			}
			receipt, err := client.Prepare(call, operation, p.Request)
			cancel()
			if saveErr := store.RecordPreparation(p.ID, receipt, err); saveErr != nil {
				_ = store.RecordPreparation(p.ID, nil, saveErr)
			}
		}
	}
}
