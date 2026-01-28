package views

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// KickoffView is the initial view for starting new projects or resuming drafts.
// It uses a Cursor-style split layout with document panel (left) and chat panel (right).
type KickoffView struct {
	// Shared components for Cursor-style layout
	chatPanel   *pkgtui.ChatPanel
	docPanel    *pkgtui.DocPanel
	splitLayout *pkgtui.SplitLayout

	recents    []RecentProject
	selected   int
	focusInput bool // true = chat panel focused, false = recents focused
	width      int
	height     int
	loading    bool
	loadingMsg string
	err        error

	// Delete confirmation state
	confirmingDelete bool
	deleteTarget     *RecentProject

	// Scan state
	scanning       bool
	scanResult     *tui.CodebaseScanResultMsg // Stored scan result for passing to interview
	scanPath       string                     // Path being scanned
	scanFiles      []string                   // Files found during scan
	scanAgentName  string                     // Name of agent being used
	scanAgentLines []string                   // Recent lines of agent output

	// Callbacks for navigation
	onProjectStart func(project *Project) tea.Cmd
	onScanCodebase func(path string) tea.Cmd
}

// RecentProject represents a project that can be resumed or continued.
type RecentProject struct {
	ID       string
	Name     string
	Status   string // "draft", "complete"
	LastOpen time.Time
	Path     string
}

// Project represents a new or existing project.
type Project struct {
	ID          string
	Name        string
	Description string
	Path        string
	CreatedAt   time.Time
	// Pre-populated answers from codebase scan (optional)
	ScanResult *tui.CodebaseScanResultMsg
}

// NewKickoffView creates a new kickoff view with Cursor-style split layout.
func NewKickoffView() *KickoffView {
	// Create shared components
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Describe what you want to build...")
	chatPanel.SetComposerHint("ctrl+g: create  ctrl+s: scan")

	docPanel := pkgtui.NewDocPanel()
	docPanel.SetTitle("What do you want to build?")
	docPanel.SetSubtitle("Describe your project vision and goals")

	splitLayout := pkgtui.NewSplitLayout(0.66) // 2/3 left (doc), 1/3 right (chat) - Cursor style
	splitLayout.SetMinWidth(100)

	v := &KickoffView{
		chatPanel:   chatPanel,
		docPanel:    docPanel,
		splitLayout: splitLayout,
		focusInput:  true,
	}
	v.seedChat()
	v.updateDocPanel()

	return v
}

// SetAgentSelector sets the shared agent selector.
func (v *KickoffView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
}

// seedChat resets the chat history with kickoff guidance.
func (v *KickoffView) seedChat() {
	v.chatPanel.ClearMessages()
	v.chatPanel.AddMessage("system", "What do you want to build?")
	v.chatPanel.AddMessage("system", "Tips:\n• Be specific about what you're building\n• Include key features or requirements\n• Mention any constraints or preferences")
	v.chatPanel.AddMessage("system", "Shortcuts:\n• Ctrl+G → Create project\n• Ctrl+S → Scan current directory\n• Tab → Switch between panels\n• F2 → Agent selector")
}

// ChatMessagesForTest exposes chat history for tests.
func (v *KickoffView) ChatMessagesForTest() []pkgtui.ChatMessage {
	return v.chatPanel.Messages()
}

// SetProjectStartCallback sets the callback for when a project is started.
func (v *KickoffView) SetProjectStartCallback(cb func(*Project) tea.Cmd) {
	v.onProjectStart = cb
}

// SetScanCodebaseCallback sets the callback for when codebase scanning is requested.
func (v *KickoffView) SetScanCodebaseCallback(cb func(path string) tea.Cmd) {
	v.onScanCodebase = cb
}

// SetAgentName sets the name of the agent being used for display.
func (v *KickoffView) SetAgentName(name string) {
	v.scanAgentName = name
}

