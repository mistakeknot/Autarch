package door

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/pkg/tui/theme"
)

// resultMsg carries one finished check. Rows are matched by Root, not index:
// the list re-ranks as results stream in, so positions are not identities.
type resultMsg struct{ p Project }

// statusMsg is a transient footer note (Zed launched, pin saved, errors).
type statusMsg string

// sessionsMsg carries one snapshot of the tmux axis (Gate B).
type sessionsMsg struct{ set SessionSet }

// The threads axis' messages. sessionsListMsg is the raw tmux list when the
// threads axis is on: the model resolves it by pane path at once and reads
// the transcripts after. threadMsg is one seat read. The pending count is set
// synchronously when the read starts, so no start message can race a result.
type sessionsListMsg struct {
	sessions []TmuxSession
	err      error
}
type threadMsg struct{ t Thread }

type checksDoneMsg struct{}

// movementMsg carries one garden's briefing result; movementsStartedMsg opens
// the stream and carries the one estate-wide sessions verdict.
type movementMsg struct{ m Movement }

type movementsStartedMsg struct {
	count       int
	sessionsErr error
}

// Layout is where the briefing sits relative to the rows. Both exist on
// purpose: the alone-vs-above question (autarch-01's open question) is
// decided against a real render, and the b key flips between them live.
type Layout int

const (
	LayoutAlone Layout = iota // the briefing is the opening screen; tab reaches the rows
	LayoutAbove               // the briefing sits above the ranked rows on one screen
)

// ParseLayout reads the --layout flag; empty means alone.
func ParseLayout(s string) (Layout, error) {
	switch s {
	case "", "alone":
		return LayoutAlone, nil
	case "above":
		return LayoutAbove, nil
	}
	return LayoutAlone, fmt.Errorf("layout %q: want alone or above", s)
}

func (l Layout) String() string {
	if l == LayoutAbove {
		return "above"
	}
	return "alone"
}

// screen is which of its two screens LayoutAlone is showing.
type screen int

const (
	screenBriefing screen = iota
	screenRows
	screenThreads
)

// BriefingOptions configures the briefing axis. A zero Since leaves the axis
// off, which is how the rows-only door and its tests keep their behavior.
type BriefingOptions struct {
	Since           time.Time
	SinceSource     string // "last visit", "first visit", "--since 36h": shown in the header
	SinceErr        error  // a stamp that exists but could not be read; stated, never hidden
	VisitPath       string // where to stamp on quit; empty means never stamp
	TranscriptsRoot string // Claude Code's per-project transcript directories
	Layout          Layout
	Now             func() time.Time // the clock, for tests; nil means time.Now
}

// widenSteps are the windows the w key cycles through before returning to the
// configured one. A stamp-only window makes a second open five minutes later
// empty; widening is the escape, and it never writes the stamp.
var widenSteps = []time.Duration{24 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}

var (
	palette = theme.Current().Semantic()

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(palette.Interactive)
	styleCoverage = lipgloss.NewStyle().Foreground(palette.FgSecondary)
	styleFooter   = lipgloss.NewStyle().Foreground(palette.FgTertiary)
	styleErr      = lipgloss.NewStyle().Foreground(palette.StatusError)
	styleSelected = lipgloss.NewStyle().Background(palette.BgSelected).Bold(true)
	styleFunded   = lipgloss.NewStyle().Foreground(palette.StatusWarning)
	stylePinned   = lipgloss.NewStyle().Foreground(palette.StatusInfo)
	styleReason   = lipgloss.NewStyle().Foreground(palette.FgTertiary)
	styleSessions = lipgloss.NewStyle().Foreground(palette.Interactive)

	// The four checker verdicts plus unchecked, each visibly distinct --
	// a DONE WHEN clause, not a styling nicety.
	verdictStyles = map[Verdict]lipgloss.Style{
		VerdictConfirmed:   lipgloss.NewStyle().Foreground(palette.StatusSuccess),
		VerdictProvisional: lipgloss.NewStyle().Foreground(palette.StatusWarning),
		VerdictInvalid:     lipgloss.NewStyle().Foreground(palette.StatusError).Bold(true),
		VerdictAbsent:      lipgloss.NewStyle().Foreground(palette.FgTertiary),
		VerdictUnchecked:   lipgloss.NewStyle().Foreground(palette.StatusError).Reverse(true),
	}
)

