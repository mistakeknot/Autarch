package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// newCardFiler builds the filer; tests replace it.
var newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Timeout: filerTimeout} }

func needsMkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "needs-mk",
		Short:         "File asks with mk as tasks cards",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(needsMkFileCmd())
	return cmd
}

// parseRootRunFlag reads `script=/abs/path,timeout=60,set=name`. The file is hashed here, so the
// card pins the exact bytes mk will be shown.
func parseRootRunFlag(v string) (*homeask.RootRun, error) {
	vals := map[string]string{}
	for _, part := range strings.Split(v, ",") {
		k, val, ok := strings.Cut(part, "=")
		if !ok || (k != "script" && k != "timeout" && k != "set") || val == "" {
			return nil, &usageError{"--root-run takes script=<absolute path>,timeout=<seconds>,set=<name>"}
		}
		if _, dup := vals[k]; dup {
			return nil, &usageError{"--root-run repeats " + k}
		}
		vals[k] = val
	}
	for _, k := range []string{"script", "timeout", "set"} {
		if vals[k] == "" {
			return nil, &usageError{"--root-run needs " + k + "="}
		}
	}
	n, err := strconv.Atoi(vals["timeout"])
	if err != nil {
		return nil, &usageError{"--root-run timeout must be a number of seconds"}
	}
	if !filepath.IsAbs(vals["script"]) {
		return nil, &usageError{"--root-run script must be an absolute path"}
	}
	raw, err := os.ReadFile(vals["script"])
	if err != nil {
		return nil, &usageError{"--root-run script is not readable: " + err.Error()}
	}
	sum := sha256.Sum256(raw)
	return &homeask.RootRun{Script: vals["script"], SHA256: hex.EncodeToString(sum[:]), Timeout: n, Set: vals["set"]}, nil
}

func needsMkFileCmd() *cobra.Command {
	var project, title, askFile, rootRun, request string
	var blocks []string
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File a needs-mk card from the asking thread; prints the card as one JSON line",
		Long: `Run inside the asking thread ($BB_THREAD_ID is required): the filer posts the
routing comment from it. --project is the tasks project. --ask-file is a JSON object with
question, options and optionally subject, recommendation, ask_key, project and project_root.

Exit codes: 2 usage or refused, 3 Home or tasks unavailable (nothing created), 4 card created
but the routing comment failed (re-run the same command), 5 not yet confirmed.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			thread := os.Getenv("BB_THREAD_ID")
			if thread == "" {
				return &usageError{"BB_THREAD_ID is required: run inside the asking bb thread"}
			}
			if project == "" || title == "" || askFile == "" {
				return &usageError{"--project, --title and --ask-file are required"}
			}
			f, err := os.Open(askFile)
			if err != nil {
				return &usageError{"--ask-file: " + err.Error()}
			}
			raw, err := io.ReadAll(io.LimitReader(f, 16*1024+1))
			f.Close()
			if err != nil {
				return &usageError{"--ask-file: " + err.Error()}
			}
			if len(raw) > 16*1024 {
				return &usageError{"--ask-file is over 16 KiB"}
			}
			var ask map[string]any
			if err := json.Unmarshal(raw, &ask); err != nil {
				return &usageError{"--ask-file is not a JSON object: " + err.Error()}
			}
			if ask == nil {
				return &usageError{"--ask-file is not a JSON object: got null"}
			}
			if _, ok := ask["project_root"]; !ok {
				ask["project_root"] = gitRoot()
			}
			if _, ok := ask["project"]; !ok {
				ask["project"] = filepath.Base(fmt.Sprint(ask["project_root"]))
			}
			req := homeask.CardRequest{Project: project, Title: title, Blocks: blocks, Ask: ask, Key: request, Thread: thread}
			if rootRun != "" {
				if req.RootRun, err = parseRootRunFlag(rootRun); err != nil {
					return err
				}
			}
			if req.Key == "" {
				ident, err := homeask.RequestIdentity(req)
				if err != nil {
					return err
				}
				req.Key = homeask.DerivedKey(thread, ident)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := newCardFiler().FileCard(ctx, req)
			if err != nil {
				return err
			}
			out, _ := json.Marshal(res)
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Tasks project (id, key or name)")
	cmd.Flags().StringVar(&title, "title", "", "Card title")
	cmd.Flags().StringArrayVar(&blocks, "blocks", nil, "Blocks ref (bead:ID, thread:thr_ID, project:ID); repeatable")
	cmd.Flags().StringVar(&askFile, "ask-file", "", "JSON file with the ask")
	cmd.Flags().StringVar(&rootRun, "root-run", "", "script=<abs path>,timeout=<seconds>,set=<name>")
	cmd.Flags().StringVar(&request, "request", "", "Request key (default: derived from thread and ask)")
	return cmd
}
