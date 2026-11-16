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
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/serve"
)

// newCardFiler builds the filer; tests replace it.
var newCardFiler = func() *homeask.CardFiler {
	return &homeask.CardFiler{Timeout: filerTimeout, ReadbackTimeout: readbackTimeout}
}

// readbackTimeout bounds how long a filing waits for Home to index the new card and report its card_key.
const readbackTimeout = 30 * time.Second

// checkFilingPreconditions runs before anything is created: serve must be up and the ask's
// project_root must be the root it resolves for the ask's project. Tests replace it.
var checkFilingPreconditions = serve.CheckFiling

func needsMkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "needs-mk",
		Short:         "File asks with mk as tasks cards",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(needsMkFileCmd())
	cmd.AddCommand(needsMkReleaseCmd())
	cmd.AddCommand(needsMkCancelHeldCmd())
	cmd.AddCommand(needsMkDismissOwnerNoticeCmd())
	cmd.AddCommand(needsMkBudgetCmd())
	cmd.AddCommand(needsMkAdoptMoveCmd())
	cmd.AddCommand(needsMkFooterCmd())
	cmd.AddCommand(needsMkExportCmd())
	cmd.AddCommand(needsMkPilotsCmd())
	cmd.AddCommand(needsMkRecheckLinksCmd())
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

// readMoveFile reads a home-move/v1 JSON file (at most 1 MiB) and validates it with the strict parser the
// plugin uses, so a move Home would show as display-only is refused here. Nothing in it is executed.
func readMoveFile(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &usageError{"--move: " + err.Error()}
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxCardFileBytes+1))
	f.Close()
	if err != nil {
		return nil, &usageError{"--move: " + err.Error()}
	}
	if len(raw) > maxCardFileBytes {
		return nil, &usageError{"--move is over 1 MiB"}
	}
	mv, err := homeask.ParseMove(string(raw))
	if err != nil {
		return nil, &usageError{"--move is not a valid home-move/v1: " + err.Error()}
	}
	return mv.Raw, nil
}

// maxCardFileBytes bounds a card JSON file: one 8000-character field of 4-byte text is about 32 KB, and an ask
// holds many such fields. maxCardText is the card text limit the Go validator enforces.
const (
	maxCardFileBytes = 1 << 20
	maxCardText      = 8000
)

// moveOnlyAsk is the ask a move card carries when the filer gave none. Home opens no moves (bead mk-1k8pg):
// the card is a plain decision, so its question says what mk owes and Done records that mk did it.
func moveOnlyAsk(title string, move map[string]any) map[string]any {
	q := strings.TrimSpace(title)
	if d := moveDetail(move); d != "" {
		q += "\n\n" + d
	}
	if r := []rune(q); len(r) > maxCardText {
		q = string(r[:maxCardText])
	}
	return map[string]any{
		"question": q,
		"options": []any{
			map[string]any{"id": "done", "label": "Done", "kind": "ruling-only"},
			map[string]any{"id": "not-now", "label": "Not now", "kind": "ruling-only"},
		},
	}
}