// Model is the door: the ranked estate, one project per row, opened through
// the briefing of what moved since mk was last here.
type Model struct {
	projects    []Project
	ranking     Ranking
	rankingPath string
	rankingErr  error
	checker     string
	checkerErr  error

	// The tmux axis. sessionsLoaded distinguishes "not yet asked" from a
	// snapshot that truly found zero sessions; sessions.Err is the third
	// state, "asked and could not look".
	sessions       SessionSet
	sessionsLoaded bool

	results   chan resultMsg
	remaining int // checks still in flight; 0 means the estate is fully reported

	// The briefing axis (autarch-01 step 1). movements is keyed by Root;
	// moveRemaining counts gardens still being read so the header can say
	// "reading N…" instead of presenting a partial estate as the whole one.
	briefing      BriefingOptions
	window        time.Time // the window in force: configured, or widened by w
	windowSource  string
	widen         int // index into widenSteps; -1 means the configured window
	movements     map[string]Movement
	moveResults   chan movementMsg
	moveRemaining int
	sessionsErr   error // the transcripts root could not be read: one estate-wide fact
	layout        Layout
	screen        screen

	// The threads axis (autarch-01 steps 2 and 3; plan 2026-09-02): every
	// live tmux seat, read from the session name, the pane command and the
	// transcript. threadsOn is the switch (WithThreads); the rows-only door
	// and its tests never see it. threadsPending counts seats still being
	// read so a partial list is never presented as the whole estate.
	threadsOpts    ThreadsOptions
	threadsOn      bool
	threads        ThreadSet
	threadResults  chan threadMsg
	threadsPending int
	threadsLoaded  bool   // one full read finished, or failed (threads.Err)
	prevScreen     screen // where t returns to
	threadSel      string // selected seat's session name; "" tracks the top
	threadOffset   int
	registry       []Seat // the note given by --registry, parsed once
	registryErr    error
	drift          []Drift

	selRoot string
	moved   bool // user has navigated; until then selection tracks the top row
	offset  int
	width   int
	height  int
	status  string
}

// NewModel builds the door over an already-discovered estate. checkerErr
// records why the checker is unavailable; the model then shows every row as
// unchecked rather than pretending the estate has no cards.
func NewModel(projects []Project, ranking Ranking, rankingPath string, rankingErr error, checker string, checkerErr error) Model {
	ranking.Apply(projects)
	Rank(projects)
	m := Model{
		projects:    projects,
		ranking:     ranking,
		rankingPath: rankingPath,
		rankingErr:  rankingErr,
		checker:     checker,
		checkerErr:  checkerErr,
		results:     make(chan resultMsg, len(projects)),
		widen:       -1,
	}
	if len(projects) > 0 {
		m.selRoot = projects[0].Root
	}
	return m
}

// WithBriefing turns the briefing axis on. It is kept off the constructor so
// the rows-only door and its tests are untouched.
func (m Model) WithBriefing(o BriefingOptions) Model {
	m.briefing = o
	m.window = o.Since
	m.windowSource = o.SinceSource
	m.widen = -1
	m.layout = o.Layout
	m.screen = screenBriefing
	m.movements = make(map[string]Movement, len(m.projects))
	m.moveResults = make(chan movementMsg, len(m.projects))
	return m
}

func (m Model) briefingOn() bool { return !m.briefing.Since.IsZero() }

func (m Model) now() time.Time {
	if m.briefing.Now != nil {
		return m.briefing.Now()
	}
	return time.Now()
}

func (m Model) Init() tea.Cmd {
	// Sessions load even when the checker is unavailable: the axes fail
	// independently, and a dead checker must not blind the tmux column or
	// the briefing.
	cmds := []tea.Cmd{m.loadSessions()}
	if m.checkerErr == nil && len(m.projects) > 0 {
		cmds = append(cmds, m.startChecks(), m.waitForResult())
	}
	if m.briefingOn() && len(m.projects) > 0 {
		cmds = append(cmds, m.startMovements(), m.waitForMovement())
	}
	return tea.Batch(cmds...)
}

