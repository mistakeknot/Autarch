# Bigend TUI Model Structure Analysis

## Overview

Bigend is a unified mission control dashboard for the Autarch toolset with dual interfaces:
- **TUI** (Terminal UI): Bubble Tea-based rich interface via `internal/bigend/tui/model.go`
- **Web**: htmx + Tailwind dashboard via `internal/bigend/web/server.go`

Both interfaces share a common discovery/aggregation layer that scans for projects with Gurgeh, Coldwine, Pollard, and Agent Mail tooling.

---

## 1. Discovery & Project Model

### Location
`internal/bigend/discovery/scanner.go`

### Core Struct: Project
```go
type Project struct {
    Path           string        `json:"path"`
    Name           string        `json:"name"`
    HasGurgeh      bool          `json:"has_gurgeh"`
    HasColdwine    bool          `json:"has_coldwine"`
    HasPollard     bool          `json:"has_pollard"`
    HasAgentMail   bool          `json:"has_agent_mail"`
    TaskStats      *TaskStats    `json:"task_stats,omitempty"`
    PollardStats   *PollardStats `json:"pollard_stats,omitempty"`
    GurgStats      *GurgStats    `json:"gurg_stats,omitempty"`
}
```

### Helper Structs

**TaskStats** - Coldwine task metrics:
```go
type TaskStats struct {
    Total      int `json:"total"`
    Todo       int `json:"todo"`
    InProgress int `json:"in_progress"`
    Review     int `json:"review"`
    Done       int `json:"done"`
    Blocked    int `json:"blocked"`
}
// Methods: PercentDone() int, ActiveCount() int
```

**GurgStats** - Gurgeh PRD metrics:
```go
type GurgStats struct {
    Total  int `json:"total"`
    Draft  int `json:"draft"`
    Active int `json:"active"`
    Done   int `json:"done"`
}
```

**PollardStats** - Pollard research metrics:
```go
type PollardStats struct {
    Sources    int `json:"sources"`
    Insights   int `json:"insights"`
    Reports    int `json:"reports"`
    LastReport string `json:"last_report,omitempty"`
}
```

### Scanner
```go
type Scanner struct {
    cfg config.DiscoveryConfig
}

// Scan finds all projects with Gurgeh or Coldwine tooling
func (s *Scanner) Scan() ([]Project, error)

// examineProject checks what tooling a project has
func (s *Scanner) examineProject(path string) Project
```

**Scan Logic:**
- Walks configured `ScanRoots` (from config)
- Detects projects with `.gurgeh`, `.praude` (legacy), `.coldwine`, `.tandemonium` (legacy), `.pollard`, `.agent_mail`
- Depth limit: 3 levels below root
- Returns deduplicated project list if any tooling found

---

## 2. TUI Model Structure

### Location
`internal/bigend/tui/model.go`

### Main App: UnifiedApp
**Location:** `internal/tui/unified_app.go`

```go
type UnifiedApp struct {
    client          *autarch.Client
    currentView     View
    
    // Agent for AI generation
    codingAgent     *agent.Agent
    agentSelector   *pkgtui.AgentSelector
    selectedAgent   string
    
    // Dashboard state
    tabs            *TabBar
    dashViews       []View
    palette         *Palette
    
    // UI state
    width, height   int
    err             error
    showHelp        bool
    signalsOverlay  *SignalsOverlay
    lastCtrlC       time.Time
    keys            pkgtui.CommonKeys
    
    // Chat settings + log pane
    chatSettings    pkgtui.ChatSettings
    chatSettingsOpen bool
    chatSettingsView *pkgtui.ChatSettingsPanel
    logPane         *pkgtui.LogPane
    logPaneVisible  bool
    logPaneAutoShown bool
    
    // Initial flow
    initialTab      string
    skipOnboarding  bool
    
    // View factory/startup + intermute integration
    createDashboardViews func(*autarch.Client) []View
    intermuteMgr         *internalIntermute.Manager
    intermuteCleanup    func()
}
```

### Bigend-specific TUI: Model
**Location:** `internal/bigend/tui/model.go`