// moveDetail is what a validated move asks of mk, in words: the command to run, the URL to open or the
// context to give. The fields passed ParseMove, so a script path and its args hold no shell metacharacters.
func moveDetail(move map[string]any) string {
	kind, _ := move["kind"].(string)
	o, _ := move[kind].(map[string]any)
	switch kind {
	case "script":
		cmd := "bash " + fmt.Sprint(o["path"])
		if args, ok := o["args"].([]any); ok {
			for _, a := range args {
				cmd += " " + fmt.Sprint(a)
			}
		}
		return "Run: " + cmd + "\nsha256: " + fmt.Sprint(o["sha256"])
	case "pr", "read":
		return fmt.Sprint(o["url"])
	case "context":
		return fmt.Sprint(o["need"])
	}
	return ""
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

var (
	// scp, ssh or bash followed by a word that is not common English ("bash is fine", "the bash scripts") reads as a command.
	mentionsCommand = regexp.MustCompile(`(?i)\b(?:scp|ssh|bash)\s+`)
	proseAfter      = regexp.MustCompile(`(?i)^(?:the|a|an|and|or|to|in|for|on|of|as|is|are|was|be|can|will|should|must|does|not|it|this|that|with|vs|scripts?|shell|keys?|access|commands?|sessions?|login|vs\.?)\b`)
	ownLineCommand  = regexp.MustCompile(`(?m)^\s*(?:\$\s+|\(?\d{1,3}[.)]\s+)?(?:bash|sudo|runuser|scp|ssh|sh|python3)\s+\S`)
)

// commandWarning is non-empty when the question mentions scp, ssh or bash with an argument but no command stands on
// its own line. Home puts a copy button on a command that has its own line; one buried in a sentence may get none.
func commandWarning(question string) string {
	if ownLineCommand.MatchString(question) {
		return ""
	}
	found := false
	for _, m := range mentionsCommand.FindAllStringSubmatchIndex(question, -1) {
		if rest := strings.TrimLeft(question[m[1]:], " \t"); rest != "" && !proseAfter.MatchString(rest) {
			found = true
		}
	}
	if !found {
		return ""
	}
	return "the question mentions scp, ssh or bash but no command stands on its own line, so Home may show no copy button for it; put each command on its own line"
}

func needsMkFileCmd() *cobra.Command {
	var project, title, askFile, rootRun, request, moveFile, ownerThread, urgency, stopped, prFlag, supersedes string
	var blocks []string
	var vizierOwned, checkOnly bool
	var cardType string
	var ui, urgent, nonBlocking bool
	var keepList string
	var attachments []string
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File a needs-mk card from the asking thread; prints the card as one JSON line",
		Long: `Run inside the asking thread ($BB_THREAD_ID is required): the filer posts the
routing comment from it. AUTARCH_CARD_WEEKLY_BUDGET sets the weekly budget (default 50).
The budget week starts Thursday 12:00 America/Los_Angeles; needs-mk budget shows
the current Pacific start and end. Legacy ISO reservations still count conservatively
in overlapping weeks when their filing time is unknown.
AUTARCH_CARD_STATE_DIR overrides the local XDG state directory.
--non-blocking queues a card until needs-mk release, reserving its slot now.
Ordinary cards filed Monday 00:00 through Thursday before 12:00 Pacific are also
held for the Thursday batch. Urgent, keep-list, merge and root-run cards bypass
this weekday hold. Release inside that window leaves ordinary cards queued;
schedule release at Thursday noon or later. Counted slots stay in their filing week.
--urgent or --keep-list marks a budget exception. Exit 0 means filed: JSON has a non-empty card_key.
Held cards exit 6, with queued:true, the request key and no card_key on stdout
(--json is also accepted). Treat exit 6 as queued, not as a filed card or an uncertain write.
--urgent exempts the budget; --urgency blocker only controls phone push and does not exempt the budget.
Exempt cards are recorded separately and never consume ordinary slots; needs-mk budget shows both tallies.
After filing, waits up to 30 seconds for Home's index. If the key remains missing, exits 4
with no success JSON: the card may have been filed. The error gives the request key;
re-run with that same --request and unchanged input to replay without duplicating it.
Readback clears ambient project context and checks the card's Tasks project in Go,
independent of the working directory; it also works with older Home plugins.
If Tasks returns no project id, readback does not filter by project.
--project is the tasks project. --ask-file is a JSON object with
question, options and optionally subject, recommendation, ask_key, project and project_root.
Optional evidence: at most 6 items, each with caption (1-200 characters) and one HTTPS url,
image reference {sha256,mime}, or before/after pair of those sources. URLs are at most 2048
characters, without credentials or controls.
URLs use lowercase ASCII LDH hosts or canonical dotted IPv4 and optional decimal port 1-65535;
userinfo, IPv6, xn-- labels and non-ASCII are refused. Paths/query/fragments use ASCII URI
characters and checked %HH escapes. Empty or relative AUTARCH_HOME_EVIDENCE selects the default.
At filing time, use image_path instead of image to copy a regular local PNG/JPEG/GIF/WebP
(not symbolic links, devices or FIFOs; magic checked, 2 MB each, 8 MB per card) into the private
store (AUTARCH_HOME_EVIDENCE, default ~/.autarch/home-evidence). Only hash and MIME are stored.
Evidence is excluded from hashes; changing only evidence is a no-op.
--move is a home-move/v1 JSON file (script, pr, read or context): the card carries it as a home-move block
and gets the mk-move label. With --move, --ask-file may be left out and the card carries a Done / Not now
ask whose question says what the move asks of mk. Nothing in the move is executed; Home derives commands from its validated fields.

--owner-thread names the thread that answers the card: the routing comment is posted as that thread, so
Home routes mk's pick to it, not to the filer. It must be a live, unarchived thread, else the filing is
refused (exit 2) and nothing is created. A later claim comment never re-routes a card.
The vizier must give exactly one of --owner-thread (another thread owns follow-through) or
--vizier-owned (its own ruling, root run or scope). Both flags together, or --vizier-owned from
any other thread, are refused with exit 2 before filing. Other threads need neither flag.

Re-filing the same ask_key updates its open, unanswered card in this project, preserving its key and routing.
--supersedes CARD also updates in place when ask_key changed; a closed, foreign-project or foreign-owner
card is refused (exit 2). An answered card remains already ruled (exit 5).
A picked closed card with the same ask_key exits 5 too; a new ask needs a new ask_key.
For closed or reopened cards that Home no longer reports as picked, the same decision
at the same pinned head is refused (exit 5) for 24 hours after its last pick. Change the
ask or pinned head to re-file during that window. An unreadable pick time also refuses
the unchanged decision. Cards Home reports as picked remain refused regardless of age
or changes to the ask, including with --supersedes.

Exit codes: 2 usage or refused, 3 Home or tasks unavailable (nothing created or edited;
includes an update's pre-edit readback with no indexed project_id), 4 card created
but the routing comment failed (re-run the same command), 5 already ruled (mk picked an option for this request),
6 accepted but held (no card filed; JSON on stdout carries queued:true and the request key).

The optional ask JSON group {key, order} links cards for display. Set it at first filing:
a group-only re-file does not change Home's stored group. A substantive edit can create a new
generation carrying a different group. Request keys and group keys are global, not project-scoped;
use specific keys.

Cards whose options are all notice acknowledgments (read, ok, close and similar labels) are
refused with exit 2: notices belong in the vizier inbox or digest. --move and --root-run are exempt.

--urgency blocker (or "urgency": "blocker" in the ask JSON) asks Home to send mk a phone push once, when the
card is first filed. It requires --stopped (or "stopped" in JSON): a non-blank single line of at most
200 characters saying what work is stopped until mk answers. Omit blocker urgency for no phone push.
A blocker is still held Monday through Thursday morning unless --urgent is passed.
Any other urgency is refused. It works on every kind of card; a machine blocker alone never pushes.`,
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
			if cmd.Flags().Changed("request") {
				if err := homeask.CheckRequestKey(request); err != nil {
					return &usageError{"--request: " + err.Error()}
				}
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
				ask = moveOnlyAsk(title, move)
			} else {
				f, err := os.Open(askFile)
				if err != nil {
					return &usageError{"--ask-file: " + err.Error()}
				}
				raw, err := io.ReadAll(io.LimitReader(f, maxCardFileBytes+1))
				f.Close()
				if err != nil {
					return &usageError{"--ask-file: " + err.Error()}
				}
				if len(raw) > maxCardFileBytes {
					return &usageError{"--ask-file is over 1 MiB"}
				}
				if err := homeask.ValidateOnPickJSON(raw); err != nil {
					return &usageError{err.Error()}
				}
				if err := json.Unmarshal(raw, &ask); err != nil {
					return &usageError{"--ask-file is not a JSON object: " + err.Error()}
				}
				if ask == nil {
					return &usageError{"--ask-file is not a JSON object: got null"}
				}
			}
			if err := homeask.PrepareEvidence(ask); err != nil {
				return &usageError{err.Error()}
			}
			if cmd.Flags().Changed("urgency") {
				if urgency != "blocker" {
					return &usageError{`--urgency: the only value is "blocker"`}
				}
				ask["urgency"] = urgency
			} else if u, ok := ask["urgency"]; ok && u != "blocker" {
				return &usageError{`"urgency" in the ask must be "blocker" when set`}
			}
			if cmd.Flags().Changed("stopped") {
				ask["stopped"] = stopped
			}
			if err := homeask.CheckStopped(ask); err != nil {
				return &usageError{err.Error()}
			}
			if _, ok := ask["project_root"]; !ok {
				ask["project_root"] = gitRoot()
			}
			if move == nil && rootRun == "" && noticeOnlyOptions(ask) {
				return &usageError{"card has no decision: a notice belongs in the vizier's inbox or digest"}
			}
			if w := commandWarning(fmt.Sprint(ask["question"])); w != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			filer := newCardFiler()
			filer.Warn = func(w string) { fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w) }
			if _, ok := ask["project"]; !ok {
				// The board's binding names the project; the folder name is wrong in a bb worktree.
				ask["project"] = filer.FolderOrBound(ctx, project, fmt.Sprint(ask["project_root"]))
			}
			// Resolve the bound Home project first: serve lists projects by that name, so the root check
			// must see it, not the tasks-key alias the ask may carry.
			filer.Check = checkFilingPreconditions
			var failures []error
			bound, warn, prepErr := filer.PrepareAsk(ctx, project, fmt.Sprint(ask["project"]), fmt.Sprint(ask["project_root"]))
			if warn != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
			}
			if prepErr == nil {
				ask["project"] = bound
			}
			mergeErr := guardMergeBase(cmd, ask, move, append([]string{title, rootRun, ownerThread, request}, blocks...), prFlag, cmd.Flags().Changed("pr"))
			if mergeErr != nil {
				failures = append(failures, mergeErr)
			}
			mergeAsk := prFlag != "" || homeask.IsMergeAsk(ask)
			if mergeAsk {
				if cmd.Flags().Changed("card-type") && cardType != "merge" {
					failures = append(failures, &usageError{"--card-type conflicts with merge ask; use --card-type merge or omit it"})
				} else if !cmd.Flags().Changed("card-type") {
					cardType = "merge"
				}
			}
			if err := filer.CheckVizierOwnership(ctx, thread, ownerThread, vizierOwned); err != nil {
				failures = append(failures, err)
			}
			if vizierOwned && cmd.Flags().Changed("owner-thread") {
				failures = append(failures, &usageError{"give exactly one of --owner-thread or --vizier-owned"})
			}
			req := homeask.CardRequest{Project: project, Title: title, Blocks: blocks, Ask: ask, Move: move, Key: request, Thread: thread, Owner: ownerThread, VizierOwned: vizierOwned, Type: cardType, UI: ui, Attachments: attachments, Urgent: urgent, NonBlocking: nonBlocking, KeepList: keepList}
			var err error
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
			if err := filer.Preflight(ctx, req); err != nil {
				failures = append(failures, err)
			}
			if err := errors.Join(failures...); err != nil {
				return err
			}
			if checkOnly {
				fmt.Fprintln(cmd.OutOrStdout(), `{"checked":true}`)
				return nil
			}
			if filer.ReadbackTimeout <= 0 {
				filer.ReadbackTimeout = readbackTimeout
			}
			res, err := filer.FileOrUpdateCard(ctx, req, supersedes)
			if err != nil {
				// needs-mk silences Cobra errors, so print recovery details explicitly.
				if res.ID != "" && res.CardKey == "" {
					fmt.Fprintln(cmd.ErrOrStderr(), err)
				}
				return err
			}
			if !res.Queued && strings.TrimSpace(res.CardKey) == "" {
				err := fmt.Errorf("%w: card may have been filed; card_key is missing; re-run with --request %s", homeask.ErrOutcomeUnknown, res.Key)
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return err
			}
			if budget, e := filer.BudgetStatus(fmt.Sprint(ask["project"])); e == nil {
				res.Budget = &budget
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(res); err != nil {
				return err
			}
			if res.Queued {
				return &homeask.HeldError{RequestKey: res.Key, Reason: res.HoldReason}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&urgent, "urgent", false, "Exempt this urgent card from the weekly budget")
	cmd.Flags().BoolVar(&nonBlocking, "non-blocking", false, "Hold for needs-mk release; reserves a weekly budget slot now")
	cmd.Flags().StringVar(&keepList, "keep-list", "", "Budget exception: spend, credentials, sign-ins/payments, tags/releases/publishes, force-pushes, standing-approvals")
	cmd.Flags().BoolVar(&ui, "ui", false, "Mark a UI card; play and merge require a screenshot or video")
	cmd.Flags().StringArrayVar(&attachments, "attach", nil, "Non-symlink, non-empty png/jpg/jpeg/webp/gif/mp4/webm media file; repeatable")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Run all preflight checks without submitting")
	cmd.Flags().StringVar(&cardType, "card-type", "other", "Card type: product, design, play, merge or other (merge inferred only when omitted; conflicting types refused)")
	cmd.Flags().Bool("json", false, "Print JSON (the default and only format); success always includes a non-empty card_key")
	cmd.Flags().StringVar(&project, "project", "", "Tasks project (id, key or name)")
	cmd.Flags().StringVar(&title, "title", "", "Card title")
	cmd.Flags().StringArrayVar(&blocks, "blocks", nil, "Blocks ref (bead:ID, thread:thr_ID, project:ID); repeatable")
	cmd.Flags().StringVar(&askFile, "ask-file", "", "JSON file with the ask")
	cmd.Flags().StringVar(&moveFile, "move", "", "home-move/v1 JSON file: what mk owes (script, pr, read or context); adds the mk-move label")
	cmd.Flags().StringVar(&rootRun, "root-run", "", "script=<abs path>,timeout=<seconds>,set=<name>")
	cmd.Flags().StringVar(&request, "request", "", "Request key (default: derived from thread and ask); when given it must be non-empty and match the Request line key pattern")
	cmd.Flags().StringVar(&stopped, "stopped", "", "Work stopped until the answer (required with blocker urgency, single line, at most 200 characters)")
	cmd.Flags().StringVar(&urgency, "urgency", "", `Optional: "blocker" requires --stopped and asks Home to push once; still held Monday through Thursday morning unless --urgent is passed; omit urgency for no phone push`)
	cmd.Flags().StringVar(&prFlag, "pr", "", "PR URL this ask merges: refused (exit 2) unless the PR's base is the repo's default branch or --into explicitly opts in to its non-default base; a \"Manually merge it at ...\" option implies it")
	cmd.Flags().StringVar(&supersedes, "supersedes", "", "Update this open, unanswered card in the same project; it must belong to this thread or --owner-thread")
	cmd.Flags().BoolVar(&vizierOwned, "vizier-owned", false, "Vizier only: this is its own ruling, root run or scope card; mutually exclusive with --owner-thread")
	cmd.Flags().String("into", "", "Explicitly accept the PR's base branch: must match exactly; required for a non-default base")
	cmd.Flags().StringVar(&ownerThread, "owner-thread", "", "Route the card's answer to this live thread instead of the filing thread (the routing comment is posted as it)")
	return cmd
}

