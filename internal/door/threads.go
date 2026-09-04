package door

import (
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The thread registry (autarch-01 steps 2/3, plan 2026-09-02). tmux session
// names are mk's registry, kept by hand a second time in an Apple Note; this
// file transcribes the live one -- terminal, window mark, topic, resume id --
// and reads what each pane is actually running, without scraping pane
// content (that stays the next slice). Same discipline as sessions.go and
// briefing.go: pure parsing tested on fixtures, an explicit could-not-read
// state per thread, nothing persisted.

// Mark encodes mk's window-position axis (GATE ruling 2, 2026-09-02): three
// overlapping three-quarter-width positions on one display -- not literal
// halves -- as mk stated it: "[ left 3/4s ... ] right 3/4s ... [] center
// 3/4s ... so i can get more horizontal space to review the terminal window".
type Mark string

const (
	MarkNone   Mark = ""
	MarkLeft   Mark = "["  // left three-quarters of the display
	MarkRight  Mark = "]"  // right three-quarters
	MarkCenter Mark = "[]" // center three-quarters
)

// Seat is what a tmux session name says about itself in mk's registry form:
//
//	<terminal><mark><topic> - <resume id>     e.g. "iterm]solwend - 21cc6bd2-…"
//
// A name in no such form is a seat with Topic = name and nothing else.
type Seat struct {
	Terminal string // "wezterm" | "iterm" | "rio" | ""
	Mark     Mark
	Topic    string // trimmed; "ushas/bridger", "jeddnet@codex"
	ResumeID string // "" when the name carries none
}

// seatPattern is mk's registry line shape, probed verbatim against the note
// (docs/research/2026-09-02-thread-registry-probe.md): an optional emulator,
// one of the three marks, the topic, and an optional " - <resume id>"
// suffix. \s+ around the dash tolerates the note's double spaces
// (`jeddnet -  f44fd423…`).
var seatPattern = regexp.MustCompile(`^(wezterm|iterm|rio)?(\[\]|\[|\])\s*(.+?)(?:\s+-\s+(\S+))?\s*$`)

// ParseSeat reads one tmux session name as mk's registry form. A name
// carrying no mark (a bare topic like "grey-area" or "28") is a seat with
// only a Topic -- the note's own drift shows tmux sessions losing their
// registry shape over time.
func ParseSeat(name string) Seat {
	m := seatPattern.FindStringSubmatch(name)
	if m == nil {
		return Seat{Topic: name}
	}
	return Seat{
		Terminal: m[1],
		Mark:     Mark(m[2]),
		Topic:    strings.TrimSpace(m[3]),
		ResumeID: m[4],
	}
}

// Runtime is what the pane is running, from #{pane_current_command}.
type Runtime string

const (
	RuntimeClaude Runtime = "claude" // command matches ^\d+\.\d+\.\d+$ (the version binary)
	RuntimeCodex  Runtime = "codex"
	RuntimeKimi   Runtime = "kimi"
	RuntimeShell  Runtime = "shell" // zsh bash fish sh: an idle seat
	RuntimeOther  Runtime = "other"
)

var claudeVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var shellCommands = map[string]bool{"zsh": true, "bash": true, "fish": true, "sh": true}

// ClassifyPane reads #{pane_current_command} into a Runtime. version is set
// only for RuntimeClaude: Claude Code's process is named by its version, and
// with auto-update unpinned on this machine that version is a free proxy for
// how long the thread has stood (probe finding: 2.1.221 on the oldest
// threads, 2.1.258 on the newest).
func ClassifyPane(cmd string) (Runtime, string) {
	switch {
	case claudeVersionPattern.MatchString(cmd):
		return RuntimeClaude, cmd
	case cmd == "claude":
		return RuntimeClaude, ""
	case cmd == "codex":
		return RuntimeCodex, ""
	case cmd == "kimi":
		return RuntimeKimi, ""
	case shellCommands[cmd]:
		return RuntimeShell, ""
	default:
		return RuntimeOther, ""
	}
}

// Thread is one live tmux session, seated and classified, with (for a claude
// thread carrying a resume id) its transcript's last real turn and the
// gardens it mentions.
type Thread struct {
	Session  string // tmux name, verbatim
	Seat     Seat
	Runtime  Runtime
	Version  string
	Activity int64

	Path            string    // pane path (kept for ResolveSessions parity)
	Transcript      string    // "" when none
	LastTurn        time.Time // zero = unknown
	Gardens         []GardenHit
	Err             error        // transcript could not be read; the row says so
	Conversation    Conversation // historical evidence, independent of runtime
	QuestionVisible bool         // question text also appears in the current pane snapshot
	PaneErr         error
}

// ThreadSet is the assembled registry: every live tmux session as a Thread,
// attributed to gardens where the evidence supports it.
type ThreadSet struct {
	Threads []Thread            // stable order: Activity desc, then Session
	ByRoot  map[string][]Thread // attributed threads per garden root, most recent first
	Pending int                 // transcripts still being read
	Err     error               // tmux could not be listed: everything else is meaningless
}

// ThreadsMinShare is the attribution floor (auto_proceed ledger entry): a
// garden earns credit at 20% of a thread's mentions, over the trailing scan
// window; the top garden always earns credit regardless of share. Exported so
// the threads subcommand attributes exactly as the screen does.
const ThreadsMinShare = 0.2

// Attribute credits a thread to a garden when the pane path resolves there
// (first pass, as ResolveSessions) or, for a thread at no garden (launched at
// the estate root), to every garden holding at least minShare of its
// mentions and always to its top garden. A thread with no transcript and no
// resolvable path is attributed to nothing.
func Attribute(threads []Thread, projects []Project, minShare float64) map[string][]Thread {
	byRoot := make(map[string][]Thread)

	roots := make([]string, 0, len(projects))
	for _, p := range projects {
		roots = append(roots, p.Root)
	}
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })

	for _, th := range threads {
		path := th.Path
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		root := ""
		for _, r := range roots {
			if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
				root = r
				break
			}
		}
		if root != "" {
			byRoot[root] = append(byRoot[root], th)
			continue
		}
		if len(th.Gardens) == 0 {
			continue
		}
		total := 0
		for _, g := range th.Gardens {
			total += g.Mentions
		}
		if total == 0 {
			continue
		}
		// th.Gardens is already sorted Mentions desc (Gardens' own contract),
		// so index 0 is always the top garden.
		for i, g := range th.Gardens {
			if i == 0 || float64(g.Mentions)/float64(total) >= minShare {
				byRoot[g.Root] = append(byRoot[g.Root], th)
			}
		}
	}
	for root := range byRoot {
		list := byRoot[root]
		sort.SliceStable(list, func(i, j int) bool {
			if !list[i].LastTurn.Equal(list[j].LastTurn) {
				return list[i].LastTurn.After(list[j].LastTurn)
			}
			return list[i].Activity > list[j].Activity
		})
	}
	return byRoot
}