// updateDocPanel updates the document panel with current content.
func (v *KickoffView) updateDocPanel() {
	v.docPanel.ClearSections()

	v.docPanel.AddSection(pkgtui.DocSection{
		Title:   "Autarch",
		Content: "Autarch is a platform for a suite of agentic tools to help you build better products. Use the chat panel on the right to start creating a PRD that will provide a solid foundation to build upon.",
		Style:   lipgloss.NewStyle().Foreground(pkgtui.ColorFg),
	})

	// Add tips section
	v.docPanel.AddSection(pkgtui.DocSection{
		Title:   "Tips",
		Content: "• Be specific about what you're building\n• Include key features or requirements\n• Mention any constraints or preferences",
		Style:   lipgloss.NewStyle().Foreground(pkgtui.ColorMuted),
	})

	// Add keyboard shortcuts section
	v.docPanel.AddSection(pkgtui.DocSection{
		Title:   "Shortcuts",
		Content: "Ctrl+G → Create project\nCtrl+S → Scan current directory\nTab → Switch between panels",
		Style:   lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim),
	})

	// If we have a scan result, show it
	if v.scanResult != nil {
		techInfo := ""
		if v.scanResult.Language != "" {
			techInfo = v.scanResult.Language
		}
		if v.scanResult.Platform != "" {
			if techInfo != "" {
				techInfo += " / "
			}
			techInfo += v.scanResult.Platform
		}
		v.docPanel.AddSection(pkgtui.DocSection{
			Title:   "Scanned Project",
			Content: fmt.Sprintf("Found: %s\nTech: %s", v.scanPath, techInfo),
			Style:   lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess),
		})
	}
}

// Init implements View
func (v *KickoffView) Init() tea.Cmd {
	return tea.Batch(
		v.chatPanel.Focus(),
		v.loadRecentProjects(),
	)
}

// recentsLoadedMsg is sent when recent projects are loaded.
type recentsLoadedMsg struct {
	recents []RecentProject
	err     error
}

// projectCreatedMsg is sent when a new project is created.
type projectCreatedMsg struct {
	project *Project
	err     error
}

// projectDeletedMsg is sent when a project is deleted.
type projectDeletedMsg struct {
	projectID string
	err       error
}

func (v *KickoffView) loadRecentProjects() tea.Cmd {
	return func() tea.Msg {
		recents, err := loadRecentProjectsFromDisk()
		return recentsLoadedMsg{recents: recents, err: err}
	}
}

// loadRecentProjectsFromDisk reads recent projects from ~/.autarch/projects/
func loadRecentProjectsFromDisk() ([]RecentProject, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(home, ".autarch", "projects")
	entries, err := os.ReadDir(projectsDir)
	if os.IsNotExist(err) {
		return nil, nil // No projects yet
	}
	if err != nil {
		return nil, err
	}

	var recents []RecentProject
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(projectsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Try to read project metadata
		status := "complete"
		metaPath := filepath.Join(projectPath, "meta.json")
		if _, err := os.Stat(filepath.Join(projectPath, "draft.json")); err == nil {
			status = "draft"
		}

		// Use directory name as project name
		name := entry.Name()
		if metaData, err := os.ReadFile(metaPath); err == nil {
			// Could parse JSON for better name, but keep it simple
			_ = metaData
		}

		recents = append(recents, RecentProject{
			ID:       entry.Name(),
			Name:     name,
			Status:   status,
			LastOpen: info.ModTime(),
			Path:     projectPath,
		})
	}

	// Sort by last open time, most recent first
	sort.Slice(recents, func(i, j int) bool {
		return recents[i].LastOpen.After(recents[j].LastOpen)
	})

	// Limit to 10 most recent
	if len(recents) > 10 {
		recents = recents[:10]
	}

	return recents, nil
}

