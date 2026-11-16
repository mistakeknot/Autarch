package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
)

func needsMkPilotsCmd() *cobra.Command {
	c := &cobra.Command{Use: "pilots", Short: "Inspect or start/stop two-week filing pilots (missing entries are off)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		p, e := newCardFiler().Pilots()
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(p)
	}}
	for _, action := range []string{"start", "stop"} {
		action := action
		c.AddCommand(&cobra.Command{Use: action + " NAME", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
			if e := newCardFiler().SetPilot(a[0], action == "start"); e != nil {
				return e
			}
			fmt.Fprintln(c.OutOrStdout(), a[0], action)
			return nil
		}})
	}
	return c
}