// loadSessions snapshots the tmux axis against the estate as discovered. With
// the threads axis on it returns the raw list instead, so the model can
// resolve by pane path at once and read the transcripts after.
func (m Model) loadSessions() tea.Cmd {
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	if m.threadsOn {
		return func() tea.Msg {
			sessions, err := ListSessions(context.Background())
			return sessionsListMsg{sessions: sessions, err: err}
		}
	}
	return func() tea.Msg {
		return sessionsMsg{set: SnapshotSessions(context.Background(), projects)}
	}
}

// startChecks fans the checker out over the estate in the background; each
// finished project lands on the results channel in completion order.
func (m Model) startChecks() tea.Cmd {
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	checker := m.checker
	results := m.results
	return func() tea.Msg {
		go CheckAll(context.Background(), checker, projects, func(_ int, p Project) {
			results <- resultMsg{p: p}
		})
		return checksStartedMsg{count: len(projects)}
	}
}

type checksStartedMsg struct{ count int }

func (m Model) waitForResult() tea.Cmd {
	results := m.results
	return func() tea.Msg { return <-results }
}

// startMovements indexes the transcripts root once (filesystem only, fast),
// then fans git out over the estate in the background, streaming gardens
// onto moveResults in completion order.
func (m Model) startMovements() tea.Cmd {
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	since := m.window
	root := m.briefing.TranscriptsRoot
	results := m.moveResults
	return func() tea.Msg {
		var idx map[string]SessionStat
		var sessErr error
		if root == "" {
			sessErr = errors.New("no transcripts root configured")
		} else {
			idx, sessErr = IndexSessions(root, projects, since)
		}
		go Movements(context.Background(), projects, since, idx, func(mv Movement) {
			results <- movementMsg{m: mv}
		})
		return movementsStartedMsg{count: len(projects), sessionsErr: sessErr}
	}
}