// Update implements View
func (v *KickoffView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Account for unified_app's content padding (Padding(1, 3) = 6 horizontal, 2 vertical)
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.splitLayout.SetSize(v.width, v.height)
		v.docPanel.SetSize(v.splitLayout.LeftWidth(), v.splitLayout.LeftHeight())
		v.chatPanel.SetSize(v.splitLayout.RightWidth(), v.splitLayout.RightHeight())
		return v, nil

	case recentsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.recents = msg.recents
		}
		return v, nil

	case projectCreatedMsg:
		v.loading = false
		v.scanning = false
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		if v.onProjectStart != nil {
			return v, v.onProjectStart(msg.project)
		}
		return v, nil

	case tui.ScanProgressMsg:
		// Update agent output display
		if msg.AgentLine != "" {
			// Keep last 8 lines
			v.scanAgentLines = append(v.scanAgentLines, msg.AgentLine)
			if len(v.scanAgentLines) > 8 {
				v.scanAgentLines = v.scanAgentLines[len(v.scanAgentLines)-8:]
			}
		}
		// Update step info
		if msg.Step != "" && msg.Step != "Analyzing" {
			v.loadingMsg = msg.Details
		}
		if len(msg.Files) > 0 {
			v.scanFiles = msg.Files
		}
		return v, nil

	case tui.CodebaseScanResultMsg:
		v.loading = false
		v.scanning = false
		v.scanAgentLines = nil // Clear agent output
		if msg.Error != nil {
			v.err = msg.Error
			return v, nil
		}
		// Store scan result and pre-fill the description
		v.scanResult = &msg
		v.chatPanel.SetValue(msg.Description)
		v.updateDocPanel()
		return v, nil

	case projectDeletedMsg:
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		// Remove from the recents list
		for i, r := range v.recents {
			if r.ID == msg.projectID {
				v.recents = append(v.recents[:i], v.recents[i+1:]...)
				break
			}
		}
		// Adjust selection if needed
		if v.selected >= len(v.recents) {
			v.selected = len(v.recents) - 1
		}
		if v.selected < 0 {
			v.selected = 0
		}
		// If no more recents, switch focus to input
		if len(v.recents) == 0 {
			v.focusInput = true
			return v, v.chatPanel.Focus()
		}
		return v, nil

	case tea.KeyMsg:
		// Handle delete confirmation first
		if v.confirmingDelete {
			switch {
			case msg.String() == "y" || msg.String() == "Y":
				// Confirmed - delete the project
				if v.deleteTarget != nil {
					target := *v.deleteTarget
					v.confirmingDelete = false
					v.deleteTarget = nil
					return v, v.deleteProject(target)
				}
				v.confirmingDelete = false
				v.deleteTarget = nil
				return v, nil
			case msg.String() == "n" || msg.String() == "N" || key.Matches(msg, commonKeys.Back):
				// Cancelled
				v.confirmingDelete = false
				v.deleteTarget = nil
				return v, nil
			}
			// Ignore other keys during confirmation
			return v, nil
		}

		// Pass most keys to input if focused
		if v.focusInput {
			switch {
			case key.Matches(msg, commonKeys.TabCycle):
				// Toggle focus to recents
				if len(v.recents) > 0 {
					v.focusInput = false
					v.chatPanel.Blur()
				}
				return v, nil

			case msg.String() == "ctrl+g":
				// Submit the project description (ctrl+g = "go")
				val := v.chatPanel.Value()
				if strings.TrimSpace(val) != "" {
					v.loading = true
					v.loadingMsg = "Creating project..."
					return v, v.createProject(val)
				}
				// Empty input — show hint
				v.chatPanel.SetComposerHint("Type a description first, then ctrl+g")
				return v, nil

			case msg.String() == "ctrl+s":
				// Scan current directory
				if v.onScanCodebase != nil {
					cwd, _ := os.Getwd()
					v.scanning = true
					v.loading = true
					v.scanPath = cwd
					v.scanFiles = findProjectFiles(cwd)
					v.loadingMsg = "Scanning codebase..."
					// Detect which agent will be used
					if detected, err := agent.DetectAgent(); err == nil && detected != nil {
						v.scanAgentName = string(detected.Type)
					}
					return v, v.onScanCodebase(cwd)
				}
				return v, nil

			case key.Matches(msg, commonKeys.Back):
				// If there's content, clear focus; otherwise do nothing
				if len(v.recents) > 0 {
					v.focusInput = false
					v.chatPanel.Blur()
				}
				return v, nil

			default:
				// Pass all other keys to the composer (including Enter for newlines)
				var cmd tea.Cmd
				v.chatPanel, cmd = v.chatPanel.Update(msg)
				return v, cmd
			}
		}

		// Recents list is focused - handle navigation
		switch {
		case key.Matches(msg, commonKeys.TabCycle):
			// Toggle focus to input
			v.focusInput = true
			return v, v.chatPanel.Focus()

		case key.Matches(msg, commonKeys.NavUp):
			if v.selected > 0 {
				v.selected--
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavDown):
			if v.selected < len(v.recents)-1 {
				v.selected++
			}
			return v, nil

		case key.Matches(msg, commonKeys.Select):
			// Enter on a selected project opens it
			if len(v.recents) > 0 {
				recent := v.recents[v.selected]
				project := &Project{
					ID:        recent.ID,
					Name:      recent.Name,
					Path:      recent.Path,
					CreatedAt: recent.LastOpen,
				}
				if v.onProjectStart != nil {
					return v, v.onProjectStart(project)
				}
			}
			return v, nil

		case msg.String() == "d" || msg.String() == "delete":
			// Show delete confirmation
			if len(v.recents) > 0 && v.selected >= 0 && v.selected < len(v.recents) {
				v.confirmingDelete = true
				v.deleteTarget = &v.recents[v.selected]
			}
			return v, nil
		}
	}

	return v, nil
}

