package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
)

func needsMkDismissOwnerNoticeCmd() *cobra.Command {
	return &cobra.Command{Use: "dismiss-owner-notice REQUEST", Short: "Acknowledge a handled release owner notice without cancelling the request", Long: "Clear only the owner-attention notice for a held, recovered or retired request. Delivery, ownership and budget records stay intact. If a held request still has an unavailable owner, a later release may flag it again.", Args: cobra.ExactArgs(1), SilenceUsage: true, RunE: func(cmd *cobra.Command, args []string) error {
		if err := newCardFiler().DismissReleaseOwnerAttention(args[0]); err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"request": args[0], "dismissed": true})
	}}
}