```go
type Model struct {
    agg           aggregatorAPI
    tmuxClient    statusClient
    tmuxCapture   *tmux.Client  // For terminal capture (separate from status detection)
    statusCache   map[string]cachedStatus
    statusTTL     time.Duration
    now           func() time.Time
    width, height int
    activeTab     Tab
    activePane    Pane
    buildInfo     string
    
    // Lists
    sessionList   list.Model
    projectsList  list.Model
    agentList     list.Model
    mcpList       list.Model
    mcpProject    string
    showMCP       bool
    
    // Terminal preview
    showTerminal  bool
    terminalPane  *TerminalPane
    
    // Filtering & grouping
    filterActive  bool
    filterInput   textinput.Model
    filterStates  map[Tab]FilterState
    groupExpanded map[string]bool
    
    // Prompts
    promptMode    promptMode
    promptInput   textinput.Model
    promptSess    *aggregator.TmuxSession
    
    // State
    err           error
    lastRefresh   time.Time
    quitting      bool
    keys          shared.CommonKeys
    helpOverlay   shared.HelpOverlay
}
```

### Tabs (Navigation)
```go
type Tab int

const (
    TabDashboard Tab = iota
    TabSessions
    TabAgents
)
```

### Panes (Layout)
```go
type Pane int

const (
    PaneProjects Pane = iota  // Left sidebar
    PaneMain                  // Main content area
    PaneTerminal              // Terminal preview (optional)
)
```

### List Items

**ProjectItem** - Project list representation:
```go
type ProjectItem struct {
    Path        string
    Name        string
    HasColdwine bool
    TaskStats   *struct {
        Todo       int
        InProgress int
        Done       int
    }
}

func (i ProjectItem) Title() string       // Returns i.Name
func (i ProjectItem) Description() string // Returns task stats or empty
func (i ProjectItem) FilterValue() string
```

**SessionItem** - Session list representation:
```go
type SessionItem struct {
    Session   aggregator.TmuxSession
    Status    tmux.Status
    AgentType string
}

func (i SessionItem) Title() string       // Returns name or agent name
func (i SessionItem) Description() string // project • agent-type • status
func (i SessionItem) FilterValue() string // name + project path
```

### Layout Function: paneWidths()
```go
func (m Model) paneWidths() (int, int, bool) {
    width := m.width
    if width <= 0 {
        return 0, 0, true
    }
    
    minLeft := 20   // Minimum left pane (projects sidebar)
    minRight := 30  // Minimum right pane (main content)
    gap := 2
    
    if width < minLeft+minRight+gap {
        return 0, width, true  // Hide left pane
    }
    
    left := width / 3
    if left < minLeft {
        left = minLeft
    }
    if width-left < minRight+gap {
        left = width - minRight - gap
    }
    
    right := width - left - gap
    return left, right, false
}
```

**Returns:** `(leftWidth, rightWidth, hideLeft)`
- Responsive: 3-column split prefers left=1/3, right=2/3
- Minimum enforcement: 20-char left pane, 30-char right pane
- Fallback: Hide left pane if total width < 52 chars

---

## 3. Shared TUI Components

### Location
`pkg/tui/components.go`

### Status Symbols & Indicators

**StatusSymbol()** - Returns just the symbol (no text):
```go
func StatusSymbol(status string) string
```

Symbol mapping:
- `●` running/working (green)
- `◐` in_progress/active/review (yellow)
- `◌` idle/draft/todo/pending/open (gray)
- `✓` done/completed/closed (green)
- `✗` error/blocked/failed/stopped (red)
- `○` waiting/paused/assigned (yellow)

**StatusIndicator()** - Returns styled symbol + text:
```go
func StatusIndicator(status string) string
// Returns: "● RUNNING", "◐ IN PROGRESS", "✓ DONE", etc.
```

**UnifiedStatusSymbol()** - Maps icdata.UnifiedStatus enum:
```go
func UnifiedStatusSymbol(s icdata.UnifiedStatus) string
```

Enum mapping (from icdata):
- `StatusActive` → `●` (green)
- `StatusBlocked` → `!` (red)
- `StatusWaiting` → `○` (yellow)
- `StatusDone` → `✓` (green)
- `StatusErr` → `✗` (red)

**UnifiedStatusIndicator()** - Styled indicator with enum:
```go
func UnifiedStatusIndicator(s icdata.UnifiedStatus) string
```

### Other Components
- **AgentBadge()** - Styled badge for agent types (Claude, Codex, Aider, Cursor)
- **PriorityBadge()** - P0/P1/P2/P3+ badges with priority colors

---

## 4. Web Dashboard Structure

### Location
`internal/bigend/web/server.go` & `internal/bigend/web/templates/`

### Server
```go
type Server struct {
    cfg          config.ServerConfig
    agg          aggregatorAPI
    statusClient statusClient
    templates    map[string]*template.Template
    srv          *http.Server
}

func NewServer(cfg config.ServerConfig, agg aggregatorAPI) *Server
```

