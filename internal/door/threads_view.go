package door

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The threads screen (plan WI-5; autarch-01 steps 2 and 3; GATE ruling 1 of
// 2026-09-02: threads get their own screen, the row stays the garden). One
// line per live tmux seat, most recently active first: what the pane runs,
// the last real turn, and the gardens the transcript touched. No pane content
// is read here; that is the next slice.

// ThreadsOptions turns the threads axis on (WithThreads). Roots are the estate
// scan roots; TranscriptsRoot is Claude Code's per-project transcript
// directory; RegistryPath, when set, is a note in mk's registry format to diff
// against the live seats.
type ThreadsOptions struct {
	Roots           []string
	TranscriptsRoot string
	CodexRoot       string
	ReadPane        func(Thread) (string, error)
	RegistryPath    string
}

// idleAfter is when a running thread reads as idle: three days without a real
// turn (auto_proceed ledger entry; the four idle threads on 2026-09-02 were
// 6-19 days, every other thread under one).
const idleAfter = 72 * time.Hour

// threadStateWidth fits the longest states, "could not read" and
// "2.1.258 no id".
const threadStateWidth = 14

// maxDriftLines caps the registry block so a long note cannot push the seats
// off the screen; the remainder is counted, not hidden.
const maxDriftLines = 8

// SortThreads orders threads most recently active first, then by session
// name, so the screen and the subcommand agree and renders are stable.
func SortThreads(ts []Thread) {
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].Activity != ts[j].Activity {
			return ts[i].Activity > ts[j].Activity
		}
		return ts[i].Session < ts[j].Session
	})
}

// threadGlyph marks the runtime: claude ◆, codex and kimi ◇, an idle shell ○,
// anything else ·.
func threadGlyph(r Runtime) string {
	switch r {
	case RuntimeClaude:
		return "◆"
	case RuntimeCodex, RuntimeKimi:
		return "◇"
	case RuntimeShell:
		return "○"
	default:
		return "·"
	}
}

// threadState is the state column and its style. The states are distinct on
// purpose (plan acceptance): running with version and recency, idle for days,
// idle shell, no id, no transcript, could not read, or a non-claude runtime.
func threadState(th Thread, now time.Time) (string, lipgloss.Style) {
	switch {
	case th.Runtime == RuntimeShell:
		return "idle shell", styleReason
	case th.Runtime != RuntimeClaude:
		return string(th.Runtime), styleReason
	case th.Seat.ResumeID == "":
		return th.Version + " no id", styleReason
	case th.Err != nil && th.Transcript == "":
		return "no transcript", styleErr
	case th.Err != nil:
		return "could not read", styleErr
	case th.LastTurn.IsZero():
		return th.Version + " no turn", styleReason
	case now.Sub(th.LastTurn) >= idleAfter:
		return "idle " + humanDur(now.Sub(th.LastTurn)), styleFunded
	default:
		return th.Version + " " + humanDur(now.Sub(th.LastTurn)), styleSessions
	}
}

// threadColumns is one seat's row as plain text, shared by the screen (which
// styles each column) and the subcommand (which prints it as is).
type threadColumns struct {
	glyph, terminal, mark, topic, id, state, gardens string
	style                                            lipgloss.Style
}

func columnsFor(th Thread, now time.Time) threadColumns {
	c := threadColumns{
		glyph:    threadGlyph(th.Runtime),
		terminal: pad(th.Seat.Terminal, 7),
		mark:     pad(string(th.Seat.Mark), 2),
		topic:    pad(th.Seat.Topic, 24),
		id:       pad(shortID(th.Seat.ResumeID), 8),
	}
	c.state, c.style = threadState(th, now)
	names := make([]string, 0, len(th.Gardens))
	for _, g := range th.Gardens {
		names = append(names, g.Name)
	}
	c.gardens = strings.Join(names, ", ")
	return c
}

