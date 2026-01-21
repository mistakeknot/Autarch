package commands

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

func writeJSON(cmd *cobra.Command, payload interface{}) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
