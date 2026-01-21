package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/agent"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
)

type Model struct {
	Title                string
	CurrentPRD           string
	RepoPath             string
	StatusBadges         []string
	Width                int
	Height               int
	Sessions             []string
	ReviewQueue          []string
	DiffFiles            []string
	Approver             Approver
	BranchLookup         BranchLookup
	Status               string
	StatusLevel          StatusLevel
	ReviewLoader         ReviewLoader
	ReviewBranches       map[string]string
	SelectedReview       int
	ConfirmApprove       bool
	PendingApproveTask   string
	ViewMode             ViewMode
	ReviewShowDiffs      bool
	ReviewInputMode      ReviewInputMode
	ReviewInput          string
	ReviewPendingReject  bool
	MVPExplainPending    bool
	MVPRevertSelect      bool
	MVPRevertIndex       int
	ReviewDetail         ReviewDetail
	ReviewDetailLoader   func(taskID string) (ReviewDetail, error)
	ReviewDiff           ReviewDiffState
	ReviewDiffLoader     func(taskID string) (ReviewDiffState, error)
	ReviewActionWriter   func(taskID, text string) error
	ReviewStoryUpdater   func(taskID, text string) error
	ReviewRejecter       func(taskID string) error
	MVPExplainWriter     func(taskID, text string) error
	MVPAcceptor          func(taskID string) error
	MVPReverter          func(taskID, path string) error
	TaskList             []TaskItem
	SelectedTask         int
	TaskLoader           func() ([]TaskItem, error)
	TaskDetail           TaskDetail
	TaskDetailLoader     func(id string) (TaskDetail, error)
	TaskStarter          func(id string) error
	TaskStopper          func(id string) error
	FocusPane            FocusPane
	RightTab             RightTab
	CoordRecipient       string
	CoordInbox           []storage.MessageDelivery
	CoordLocks           []storage.Reservation
	CoordSelected        int
	CoordScroll          int
	CoordUrgentOnly      bool
	CoordRecipientFilter CoordRecipientFilter
	CoordInboxLoader     func(recipient string, limit int, urgentOnly bool) ([]storage.MessageDelivery, error)
	CoordLocksLoader     func(limit int) ([]storage.Reservation, error)
	Now                  func() time.Time
	CtrlCAt              time.Time
	SearchMode           bool
	SearchQuery          string
	FilterMode           string
	PaletteOpen          bool
	SettingsOpen         bool
	HelpOpen             bool
	QuickTaskMode        bool
	QuickTaskInput       string
	QuickTaskCreator     func(raw string) (string, error)
}

type BranchLookup func(taskID string) (string, error)
type ReviewLoader func() ([]string, error)

type ViewMode string

const (
	ViewFleet  ViewMode = "fleet"
	ViewReview ViewMode = "review"
)

type FocusPane string

const (
	FocusTasks  FocusPane = "tasks"
	FocusDetail FocusPane = "detail"
)

type RightTab string

const (
	RightTabDetails RightTab = "details"
	RightTabCoord   RightTab = "coord"
)

type CoordRecipientFilter string

const (
	CoordRecipientFilterAll      CoordRecipientFilter = "all"
	CoordRecipientFilterMe       CoordRecipientFilter = "me"
	CoordRecipientFilterMentions CoordRecipientFilter = "mentions"
)

type ReviewInputMode string

const (
	ReviewInputNone     ReviewInputMode = "none"
	ReviewInputFeedback ReviewInputMode = "feedback"
	ReviewInputStory    ReviewInputMode = "story"
)

type tickMsg struct{}

const refreshInterval = 2 * time.Second
const ctrlCWindow = 2 * time.Second

type StatusLevel string

const (
	StatusInfo  StatusLevel = "info"
	StatusError StatusLevel = "error"
)