// threadFixedWidth is everything left of the gardens column: selector 2,
// glyph 1+1, terminal 7+1, mark 2+1, topic 24+1, id 8+1, state 14+1.
const threadFixedWidth = 2 + 2 + 8 + 3 + 25 + 9 + threadStateWidth + 1

// ThreadLine is one thread as plain text in the screen's column order, for
// the threads subcommand and for tests; width bounds the gardens column.
func ThreadLine(th Thread, now time.Time, width int) string {
	c := columnsFor(th, now)
	rest := width - threadFixedWidth
	if rest < 8 {
		rest = 8
	}
	return c.glyph + " " + c.terminal + " " + c.mark + " " + c.topic + " " + c.id + " " +
		pad(c.state, threadStateWidth) + " " + truncate(c.gardens, rest)
}

func (m Model) renderThread(th Thread, selected bool, now time.Time) string {
	c := columnsFor(th, now)
	rest := m.lineWidth() - threadFixedWidth
	if rest < 8 {
		rest = 8
	}
	line := c.style.Render(c.glyph) + " " + c.terminal + " " + c.mark + " " + c.topic + " " +
		styleReason.Render(c.id) + " " +
		c.style.Render(pad(c.state, threadStateWidth)) + " " +
		styleSessions.Render(truncate(c.gardens, rest))
	if selected {
		return styleSelected.Render("> ") + line
	}
	return "  " + line
}

// ThreadsHeader states the seats as counts: how many, what runs, how many sit
// at a shell, how many names carry no mark. pending > 0 appends the read
// still in progress so a partial list is never presented as the whole estate.
func ThreadsHeader(ts ThreadSet, pending int) string {
	var claude, codex, other, shells, unmarked int
	for _, th := range ts.Threads {
		switch th.Runtime {
		case RuntimeClaude:
			claude++
		case RuntimeCodex:
			codex++
		case RuntimeShell:
			shells++
		default:
			other++
		}
		if th.Seat.Mark == MarkNone {
			unmarked++
		}
	}
	running := claude + codex + other
	s := fmt.Sprintf("threads: %d seats · %d running (claude %d · codex %d · other %d) · %d idle shells · %d unmarked",
		len(ts.Threads), running, claude, codex, other, shells, unmarked)
	if pending > 0 {
		s += fmt.Sprintf(" · reading %d…", pending)
	}
	return s
}

// registryLines is the drift block: absent without --registry, otherwise the
// note's path and every disagreement, capped at maxDriftLines.
func (m Model) registryLines() []string {
	path := m.threadsOpts.RegistryPath
	if path == "" {
		return nil
	}
	w := m.lineWidth()
	switch {
	case m.registryErr != nil:
		return []string{styleErr.Render(truncate("registry: could not read "+path+": "+m.registryErr.Error(), w))}
	case !m.threadsLoaded:
		return []string{styleCoverage.Render(truncate("registry: "+path+" · comparing after the read…", w))}
	case len(m.drift) == 0:
		return []string{styleCoverage.Render(truncate("registry: "+path+" · no drift", w))}
	}
	lines := []string{styleCoverage.Render(truncate(fmt.Sprintf("registry: %s · %s", path, plural(len(m.drift), "drift")), w))}
	shown := m.drift
	if len(shown) > maxDriftLines {
		shown = shown[:maxDriftLines]
	}
	for _, d := range shown {
		lines = append(lines, styleFunded.Render(truncate("  "+DriftLine(d), w)))
	}
	if more := len(m.drift) - len(shown); more > 0 {
		lines = append(lines, styleCoverage.Render(fmt.Sprintf("  +%d more", more)))
	}
	return lines
}