func (v *KickoffView) createProject(description string) tea.Cmd {
	// Capture scan result before the goroutine
	scanResult := v.scanResult

	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return projectCreatedMsg{err: err}
		}

		// Generate project ID and slug
		projectID := uuid.New().String()
		slug := slugify(description)
		if len(slug) > 30 {
			slug = slug[:30]
		}
		slug = fmt.Sprintf("%s-%s", slug, projectID[:8])

		projectPath := filepath.Join(home, ".autarch", "projects", slug)
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return projectCreatedMsg{err: err}
		}

		project := &Project{
			ID:          projectID,
			Name:        slug,
			Description: description,
			Path:        projectPath,
			CreatedAt:   time.Now(),
			ScanResult:  scanResult,
		}

		return projectCreatedMsg{project: project}
	}
}

func (v *KickoffView) deleteProject(recent RecentProject) tea.Cmd {
	return func() tea.Msg {
		// Delete the project directory
		if recent.Path != "" {
			if err := os.RemoveAll(recent.Path); err != nil {
				return projectDeletedMsg{projectID: recent.ID, err: err}
			}
		}
		return projectDeletedMsg{projectID: recent.ID}
	}
}

// findProjectFiles looks for relevant project files and returns their names.
func findProjectFiles(path string) []string {
	priorities := []string{
		"README.md",
		"README",
		"readme.md",
		"CLAUDE.md",
		"AGENTS.md",
		"docs/README.md",
		"docs/index.md",
		"PRD.md",
		"SPEC.md",
		"package.json",
		"go.mod",
		"Cargo.toml",
		"pyproject.toml",
		"requirements.txt",
	}

	var found []string
	for _, f := range priorities {
		fullPath := filepath.Join(path, f)
		if _, err := os.Stat(fullPath); err == nil {
			found = append(found, f)
		}
	}
	return found
}

// slugify converts a description to a URL-friendly slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)

	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")

	return s
}

// View implements View
func (v *KickoffView) View() string {
	if v.err != nil {
		return tui.ErrorView(v.err)
	}

	// Cursor-style split layout:
	// Left pane (2/3): Main document view - shows scan progress and results
	// Right pane (1/3): Chat panel for conversation/input
	leftContent := v.docPanel.View()
	rightContent := v.renderRightPane()

	if v.loading {
		// During loading, show progress in the LEFT (main document) pane
		// The right chat pane remains available for the user
		leftContent = v.renderScanProgressPane()
	}

	return v.splitLayout.Render(leftContent, rightContent)
}

