package door

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The sessions axis (Gate B). Structure mirrors check.go on purpose: a pure
// resolution function tested with fixtures, a thin exec wrapper over the live
// tmux server, and an explicit could-not-look state. A missing tmux binary is
// UNCHECKED; a tmux server that is not running is a real zero -- no server
// means no sessions exist, which is a measurement, not a failure to measure.

// TmuxSession is one live session: its name and the current path of the
// active pane in its active window, which is where the session "is".
type TmuxSession struct {
	Name     string
	Path     string
	Activity int64  // #{session_activity}, unix seconds; newer = more recent
	Command  string // #{pane_current_command}: what the pane is running now
}

// SessionSet is the resolved sessions axis. The GATE clause lives here:
// resolution is a fraction (Resolved of Total) and every session that
// resolved to no project is named in Unresolved -- a silently dropped
// session is a failing test, not a rendering choice.
type SessionSet struct {
	Total      int
	Resolved   int
	Unresolved []string                 // session names, sorted
	ByRoot     map[string][]TmuxSession // project root -> sessions, most recently active first
	Err        error                    // why we could not look at all; the set is then meaningless
}

// Count reports the live-session count for one project root.
func (s SessionSet) Count(root string) int {
	return len(s.ByRoot[root])
}

// Target picks the session enter should switch to: the most recently active
// session resolved to the root. ok is false when the root has none.
func (s SessionSet) Target(root string) (TmuxSession, bool) {
	list := s.ByRoot[root]
	if len(list) == 0 {
		return TmuxSession{}, false
	}
	return list[0], true
}

// ResolveSessions maps each session to the project whose root contains its
// path, longest root first so a project nested under another scan root wins.
// Session paths are symlink-resolved to match DiscoverProjects' roots.
func ResolveSessions(sessions []TmuxSession, projects []Project) SessionSet {
	set := SessionSet{
		Total:  len(sessions),
		ByRoot: make(map[string][]TmuxSession),
	}
	roots := make([]string, 0, len(projects))
	for _, p := range projects {
		roots = append(roots, p.Root)
	}
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })

	for _, s := range sessions {
		path := s.Path
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
		if root == "" {
			set.Unresolved = append(set.Unresolved, s.Name)
			continue
		}
		set.Resolved++
		set.ByRoot[root] = append(set.ByRoot[root], s)
	}
	for root := range set.ByRoot {
		list := set.ByRoot[root]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Activity > list[j].Activity })
	}
	sort.Strings(set.Unresolved)
	return set
}

// listSessionsTimeout bounds one tmux query; tmux answers locally and fast,
// so a stall this long is a fact worth reporting as UNCHECKED.
const listSessionsTimeout = 5 * time.Second

// ListSessions asks the tmux server (the one the environment points at) for
// every session's name, activity, and active-pane path. A server that is not
// running returns an empty slice and no error: zero sessions is the true
// state of a stopped server. Everything else -- tmux missing, a malformed
// answer -- is an error, which callers must surface as UNCHECKED rather than
// as an empty estate.
func ListSessions(ctx context.Context) ([]TmuxSession, error) {
	ctx, cancel := context.WithTimeout(ctx, listSessionsTimeout)
	defer cancel()

	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux not on PATH: %w", err)
	}
	// One line per pane; the window_active+pane_active filter leaves exactly
	// one line per session (its focused pane), which is where the session is.
	// pane_current_command is the sixth field -- what the pane is running now
	// (the thread registry's liveness axis, WI-1/WI-4).
	out, err := exec.CommandContext(ctx, tmuxBin, "list-panes", "-a",
		"-F", "#{window_active}\x1f#{pane_active}\x1f#{session_name}\x1f#{session_activity}\x1f#{pane_current_path}\x1f#{pane_current_command}").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		// A missing socket or stopped server is a real empty inventory.
		// Access and protocol failures only tell us we could not inspect it.
		stopped := strings.HasPrefix(message, "no server running on ") ||
			(strings.HasPrefix(message, "error connecting to ") &&
				(strings.HasSuffix(message, "(No such file or directory)") || strings.HasSuffix(message, "(Connection refused)")))
		if ctx.Err() == nil && stopped {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %v: %s", err, message)
	}
	return parseSessionLines(string(out))
}

// sessionFields is how many \x1f-delimited fields ListSessions' -F format
// produces per line; a line with any other count is unparseable, not a
// partial answer.
const sessionFields = 6

// parseSessionLines is ListSessions' parser, split out so fixtures can drive
// it without a live tmux server.
func parseSessionLines(out string) ([]TmuxSession, error) {
	var sessions []TmuxSession
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) == 1 {
			// Some tmux versions escape control bytes in command output.
			// Split the observed octal separator without interpreting paths or
			// session names as a general escape language.
			parts = strings.Split(line, `\037`)
		}
		if len(parts) != sessionFields {
			return nil, fmt.Errorf("tmux list-panes: unparseable line %q", line)
		}
		if parts[0] != "1" || parts[1] != "1" {
			continue
		}
		activity, _ := strconv.ParseInt(parts[3], 10, 64)
		sessions = append(sessions, TmuxSession{Name: parts[2], Activity: activity, Path: parts[4], Command: parts[5]})
	}
	return sessions, nil
}

// SnapshotSessions is the live path: list, then resolve against the estate.
// On failure the returned set carries Err and nothing else is meaningful.
func SnapshotSessions(ctx context.Context, projects []Project) SessionSet {
	sessions, err := ListSessions(ctx)
	if err != nil {
		return SessionSet{Err: err}
	}
	return ResolveSessions(sessions, projects)
}

// sessionsLine states the sessions axis for the header, as a fraction with
// every unresolvable named (the GATE), or as UNCHECKED when we could not look.
func sessionsLine(s SessionSet) string {
	if s.Err != nil {
		return "sessions: UNCHECKED (" + s.Err.Error() + ")"
	}
	if s.Total == 0 {
		return "sessions: 0/0 — no tmux server or no sessions"
	}
	line := fmt.Sprintf("sessions: %d/%d resolved", s.Resolved, s.Total)
	if len(s.Unresolved) > 0 {
		line += " · unresolved: " + strings.Join(s.Unresolved, ", ")
	}
	return line
}