func NewModel() Model {
	recipient := os.Getenv("TAND_MAIL_RECIPIENT")
	if strings.TrimSpace(recipient) == "" {
		recipient = os.Getenv("USER")
	}
	return Model{
		Title:                "Tandemonium",
		StatusBadges:         []string{},
		Sessions:             []string{},
		ReviewQueue:          []string{},
		DiffFiles:            []string{},
		Status:               "ready",
		StatusLevel:          StatusInfo,
		ReviewBranches:       map[string]string{},
		ConfirmApprove:       true,
		ViewMode:             ViewFleet,
		FocusPane:            FocusTasks,
		RightTab:             RightTabDetails,
		CoordRecipient:       recipient,
		CoordSelected:        0,
		CoordScroll:          0,
		CoordRecipientFilter: CoordRecipientFilterAll,
		Now:                  time.Now,
		FilterMode:           "all",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), watchCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil
	case watchMsg:
		m.RefreshTasks()
		m.RefreshTaskDetail()
		m.RefreshCoordination()
		return m, watchCmd()
	case tickMsg:
		m.RefreshTasks()
		m.RefreshTaskDetail()
		m.RefreshCoordination()
		return m, tickCmd()
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			nowFn := m.Now
			if nowFn == nil {
				nowFn = time.Now
			}
			now := nowFn()
			if !m.CtrlCAt.IsZero() && now.Sub(m.CtrlCAt) <= ctrlCWindow {
				return m, tea.Quit
			}
			m.CtrlCAt = now
			m.SetStatusInfo("press ctrl+c again to quit")
			return m, nil
		}
		if msg.Type == tea.KeyCtrlK {
			m.PaletteOpen = !m.PaletteOpen
			if m.PaletteOpen {
				m.SettingsOpen = false
				m.HelpOpen = false
			}
			return m, nil
		}
		if msg.String() == "," {
			m.SettingsOpen = !m.SettingsOpen
			if m.SettingsOpen {
				m.PaletteOpen = false
				m.HelpOpen = false
			}
			return m, nil
		}
		if msg.String() == "?" {
			m.HelpOpen = !m.HelpOpen
			if m.HelpOpen {
				m.PaletteOpen = false
				m.SettingsOpen = false
			}
			return m, nil
		}
		if msg.String() == "esc" && (m.PaletteOpen || m.SettingsOpen || m.HelpOpen) {
			m.PaletteOpen = false
			m.SettingsOpen = false
			m.HelpOpen = false
			return m, nil
		}
		if m.PaletteOpen || m.SettingsOpen || m.HelpOpen {
			return m, nil
		}
		if m.SearchMode {
			switch msg.Type {
			case tea.KeyEnter:
				m.SearchMode = false
			case tea.KeyEsc:
				m.SearchMode = false
				m.SearchQuery = ""
			case tea.KeyBackspace:
				if len(m.SearchQuery) > 0 {
					m.SearchQuery = string([]rune(m.SearchQuery)[:len([]rune(m.SearchQuery))-1])
				}
			case tea.KeyRunes:
				m.SearchQuery += string(msg.Runes)
			}
			m.ClampTaskSelection()
			m.ensureTaskDetail()
			return m, nil
		}
		if m.QuickTaskMode {
			switch msg.Type {
			case tea.KeyEnter:
				m.handleQuickTaskSubmit()
			case tea.KeyEsc:
				m.QuickTaskMode = false
				m.QuickTaskInput = ""
			case tea.KeyBackspace:
				if len(m.QuickTaskInput) > 0 {
					m.QuickTaskInput = string([]rune(m.QuickTaskInput)[:len([]rune(m.QuickTaskInput))-1])
				}
			case tea.KeyRunes:
				m.QuickTaskInput += string(msg.Runes)
			}
			return m, nil
		}
		if msg.Type == tea.KeyTab && m.ViewMode == ViewFleet {
			if m.FocusPane == FocusTasks {
				m.FocusPane = FocusDetail
			} else {
				m.FocusPane = FocusTasks
			}
			return m, nil
		}
		if msg.String() == "c" && m.ViewMode == ViewFleet {
			if m.RightTab == RightTabDetails {
				m.RightTab = RightTabCoord
				m.CoordScroll = 0
				m.CoordSelected = 0
				m.RefreshCoordination()
			} else {
				m.RightTab = RightTabDetails
			}
			return m, nil
		}
		if msg.String() == "n" && m.ViewMode == ViewFleet && m.PendingApproveTask == "" {
			m.QuickTaskMode = true
			m.QuickTaskInput = ""
			return m, nil
		}
		if msg.String() == "/" && m.ViewMode == ViewFleet {
			m.SearchMode = true
			return m, nil
		}
		if msg.String() == "x" && m.ViewMode == ViewFleet {
			m.handleTaskStop()
			return m, nil
		}
		if msg.String() == "r" && m.ViewMode == ViewFleet {
			if m.FocusPane == FocusDetail && m.RightTab == RightTabCoord {
				m.cycleCoordRecipientFilter()
				return m, nil
			}
			m.handleTaskReview()
			return m, nil
		}
		if msg.String() == "u" && m.ViewMode == ViewFleet {
			if m.FocusPane == FocusDetail && m.RightTab == RightTabCoord {
				m.CoordUrgentOnly = !m.CoordUrgentOnly
				m.ClampCoordSelection()
				m.adjustCoordScroll()
				return m, nil
			}
		}
		if m.ViewMode == ViewReview && m.MVPRevertSelect {
			switch msg.String() {
			case "j", "down":
				if m.MVPRevertIndex < len(m.ReviewDetail.Files)-1 {
					m.MVPRevertIndex++
				}
			case "k", "up":
				if m.MVPRevertIndex > 0 {
					m.MVPRevertIndex--
				}
			case "enter":
				m.handleMVPRevertConfirm()
			case "b", "esc":
				m.MVPRevertSelect = false
			}
			return m, nil
		}
		if m.ViewMode == ViewReview && m.ReviewShowDiffs {
			if msg.String() == "b" {
				m.ReviewShowDiffs = false
				return m, nil
			}
			m.handleReviewDiffKey(msg.String())
			return m, nil
		}
		switch msg.String() {
		case "R":
			if m.ViewMode == ViewReview && m.ReviewDetail.Alignment == "out" {
				m.handleMVPRevertStart()
				return m, nil
			}
			m.ViewMode = ViewReview
			return m, nil
		case "d":
			if m.ViewMode == ViewReview {
				m.handleReviewDiff()
				return m, nil
			}
			if m.ViewMode == ViewFleet {
				m.FilterMode = "done"
				m.ClampTaskSelection()
				m.ensureTaskDetail()
				return m, nil
			}
		case "b":
			m.ViewMode = ViewFleet
			return m, nil
		case "i":
			if m.ViewMode == ViewFleet {
				if err := project.Init("."); err != nil {
					m.SetStatusError(err.Error())
				} else {
					m.SetStatusInfo("initialized .tandemonium")
				}
				return m, nil
			}
		case "s":
			if m.ViewMode == ViewFleet {
				m.handleTaskStart()
				return m, nil
			}
		case "j", "down":
			if m.ViewMode == ViewFleet {
				if m.FocusPane == FocusDetail && m.RightTab == RightTabCoord {
					m.moveCoordSelection(1)
					return m, nil
				}
				filtered := m.filteredTasks()
				if m.FocusPane == FocusTasks && len(filtered) > 0 && m.SelectedTask < len(filtered)-1 {
					m.SelectedTask++
					m.ensureTaskDetail()
				}
			} else if len(m.ReviewQueue) > 0 && m.SelectedReview < len(m.ReviewQueue)-1 {
				m.SelectedReview++
			}
		case "k", "up":
			if m.ViewMode == ViewFleet {
				if m.FocusPane == FocusDetail && m.RightTab == RightTabCoord {
					m.moveCoordSelection(-1)
					return m, nil
				}
				if m.FocusPane == FocusTasks && m.SelectedTask > 0 {
					m.SelectedTask--
					m.ensureTaskDetail()
				}
			} else if m.SelectedReview > 0 {
				m.SelectedReview--
			}
		case "enter":
			if m.ViewMode == ViewReview {
				if m.ReviewInputMode != ReviewInputNone {
					m.handleReviewSubmit()
					return m, nil
				}
				m.handleReviewEnter()
				return m, nil
			}
			if len(m.ReviewQueue) > 0 {
				idx := m.SelectedReview
				if idx < 0 || idx >= len(m.ReviewQueue) {
					idx = 0
				}
				taskID := m.ReviewQueue[idx]
				if m.ConfirmApprove {
					m.PendingApproveTask = taskID
					m.SetStatusInfo("confirm approve " + taskID + " (y/n)")
					return m, nil
				}
				if err := m.approveTaskByID(taskID); err != nil {
					m.SetStatusError(err.Error())
					return m, nil
				}
			}
		case "a":
			if m.ViewMode == ViewReview {
				if m.ReviewInputMode != ReviewInputNone {
					m.handleReviewSubmit()
					return m, nil
				}
				m.handleReviewEnter()
				return m, nil
			}
			if len(m.ReviewQueue) > 0 {
				idx := m.SelectedReview
				if idx < 0 || idx >= len(m.ReviewQueue) {
					idx = 0
				}
				taskID := m.ReviewQueue[idx]
				if m.ConfirmApprove {
					m.PendingApproveTask = taskID
					m.SetStatusInfo("confirm approve " + taskID + " (y/n)")
					return m, nil
				}
				if err := m.approveTaskByID(taskID); err != nil {
					m.SetStatusError(err.Error())
					return m, nil
				}
			}
			if m.ViewMode == ViewFleet {
				m.FilterMode = "all"
				m.ClampTaskSelection()
				m.ensureTaskDetail()
				return m, nil
			}
		case "y":
			if m.PendingApproveTask != "" {
				taskID := m.PendingApproveTask
				m.PendingApproveTask = ""
				if err := m.approveTaskByID(taskID); err != nil {
					m.SetStatusError(err.Error())
					return m, nil
				}
			}
		case "n":
			if m.PendingApproveTask != "" {
				m.PendingApproveTask = ""
				m.SetStatusInfo("approve cancelled")
			}
		case "o":
			if m.ViewMode == ViewFleet {
				m.FilterMode = "open"
				m.ClampTaskSelection()
				m.ensureTaskDetail()
				return m, nil
			}
		case "v":
			if m.ViewMode == ViewFleet {
				m.FilterMode = "review"
				m.ClampTaskSelection()
				m.ensureTaskDetail()
				return m, nil
			}
		case "f":
			if m.ViewMode == ViewReview {
				m.ReviewInputMode = ReviewInputFeedback
				m.ReviewInput = ""
				m.MVPExplainPending = false
			}
		case "x":
			if m.ViewMode == ViewReview && m.ReviewDetail.Alignment == "out" {
				m.ReviewInputMode = ReviewInputFeedback
				m.ReviewInput = ""
				m.MVPExplainPending = true
			}
		case "r":
			if m.ViewMode == ViewReview {
				m.ReviewInputMode = ReviewInputFeedback
				m.ReviewInput = ""
				m.ReviewPendingReject = true
				m.MVPExplainPending = false
			}
		case "e":
			if m.ViewMode == ViewReview {
				m.ReviewInputMode = ReviewInputStory
				m.ReviewInput = ""
				m.MVPExplainPending = false
			}
		case "A":
			if m.ViewMode == ViewReview && m.ReviewDetail.Alignment == "out" {
				m.handleMVPAccept()
			}
		case "esc":
			if m.ViewMode == ViewReview && m.ReviewInputMode != ReviewInputNone {
				m.ReviewInputMode = ReviewInputNone
				m.ReviewInput = ""
				m.ReviewPendingReject = false
				m.MVPExplainPending = false
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	render := func(out string) string {
		out = strings.TrimSuffix(out, "\n")
		composed := m.topBarLine() + "\n" + out + "\n" + m.bottomBarLine() + "\n"
		return fitToHeight(composed, m.Height)
	}
	if m.PaletteOpen {
		return render("COMMAND PALETTE\n\nType to search (stub)\n\n[esc] close\n")
	}
	if m.SettingsOpen {
		return render("SETTINGS\n\nSettings UI (stub)\n\n[esc] close\n")
	}
	if m.HelpOpen {
		return render("HELP\n\nHelp modal (stub)\n\n[esc] close\n")
	}
	if m.QuickTaskMode {
		return render("NEW QUICK TASK\n\n" +
			"Describe task:\n" + m.QuickTaskInput + "\n\n" +
			"[enter] create  [esc] cancel\n")
	}
	if m.ViewMode == ViewReview {
		if m.ReviewShowDiffs {
			return render(m.viewReviewDiff())
		}
		out := "REVIEW - " + m.ReviewDetail.TaskID + ": " + m.ReviewDetail.Title + "\n\n"
		if m.ReviewDetail.Alignment == "out" {
			out += "MVP SCOPE WARNING\n[A]ccept  [R]evert file  [x]plain\n\n"
		}
		if m.MVPRevertSelect {
			out += "REVERT FILE\n"
			for i, f := range m.ReviewDetail.Files {
				prefix := "- "
				if i == m.MVPRevertIndex {
					prefix = "> "
				}
				out += prefix + f.Path + "\n"
			}
			out += "\n[j/k] select  [enter] confirm  [b]ack\n\n"
		}
		if m.ReviewInputMode == ReviewInputFeedback {
			out += "FEEDBACK: " + m.ReviewInput + "\n\n"
		}
		if m.ReviewInputMode == ReviewInputStory {
			out += "EDIT STORY: " + m.ReviewInput + "\n\n"
		}
		out += "SUMMARY\n" + m.ReviewDetail.Summary + "\n\n"
		switch m.ReviewDetail.StoryDrift {
		case "changed":
			out += "STORY DRIFT DETECTED\nStored story hash differs from current text.\n\n"
		case "unknown":
			out += "Story drift: unknown\n\n"
		}
		if m.ReviewDetail.Alignment != "" {
			out += "ALIGNMENT\n"
			switch m.ReviewDetail.Alignment {
			case "mvp":
				out += "Alignment: MVP\n\n"
			case "out":
				out += "Alignment: out of scope\n\n"
			default:
				out += "Alignment: unknown\n\n"
			}
		}
		out += "FILES CHANGED\n"
		for _, f := range m.ReviewDetail.Files {
			out += "- " + f.Path + " +" + fmt.Sprintf("%d", f.Added) + " -" + fmt.Sprintf("%d", f.Deleted) + "\n"
		}
		out += "\nTESTS: " + m.ReviewDetail.TestsSummary + "\n\n"
		out += "ACCEPTANCE CRITERIA\n"
		for _, ac := range m.ReviewDetail.AcceptanceCriteria {
			out += "- " + ac + "\n"
		}
		out += "\n[d]iff  [a]pprove  [f]eedback  [r]eject  [e]dit story  [b]ack\n"
		return render(out)
	}
	header := m.Title + " / Tasks\n"
	filterLabel := m.FilterMode
	if filterLabel == "" {
		filterLabel = "all"
	}
	searchLabel := strings.TrimSpace(m.SearchQuery)
	if searchLabel == "" {
		searchLabel = "-"
	}
	coordSummary := ""
	if m.RightTab == RightTabCoord {
		coordSummary = fmt.Sprintf(" | coord: urgent=%s recipient=%s", coordOnOff(m.CoordUrgentOnly), m.coordRecipientFilterLabel())
	}
	summary := fmt.Sprintf("Tasks: %d | Running: %d | Review: %d | filter: %s | search: %s%s\n\n",
		len(m.TaskList),
		countStatus(m.TaskList, "in_progress"),
		countStatus(m.TaskList, "review"),
		filterLabel,
		searchLabel,
		coordSummary,
	)
	out := header + summary

	leftTitle := "TASKS"
	if m.FocusPane == FocusTasks {
		leftTitle += " *"
	}
	rightTitle := rightTabLine(m.RightTab)

	left := []string{
		leftTitle,
		"TYPE PRI ST  ID     TITLE                AGE ASG",
		strings.Repeat("-", 56),
	}
	filtered := m.filteredTasks()
	for i, t := range filtered {
		prefix := "  "
		if i == m.SelectedTask {
			prefix = "> "
		}
		row := formatTaskRow(t)
		if i != m.SelectedTask {
			row = colorize(row, "90")
		}
		left = append(left, prefix+row)
	}
	if len(m.TaskList) == 0 {
		left = append(left, "No tasks yet.")
		left = append(left, "[n] new quick task  [i] init  [?] help")
		left = append(left, renderEmptyState()...)
	} else if len(filtered) == 0 {
		left = append(left, "No tasks match filters.")
	}

	leftWidth := maxLineLen(left)
	if leftWidth < 44 {
		leftWidth = 44
	}
	if leftWidth > 72 {
		leftWidth = 72
	}
	rightWidth := 60
	if m.Width > 0 {
		rightWidth = m.Width - leftWidth - 5
		if rightWidth < 20 {
			rightWidth = 20
		}
	}

	detail := m.currentTaskDetail()
	right := []string{
		rightTitle,
		strings.Repeat("-", 30),
	}
	if m.RightTab == RightTabCoord {
		coordLines, selectedLine := m.coordBodyLines()
		maxLines := len(left)
		if maxLines <= 0 {
			maxLines = m.coordMaxLines()
		}
		scroll := coordScrollWindow(coordLines, selectedLine, maxLines, m.CoordScroll)
		if scroll != m.CoordScroll {
			m.CoordScroll = scroll
		}
		coordLines = sliceWindow(coordLines, scroll, maxLines)
		right = append(right, coordLines...)
	} else if detail.ID == "" {
		right = append(right, "ID: -")
		right = append(right, "Status: -")
		right = append(right, "Priority: -")
		right = append(right, "Assignee: -")
		right = append(right, "Created: -")
		right = append(right, "Labels: -")
		right = append(right, "")
		right = append(right, "Summary")
		right = append(right, "  -")
		right = append(right, "Acceptance Criteria")
		right = append(right, "  -")
		right = append(right, "Recent Activity")
		right = append(right, "  -")
	} else {
		right = append(right, "ID: "+detail.ID)
		right = append(right, "Title: "+detail.Title)
		right = append(right, "Status: "+statusBadge(detail.Status))
		right = append(right, "Priority: -")
		right = append(right, "Assignee: -")
		right = append(right, "Created: -")
		right = append(right, "Labels: -")
		sessionLine := "Session: " + sessionBadge(detail.SessionState)
		if detail.SessionState != "" {
			sessionLine += " " + detail.SessionState
		}
		right = append(right, sessionLine)
		md := "## Summary\n"
		if detail.Summary != "" {
			md += detail.Summary + "\n\n"
		} else {
			md += "-\n\n"
		}
		md += "## Acceptance Criteria\n-\n\n"
		md += "## Recent Activity\n"
		if detail.LastLine != "" {
			md += detail.LastLine + "\n\n"
		} else {
			md += "-\n\n"
		}
		if rendered, err := renderMarkdown(md, rightWidth); err == nil {
			for _, line := range strings.Split(strings.TrimSuffix(rendered, "\n"), "\n") {
				right = append(right, line)
			}
		}
	}
	out += renderTwoPane(left, right, leftWidth, 1)

	if len(m.TaskList) == 0 {
		out += "\nKEYS: n new task, i init, ? help\n"
	} else {
		keysLine := "n new task, s start, x stop, r review, c coord, / search, a/o/v/d filter, tab focus, ? help"
		if m.RightTab == RightTabCoord && m.FocusPane == FocusDetail {
			keysLine = "n new task, s start, x stop, r recipients, u urgent, c coord, / search, a/o/v/d filter, tab focus, ? help"
		}
		out += "\nKEYS: " + keysLine + "\n"
	}
	return render(out)
}

func (m *Model) handleQuickTaskSubmit() {
	raw := strings.TrimSpace(m.QuickTaskInput)
	if raw == "" {
		m.SetStatusError("task description required")
		return
	}
	creator := m.QuickTaskCreator
	if creator == nil {
		creator = func(input string) (string, error) {
			root, err := project.FindRoot(".")
			if err != nil {
				return "", err
			}
			path, err := specs.CreateQuickSpec(project.SpecsDir(root), input, time.Now())
			if err != nil {
				return "", err
			}
			id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			db, err := storage.OpenShared(project.StateDBPath(root))
			if err != nil {
				return "", err
			}
			if err := storage.Migrate(db); err != nil {
				return "", err
			}
			if err := storage.InsertTask(db, storage.Task{ID: id, Title: firstLine(input), Status: "assigned"}); err != nil {
				return "", err
			}
			return id, nil
		}
		m.QuickTaskCreator = creator
	}
	id, err := creator(raw)
	if err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.TaskList = append(m.TaskList, TaskItem{ID: id, Title: firstLine(raw), Status: "assigned"})
	m.QuickTaskMode = false
	m.QuickTaskInput = ""
	m.SetStatusInfo("created quick task " + id)
}

func firstLine(input string) string {
	parts := strings.SplitN(strings.TrimSpace(input), "\n", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func (m *Model) currentTaskDetail() TaskDetail {
	task, ok := m.selectedTask()
	if !ok {
		return TaskDetail{}
	}
	if m.TaskDetail.ID == task.ID {
		return m.TaskDetail
	}
	if m.TaskDetailLoader != nil {
		if detail, err := m.TaskDetailLoader(task.ID); err == nil {
			return detail
		}
	}
	return TaskDetail{
		ID:           task.ID,
		Title:        task.Title,
		Status:       task.Status,
		SessionState: task.SessionState,
	}
}

func statusBadge(status string) string {
	switch status {
	case "in_progress":
		return colorize("[RUN]", "32")
	case "review":
		return colorize("[REV]", "33")
	case "blocked":
		return colorize("[BLK]", "31")
	case "done":
		return colorize("[DONE]", "32")
	case "assigned":
		return colorize("[ASGN]", "34")
	case "todo":
		return colorize("[TODO]", "90")
	default:
		return colorize("[UNKN]", "90")
	}
}

func sessionBadge(state string) string {
	switch state {
	case "working":
		return colorize("[RUN]", "32")
	case "paused":
		return colorize("[PAUS]", "33")
	case "done":
		return colorize("[DONE]", "32")
	case "stopped":
		return colorize("[STOP]", "31")
	case "":
		return colorize("[----]", "90")
	default:
		return colorize("[....]", "90")
	}
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func formatTaskRow(task TaskItem) string {
	typ := "-"
	pri := "-"
	age := "-"
	asg := "-"
	return fmt.Sprintf("%-4s %-3s %-4s %-6s %-20s %-3s %-3s",
		typ,
		pri,
		statusBadge(task.Status),
		task.ID,
		truncate(task.Title, 20),
		age,
		asg,
	)
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value[:width]
	}
	return value + strings.Repeat(" ", width-len(value))
}

func renderTwoPane(left, right []string, width, gap int) string {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}
	sep := " | "
	pad := strings.Repeat(" ", gap)
	var out strings.Builder
	for i := 0; i < max; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		out.WriteString(padRight(l, width))
		out.WriteString(pad)
		out.WriteString(sep)
		out.WriteString(pad)
		out.WriteString(r)
		out.WriteString("\n")
	}
	return out.String()
}

func fitToHeight(out string, height int) string {
	if height <= 0 {
		return out
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

func countStatus(list []TaskItem, status string) int {
	count := 0
	for _, t := range list {
		if t.Status == status {
			count++
		}
	}
	return count
}

func renderEmptyState() []string {
	return []string{
		"",
		"Quick start",
		"1) init project: press i",
		"2) new task: press n",
		"3) start task: press s",
	}
}

func (m *Model) filteredTasks() []TaskItem {
	filtered := make([]TaskItem, 0, len(m.TaskList))
	query := strings.ToLower(strings.TrimSpace(m.SearchQuery))
	for _, t := range m.TaskList {
		if !matchesFilter(t, m.FilterMode) {
			continue
		}
		if query != "" {
			hay := strings.ToLower(t.ID + " " + t.Title + " " + t.Status + " " + t.SessionState)
			if !strings.Contains(hay, query) {
				continue
			}
		}
		filtered = append(filtered, t)
	}
	return filtered
}

func matchesFilter(task TaskItem, mode string) bool {
	switch mode {
	case "open":
		return task.Status == "assigned" || task.Status == "todo" || task.Status == "in_progress"
	case "review":
		return task.Status == "review"
	case "done":
		return task.Status == "done"
	default:
		return true
	}
}

func colorize(text, code string) string {
	if !useColor() {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if term == "" || term == "dumb" {
		return false
	}
	return true
}

func maxLineLen(lines []string) int {
	max := 0
	for _, line := range lines {
		if len(line) > max {
			max = len(line)
		}
	}
	return max
}

func rightTabLine(tab RightTab) string {
	if tab == RightTabCoord {
		return "DETAILS | COORD *"
	}
	return "DETAILS * | COORD"
}

func coordScrollWindow(lines []string, selectedLine int, maxLines int, current int) int {
	if maxLines <= 0 || len(lines) <= maxLines {
		return 0
	}
	if current < 0 {
		current = 0
	}
	if current > len(lines)-maxLines {
		current = len(lines) - maxLines
	}
	if selectedLine >= 0 {
		if selectedLine < current {
			current = selectedLine
		}
		if selectedLine >= current+maxLines {
			current = selectedLine - maxLines + 1
		}
	}
	if current < 0 {
		current = 0
	}
	if current > len(lines)-maxLines {
		current = len(lines) - maxLines
	}
	return current
}

func sliceWindow(lines []string, start int, maxLines int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	if start < 0 {
		start = 0
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	return lines[start:end]
}

func (m *Model) RefreshReviewBranches() {
	if m.BranchLookup == nil || len(m.ReviewQueue) == 0 {
		m.ReviewBranches = map[string]string{}
		return
	}
	branches := map[string]string{}
	for _, id := range m.ReviewQueue {
		if branch, err := m.BranchLookup(id); err == nil {
			branches[id] = branch
		}
	}
	m.ReviewBranches = branches
}

func (m *Model) ClampReviewSelection() {
	if len(m.ReviewQueue) == 0 {
		m.SelectedReview = 0
		return
	}
	if m.SelectedReview < 0 {
		m.SelectedReview = 0
		return
	}
	if m.SelectedReview >= len(m.ReviewQueue) {
		m.SelectedReview = len(m.ReviewQueue) - 1
	}
}

func (m *Model) handleReviewEnter() {
	if m.ReviewInputMode == ReviewInputNone {
		m.ensureReviewDetail()
		if len(m.ReviewQueue) == 0 {
			return
		}
		idx := m.SelectedReview
		if idx < 0 || idx >= len(m.ReviewQueue) {
			idx = 0
		}
		taskID := m.ReviewQueue[idx]
		if m.ConfirmApprove {
			m.PendingApproveTask = taskID
			m.SetStatusInfo("confirm approve " + taskID + " (y/n)")
			return
		}
		if err := m.approveTaskByID(taskID); err != nil {
			m.SetStatusError(err.Error())
		}
		return
	}
	if m.ReviewInputMode == ReviewInputFeedback {
		m.ReviewInputMode = ReviewInputNone
		m.ReviewPendingReject = false
		m.SetStatusInfo("feedback captured")
		return
	}
	if m.ReviewInputMode == ReviewInputStory {
		m.ReviewInputMode = ReviewInputNone
		m.SetStatusInfo("story updated")
	}
}

func (m *Model) handleReviewSubmit() {
	if m.ReviewInputMode == ReviewInputNone {
		return
	}
	taskID := m.ReviewDetail.TaskID
	if taskID == "" {
		m.SetStatusError("no review task selected")
		return
	}
	if m.ReviewInputMode == ReviewInputFeedback {
		if m.ReviewDetail.Alignment == "out" && !m.ReviewPendingReject && m.MVPExplainPending {
			m.handleMVPExplainSubmit()
			return
		}
		writer := m.ReviewActionWriter
		if writer == nil {
			writer = func(id, text string) error {
				root, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				path, err := project.TaskSpecPath(root, id)
				if err != nil {
					return err
				}
				return specs.AppendReviewFeedback(path, text)
			}
			m.ReviewActionWriter = writer
		}
		if err := writer(taskID, m.ReviewInput); err != nil {
			m.SetStatusError(err.Error())
			return
		}
		if m.ReviewPendingReject {
			rejecter := m.ReviewRejecter
			if rejecter == nil {
				rejecter = func(id string) error {
					root, err := project.FindRoot(".")
					if err != nil {
						return err
					}
					db, err := storage.OpenShared(project.StateDBPath(root))
					if err != nil {
						return err
					}
					return storage.RejectTask(db, id)
				}
				m.ReviewRejecter = rejecter
			}
			if err := rejecter(taskID); err != nil {
				m.SetStatusError(err.Error())
				return
			}
			m.SetStatusInfo("task rejected + requeued")
		} else {
			m.SetStatusInfo("feedback saved")
		}
		m.ReviewInputMode = ReviewInputNone
		m.ReviewInput = ""
		m.ReviewPendingReject = false
		m.ensureReviewDetail()
		m.RefreshReviewQueue()
		return
	}
	if m.ReviewInputMode == ReviewInputStory {
		updater := m.ReviewStoryUpdater
		if updater == nil {
			updater = func(id, text string) error {
				root, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				path, err := project.TaskSpecPath(root, id)
				if err != nil {
					return err
				}
				return specs.UpdateUserStory(path, text)
			}
			m.ReviewStoryUpdater = updater
		}
		if err := updater(taskID, m.ReviewInput); err != nil {
			m.SetStatusError(err.Error())
			return
		}
		m.ReviewInputMode = ReviewInputNone
		m.ReviewInput = ""
		m.ReviewPendingReject = false
		m.SetStatusInfo("story updated")
		m.ensureReviewDetail()
		return
	}
}

func (m *Model) handleMVPExplainSubmit() {
	if m.ReviewInputMode == ReviewInputNone {
		return
	}
	taskID := m.ReviewDetail.TaskID
	if taskID == "" {
		m.SetStatusError("no review task selected")
		return
	}
	writer := m.MVPExplainWriter
	if writer == nil {
		writer = func(id, text string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			path, err := project.TaskSpecPath(root, id)
			if err != nil {
				return err
			}
			return specs.AppendMVPExplanation(path, text)
		}
		m.MVPExplainWriter = writer
	}
	if err := writer(taskID, m.ReviewInput); err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.ReviewInputMode = ReviewInputNone
	m.ReviewInput = ""
	m.ReviewPendingReject = false
	m.MVPExplainPending = false
	m.SetStatusInfo("mvp explanation saved")
	m.ensureReviewDetail()
}

func (m *Model) handleMVPAccept() {
	acceptor := m.MVPAcceptor
	if acceptor == nil {
		acceptor = func(id string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			path, err := project.TaskSpecPath(root, id)
			if err != nil {
				return err
			}
			return specs.AcknowledgeMVPOverride(path)
		}
		m.MVPAcceptor = acceptor
	}
	taskID := m.ReviewDetail.TaskID
	if taskID == "" {
		m.SetStatusError("no review task selected")
		return
	}
	if err := acceptor(taskID); err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.SetStatusInfo("mvp scope updated")
	m.ensureReviewDetail()
}

func (m *Model) handleMVPRevertStart() {
	if len(m.ReviewDetail.Files) == 0 {
		m.SetStatusInfo("no files to revert")
		return
	}
	m.MVPRevertSelect = true
	m.MVPRevertIndex = 0
}

func (m *Model) handleMVPRevertConfirm() {
	if !m.MVPRevertSelect {
		return
	}
	if m.MVPRevertIndex < 0 || m.MVPRevertIndex >= len(m.ReviewDetail.Files) {
		m.SetStatusError("invalid file selection")
		return
	}
	taskID := m.ReviewDetail.TaskID
	if taskID == "" {
		m.SetStatusError("no review task selected")
		return
	}
	path := m.ReviewDetail.Files[m.MVPRevertIndex].Path
	reverter := m.MVPReverter
	if reverter == nil {
		reverter = func(id, filePath string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			cfg, err := config.LoadFromProject(root)
			if err != nil {
				return err
			}
			runner := &git.ExecRunner{}
			base, err := reviewBaseBranch(cfg, runner)
			if err != nil {
				return err
			}
			branch, err := git.BranchForTask(runner, id)
			if err != nil {
				return err
			}
			if _, err := runner.Run("git", "checkout", branch); err != nil {
				return err
			}
			if err := git.RevertFile(runner, base, filePath); err != nil {
				return err
			}
			if _, err := runner.Run("git", "add", filePath); err != nil {
				return err
			}
			msg := fmt.Sprintf("chore: revert %s for MVP scope", filePath)
			if _, err := runner.Run("git", "commit", "-m", msg); err != nil {
				return err
			}
			return nil
		}
		m.MVPReverter = reverter
	}
	if err := reverter(taskID, path); err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.MVPRevertSelect = false
	m.SetStatusInfo("reverted " + path)
	m.ensureReviewDetail()
}

func (m *Model) RefreshReviewQueue() {
	loader := m.ReviewLoader
	if loader == nil {
		loader = LoadReviewQueueFromProject
		m.ReviewLoader = loader
	}
	queue, err := loader()
	if err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.ReviewQueue = queue
	m.ClampReviewSelection()
	m.RefreshReviewBranches()
}

func (m *Model) handleReviewDiff() {
	if len(m.ReviewQueue) == 0 {
		m.SetStatusInfo("no review tasks")
		return
	}
	idx := m.SelectedReview
	if idx < 0 || idx >= len(m.ReviewQueue) {
		idx = 0
	}
	taskID := m.ReviewQueue[idx]
	loader := m.ReviewDiffLoader
	if loader == nil {
		loader = LoadReviewDiff
		m.ReviewDiffLoader = loader
	}
	state, err := loader(taskID)
	if err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.ReviewDiff = state
	m.ReviewShowDiffs = true
}

func (m *Model) ensureReviewDetail() {
	if len(m.ReviewQueue) == 0 {
		return
	}
	idx := m.SelectedReview
	if idx < 0 || idx >= len(m.ReviewQueue) {
		idx = 0
	}
	taskID := m.ReviewQueue[idx]
	loader := m.ReviewDetailLoader
	if loader == nil {
		loader = LoadReviewDetail
		m.ReviewDetailLoader = loader
	}
	if detail, err := loader(taskID); err == nil {
		m.ReviewDetail = detail
	}
}

func (m *Model) approveTaskByID(taskID string) error {
	approver := m.Approver
	if approver == nil {
		approver = &ApproveAdapter{}
		m.Approver = approver
	}
	lookup := m.BranchLookup
	if lookup == nil {
		lookup = func(taskID string) (string, error) {
			return git.BranchForTask(&git.ExecRunner{}, taskID)
		}
		m.BranchLookup = lookup
	}
	branch, err := lookup(taskID)
	if err != nil {
		return fmt.Errorf("branch lookup failed: %w", err)
	}
	if err := m.ApproveTask(approver, taskID, branch); err != nil {
		return fmt.Errorf("approve failed: %w", err)
	}
	loader := m.ReviewLoader
	if loader == nil {
		loader = LoadReviewQueueFromProject
		m.ReviewLoader = loader
	}
	queue, err := loader()
	if err != nil {
		return fmt.Errorf("review refresh failed: %w", err)
	}
	m.ReviewQueue = queue
	m.ClampReviewSelection()
	m.RefreshReviewBranches()
	m.SetStatusInfo("approved " + taskID + " (merged " + branch + ")")
	return nil
}

func (m *Model) SetStatus(level StatusLevel, message string) {
	m.StatusLevel = level
	m.Status = message
}

func (m *Model) SetStatusError(message string) {
	m.SetStatus(StatusError, message)
}

func (m *Model) SetStatusInfo(message string) {
	m.SetStatus(StatusInfo, message)
}

func (m *Model) selectedTask() (TaskItem, bool) {
	filtered := m.filteredTasks()
	if len(filtered) == 0 {
		return TaskItem{}, false
	}
	idx := m.SelectedTask
	if idx < 0 || idx >= len(filtered) {
		idx = 0
	}
	return filtered[idx], true
}

func (m *Model) handleTaskStart() {
	task, ok := m.selectedTask()
	if !ok {
		m.SetStatusInfo("no tasks to start")
		return
	}
	taskID := task.ID
	starter := m.TaskStarter
	if starter == nil {
		starter = func(id string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			if err := os.MkdirAll(project.WorktreesDir(root), 0o755); err != nil {
				return err
			}
			worktree := filepath.Join(project.WorktreesDir(root), id)
			branch := "feature/" + id
			if err := git.CreateWorktree(root, worktree, branch); err != nil {
				return err
			}
			logPath := filepath.Join(project.SessionsDir(root), agent.SessionID(id)+".log")
			session := tmux.Session{ID: agent.SessionID(id), Workdir: worktree, LogPath: logPath}
			if err := tmux.StartSession(&tmux.ExecRunner{}, session); err != nil {
				return err
			}
			db, err := storage.OpenShared(project.StateDBPath(root))
			if err != nil {
				return err
			}
			if err := storage.Migrate(db); err != nil {
				return err
			}
			if err := storage.UpdateTaskStatus(db, id, "in_progress"); err != nil {
				return err
			}
			_ = storage.InsertSession(db, storage.Session{ID: session.ID, TaskID: id, State: "working", Offset: 0})
			return nil
		}
		m.TaskStarter = starter
	}
	if err := starter(taskID); err != nil {
		m.SetStatusError(err.Error())
		return
	}
	for i := range m.TaskList {
		if m.TaskList[i].ID == taskID {
			m.TaskList[i].Status = "in_progress"
		}
	}
	m.SetStatusInfo("started " + taskID)
}

func (m *Model) handleTaskStop() {
	task, ok := m.selectedTask()
	if !ok {
		m.SetStatusInfo("no tasks to stop")
		return
	}
	taskID := task.ID
	stopper := m.TaskStopper
	if stopper == nil {
		stopper = func(id string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			db, err := storage.OpenShared(project.StateDBPath(root))
			if err != nil {
				return err
			}
			if err := storage.Migrate(db); err != nil {
				return err
			}
			sessionID := agent.SessionID(id)
			if session, err := storage.FindSessionByTask(db, id); err == nil {
				sessionID = session.ID
			}
			if err := tmux.StopSession(&tmux.ExecRunner{}, sessionID); err != nil {
				return err
			}
			_ = storage.UpdateSessionState(db, sessionID, "stopped")
			if err := storage.UpdateTaskStatus(db, id, "blocked"); err != nil {
				return err
			}
			return nil
		}
		m.TaskStopper = stopper
	}
	if err := stopper(taskID); err != nil {
		m.SetStatusError(err.Error())
		return
	}
	for i := range m.TaskList {
		if m.TaskList[i].ID == taskID {
			m.TaskList[i].Status = "blocked"
		}
	}
	m.SetStatusInfo("stopped " + taskID)
}

func (m *Model) handleTaskReview() {
	task, ok := m.selectedTask()
	if !ok {
		m.SetStatusInfo("no tasks to review")
		return
	}
	if task.Status != "review" {
		m.SetStatusInfo("task not ready for review")
		return
	}
	m.ViewMode = ViewReview
	m.ensureReviewDetail()
}

func (m *Model) RefreshTasks() {
	loader := m.TaskLoader
	if loader == nil {
		loader = LoadTasksFromProject
		m.TaskLoader = loader
	}
	list, err := loader()
	if err != nil {
		m.SetStatusError(err.Error())
		return
	}
	m.TaskList = list
	m.ClampTaskSelection()
	m.ensureTaskDetail()
}

func (m *Model) RefreshTaskDetail() {
	task, ok := m.selectedTask()
	if !ok {
		m.TaskDetail = TaskDetail{}
		return
	}
	loader := m.TaskDetailLoader
	if loader == nil {
		loader = LoadTaskDetail
		m.TaskDetailLoader = loader
	}
	detail, err := loader(task.ID)
	if err != nil {
		m.TaskDetail = TaskDetail{
			ID:           task.ID,
			Title:        task.Title,
			Status:       task.Status,
			SessionState: task.SessionState,
		}
		return
	}
	if detail.Status == "" {
		detail.Status = task.Status
	}
	if detail.SessionState == "" {
		detail.SessionState = task.SessionState
	}
	m.TaskDetail = detail
}

func (m *Model) RefreshCoordination() {
	if m.RightTab != RightTabCoord {
		return
	}
	if strings.TrimSpace(m.CoordRecipient) == "" {
		m.CoordInbox = nil
	} else {
		inboxLoader := m.CoordInboxLoader
		if inboxLoader == nil {
			inboxLoader = LoadCoordInboxFromProject
			m.CoordInboxLoader = inboxLoader
		}
		inbox, err := inboxLoader(m.CoordRecipient, 6, m.CoordUrgentOnly)
		if err != nil {
			m.SetStatusError(err.Error())
		} else {
			m.CoordInbox = inbox
		}
	}
	locksLoader := m.CoordLocksLoader
	if locksLoader == nil {
		locksLoader = LoadCoordLocksFromProject
		m.CoordLocksLoader = locksLoader
	}
	locks, err := locksLoader(6)
	if err != nil {
		m.SetStatusError(err.Error())
	} else {
		m.CoordLocks = locks
	}
	m.ClampCoordSelection()
	m.adjustCoordScroll()
}

func (m *Model) ClampCoordSelection() {
	total := m.coordSelectableCount()
	if total <= 0 {
		m.CoordSelected = 0
		return
	}
	if m.CoordSelected < 0 {
		m.CoordSelected = 0
	} else if m.CoordSelected >= total {
		m.CoordSelected = total - 1
	}
}

func (m *Model) moveCoordSelection(delta int) {
	total := m.coordSelectableCount()
	if total <= 0 {
		return
	}
	next := m.CoordSelected + delta
	if next < 0 {
		next = 0
	} else if next >= total {
		next = total - 1
	}
	if next != m.CoordSelected {
		m.CoordSelected = next
		m.adjustCoordScroll()
	}
}

func (m *Model) coordSelectableCount() int {
	count := 0
	inbox := m.filteredCoordInbox()
	if len(inbox) > 0 {
		count += len(inbox)
	}
	locks := m.filteredCoordLocks()
	if len(locks) > 0 {
		count += len(locks)
	}
	return count
}

func (m *Model) coordBodyLines() ([]string, int) {
	lines := []string{}
	selectedLine := -1
	itemIndex := 0
	selectedMsg, selectedLock := m.coordSelectedItem()
	inbox := m.filteredCoordInbox()
	locks := m.filteredCoordLocks()
	lines = append(lines, fmt.Sprintf("COORD: inbox=%d locks=%d urgent=%s recipient=%s", len(inbox), len(locks), coordOnOff(m.CoordUrgentOnly), m.coordRecipientFilterLabel()))
	lines = append(lines, "INBOX")
	if strings.TrimSpace(m.CoordRecipient) == "" {
		lines = append(lines, "  (set TAND_MAIL_RECIPIENT)")
	} else {
		if len(inbox) == 0 {
			lines = append(lines, "  - no messages")
		} else {
			for _, msg := range inbox {
				prefix := "  "
				if itemIndex == m.CoordSelected {
					prefix = "> "
					selectedLine = len(lines)
				}
				lines = append(lines, prefix+fmt.Sprintf("%s: %s", msg.Message.Sender, msg.Message.Subject))
				itemIndex++
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, "LOCKS")
	if len(locks) == 0 {
		lines = append(lines, "  - none")
	} else {
		for _, lock := range locks {
			prefix := "  "
			if itemIndex == m.CoordSelected {
				prefix = "> "
				selectedLine = len(lines)
			}
			lines = append(lines, prefix+fmt.Sprintf("%s (%s)", lock.Path, lock.Owner))
			itemIndex++
		}
	}
	lines = append(lines, "")
	lines = append(lines, "PREVIEW")
	if selectedMsg != nil {
		lines = append(lines, "  Subject: "+coordSnippet(selectedMsg.Message.Subject, 64))
		lines = append(lines, "  From: "+coordSnippet(selectedMsg.Message.Sender, 64))
		lines = append(lines, "  Body: "+coordSnippet(selectedMsg.Message.Body, 64))
	} else if selectedLock != nil {
		lines = append(lines, "  Lock: "+coordSnippet(selectedLock.Path, 64))
		lines = append(lines, "  Owner: "+coordSnippet(selectedLock.Owner, 64))
		lines = append(lines, "  Reason: "+coordSnippet(selectedLock.Reason, 64))
	} else {
		lines = append(lines, "  - none")
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Filters: urgent=%s recipient=%s", coordOnOff(m.CoordUrgentOnly), m.coordRecipientFilterLabel()))
	lines = append(lines, "Hints: u urgent, r recipient, tand mail inbox")
	return lines, selectedLine
}

func (m *Model) filteredCoordInbox() []storage.MessageDelivery {
	inbox := m.CoordInbox
	if len(inbox) == 0 {
		return inbox
	}
	filter := m.coordRecipientFilter()
	recipient := strings.TrimSpace(m.CoordRecipient)
	filtered := make([]storage.MessageDelivery, 0, len(inbox))
	for _, msg := range inbox {
		if m.CoordUrgentOnly && !coordIsUrgent(msg.Message.Importance) {
			continue
		}
		if filter == CoordRecipientFilterMentions && !coordMentionsRecipient(msg.Message, recipient) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func (m *Model) filteredCoordLocks() []storage.Reservation {
	locks := m.CoordLocks
	if len(locks) == 0 {
		return locks
	}
	filter := m.coordRecipientFilter()
	if filter == CoordRecipientFilterAll {
		return locks
	}
	recipient := strings.TrimSpace(m.CoordRecipient)
	if recipient == "" {
		return nil
	}
	filtered := make([]storage.Reservation, 0, len(locks))
	for _, lock := range locks {
		if strings.EqualFold(lock.Owner, recipient) {
			filtered = append(filtered, lock)
		}
	}
	return filtered
}

func (m *Model) coordRecipientFilter() CoordRecipientFilter {
	if m.CoordRecipientFilter == "" {
		return CoordRecipientFilterAll
	}
	return m.CoordRecipientFilter
}

func (m *Model) coordSelectedItem() (*storage.MessageDelivery, *storage.Reservation) {
	index := m.CoordSelected
	if index < 0 {
		return nil, nil
	}
	inbox := m.filteredCoordInbox()
	if index < len(inbox) {
		item := inbox[index]
		return &item, nil
	}
	index -= len(inbox)
	locks := m.filteredCoordLocks()
	if index >= 0 && index < len(locks) {
		item := locks[index]
		return nil, &item
	}
	return nil, nil
}

func (m *Model) coordRecipientFilterLabel() string {
	switch m.coordRecipientFilter() {
	case CoordRecipientFilterMe:
		return "me"
	case CoordRecipientFilterMentions:
		return "@mentions"
	default:
		return "all"
	}
}

func (m *Model) cycleCoordRecipientFilter() {
	switch m.coordRecipientFilter() {
	case CoordRecipientFilterAll:
		m.CoordRecipientFilter = CoordRecipientFilterMe
	case CoordRecipientFilterMe:
		m.CoordRecipientFilter = CoordRecipientFilterMentions
	default:
		m.CoordRecipientFilter = CoordRecipientFilterAll
	}
	m.ClampCoordSelection()
	m.adjustCoordScroll()
}

func coordOnOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func coordSnippet(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if strings.Contains(value, "\n") {
		value = strings.SplitN(value, "\n", 2)[0]
	}
	if max <= 0 {
		return value
	}
	if len(value) > max {
		if max <= 3 {
			return value[:max]
		}
		return value[:max-3] + "..."
	}
	return value
}

func (m *Model) topBarLine() string {
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = "Praude"
	}
	prd := strings.TrimSpace(m.CurrentPRD)
	if prd == "" {
		prd = "-"
	}
	repo := strings.TrimSpace(m.RepoPath)
	if repo == "" {
		repo = "-"
	}
	status := "-"
	if len(m.StatusBadges) > 0 {
		status = strings.Join(m.StatusBadges, ",")
	}
	return fmt.Sprintf("%s | PRD: %s | repo: %s | status: %s", title, prd, repo, status)
}

func (m *Model) bottomBarLine() string {
	status := strings.TrimSpace(m.Status)
	if status == "" {
		status = "-"
	}
	if m.StatusLevel == StatusError && status != "-" {
		status = "ERROR: " + status
	}
	return fmt.Sprintf("MODE: %s | FOCUS: %s | STATUS: %s", m.modeLabel(), m.focusLabel(), status)
}

func (m *Model) modeLabel() string {
	switch m.ViewMode {
	case ViewReview:
		return "REVIEW"
	default:
		return "VIEW"
	}
}

func (m *Model) focusLabel() string {
	if m.FocusPane == FocusDetail {
		return "SIDE"
	}
	return "DOC"
}

func coordIsUrgent(importance string) bool {
	return strings.EqualFold(strings.TrimSpace(importance), "urgent")
}

func coordMentionsRecipient(msg storage.Message, recipient string) bool {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return false
	}
	needle := "@" + strings.ToLower(recipient)
	subject := strings.ToLower(msg.Subject)
	body := strings.ToLower(msg.Body)
	return strings.Contains(subject, needle) || strings.Contains(body, needle)
}

func (m *Model) adjustCoordScroll() {
	lines, selectedLine := m.coordBodyLines()
	maxLines := m.coordMaxLines()
	m.CoordScroll = coordScrollWindow(lines, selectedLine, maxLines, m.CoordScroll)
}

func (m *Model) coordMaxLines() int {
	if m.Height <= 0 {
		return 12
	}
	max := m.Height - 10
	if max < 6 {
		return 6
	}
	return max
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) ensureTaskDetail() {
	task, ok := m.selectedTask()
	if !ok {
		m.TaskDetail = TaskDetail{}
		return
	}
	if m.TaskDetail.ID == task.ID {
		return
	}
	loader := m.TaskDetailLoader
	if loader == nil {
		loader = LoadTaskDetail
		m.TaskDetailLoader = loader
	}
	detail, err := loader(task.ID)
	if err != nil {
		m.TaskDetail = TaskDetail{
			ID:           task.ID,
			Title:        task.Title,
			Status:       task.Status,
			SessionState: task.SessionState,
		}
		return
	}
	if detail.Status == "" {
		detail.Status = task.Status
	}
	if detail.SessionState == "" {
		detail.SessionState = task.SessionState
	}
	m.TaskDetail = detail
}

func (m *Model) ClampTaskSelection() {
	filtered := m.filteredTasks()
	if len(filtered) == 0 {
		m.SelectedTask = 0
		return
	}
	if m.SelectedTask < 0 {
		m.SelectedTask = 0
		return
	}
	if m.SelectedTask >= len(filtered) {
		m.SelectedTask = len(filtered) - 1
	}
}

func (m *Model) LoadDiffs(r git.Runner, rev string) error {
	files, err := LoadDiffFiles(r, rev)
	if err != nil {
		return err
	}
	m.DiffFiles = files
	return nil
}