// noticeOnlyOptions checks labels rather than ids; punctuation and case do not
// turn an acknowledgment into a decision. Malformed or empty options are left
// to the existing schema validator.
func noticeOnlyOptions(ask map[string]any) bool {
	opts, _ := ask["options"].([]any)
	if len(opts) == 0 {
		return false
	}
	for _, o := range opts {
		m, _ := o.(map[string]any)
		label, _ := m["label"].(string)
		label = regexp.MustCompile(`^\s*[A-Za-z]\)\s*`).ReplaceAllString(label, "")
		label = strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) || unicode.IsSpace(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, label)
		switch label {
		case "read", "ok", "okay", "close", "closed", "gotit", "noted", "acknowledge", "acknowledged", "seen", "dismiss":
		default:
			return false
		}
	}
	return true
}

func needsMkReleaseCmd() *cobra.Command {
	return &cobra.Command{Use: "release", Short: "Send held cards grouped by type; ordinary cards wait until Thursday noon Pacific", SilenceUsage: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		f := newCardFiler()
		f.Check = checkFilingPreconditions
		report, err := f.ReleaseDetailed(cmd.Context())
		var entries *homeask.ReleaseEntryErrors
		if errors.As(err, &entries) && entries.DataOnly() {
			err = nil
		}
		printErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		return errors.Join(err, printErr)
	}}
}
func needsMkCancelHeldCmd() *cobra.Command {
	return &cobra.Command{Use: "cancel-held REQUEST", Short: "Cancel an unsent card and refund its budget slot", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return newCardFiler().CancelHeld(args[0]) }}
}

func needsMkBudgetCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{Use: "budget", Short: "Show the Pacific Thursday-noon week bounds and ordinary/exempt card counts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if project == "" {
			return &usageError{"--project must name the Home project"}
		}
		status, err := newCardFiler().BudgetStatus(project)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
	}}
	cmd.Flags().StringVar(&project, "project", "", "Home project name (case-insensitive)")
	return cmd
}