// threadsLines is the threads screen body: the registry block when a note was
// given, the header, then one line per seat inside the scroll window.
// UNCHECKED when tmux could not be listed, and then no rows at all.
func (m Model) threadsLines(max int) []string {
	if max < 1 {
		return nil
	}
	now := m.now()
	w := m.lineWidth()
	lines := m.registryLines()
	switch {
	case m.threads.Err != nil:
		lines = append(lines, styleErr.Render(truncate("threads: UNCHECKED ("+m.threads.Err.Error()+")", w)))
		return capLines(lines, max)
	case !m.sessionsLoaded && !m.threadsLoaded:
		lines = append(lines, styleCoverage.Render("threads: listing tmux…"))
		return capLines(lines, max)
	default:
		lines = append(lines, styleCoverage.Render(truncate(ThreadsHeader(m.threads, m.threadsPending), w)))
	}
	room := max - len(lines)
	list := m.threads.Threads
	end := m.threadOffset + room
	if end > len(list) {
		end = len(list)
	}
	sel := m.threadIndex()
	for i := m.threadOffset; i >= 0 && i < end; i++ {
		lines = append(lines, m.renderThread(list[i], i == sel, now))
	}
	return capLines(lines, max)
}

func capLines(lines []string, max int) []string {
	if len(lines) > max {
		return lines[:max]
	}
	return lines
}

// threadHeaderLines is how many lines the registry block and header take, so
// the scroll window knows its room.
func (m Model) threadHeaderLines() int { return len(m.registryLines()) + 1 }

// threadIndex is the selected seat's position; an empty selection tracks the
// top of the list, and a selection that vanished on re-read falls back to it.
func (m Model) threadIndex() int {
	list := m.threads.Threads
	if len(list) == 0 {
		return -1
	}
	if m.threadSel == "" {
		return 0
	}
	for i := range list {
		if list[i].Session == m.threadSel {
			return i
		}
	}
	return 0
}

func (m *Model) clampThreadScroll() {
	sel := m.threadIndex()
	if sel < 0 {
		m.threadOffset = 0
		return
	}
	room := m.visibleRows() - m.threadHeaderLines()
	if room < 1 {
		room = 1
	}
	if sel < m.threadOffset {
		m.threadOffset = sel
	}
	if sel >= m.threadOffset+room {
		m.threadOffset = sel - room + 1
	}
}

// handleThreadsKey is the threads screen's own keymap: move, enter a seat,
// or return (t or tab) to the screen that was showing before.
func (m Model) handleThreadsKey(key string) (tea.Model, tea.Cmd) {
	list := m.threads.Threads
	sel := m.threadIndex()
	switch key {
	case "t", "tab":
		m.screen = m.prevScreen
		return m, nil
	case "up", "k":
		if sel > 0 {
			m.threadSel = list[sel-1].Session
		}
	case "down", "j":
		if sel >= 0 && sel < len(list)-1 {
			m.threadSel = list[sel+1].Session
		}
	case "g", "home":
		if len(list) > 0 {
			m.threadSel = list[0].Session
		}
	case "G", "end":
		if len(list) > 0 {
			m.threadSel = list[len(list)-1].Session
		}
	case "enter":
		if sel >= 0 {
			return m, switchToThread(list[sel])
		}
	case "i":
		if sel >= 0 {
			m.detailSession = list[sel].Session
			m.detailFrom = screenThreads
			m.detailOffset = 0
			m.screen = screenQuestion
		}
	}
	m.clampThreadScroll()
	return m, nil
}

// switchToThread is switchToSession for a seat that need not belong to any
// garden: entry stays switch-client (GATE ruling 3), the topic names the seat.
func switchToThread(th Thread) tea.Cmd {
	return switchToSession(Project{Name: th.Seat.Topic}, TmuxSession{Name: th.Session, Path: th.Path, Activity: th.Activity})
}

// finishThreads is the end of one read: order the seats, attribute them to
// gardens, hand the rows the same attribution, and diff the note if one was
// given. Called when the last pending thread lands.
func (m *Model) finishThreads() {
	SortThreads(m.threads.Threads)
	m.threads.ByRoot = Attribute(m.threads.Threads, m.projects, ThreadsMinShare)
	m.sessions = m.threads.Sessions()
	m.sessionsLoaded = true
	m.threadsLoaded = true
	if m.registry != nil {
		m.drift = DiffRegistry(m.registry, m.threads.Threads)
	}
	m.clampThreadScroll()
}
