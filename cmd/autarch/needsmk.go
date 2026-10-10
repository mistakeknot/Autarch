package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/serve"
)

// newCardFiler builds the filer; tests replace it.
var newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Timeout: filerTimeout} }

// checkFilingPreconditions runs before anything is created: serve must be up and the ask's
// project_root must be the root it resolves for the ask's project. Tests replace it.
var checkFilingPreconditions = func(project, root string) error {
	ps, err := serve.FetchProjects(serveURL(), serveTokenPath(), serveProbeTimeout)
	if err != nil {
		hint := ""
		if errors.Is(err, serve.ErrNotRunning) {
			hint = "; start `autarch serve` (see AGENTS.md)"
		}
		return fmt.Errorf("%w: %v%s: nothing was filed", homeask.ErrHomeDown, err, hint)
	}
	if err := serve.CheckAsk(ps, project, root); err != nil {
		return fmt.Errorf("%w: %v", homeask.ErrInvalid, err)
	}
	return nil
}

// serveProbeTimeout bounds the serve probe so a dead serve costs seconds, not a minute.
const serveProbeTimeout = 3 * time.Second

// serveURL is where `autarch serve` listens (AUTARCH_SERVE_URL, default the serve address).
func serveURL() string {
	if u := os.Getenv("AUTARCH_SERVE_URL"); u != "" {
		return u
	}
	return "http://" + serve.DefaultAddr
}

// serveTokenPath is serve's bearer token (AUTARCH_SERVE_TOKEN_FILE, default ~/.autarch/serve.token).
func serveTokenPath() string {
	if p := os.Getenv("AUTARCH_SERVE_TOKEN_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".autarch", "serve.token")
}

func needsMkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "needs-mk",
		Short:         "File asks with mk as tasks cards",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(needsMkFileCmd())
	cmd.AddCommand(needsMkAdoptMoveCmd())
	cmd.AddCommand(needsMkFooterCmd())
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

// readMoveFile reads a home-move/v1 JSON file (at most 16 KiB) and validates it with the strict parser the
// plugin uses, so a move Home would show as display-only is refused here. Nothing in it is executed.
func readMoveFile(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &usageError{"--move: " + err.Error()}
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16*1024+1))
	f.Close()
	if err != nil {
		return nil, &usageError{"--move: " + err.Error()}
	}
	if len(raw) > 16*1024 {
		return nil, &usageError{"--move is over 16 KiB"}
	}
	mv, err := homeask.ParseMove(string(raw))
	if err != nil {
		return nil, &usageError{"--move is not a valid home-move/v1: " + err.Error()}
	}
	return mv.Raw, nil
}

// moveOnlyAsk is the minimal ruling-only ask a move card carries when the filer gave none: the card needs a
// home-ask block to be a card, and mk's pick keeps the card open until independent evidence closes the move.
func moveOnlyAsk(title string) map[string]any {
	return map[string]any{
		"question": strings.TrimSpace(title),
		"options": []any{
			map[string]any{"id": "noted", "label": "Noted", "kind": "ruling-only"},
			map[string]any{"id": "not-now", "label": "Not now", "kind": "ruling-only"},
		},
	}
}

func needsMkAdoptMoveCmd() *cobra.Command {
	var card, moveFile string
	cmd := &cobra.Command{
		Use:   "adopt-move",
		Short: "Attach a home-move/v1 block to a card this thread already filed; prints the card as one JSON line",
		Long: `Run inside the asking thread ($BB_THREAD_ID is required). --card is the tasks card id and --move is a
home-move/v1 JSON file (kind script, pr, read or context). The card must already parse as a Home card and
must have been filed from this thread. The block is validated with the same strict parser Home uses, the
card gets the mk-move label, and nothing in the file is executed. Adopting the same move again changes
nothing; a different move on the same card is refused.

Exit codes: 2 usage or refused, 3 Home or tasks unavailable.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			thread := os.Getenv("BB_THREAD_ID")
			if thread == "" {
				return &usageError{"BB_THREAD_ID is required: run inside the asking bb thread"}
			}
			if card == "" || moveFile == "" {
				return &usageError{"--card and --move are required"}
			}
			mv, err := readMoveFile(moveFile)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := newCardFiler().AdoptMove(ctx, card, thread, mv)
			if err != nil {
				return err
			}
			out, _ := json.Marshal(res)
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&card, "card", "", "Tasks card id")
	cmd.Flags().StringVar(&moveFile, "move", "", "home-move/v1 JSON file")
	return cmd
}

func needsMkFileCmd() *cobra.Command {
	var project, title, askFile, rootRun, request, moveFile string
	var blocks []string
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File a needs-mk card from the asking thread; prints the card as one JSON line",
		Long: `Run inside the asking thread ($BB_THREAD_ID is required): the filer posts the
routing comment from it. --project is the tasks project. --ask-file is a JSON object with
question, options and optionally subject, recommendation, ask_key, project and project_root.
--move is a home-move/v1 JSON file (script, pr, read or context): the card carries it as a home-move block
and gets the mk-move label. With --move, --ask-file may be left out and the card carries a minimal
ruling-only ask. Nothing in the move is executed; Home derives commands from its validated fields.

Exit codes: 2 usage or refused, 3 Home or tasks unavailable (nothing created), 4 card created
but the routing comment failed (re-run the same command), 5 already ruled (mk picked an option for this request).`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			thread := os.Getenv("BB_THREAD_ID")
			if thread == "" {
				return &usageError{"BB_THREAD_ID is required: run inside the asking bb thread"}
			}
			if project == "" || title == "" || (askFile == "" && moveFile == "") {
				return &usageError{"--project, --title and --ask-file are required (--ask-file may be left out with --move)"}
			}
			var move map[string]any
			if moveFile != "" {
				var err error
				if move, err = readMoveFile(moveFile); err != nil {
					return err
				}
			}
			var ask map[string]any
			if askFile == "" {
				ask = moveOnlyAsk(title)
			} else {
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
				if err := json.Unmarshal(raw, &ask); err != nil {
					return &usageError{"--ask-file is not a JSON object: " + err.Error()}
				}
				if ask == nil {
					return &usageError{"--ask-file is not a JSON object: got null"}
				}
			}
			if _, ok := ask["project_root"]; !ok {
				ask["project_root"] = gitRoot()
			}
			if _, ok := ask["project"]; !ok {
				ask["project"] = filepath.Base(fmt.Sprint(ask["project_root"]))
			}
			if err := checkFilingPreconditions(fmt.Sprint(ask["project"]), fmt.Sprint(ask["project_root"])); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			bound, warn, err := newCardFiler().ResolveAskProject(ctx, project, fmt.Sprint(ask["project"]))
			if err != nil {
				return err
			}
			if warn != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
			}
			ask["project"] = bound
			req := homeask.CardRequest{Project: project, Title: title, Blocks: blocks, Ask: ask, Move: move, Key: request, Thread: thread}
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
	cmd.Flags().StringVar(&moveFile, "move", "", "home-move/v1 JSON file: what mk owes (script, pr, read or context); adds the mk-move label")
	cmd.Flags().StringVar(&rootRun, "root-run", "", "script=<abs path>,timeout=<seconds>,set=<name>")
	cmd.Flags().StringVar(&request, "request", "", "Request key (default: derived from thread and ask)")
	return cmd
}