### Aggregator API Interface
```go
type aggregatorAPI interface {
    GetState() aggregator.State
    Refresh(ctx context.Context) error
    GetProject(path string) *discovery.Project
    GetProjectTasks(projectPath string) (map[string][]coldwine.Task, error)
    GetAgent(name string) *aggregator.Agent
    GetIntermuteAgent(ctx context.Context, name string) (*intermute.Agent, error)
    GetAgentMessages(ctx context.Context, agentID string, limit int) ([]intermute.Message, error)
    GetAgentReservations(ctx context.Context, agentID string) ([]intermute.Reservation, error)
    GetActiveReservations(ctx context.Context) ([]intermute.Reservation, error)
    NewSession(name, projectPath, agentType string) error
    RestartSession(name, projectPath, agentType string) error
    RenameSession(oldName, newName string) error
    ForkSession(name, projectPath, agentType string) error
    AttachSession(name string) error
    StartMCP(ctx context.Context, projectPath, component string) error
    StopMCP(projectPath, component string) error
}
```

### Web Group Helper
```go
type webGroup[T any] struct {
    Name  string
    Path  string
    Items []T
}
```

### Templates
**projects.html** - Project dashboard with:
- Project name + path
- Tooling badges (praude/gurgeh, tandemonium/coldwine, agent-mail)
- Task progress bar: Done/Total with percentage
- Grid display: Todo, In Progress, Review, Done, Blocked counts
- Task board link (`/projects/{path}/tasks`)
- MCP components status grid with Start/Stop buttons

**Template Data Fields:**
```
.Projects        // []Project from discovery
.MCP             // map[string][]MCPComponent
.TaskStats       // Task metrics
.HasPraude       // Legacy Gurgeh indicator
.HasTandemonium  // Legacy Coldwine indicator
.HasAgentMail    // Agent Mail status
```

---

## 5. Sidebar Structure

### Location
`pkg/tui/sidebar.go`

### Sidebar Component
```go
type Sidebar struct {
    items     []SidebarItem
    selected  int
    collapsed bool
    width     int    // Fixed 20 chars when expanded, 0 when collapsed
    height    int
    focused   bool
}

func NewSidebar(items []SidebarItem) *Sidebar

// Rendering
func (s *Sidebar) Render() string
// Iterates items, calls renderItem() for each, joins with newlines
```

### SidebarItem Interface
```go
type SidebarItem interface {
    // ... not shown in this code excerpt
}
```

**Used for:** Project list, section headers, navigation items

---

## 6. Integration Points

### UnifiedApp → Bigend Model
- Creates `Model` as a `View` delegate
- Routes messages to Model.Update()
- Calls Model.View() for render

### Discovery → Web Dashboard
- `Scanner.Scan()` → `[]Project`
- Project struct marshals to JSON for templates
- TaskStats/GurgStats/PollardStats render in project cards

### Discovery → TUI Model
- `ProjectItem` wraps `Project` for list rendering
- `paneWidths()` calculates sidebar + main pane layout
- `StatusSymbol()` / `StatusIndicator()` format task states

### Web → TUI Sync
- Both read from shared `aggregator.State`
- Both call `Refresh()` on aggregator
- Both use discovery `Scanner.Scan()`

---

## 7. Key Design Patterns

### Responsive Layout
- `paneWidths()` enforces min/max constraints
- Sidebar fixed 20-char width when shown
- Main pane gets remaining space (min 30 chars)
- Graceful fallback: hide left pane < 52 chars wide

### Status Representation
- Unified enum `icdata.UnifiedStatus` (Active, Blocked, Waiting, Done, Error)
- String-based fallback for string status values
- Symbol + color consistent across TUI and Web

### Data Flow
```
discovery.Scanner.Scan()
    ↓
[]discovery.Project
    ├→ TUI: ProjectItem list → paneWidths() layout → renderItem()
    └→ Web: JSON template context → projects.html
```

### Aggregation
- Central `aggregatorAPI` interface
- TUI Model implements via tmux status detection
- Web Server implements via Coldwine/Gurgeh/Pollard APIs

---

## 8. File Locations Summary

| Component | Path |
|-----------|------|
| Project struct | `internal/bigend/discovery/scanner.go` |
| Scanner | `internal/bigend/discovery/scanner.go` |
| TUI Model | `internal/bigend/tui/model.go` |
| UnifiedApp | `internal/tui/unified_app.go` |
| Sidebar | `pkg/tui/sidebar.go` |
| Status components | `pkg/tui/components.go` |
| Web Server | `internal/bigend/web/server.go` |
| Web Templates | `internal/bigend/web/templates/*.html` |
| Dashboard views | `internal/tui/views/bigend.go` |