// Sessions builds the SessionSet the rows already read (Count, Target) from
// an attributed ThreadSet, so garden rows and enter share the same
// attribution as the threads screen. The enter target is the thread with the
// newest LastTurn (falling back to Activity when LastTurn is unknown or
// tied), which is what ByRoot's own ordering already produces.
func (ts ThreadSet) Sessions() SessionSet {
	set := SessionSet{
		Total:  len(ts.Threads),
		ByRoot: make(map[string][]TmuxSession),
	}
	attributed := make(map[string]bool)
	for root, threads := range ts.ByRoot {
		for _, th := range threads {
			set.ByRoot[root] = append(set.ByRoot[root], TmuxSession{
				Name: th.Session, Path: th.Path, Activity: th.Activity, Command: "",
			})
			attributed[th.Session] = true
		}
	}
	for _, th := range ts.Threads {
		if attributed[th.Session] {
			set.Resolved++
		} else {
			set.Unresolved = append(set.Unresolved, th.Session)
		}
	}
	sort.Strings(set.Unresolved)
	return set
}

// hasResumeID reports whether a seat carries a resume id worth looking up --
// a bare shell or a topic with no id in the note has none.
func hasResumeID(s Seat) bool { return s.ResumeID != "" }

// readThreadsWorkers bounds the transcript-reading fan-out, mirroring
// Movements/CheckAll.
const readThreadsWorkers = 8

// ReadThreads seats and classifies every session, then for each claude
// thread carrying a resume id resolves its transcript, last turn, and garden
// mentions with bounded parallelism (8 workers, as Movements), streaming
// each finished thread to onThread from a worker goroutine. Non-claude
// threads and claude threads with no resume id are emitted immediately with
// no transcript lookup, never dropped. roots is accepted per the plan's
// signature for the estate's scan roots (known to the caller); Gardens
// derives its own estate-root set from projects (see topLevelGardenRoots),
// so roots is not consumed here -- logged in the plan's Question ledger.
func ReadThreads(ctx context.Context, sessions []TmuxSession, projects []Project, roots []string, transcriptsRoot string, onThread func(Thread)) {
	ReadThreadsWithCodex(ctx, sessions, projects, roots, transcriptsRoot, "", onThread)
}

func ReadThreadsWithCodex(ctx context.Context, sessions []TmuxSession, projects []Project, roots []string, transcriptsRoot, codexRoot string, onThread func(Thread)) {
	_ = ctx
	_ = roots
	sem := make(chan struct{}, readThreadsWorkers)
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func(s TmuxSession) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			seat := ParseSeat(s.Name)
			rt, version := ClassifyPane(s.Command)
			th := Thread{
				Session: s.Name, Seat: seat, Runtime: rt, Version: version,
				Activity: s.Activity, Path: s.Path,
			}
			if !hasResumeID(seat) {
				onThread(th)
				return
			}
			path, provider, err := FindConversation(transcriptsRoot, codexRoot, seat.ResumeID)
			if err != nil {
				th.Err = err
				onThread(th)
				return
			}
			th.Transcript = path
			conversation, err := ReadConversation(path, provider)
			if err != nil {
				th.Err = err
				onThread(th)
				return
			}
			th.Conversation = conversation
			th.LastTurn = conversation.Updated
			gardens, err := Gardens(path, projects, gardensScanBytes)
			if err != nil {
				th.Err = err
				onThread(th)
				return
			}
			th.Gardens = gardens
			onThread(th)
		}(sessions[i])
	}
	wg.Wait()
}