// renderScanProgressPane renders the left (main document) pane during scanning.
// Shows scan progress, files found, and agent output in the main view area.
func (v *KickoffView) renderScanProgressPane() string {
	var sections []string

	spinnerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)
	msg := v.loadingMsg
	if msg == "" {
		msg = "Loading..."
	}
	sections = append(sections, spinnerStyle.Render(msg))

	// Show more details during scanning
	if v.scanning {
		detailStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorMuted).
			Italic(true)
		pathStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorSecondary)
		fileStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorSuccess)
		agentStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorPrimary).
			Bold(true)

		sections = append(sections, "")
		sections = append(sections, detailStyle.Render("Path: ")+pathStyle.Render(v.scanPath))
		sections = append(sections, "")

		// Show files found
		if len(v.scanFiles) > 0 {
			sections = append(sections, detailStyle.Render("Files found:"))
			for _, f := range v.scanFiles {
				sections = append(sections, "  "+fileStyle.Render("✓ "+f))
			}
		} else {
			sections = append(sections, detailStyle.Render("No project files found"))
		}

		sections = append(sections, "")
		agentName := v.scanAgentName
		if agentName == "" {
			agentName = "coding agent"
		}
		sections = append(sections, detailStyle.Render("Analyzing with ")+agentStyle.Render(agentName)+detailStyle.Render("..."))

		// Show live agent output
		if len(v.scanAgentLines) > 0 {
			sections = append(sections, "")
			outputStyle := lipgloss.NewStyle().
				Foreground(pkgtui.ColorFgDim).
				Padding(0, 1).
				Width(min(70, v.width-8))
			// Removed: .Background(pkgtui.ColorBgLight) - causes blue bar artifact

			// Show agent output in a box
			var outputLines []string
			for _, line := range v.scanAgentLines {
				// Truncate long lines
				if len(line) > 66 {
					line = line[:63] + "..."
				}
				outputLines = append(outputLines, line)
			}
			outputBox := outputStyle.Render(strings.Join(outputLines, "\n"))
			sections = append(sections, outputBox)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderRightPane renders the right side: composer with recents below.
func (v *KickoffView) renderRightPane() string {
	var sections []string

	// Composer for project description
	sections = append(sections, v.chatPanel.View())

	// Recent projects section (if any)
	if len(v.recents) > 0 {
		sections = append(sections, "")

		recentHeaderStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorSecondary).
			Bold(true)
		sections = append(sections, recentHeaderStyle.Render("Recent Projects"))

		// Recent projects list
		var recentLines []string
		maxRecents := 5 // Limit to avoid overflow
		displayRecents := v.recents
		if len(displayRecents) > maxRecents {
			displayRecents = displayRecents[:maxRecents]
		}
		for i, r := range displayRecents {
			line := v.renderRecentProject(r, i == v.selected && !v.focusInput)
			recentLines = append(recentLines, line)
		}

		recentsContent := strings.Join(recentLines, "\n")
		recentsStyle := lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder())

		if !v.focusInput {
			recentsStyle = recentsStyle.BorderForeground(pkgtui.ColorPrimary)
		} else {
			recentsStyle = recentsStyle.BorderForeground(pkgtui.ColorMuted)
		}

		sections = append(sections, recentsStyle.Render(recentsContent))
	}

	// Delete confirmation
	if v.confirmingDelete && v.deleteTarget != nil {
		sections = append(sections, "")
		confirmBox := lipgloss.NewStyle().
			Background(pkgtui.ColorBgLight).
			Foreground(pkgtui.ColorWarning).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(pkgtui.ColorWarning)
		sections = append(sections, confirmBox.Render(
			fmt.Sprintf("Delete \"%s\"? y/n", v.deleteTarget.Name),
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (v *KickoffView) renderRecentProject(r RecentProject, selected bool) string {
	// Status icon
	var icon string
	var iconStyle lipgloss.Style
	if r.Status == "draft" {
		icon = "◐"
		iconStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
	} else {
		icon = "●"
		iconStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	}

	// Time ago
	timeAgo := timeAgoString(r.LastOpen)
	timeStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)

	if selected {
		// Selected row - subtle highlight
		selectedStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorPrimary).
			Bold(true)
		selectorStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorPrimary)

		return fmt.Sprintf("%s %s %s  %s",
			selectorStyle.Render("›"),
			iconStyle.Render(icon),
			selectedStyle.Render(r.Name),
			timeStyle.Render(timeAgo),
		)
	}

	// Unselected row
	nameStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)
	return fmt.Sprintf("  %s %s  %s",
		iconStyle.Render(icon),
		nameStyle.Render(r.Name),
		timeStyle.Render(timeAgo),
	)
}

func timeAgoString(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// Focus implements View
func (v *KickoffView) Focus() tea.Cmd {
	v.focusInput = true
	v.seedChat()
	return v.chatPanel.Focus()
}

// Blur implements View
func (v *KickoffView) Blur() {
	v.chatPanel.Blur()
}

// Name implements View
func (v *KickoffView) Name() string {
	return "Kickoff"
}

// ShortHelp implements View
func (v *KickoffView) ShortHelp() string {
	if v.focusInput {
		if v.onScanCodebase != nil {
			return "ctrl+g create  ctrl+s scan  F2 agent  tab switch"
		}
		return "ctrl+g create  F2 agent  tab switch"
	}
	// Recents list focused
	return "enter open  d delete  F2 agent  tab switch"
}

// FullHelp implements FullHelpProvider
func (v *KickoffView) FullHelp() []tui.HelpBinding {
	return []tui.HelpBinding{
		{Key: "ctrl+g", Description: "Create new project from description"},
		{Key: "ctrl+s", Description: "Scan current directory for existing project"},
		{Key: "tab", Description: "Switch between input and recent projects"},
		{Key: "j/k", Description: "Navigate recent projects list"},
		{Key: "enter", Description: "Open selected project"},
		{Key: "d", Description: "Delete selected project"},
		{Key: "esc", Description: "Switch to recent projects list"},
	}
}

// Commands implements CommandProvider
func (v *KickoffView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Project",
			Description: "Start a new project",
			Action: func() tea.Cmd {
				v.focusInput = true
				return v.chatPanel.Focus()
			},
		},
	}
}