func (m Model) waitForMovement() tea.Cmd {
	results := m.moveResults
	return func() tea.Msg { return <-results }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case checksStartedMsg:
		m.remaining = msg.count
		return m, nil

	case resultMsg:
		for i := range m.projects {
			if m.projects[i].Root == msg.p.Root {
				m.projects[i] = msg.p
				break
			}
		}
		Rank(m.projects)
		// Until the user navigates, selection stays on the top row -- the
		// weakest card, which is what the door opens to show. Following a
		// project identity through the re-rank before any keypress would
		// scroll the view to wherever that project happened to land.
		if !m.moved && len(m.projects) > 0 {
			m.selRoot = m.projects[0].Root
		}
		m.clampScroll()
		if m.remaining > 0 {
			m.remaining--
		}
		if m.remaining > 0 {
			return m, m.waitForResult()
		}
		return m, nil

	case sessionsMsg:
		m.sessions = msg.set
		m.sessionsLoaded = true
		return m, nil

	case sessionsListMsg:
		// tmux could not be listed: both axes carry the same fact, and no
		// seat is rendered (UNCHECKED is never an empty estate).
		if msg.err != nil {
			m.sessions = SessionSet{Err: msg.err}
			m.sessionsLoaded = true
			m.threads = ThreadSet{Err: msg.err, ByRoot: make(map[string][]Thread)}
			m.threadsLoaded = true
			m.threadsPending = 0
			return m, nil
		}
		// Path resolution lands at once; attribution upgrades it when the
		// transcripts have been read (finishThreads).
		m.sessions = ResolveSessions(msg.sessions, m.projects)
		m.sessionsLoaded = true
		m.threads = ThreadSet{ByRoot: make(map[string][]Thread)}
		m.threadsLoaded = false
		m.drift = nil
		m.threadsPending = len(msg.sessions)
		if m.threadsPending == 0 {
			m.finishThreads()
			return m, nil
		}
		return m, tea.Batch(m.startThreads(msg.sessions), m.waitForThread())

	case threadMsg:
		m.threads.Threads = append(m.threads.Threads, msg.t)
		if m.threadsPending > 0 {
			m.threadsPending--
		}
		if m.threadsPending > 0 {
			return m, m.waitForThread()
		}
		m.finishThreads()
		return m, nil

	case movementsStartedMsg:
		m.moveRemaining = msg.count
		m.sessionsErr = msg.sessionsErr
		return m, nil

	case movementMsg:
		// A result from a window that is no longer in force is dropped;
		// restarts are gated on an idle stream, so this is belt and braces.
		if msg.m.Since.Equal(m.window) {
			m.movements[msg.m.Root] = msg.m
		}
		if m.moveRemaining > 0 {
			m.moveRemaining--
		}
		if m.moveRemaining > 0 {
			return m, m.waitForMovement()
		}
		return m, nil

	case statusMsg:
		m.status = string(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "ctrl+c", "esc":
		return m, m.quit()
	case "r":
		return m.reread()
	}
	// The threads screen has its own keymap; t opens it from anywhere else
	// (the briefing included: reading the seats is orientation, not an
	// action) and remembers where to return.
	if m.threadsOn {
		if m.screen == screenThreads {
			return m.handleThreadsKey(key)
		}
		if key == "t" {
			m.prevScreen = m.screen
			m.screen = screenThreads
			m.clampThreadScroll()
			return m, nil
		}
	}
	if m.briefingOn() {
		switch key {
		case "b":
			if m.layout == LayoutAlone {
				m.layout = LayoutAbove
			} else {
				m.layout = LayoutAlone
			}
			m.clampScroll()
			return m, nil
		case "tab":
			if m.layout == LayoutAlone {
				if m.screen == screenBriefing {
					m.screen = screenRows
				} else {
					m.screen = screenBriefing
				}
			}
			return m, nil
		case "w":
			if m.moveRemaining == 0 {
				return m.widenWindow()
			}
			return m, nil
		}
		// On the briefing screen orientation has no actions: nothing may ask
		// for a decision until mk reaches for the rows (autarch-01, closed
		// decision 2). Every other key is inert here.
		if m.layout == LayoutAlone && m.screen == screenBriefing {
			return m, nil
		}
	}
	// Any other key is the user taking the wheel: selection stops tracking
	// the top row and starts following the selected project through re-ranks.
	m.moved = true

	sel := m.selIndex()
	switch key {
	case "up", "k":
		if sel > 0 {
			m.selRoot = m.projects[sel-1].Root
		}
	case "down", "j":
		if sel >= 0 && sel < len(m.projects)-1 {
			m.selRoot = m.projects[sel+1].Root
		}
	case "g", "home":
		if len(m.projects) > 0 {
			m.selRoot = m.projects[0].Root
		}
	case "G", "end":
		if len(m.projects) > 0 {
			m.selRoot = m.projects[len(m.projects)-1].Root
		}
	case "enter":
		// Decision 9: a row with live sessions switches the tmux client
		// there; a row without falls back to the card, which is where
		// backfill on first touch starts.
		if sel >= 0 {
			p := m.projects[sel]
			if target, ok := m.sessions.Target(p.Root); ok {
				return m, switchToSession(p, target)
			}
			return m, openInZed(p)
		}
	case "z":
		if sel >= 0 {
			return m, openInZed(m.projects[sel])
		}
	case "p":
		if sel >= 0 {
			return m.togglePin(m.projects[sel])
		}
	}
	m.clampScroll()
	return m, nil
}

// quit stamps the visit -- a completed walk closes the window -- then leaves.
// A failed stamp cannot be shown (the screen is about to go) and is not
// fatal; it fails wide: the previous stamp stands and the next window is
// wider, never narrower.
func (m Model) quit() tea.Cmd {
	if m.briefingOn() && m.briefing.VisitPath != "" {
		_ = SaveVisit(m.briefing.VisitPath, m.now())
	}
	return tea.Quit
}

// reread re-reads whatever is idle: cards and tmux (the original r), and the
// briefing when it is on. Nothing in flight is restarted -- a second batch
// streaming into the same channel would let stale results land in the new
// window's map.
func (m Model) reread() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.checkerErr == nil && m.remaining == 0 && len(m.projects) > 0 {
		for i := range m.projects {
			m.projects[i].Verdict = VerdictUnchecked
			m.projects[i].Err = nil
		}
		cmds = append(cmds, m.startChecks(), m.waitForResult())
		// A thread read still streaming is not restarted: a second batch on
		// the same channel would let stale seats land in the new list.
		if !m.threadsOn || m.threadsPending == 0 {
			m.sessionsLoaded = false
			cmds = append(cmds, m.loadSessions())
		}
	}
	if m.briefingOn() && m.moveRemaining == 0 && len(m.projects) > 0 {
		m.movements = make(map[string]Movement, len(m.projects))
		cmds = append(cmds, m.startMovements(), m.waitForMovement())
	}
	if len(cmds) == 0 {
		return m, nil
	}
	m.status = "re-reading the estate"
	return m, tea.Batch(cmds...)
}

