package door

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

// The sessions axis (Gate B). Structure mirrors check.go on purpose: a pure
// resolution function tested with fixtures, a thin exec wrapper over the live
// tmux server, and an explicit could-not-look state. A missing tmux binary is
// UNCHECKED; a tmux server that is not running is a real zero -- no server
// means no sessions exist, which is a measurement, not a failure to measure.

// TmuxSession retains the existing inventory API name but represents one pane,
// including splits and inactive windows. Names are display labels only.
type TmuxSession struct {
	Target     agenttransport.Target
	WindowName string
	Dead       bool
	Name       string
	Path       string
	Activity   int64  // #{session_activity}, unix seconds; newer = more recent
	Command    string // #{pane_current_command}: what the pane is running now
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
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Activity != list[j].Activity {
				return list[i].Activity > list[j].Activity
			}
			return list[i].Key() < list[j].Key()
		})
	}
	sort.Strings(set.Unresolved)
	return set
}

// listSessionsTimeout bounds one tmux query; tmux answers locally and fast,
// so a stall this long is a fact worth reporting as UNCHECKED.
const listSessionsTimeout = 5 * time.Second

// ListSessions asks the tmux server (the one the environment points at) for
// every pane's exact identity, labels, activity, and path. A server that is not
// running returns an empty slice and no error: zero sessions is the true
// state of a stopped server. Everything else -- tmux missing, a malformed
// answer -- is an error, which callers must surface as UNCHECKED rather than
// as an empty estate.
func ListSessions(ctx context.Context) ([]TmuxSession, error) {
	panes, err := agenttransport.NewTmux(nil, "").List(ctx)
	if err != nil {
		return nil, err
	}
	return paneSessions(panes), nil
}

func paneSessions(panes []agenttransport.Pane) []TmuxSession {
	sessions := make([]TmuxSession, 0, len(panes))
	for _, p := range panes {
		sessions = append(sessions, TmuxSession{Target: p.Target, WindowName: p.WindowName, Dead: p.Dead, Name: p.SessionName, Path: p.Path, Activity: p.Activity, Command: p.Command})
	}
	return sessions
}

func parseSessionLines(out string) ([]TmuxSession, error) {
	panes, err := agenttransport.ParsePanes(out)
	if err != nil {
		return nil, err
	}
	return paneSessions(panes), nil
}

// Key is a pane identity for navigation; name-only fixtures retain their old key.
// Production listing always supplies a validated Target. Input never falls back.
func (s TmuxSession) Key() string {
	if s.Target.Socket == "" {
		return s.Name
	}
	return s.Target.Key()
}
func (s TmuxSession) Clone() TmuxSession { return s }

type PaneGroup struct {
	Root  string
	Panes []TmuxSession
}

// GroupPanes returns deterministic project groups and an explicit unassigned
// group. Shared workspace roots cannot be assigned from a session's name.
func GroupPanes(panes []TmuxSession, projects []Project, query string) []PaneGroup {
	set := ResolveSessions(panes, projects)
	roots := make([]string, 0, len(set.ByRoot))
	assigned := map[string]bool{}
	for root, list := range set.ByRoot {
		roots = append(roots, root)
		for _, p := range list {
			assigned[p.Key()] = true
		}
	}
	sort.Strings(roots)
	var groups []PaneGroup
	add := func(root string, list []TmuxSession) {
		var matched []TmuxSession
		for _, p := range list {
			if paneMatches(p, root, query) {
				matched = append(matched, p)
			}
		}
		if len(matched) > 0 {
			sort.Slice(matched, func(i, j int) bool { return matched[i].Key() < matched[j].Key() })
			groups = append(groups, PaneGroup{Root: root, Panes: matched})
		}
	}
	for _, root := range roots {
		add(root, set.ByRoot[root])
	}
	var unassigned []TmuxSession
	for _, p := range panes {
		if !assigned[p.Key()] {
			unassigned = append(unassigned, p)
		}
	}
	add("", unassigned)
	return groups
}
func paneMatches(p TmuxSession, root, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{root, p.Name, p.WindowName, p.Path, p.Command, p.Target.Key()}, " "))
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
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
