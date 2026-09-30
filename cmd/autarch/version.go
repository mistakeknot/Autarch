package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

type versionInfo struct {
	VCS struct {
		Revision string `json:"revision"`
		Modified bool   `json:"modified"`
		Stamped  bool   `json:"stamped"`
	} `json:"vcs"`
	SHA256 string `json:"sha256"`
}

// executableSHA256 hashes the running binary, the value serve's /health also reports.
func executableSHA256() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readVersion() (versionInfo, error) {
	var v versionInfo
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				v.VCS.Revision = s.Value
				v.VCS.Stamped = true
			case "vcs.modified":
				v.VCS.Modified = s.Value == "true"
			}
		}
	}
	sum, err := executableSHA256()
	if err != nil {
		return v, fmt.Errorf("hash executable: %w", err)
	}
	v.SHA256 = sum
	return v, nil
}

func versionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the build revision and the executable's sha256",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := readVersion()
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "revision %s modified=%t sha256 %s\n", v.VCS.Revision, v.VCS.Modified, v.SHA256)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return cmd
}