// widenWindow steps the briefing window through widenSteps and back to the
// configured one, re-reading each time. The stamp is never touched.
func (m Model) widenWindow() (tea.Model, tea.Cmd) {
	m.widen++
	if m.widen >= len(widenSteps) {
		m.widen = -1
		m.window, m.windowSource = m.briefing.Since, m.briefing.SinceSource
	} else {
		step := widenSteps[m.widen]
		m.window = m.now().Add(-step)
		m.windowSource = "widened to " + humanDur(step)
	}
	m.movements = make(map[string]Movement, len(m.projects))
	m.status = "re-reading since " + m.window.Local().Format("Mon 2 Jan 15:04")
	return m, tea.Batch(m.startMovements(), m.waitForMovement())
}

// openInZed hands the card path to the zed CLI. An absent card opens too --
// that is ruling 12's backfill-on-first-touch made literal: the door opens
// the empty spot where the card goes.
func openInZed(p Project) tea.Cmd {
	path := p.CardPath
	name := p.Name
	return func() tea.Msg {
		cmd := exec.Command("zed", path)
		if err := cmd.Start(); err != nil {
			return statusMsg(fmt.Sprintf("zed failed: %v", err))
		}
		go func() { _ = cmd.Wait() }() // reap; the door does not babysit Zed
		return statusMsg(fmt.Sprintf("opened %s/docs/why.md in Zed", name))
	}
}

// switchToSession moves mk to the project's most recently active tmux
// session (decision 9). Inside tmux that is switch-client -- the door keeps
// running in its own pane. Outside tmux there is no client to switch, so the
// door execs an attach and resumes when that client detaches. The "=" prefix
// pins tmux to an exact session-name match instead of prefix matching.
func switchToSession(p Project, s TmuxSession) tea.Cmd {
	name, pname := s.Name, p.Name
	if os.Getenv("TMUX") == "" {
		c := exec.Command("tmux", "attach-session", "-t", "="+name)
		return tea.ExecProcess(c, func(err error) tea.Msg {
			if err != nil {
				return statusMsg(fmt.Sprintf("tmux attach %s: %v", name, err))
			}
			return statusMsg(fmt.Sprintf("detached from %s (%s)", name, pname))
		})
	}
	return func() tea.Msg {
		out, err := exec.Command("tmux", "switch-client", "-t", "="+name).CombinedOutput()
		if err != nil {
			return statusMsg(fmt.Sprintf("tmux switch-client %s: %v: %s", name, err, strings.TrimSpace(string(out))))
		}
		return statusMsg(fmt.Sprintf("switched to %s (%s)", name, pname))
	}
}

func (m Model) togglePin(p Project) (tea.Model, tea.Cmd) {
	if p.Funded {
		m.status = p.Name + " is funded -- funded outranks pins, nothing to toggle"
		return m, nil
	}
	pinned := m.ranking.TogglePin(p.Name)
	if err := SaveRanking(m.rankingPath, m.ranking); err != nil {
		m.status = fmt.Sprintf("pin not saved: %v", err)
		return m, nil
	}
	m.ranking.Apply(m.projects)
	Rank(m.projects)
	m.clampScroll()
	if pinned {
		m.status = "pinned " + p.Name
	} else {
		m.status = "unpinned " + p.Name
	}
	return m, nil
}

func (m Model) selIndex() int {
	for i := range m.projects {
		if m.projects[i].Root == m.selRoot {
			return i
		}
	}
	return -1
}

// chromeHeight is the door's own frame: title line plus the four footer
// lines (cards fraction, sessions fraction, keys, status).
const chromeHeight = 5

// briefingHeight is how many lines the briefing takes in LayoutAbove: its two
// header lines plus every moved garden plus one for the tail, capped at half
// the screen so the rows keep the other half.
func (m Model) briefingHeight() int {
	if !m.briefingOn() || m.layout != LayoutAbove {
		return 0
	}
	want := 3 + len(m.movedSorted())
	if limit := m.height / 2; want > limit {
		want = limit
	}
	if want < 3 {
		want = 3
	}
	return want
}

func (m *Model) visibleRows() int {
	rows := m.height - chromeHeight
	if m.briefingOn() && m.layout == LayoutAbove {
		rows -= m.briefingHeight() + 1 // plus the rule beneath the briefing
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) clampScroll() {
	sel := m.selIndex()
	if sel < 0 {
		m.offset = 0
		return
	}
	rows := m.visibleRows()
	if sel < m.offset {
		m.offset = sel
	}
	if sel >= m.offset+rows {
		m.offset = sel - rows + 1
	}
}

func (m Model) lineWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("AUTARCH"))
	b.WriteString(styleCoverage.Render(fmt.Sprintf("  %d projects", len(m.projects))))
	b.WriteString("\n")

	rows := m.visibleRows()

	// Orientation before obligation: in LayoutAlone the briefing is the whole
	// opening screen, and the rows wait behind tab.
	if m.briefingOn() && m.layout == LayoutAlone && m.screen == screenBriefing {
		lines := m.briefingLines(rows)
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
		for i := len(lines); i < rows; i++ {
			b.WriteString("\n")
		}
		b.WriteString(m.renderFooter())
		return b.String()
	}
	if m.briefingOn() && m.layout == LayoutAbove {
		h := m.briefingHeight()
		lines := m.briefingLines(h)
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
		for i := len(lines); i < h; i++ {
			b.WriteString("\n")
		}
		b.WriteString(styleFooter.Render(strings.Repeat("─", m.lineWidth())))
		b.WriteString("\n")
	}

	// The threads screen takes the list area: the whole body in LayoutAlone
	// (or with no briefing), the area beneath the briefing in LayoutAbove.
	if m.threadsOn && m.screen == screenThreads {
		lines := m.threadsLines(rows)
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
		for i := len(lines); i < rows; i++ {
			b.WriteString("\n")
		}
		b.WriteString(m.renderFooter())
		return b.String()
	}

	end := m.offset + rows
	if end > len(m.projects) {
		end = len(m.projects)
	}
	sel := m.selIndex()
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.projects[i], i == sel))
		b.WriteString("\n")
	}
	for i := end - m.offset; i < rows; i++ {
		b.WriteString("\n")
	}

	b.WriteString(m.renderFooter())
	return b.String()
}

// movedSorted is the briefing's body: every garden that moved, newest first,
// ties by name so the order is stable across renders.
func (m Model) movedSorted() []Movement {
	var out []Movement
	for _, mv := range m.movements {
		if mv.Moved() {
			out = append(out, mv)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Latest.Equal(out[j].Latest) {
			return out[i].Latest.After(out[j].Latest)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// briefingLines renders the catch-up briefing as at most max lines, plain
// text laid out first and styled after (this repo's ANSI-truncation rule).
// The quiet default is literal: only gardens that moved get a line; gardens
// the door could not read are named, because unread is not quiet.
func (m Model) briefingLines(max int) []string {
	if max < 1 {
		return nil
	}
	now := m.now()
	gardens := m.movedSorted()
	var unread []string
	commits, dirty, sessions := 0, 0, 0
	for _, mv := range m.movements {
		if mv.Err != nil {
			unread = append(unread, mv.Name)
			continue
		}
		commits += len(mv.Commits)
		dirty += mv.Dirty
		sessions += mv.Sessions
	}
	sort.Strings(unread)

	when := m.window.Local().Format("Mon 2 Jan 15:04")
	head := fmt.Sprintf("since %s · %s · %s", when, humanDur(now.Sub(m.window)), m.windowSource)
	if m.briefing.SinceErr != nil && m.widen < 0 {
		head += " · " + m.briefing.SinceErr.Error()
	}
	if m.moveRemaining > 0 {
		head += fmt.Sprintf(" · reading %d…", m.moveRemaining)
	}
	sess := plural(sessions, "claude session")
	if m.sessionsErr != nil {
		sess = "claude sessions: UNCHECKED (" + m.sessionsErr.Error() + ")"
	}
	totals := fmt.Sprintf("%d of %d gardens moved · %s · %d uncommitted · %s",
		len(gardens), len(m.projects), plural(commits, "commit"), dirty, sess)

	lines := []string{
		styleCoverage.Render(truncate(head, m.lineWidth())),
		styleCoverage.Render(truncate(totals, m.lineWidth())),
	}

	// Tail lines survive the cap: the unread list, and the nothing-moved
	// verdict once every garden has been read.
	var tail []string
	done := m.moveRemaining == 0 && len(m.movements) > 0
	if done && len(gardens) == 0 {
		tail = append(tail, styleCoverage.Render("  nothing moved since "+when))
	}
	if len(unread) > 0 {
		tail = append(tail, styleErr.Render(truncate("  could not read: "+strings.Join(unread, ", "), m.lineWidth())))
	}

	room := max - len(lines) - len(tail)
	if room < 0 {
		room = 0
	}
	shown := gardens
	if len(shown) > room {
		if room > 0 {
			shown = shown[:room-1]
		} else {
			shown = nil
		}
	}
	for _, mv := range shown {
		lines = append(lines, m.renderMovement(mv, now))
	}
	if more := len(gardens) - len(shown); more > 0 && room > 0 {
		lines = append(lines, styleCoverage.Render(fmt.Sprintf("  +%d more", more)))
	}
	lines = append(lines, tail...)
	if len(lines) > max {
		lines = lines[:max]
	}
	return lines
}

// renderMovement is one garden's line: its name, the facts that earned it the
// line, the newest commit subject, and how long ago it last moved.
func (m Model) renderMovement(mv Movement, now time.Time) string {
	width := m.lineWidth()
	nameWidth := 24
	if nameWidth > width/3 {
		nameWidth = width / 3
	}
	if nameWidth < 8 {
		nameWidth = 8
	}
	// 42 fits the busiest honest line seen live ("83 commits · 140
	// uncommitted · 56 sessions") without an ellipsis.
	const factsWidth, agoWidth = 42, 8

	var facts []string
	if n := len(mv.Commits); n > 0 {
		facts = append(facts, plural(n, "commit"))
	}
	if mv.Dirty > 0 {
		facts = append(facts, fmt.Sprintf("%d uncommitted", mv.Dirty))
	}
	if mv.Sessions > 0 {
		facts = append(facts, plural(mv.Sessions, "session"))
	}
	subject := ""
	if len(mv.Commits) > 0 {
		subject = mv.Commits[0].Subject
	}
	ago := ""
	if !mv.Latest.IsZero() {
		ago = humanAgo(now.Sub(mv.Latest))
	}
	subjectWidth := width - 2 - nameWidth - 1 - factsWidth - 1 - agoWidth - 1
	if subjectWidth < 0 {
		subjectWidth = 0
	}
	return "  " +
		pad(mv.Name, nameWidth) + " " +
		styleSessions.Render(pad(strings.Join(facts, " · "), factsWidth)) + " " +
		styleReason.Render(pad(truncate(subject, subjectWidth), subjectWidth)) + " " +
		styleCoverage.Render(pad(ago, agoWidth))
}

// humanDur is the briefing's coarse clock: minutes under an hour, hours under
// two days, days beyond.
func humanDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func humanAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	return humanDur(d) + " ago"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// coverageLine states the cards axis as a fraction of the whole estate.
// Unchecked is named whenever nonzero: a dead checker must read as a dead
// checker, not as an estate with no cards.
func coverageLine(c Coverage) string {
	s := fmt.Sprintf("cards: %d confirmed · %d provisional · %d invalid · %d absent",
		c.Confirmed, c.Provisional, c.Invalid, c.Absent)
	if c.Unchecked > 0 {
		s += fmt.Sprintf(" · %d UNCHECKED", c.Unchecked)
	}
	return s
}

// renderRow lays the row out as plain fixed-width cells first and styles each
// cell afterwards, so no styled string is ever truncated (the []rune-on-ANSI
// bug class this repo's rules name).
func (m Model) renderRow(p Project, selected bool) string {
	nameWidth := m.width - 30 // marker 2 + verdict 13 + score 7 + sessions 4 + padding
	if nameWidth < 12 {
		nameWidth = 12
	}
	if nameWidth > 32 {
		nameWidth = 32
	}

	marker := "  "
	markerStyle := lipgloss.NewStyle()
	if p.Funded {
		marker, markerStyle = "★ ", styleFunded
	} else if p.Pinned {
		marker, markerStyle = "⚑ ", stylePinned
	}

	score := "  — "
	if p.Verdict != VerdictUnchecked {
		score = fmt.Sprintf("%d/%d ", p.Strength.Score, p.Strength.Of)
	}

	// The sessions cell stays blank until a snapshot lands, and stays blank
	// on an UNCHECKED snapshot -- rendering 0 for a count nobody measured is
	// the empty-result-read-as-zero failure, same rule as the score column.
	sess := "    "
	if m.sessionsLoaded && m.sessions.Err == nil {
		if n := m.sessions.Count(p.Root); n > 0 {
			sess = pad(fmt.Sprintf("◆%d", n), 4)
		}
	}

	name := pad(p.Name, nameWidth)
	verdict := pad(strings.ToUpper(string(p.Verdict)), 12)

	note := ""
	if p.Verdict == VerdictInvalid && p.Reason != "" {
		note = truncate(p.Reason, m.width-nameWidth-30)
	}
	if p.Verdict == VerdictUnchecked && p.Err != nil {
		note = truncate(p.Err.Error(), m.width-nameWidth-30)
	}

	line := markerStyle.Render(marker) +
		lipgloss.NewStyle().Render(name) + " " +
		verdictStyles[p.Verdict].Render(verdict) +
		fmt.Sprintf("%5s", score) + " " +
		styleSessions.Render(sess) +
		styleReason.Render(note)
	if selected {
		return styleSelected.Render("> ") + line
	}
	return "  " + line
}

// renderFooter states both axes as fractions -- cards confirmed and sessions
// resolved (Gate B clause c) -- then keys, then errors or transient status.
func (m Model) renderFooter() string {
	var b strings.Builder
	b.WriteString(styleCoverage.Render(coverageLine(Cover(m.projects))))
	b.WriteString("\n")
	switch {
	case !m.sessionsLoaded:
		b.WriteString(styleCoverage.Render("sessions: checking…"))
	case m.sessions.Err != nil:
		b.WriteString(styleErr.Render(sessionsLine(m.sessions)))
	default:
		b.WriteString(styleCoverage.Render(sessionsLine(m.sessions)))
	}
	b.WriteString("\n")

	var parts []string
	switch {
	case m.threadsOn && m.screen == screenThreads:
		parts = []string{"↑/↓ move", "enter switch", "t back", "r re-read", "q quit"}
	case m.briefingOn() && m.layout == LayoutAlone && m.screen == screenBriefing:
		parts = []string{"tab rows", "w widen", "b layout", "r re-read", "q quit"}
	case m.briefingOn() && m.layout == LayoutAlone:
		parts = []string{"↑/↓ move", "enter switch/open", "z card in Zed", "p pin", "tab briefing", "b layout", "r re-read", "q quit"}
	case m.briefingOn():
		parts = []string{"↑/↓ move", "enter switch/open", "z card in Zed", "p pin", "w widen", "b layout", "r re-read", "q quit"}
	default:
		parts = []string{"↑/↓ move", "enter switch/open", "z card in Zed", "p pin", "r re-check", "q quit"}
	}
	if m.threadsOn && m.screen != screenThreads && len(parts) > 0 {
		withThreads := make([]string, 0, len(parts)+1)
		withThreads = append(withThreads, parts[:len(parts)-1]...)
		withThreads = append(withThreads, "t threads", parts[len(parts)-1])
		parts = withThreads
	}
	line := styleFooter.Render(strings.Join(parts, " · "))
	if m.remaining > 0 {
		line += styleCoverage.Render(fmt.Sprintf("  checking %d…", m.remaining))
	}
	switch {
	case m.checkerErr != nil:
		line += "\n" + styleErr.Render("checker unavailable: "+m.checkerErr.Error())
	case m.rankingErr != nil:
		line += "\n" + styleErr.Render("ranking file broken (order degraded to weakest-first): "+m.rankingErr.Error())
	case m.status != "":
		line += "\n" + styleCoverage.Render(m.status)
	default:
		line += "\n"
	}
	b.WriteString(line)
	return b.String()
}

// pad fixes a plain (unstyled) string to width, truncating with an ellipsis.
func pad(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		if width < 1 {
			return ""
		}
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

// truncate shortens a plain (unstyled) string to width.
func truncate(s string, width int) string {
	if width <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
