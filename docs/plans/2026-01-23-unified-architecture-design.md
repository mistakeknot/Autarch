# Vauxpraudemonium Unified Architecture Design

**Date:** 2026-01-23

## Mission Statements

| Tool | Mission |
|------|---------|
| **Praude** | Define what to build through versioned PRDs (MVP, V1, V2) containing features, requirements, and success criteria. |
| **Pollard** | Continuous research intelligence - competitive landscape, user flows, open source patterns, industry trends. Enriches Praude and Tandemonium artifacts. |
| **Tandemonium** | Track execution by breaking features into epics, stories, and tasks. The task database and progress tracker. |
| **Vauxhall** | Global mission control that observes all projects AND directs agents to work on tasks. |
| **Intermute** | Agent coordination layer (messages, reservations, conflict resolution). Separate project, successor to MCP Agent Mail. |

## Work Item Hierarchy

```
PRD (version scope: MVP, V1, V2...)           [Praude]
  └── Feature (major capability)              [Praude]
       └── Epic (implementation chunk)        [Tandemonium]
            └── Story (user-facing deliverable) [Tandemonium]
                 └── Task (atomic work item)  [Tandemonium]
```

**Example:**
```
PRD: "Vauxhall MVP"
  └── Feature: "Agent Monitoring"
       └── Epic: "tmux Integration"
            └── Story: "Show all tmux sessions"
                 └── Task: "Parse tmux list-sessions output"
```

## Tool Responsibilities

| Tool | Scope | Writes | Reads |
|------|-------|--------|-------|
| **Praude** | Per-project | `.praude/` (PRDs, features) | `.pollard/` (research insights) |
| **Pollard** | Per-project + Global | `.pollard/` (research, patterns, comps) | Web, GitHub, industry sources |
| **Tandemonium** | Per-project | `.tandemonium/` (epics, stories, tasks) | `.praude/`, `.pollard/`, `docs/`, codebase, git |
| **Vauxhall** | Global | (agent commands) | `.praude/`, `.pollard/`, `.tandemonium/`, tmux, Intermute |

## Data Flow

```
┌──────────┐     ┌──────────┐     ┌─────────────┐     ┌──────────┐     ┌──────────┐
│ Pollard  │────▶│  Praude  │────▶│ Tandemonium │────▶│ Vauxhall │────▶│ Intermute│
│(Research)│     │(PRD+Feat)│     │(Epic→Task)  │     │ (Agents) │     │ (Coord)  │
└──────────┘     └──────────┘     └─────────────┘     └──────────┘     └──────────┘
      │                                   │
      └───────────────────────────────────┘
              (continuous enrichment)
```

## Unified Shell

Single application with top-level tabs, using Vauxhall's look and feel as the template:

```
┌─────────────────────────────────────────────────────────────────────┐
│  [Vauxhall]  [Pollard]  [Praude]  [Tandemonium]  project: shadow-work│
├───────────────────┬─────────────────────────────────────────────────┤
│                   │                                                 │
│  Projects         │  Tool-specific content                          │
│  ─────────        │  (list + detail pattern)                        │
│  > shadow-work    │                                                 │
│    jawncloud      │                                                 │
│    interdoc       │                                                 │
│                   │                                                 │
└───────────────────┴─────────────────────────────────────────────────┘
```

### Key Bindings
- `1` / `2` / `3` / `4` - Switch to Vauxhall / Pollard / Praude / Tandemonium
- `Tab` / `Shift+Tab` - Switch sub-tabs within tool
- `?` - Help (tool-specific)
- `j/k` - Navigation, `Enter` - Select, `Esc` - Back
- `/` - Filter/search
- `h/l` or `←/→` - Switch panes

### Context
- Projects list always visible in left pane (Vauxhall pattern)
- Pollard, Praude and Tandemonium require a project selected
- Switching to Praude/Tandemonium from Vauxhall keeps current project
- Project selection persists across tool switches

## Unified Shell Implementation

### Approach: Shared Library

Extract tool UIs into shared packages that can be composed into a single binary:

```
pkg/
├── tui/                    # Shared styles (existing)
│   ├── colors.go
│   ├── styles.go
│   └── components.go
├── shell/                  # NEW: Unified shell framework
│   ├── shell.go            # Main shell model
│   ├── tabs.go             # Top-level tool tabs
│   ├── projects.go         # Projects pane (always visible)
│   └── context.go          # Shared context (selected project)
└── toolpane/               # NEW: Tool pane interface
    └── interface.go        # Interface that each tool implements

internal/
├── vauxhall/tui/           # Implements toolpane.Pane interface
├── praude/tui/             # Implements toolpane.Pane interface
└── tandemonium/tui/        # Implements toolpane.Pane interface
```

### Tool Pane Interface

Each tool implements a common interface:

```go
package toolpane

import tea "github.com/charmbracelet/bubbletea"

// Context shared across all tool panes
type Context struct {
    ProjectPath  string     // Selected project path
    ProjectName  string     // Project basename
    Width        int        // Available width
    Height       int        // Available height
}

// Pane is implemented by each tool's TUI
type Pane interface {
    // Init initializes the pane with context
    Init(ctx Context) tea.Cmd

    // Update handles messages
    Update(msg tea.Msg, ctx Context) (Pane, tea.Cmd)

    // View renders the pane
    View(ctx Context) string

    // Name returns the tool name for the tab bar
    Name() string

    // SubTabs returns the tool's internal tabs (if any)
    SubTabs() []string

    // ActiveSubTab returns current sub-tab index
    ActiveSubTab() int

    // SetSubTab switches to a sub-tab
    SetSubTab(index int) tea.Cmd

    // NeedsProject returns true if tool requires project context
    NeedsProject() bool
}
```

### Shell Model

The unified shell composes tool panes:

```go
package shell

type Model struct {
    // Layout
    width, height int

    // Projects pane (always visible)
    projectsList  list.Model
    projectsWidth int

    // Tool tabs
    tools        []toolpane.Pane  // [Vauxhall, Praude, Tandemonium]
    activeTool   int              // 0, 1, or 2

    // Shared context
    ctx          toolpane.Context
}

func (m Model) View() string {
    // Top bar with tool tabs
    tabBar := m.renderTabBar()

    // Left pane: projects list
    projectsPane := m.renderProjectsPane()

    // Right pane: active tool content
    toolPane := m.tools[m.activeTool].View(m.ctx)

    // Compose layout
    content := lipgloss.JoinHorizontal(
        lipgloss.Top,
        projectsPane,
        toolPane,
    )

    return lipgloss.JoinVertical(
        lipgloss.Left,
        tabBar,
        content,
    )
}
```

### Migration Path

1. **Phase 1**: Create `pkg/shell` and `pkg/toolpane` packages
2. **Phase 2**: Refactor Vauxhall TUI to implement `toolpane.Pane`
3. **Phase 3**: Refactor Praude TUI to implement `toolpane.Pane`
4. **Phase 4**: Refactor Tandemonium TUI to implement `toolpane.Pane`
5. **Phase 5**: Create unified `cmd/vaux` binary that composes all three

### Vauxhall Patterns to Adopt

The following Vauxhall patterns should be adopted by Praude and Tandemonium:

| Pattern | Vauxhall Implementation | Adoption |
|---------|------------------------|----------|
| **Two-pane layout** | Projects list (left) + Content (right) | All tools use this |
| **Filter input** | `/` activates filter, typed text filters list | All tools adopt |
| **Status filtering** | `!running`, `!waiting` syntax | Praude: `!draft`, `!approved`; Tandemonium: `!todo`, `!review` |
| **Group headers** | Projects grouped with expand/collapse | Tasks grouped by epic, PRDs by version |
| **Tab bar** | Internal tabs (Dashboard, Sessions, Agents) | Each tool keeps its tabs |
| **Status indicators** | `● RUNNING`, `○ WAITING`, etc. | Shared via `pkg/tui` |
| **Agent badges** | Colored badges for Claude, Codex, etc. | Shared via `pkg/tui` |

## Integration Contracts

### Praude → Tandemonium
- Tandemonium reads `.praude/specs/*.yaml` for approved PRDs
- PRD contains: `id`, `title`, `version`, `status`, `features[]`
- Each feature contains: `id`, `title`, `requirements[]`, `cujs[]`, `files_to_modify[]`
- Tandemonium generates epics/stories/tasks referencing `feature_ref: FEAT-001`

### Pollard → Praude
- Praude reads `.pollard/insights/*.yaml` for research context
- Research enriches PRD drafting with competitive landscape, patterns, and prior art

### Pollard → Tandemonium
- Tandemonium reads `.pollard/patterns/*.yaml` for implementation references
- Patterns inform epic/story breakdown with concrete examples and anti-patterns

---

## Pollard Research Intelligence

Pollard is the continuous research and intelligence gathering module that enriches Praude and Tandemonium artifacts with external context.

### Mission
Gather competitive landscape data, user flow patterns, open source implementations, and industry trends on a continuous basis to improve the quality and context of all planning and execution artifacts.

### File Structure
```
.pollard/
├── config.yaml              # Research sources and schedules
├── insights/                # High-level research findings
│   ├── competitive.yaml     # Competitor analysis
│   ├── trends.yaml          # Industry trends
│   └── user-research.yaml   # User flow patterns
├── patterns/                # Implementation patterns
│   ├── ui-patterns.yaml     # UI/UX patterns with examples
│   ├── arch-patterns.yaml   # Architecture patterns
│   └── anti-patterns.yaml   # What to avoid
├── sources/                 # Raw collected data
│   ├── github/              # Open source repos analyzed
│   ├── articles/            # Industry articles
│   └── screenshots/         # Competitive UI screenshots
└── reports/                 # Generated reports
    └── 2026-01-23-landscape.md
```

### Research Schema

**Insight (competitive/trends/user-research):**
```yaml
# .pollard/insights/competitive.yaml
id: "COMP-001"
title: "Linear vs Jira Task Management"
category: "competitive"
collected_at: "2026-01-23T12:00:00Z"
sources:
  - url: "https://linear.app"
    type: "product"
  - url: "https://github.com/linear/linear"
    type: "github"

findings:
  - title: "Keyboard-first navigation"
    relevance: "high"
    description: "Linear uses cmd+k for everything"
    evidence: ["screenshot-001.png"]

  - title: "Issue hierarchy"
    relevance: "medium"
    description: "Project > Cycle > Issue, no epic concept"

recommendations:
  - feature_hint: "Agent command palette"
    priority: "p1"
    rationale: "Keyboard-first aligns with TUI philosophy"
```

**Pattern (ui/arch/anti):**
```yaml
# .pollard/patterns/ui-patterns.yaml
id: "PAT-001"
title: "Two-pane list-detail layout"
category: "ui"
collected_at: "2026-01-23T12:00:00Z"

description: |
  Master-detail pattern with list on left, detail on right.
  Common in email clients, IDEs, task managers.

examples:
  - name: "Linear"
    url: "https://linear.app"
    screenshot: "linear-two-pane.png"
    notes: "Clean separation, keyboard nav"

  - name: "Sublime Text"
    url: "https://sublimetext.com"
    notes: "Sidebar + editor + minimap"

  - name: "lazygit"
    url: "https://github.com/jesseduffield/lazygit"
    notes: "TUI implementation, panels"

implementation_hints:
  - "Use h/l for pane switching"
  - "Maintain selection state per pane"
  - "Preview on hover, open on Enter"

anti_patterns:
  - "Modal dialogs blocking navigation"
  - "Too many nested levels (>3)"
```

### Research Sources

| Source Type | Examples | Collection Method |
|-------------|----------|-------------------|
| **GitHub repos** | lazygit, linear, plane | Clone, analyze structure, extract patterns |
| **Product sites** | Linear, Notion, Coda | Screenshot, document flows |
| **Articles** | Maggie Appleton, etc. | Summarize, extract principles |
| **APIs** | Product Hunt, HN | Trending tools, discussions |
| **User research** | Interviews, surveys | Structured notes |

### Research Agents

Pollard uses background agents for continuous collection:

```yaml
# .pollard/config.yaml
agents:
  - name: github-scout
    schedule: "daily"
    sources:
      - query: "topic:cli topic:tui language:go"
        limit: 50
      - query: "topic:agent-orchestration"
        limit: 20
    output: sources/github/

  - name: competitor-tracker
    schedule: "weekly"
    targets:
      - url: "https://linear.app/changelog"
        type: "changelog"
      - url: "https://www.notion.so/releases"
        type: "changelog"
    output: insights/competitive.yaml

  - name: trend-watcher
    schedule: "daily"
    sources:
      - type: "hackernews"
        query: "AI agents OR LLM tools"
      - type: "producthunt"
        category: "developer-tools"
    output: insights/trends.yaml
```

### Pollard CLI Commands

```bash
# Run all research agents
pollard scan

# Run specific agent
pollard scan --agent github-scout

# Generate landscape report
pollard report --type landscape

# Search collected patterns
pollard search "two-pane layout"

# Link insight to Praude feature
pollard link COMP-001 --feature FEAT-003

# Export for Praude context
pollard export --format praude > .praude/context.yaml
```

### Pollard in Unified Shell

```
┌─────────────────────────────────────────────────────────────────────┐
│  [Vauxhall]  [Pollard]  [Praude]  [Tandemonium]  project: vauxhall  │
├────────────────────────────────────────────────────────────────────┬┤
│  [Insights]  [Patterns]  [Sources]  [Reports]                       │
├───────────────────┬─────────────────────────────────────────────────┤
│                   │                                                 │
│  Insights         │  COMP-001: Linear vs Jira                       │
│  ─────────        │  ───────────────────────────────────────────    │
│  > COMP-001       │  Category: competitive                          │
│    COMP-002       │  Collected: 2026-01-23                          │
│    TREND-001      │                                                 │
│    USER-001       │  Findings:                                      │
│                   │  • Keyboard-first navigation [HIGH]             │
│  Filters          │  • Issue hierarchy [MEDIUM]                     │
│  ─────────        │                                                 │
│  [x] competitive  │  Recommendations:                               │
│  [x] trends       │  • Add command palette (P1)                     │
│  [ ] user         │                                                 │
│                   │  [Link to Feature] [View Sources]               │
└───────────────────┴─────────────────────────────────────────────────┘
```

### Integration with Other Tools

**Praude Integration:**
- When drafting PRDs, Praude surfaces relevant insights from `.pollard/insights/`
- Competitive analysis informs feature prioritization
- User research validates requirements

**Tandemonium Integration:**
- When breaking down epics, reference implementation patterns from `.pollard/patterns/`
- Anti-patterns inform story acceptance criteria
- Open source examples provide implementation hints

**Vauxhall Integration:**
- Research agents appear in Vauxhall's agent list
- Scan progress visible in mission control
- Alerts when new relevant patterns found

## Praude PRD Schema

### File Structure
```
.praude/
├── specs/
│   ├── mvp.yaml      # MVP version PRD
│   ├── v1.yaml       # V1 version PRD
│   └── v2.yaml       # V2 version PRD
└── research/
    └── *.md          # Research documents
```

### PRD Schema (Go types)
```go
type PRD struct {
    ID        string    `yaml:"id"`        // "MVP", "V1", "V2"
    Title     string    `yaml:"title"`     // "Vauxhall MVP"
    Version   string    `yaml:"version"`   // "mvp", "v1", "v2"
    Status    string    `yaml:"status"`    // draft, approved, in_progress, done
    CreatedAt string    `yaml:"created_at"`
    Features  []Feature `yaml:"features"`
}

type Feature struct {
    ID                   string                `yaml:"id"`       // "FEAT-001"
    Title                string                `yaml:"title"`
    Status               string                `yaml:"status"`   // draft, approved, in_progress, done
    Summary              string                `yaml:"summary"`
    Requirements         []string              `yaml:"requirements"`
    AcceptanceCriteria   []AcceptanceCriterion `yaml:"acceptance_criteria"`
    FilesToModify        []FileChange          `yaml:"files_to_modify"`
    CriticalUserJourneys []CriticalUserJourney `yaml:"critical_user_journeys"`
    Complexity           string                `yaml:"complexity"` // low, medium, high
    Priority             int                   `yaml:"priority"`   // 0-4
}

type AcceptanceCriterion struct {
    ID          string `yaml:"id"`
    Description string `yaml:"description"`
}

type FileChange struct {
    Action      string `yaml:"action"`      // create, modify, delete
    Path        string `yaml:"path"`
    Description string `yaml:"description"`
}

type CriticalUserJourney struct {
    ID                 string   `yaml:"id"`
    Title              string   `yaml:"title"`
    Priority           string   `yaml:"priority"` // p0, p1, p2
    Steps              []string `yaml:"steps"`
    SuccessCriteria    []string `yaml:"success_criteria"`
    LinkedRequirements []string `yaml:"linked_requirements"` // REQ-001, REQ-002
}
```

### Example PRD
```yaml
# .praude/specs/mvp.yaml
id: "MVP"
title: "Vauxhall MVP"
version: "mvp"
status: "in_progress"
created_at: "2026-01-23T12:00:00Z"

features:
  - id: "FEAT-001"
    title: "Agent Monitoring"
    status: "approved"
    summary: "Monitor all AI agents across projects"
    requirements:
      - "REQ-001: List all tmux sessions"
      - "REQ-002: Detect agent sessions by heuristics"
      - "REQ-003: Link sessions to projects"
    acceptance_criteria:
      - id: "AC-001"
        description: "Sessions appear within 5 seconds of creation"
      - id: "AC-002"
        description: "Agent detection accuracy >= 90%"
    files_to_modify:
      - action: "create"
        path: "internal/vauxhall/tmux/client.go"
        description: "tmux session listing and detection"
    critical_user_journeys:
      - id: "CUJ-001"
        title: "View active agents"
        priority: "p0"
        steps:
          - "Open Vauxhall dashboard"
          - "See list of active sessions"
          - "Identify which are agent sessions"
        success_criteria:
          - "All sessions visible"
          - "Agent sessions marked with indicator"
        linked_requirements:
          - "REQ-001"
          - "REQ-002"
    complexity: "medium"
    priority: 1

  - id: "FEAT-002"
    title: "Project Discovery"
    status: "draft"
    # ... more features
```

### Migration from Current Format
Existing PRD-001, PRD-002, etc. specs will be auto-migrated:
1. All specs become features in an "MVP" PRD
2. Spec IDs (PRD-001) become feature IDs (FEAT-001)
3. StrategicContext fields move to PRD level where applicable

### Praudemaps: Visual PRD Sequences

Praudemaps visualize the sequence and dependencies of PRDs and features across product versions.

**Purpose:**
- Show the roadmap from MVP through V1, V2, etc.
- Visualize feature dependencies across versions
- Track progress through the product evolution
- Communicate product vision to stakeholders

**Praudemap Schema:**

```yaml
# .praude/praudemap.yaml
name: "Vauxhall Product Roadmap"
versions:
  - id: MVP
    title: "Minimum Viable Product"
    target_date: "2026-02"
    features:
      - id: FEAT-001
        title: "Agent Monitoring"
        status: in_progress
        depends_on: []
      - id: FEAT-002
        title: "Project Discovery"
        status: draft
        depends_on: []
      - id: FEAT-003
        title: "Session Streaming"
        status: draft
        depends_on: [FEAT-001]

  - id: V1
    title: "Version 1.0"
    target_date: "2026-04"
    features:
      - id: FEAT-004
        title: "Agent Orchestration"
        status: planned
        depends_on: [FEAT-001, FEAT-002]
      - id: FEAT-005
        title: "Intermute Integration"
        status: planned
        depends_on: [FEAT-004]

  - id: V2
    title: "Version 2.0"
    target_date: "2026-Q3"
    features:
      - id: FEAT-006
        title: "Multi-Host Support"
        status: planned
        depends_on: [FEAT-004, FEAT-005]
```

**Praudemap Visualization:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Praudemap: Vauxhall Product Roadmap                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  MVP (Feb 2026)              V1 (Apr 2026)         V2 (Q3 2026)        │
│  ─────────────               ──────────────        ────────────         │
│                                                                         │
│  ┌────────────┐              ┌────────────┐        ┌────────────┐      │
│  │ FEAT-001   │─────────────▶│ FEAT-004   │───────▶│ FEAT-006   │      │
│  │ Agent Mon. │              │ Orchestr.  │        │ Multi-Host │      │
│  │ ◐ progress │              │ ○ planned  │        │ ○ planned  │      │
│  └────────────┘              └────────────┘        └────────────┘      │
│        │                           │                     ▲              │
│        │                           ▼                     │              │
│  ┌────────────┐              ┌────────────┐              │              │
│  │ FEAT-002   │─────────────▶│ FEAT-005   │──────────────┘              │
│  │ Discovery  │              │ Intermute  │                             │
│  │ ○ draft    │              │ ○ planned  │                             │
│  └────────────┘              └────────────┘                             │
│        │                                                                │
│        ▼                                                                │
│  ┌────────────┐                                                         │
│  │ FEAT-003   │                                                         │
│  │ Streaming  │                                                         │
│  │ ○ draft    │                                                         │
│  └────────────┘                                                         │
│                                                                         │
│  Legend: ● done  ◐ in_progress  ○ draft/planned  ─▶ depends on         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Praudemap CLI Commands:**

```bash
# Generate praudemap visualization
praude map

# Show praudemap for specific version
praude map --version=MVP

# Export praudemap as SVG/PNG
praude map --export=roadmap.svg

# Show feature dependencies
praude map --deps FEAT-004

# Interactive praudemap in TUI
praude map --tui
```

**Praudemap in Unified Shell:**

The Praude tab includes a "Map" sub-tab showing the visual praudemap:

```
┌─────────────────────────────────────────────────────────────────────────┐
│  [Vauxhall]  [Praude]  [Tandemonium]                                    │
│               ▔▔▔▔▔▔                                                    │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ [PRDs] [Features] [Map] [Research]                               │  │
│  │                   ▔▔▔                                            │  │
│  │                                                                  │  │
│  │  (Praudemap visualization here)                                  │  │
│  │                                                                  │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

**Praudemap → Tandemonium Integration:**

When Tandemonium runs `init`, it reads the praudemap to understand:
- Which version is currently active (MVP, V1, etc.)
- Feature dependencies for task ordering
- Overall product context for generated tasks

```go
// Tandemonium reads praudemap for context
func (t *Tandemonium) Init(opts InitOptions) error {
    // Load praudemap if exists
    praudemap, err := praude.LoadPraudemap(opts.ProjectPath)
    if err == nil {
        // Use active version's features
        activeVersion := praudemap.GetActiveVersion()
        for _, feat := range activeVersion.Features {
            if feat.Status == "approved" || feat.Status == "in_progress" {
                t.GenerateEpicsFromFeature(feat)
            }
        }
    }
    // ...
}
```

### Tandemonium → Vauxhall
- Vauxhall reads `.tandemonium/state.db` (SQLite, read-only)
- Tables: `epics`, `stories`, `tasks`, `agents`, `worktrees`
- Vauxhall also reads `.tandemonium/specs/*.yaml` for task specs

## Tandemonium Task Schema

### File Structure
```
.tandemonium/
├── state.db           # SQLite database for tasks, agents, worktrees
├── specs/
│   └── epics/
│       ├── EPIC-001.yaml  # Generated from FEAT-001
│       └── EPIC-002.yaml
└── worktrees/         # Git worktree directories for agents
```

### Epic/Story/Task Schema (Go types)
```go
type Status string

const (
    StatusTodo       Status = "todo"
    StatusInProgress Status = "in_progress"
    StatusReview     Status = "review"
    StatusBlocked    Status = "blocked"
    StatusDone       Status = "done"
)

type Priority string

const (
    PriorityP0 Priority = "p0"  // Critical
    PriorityP1 Priority = "p1"  // High
    PriorityP2 Priority = "p2"  // Medium
    PriorityP3 Priority = "p3"  // Low
)

type Epic struct {
    ID                 string   `yaml:"id"`          // "EPIC-001"
    FeatureRef         string   `yaml:"feature_ref"` // "FEAT-001" (link to Praude)
    Title              string   `yaml:"title"`
    Summary            string   `yaml:"summary"`
    Status             Status   `yaml:"status"`
    Priority           Priority `yaml:"priority"`
    AcceptanceCriteria []string `yaml:"acceptance_criteria"`
    Stories            []Story  `yaml:"stories"`
    CreatedAt          string   `yaml:"created_at"`
    UpdatedAt          string   `yaml:"updated_at"`
}

type Story struct {
    ID                 string   `yaml:"id"`          // "STORY-001"
    EpicRef            string   `yaml:"epic_ref"`    // "EPIC-001"
    Title              string   `yaml:"title"`
    Summary            string   `yaml:"summary"`
    Status             Status   `yaml:"status"`
    Priority           Priority `yaml:"priority"`
    AcceptanceCriteria []string `yaml:"acceptance_criteria"`
    Tasks              []Task   `yaml:"tasks"`
    AssignedAgent      string   `yaml:"assigned_agent,omitempty"`
    WorktreePath       string   `yaml:"worktree_path,omitempty"`
}

type Task struct {
    ID           string   `yaml:"id"`          // "TASK-001"
    StoryRef     string   `yaml:"story_ref"`   // "STORY-001"
    Title        string   `yaml:"title"`
    Description  string   `yaml:"description"`
    Status       Status   `yaml:"status"`
    Priority     Priority `yaml:"priority"`
    Complexity   string   `yaml:"complexity"`  // trivial, simple, medium, complex
    FilesToModify []string `yaml:"files_to_modify"`
    AssignedAgent string  `yaml:"assigned_agent,omitempty"`
}
```

### Example Epic (generated from Feature)
```yaml
# .tandemonium/specs/epics/EPIC-001.yaml
id: "EPIC-001"
feature_ref: "FEAT-001"  # Links to Praude feature
title: "tmux Integration"
summary: "List and monitor tmux sessions for agent detection"
status: "in_progress"
priority: "p1"
acceptance_criteria:
  - "All sessions visible within 5s"
  - "Agent detection >= 90% accuracy"
created_at: "2026-01-23T12:00:00Z"
updated_at: "2026-01-23T14:30:00Z"

stories:
  - id: "STORY-001"
    epic_ref: "EPIC-001"
    title: "List tmux sessions"
    summary: "Parse tmux list-sessions and display in dashboard"
    status: "done"
    priority: "p0"
    acceptance_criteria:
      - "Sessions sorted by creation time"
      - "Show attached/detached status"
    assigned_agent: "claude-1"
    worktree_path: ".tandemonium/worktrees/STORY-001"
    tasks:
      - id: "TASK-001"
        story_ref: "STORY-001"
        title: "Parse tmux list-sessions output"
        description: "Create tmux client that parses session metadata"
        status: "done"
        priority: "p0"
        complexity: "simple"
        files_to_modify:
          - "internal/vauxhall/tmux/client.go"

  - id: "STORY-002"
    epic_ref: "EPIC-001"
    title: "Detect agent sessions"
    status: "in_progress"
    priority: "p1"
    # ...
```

### SQLite Tables
```sql
CREATE TABLE epics (
    id TEXT PRIMARY KEY,
    feature_ref TEXT,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    priority TEXT,
    created_at TEXT,
    updated_at TEXT
);

CREATE TABLE stories (
    id TEXT PRIMARY KEY,
    epic_ref TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    priority TEXT,
    assigned_agent TEXT,
    worktree_path TEXT,
    FOREIGN KEY (epic_ref) REFERENCES epics(id)
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    story_ref TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    priority TEXT,
    complexity TEXT,
    assigned_agent TEXT,
    FOREIGN KEY (story_ref) REFERENCES stories(id)
);

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,  -- claude, codex, gemini, custom
    status TEXT NOT NULL, -- idle, working, blocked, waiting
    current_task TEXT,
    session_id TEXT,
    registered_at TEXT
);

CREATE TABLE worktrees (
    path TEXT PRIMARY KEY,
    story_ref TEXT,
    branch TEXT,
    created_at TEXT,
    FOREIGN KEY (story_ref) REFERENCES stories(id)
);
```

### Linking: Praude Feature → Tandemonium Epic

When Tandemonium runs `init`, it reads Praude features and generates epics:

```
Praude: .praude/specs/mvp.yaml
  └── Feature: FEAT-001 "Agent Monitoring"
       │
       ▼
Tandemonium: .tandemonium/specs/epics/EPIC-001.yaml
  └── Epic: EPIC-001 (feature_ref: "FEAT-001")
       └── Story: STORY-001
            └── Task: TASK-001
```

### Vauxhall → Intermute
- Register/unregister agents
- Send messages to agents
- Query file reservations
- Resolve conflicts

### Vauxhall → Agents
- Launches agents via tmux commands
- Assigns tasks by writing to agent's Intermute inbox
- Monitors via tmux capture-pane

## Vauxhall Daemon Architecture

Based on schmux patterns, Vauxhall runs as a daemon:

```
┌─────────────────────────────────────────────────────┐
│                 Vauxhall Daemon                     │
│  ┌───────────┐  ┌───────────┐  ┌───────────────┐   │
│  │ HTTP API  │  │ WebSocket │  │ TUI (attached)│   │
│  │ :7337     │  │ /ws/*     │  │               │   │
│  └───────────┘  └───────────┘  └───────────────┘   │
│         │              │               │            │
│  ┌──────┴──────────────┴───────────────┴──────┐    │
│  │              Core Engine                    │    │
│  │  - Session manager (tmux)                   │    │
│  │  - Project scanner                          │    │
│  │  - Aggregator (.praude, .tandemonium)       │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

### Key Endpoints (schmux-inspired)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/sessions` | GET | List sessions with status, workspace, git info |
| `/api/spawn` | POST | Launch agent with target, prompt, workspace |
| `/api/dispose/{id}` | POST | Terminate session cleanly |
| `/api/projects` | GET | List discovered projects |
| `/api/tasks/{project}` | GET | Tasks from Tandemonium |
| `/ws/terminal/{id}` | WS | Stream terminal output |

### Run Targets

Agents defined as configurable targets:

```yaml
targets:
  - name: claude
    type: builtin
    command: claude
  - name: codex
    type: builtin
    command: codex
  - name: gemini
    type: builtin
    command: gemini
  - name: custom-reviewer
    type: custom
    command: "claude --system-prompt reviewer.md"
```

## Tandemonium Init Workflow

### Sources
1. `.praude/specs/*.yaml` - Approved PRDs with features
2. `docs/plans/*-plan.md` - Markdown implementation plans
3. Codebase - `TODO`, `FIXME`, `HACK` comments
4. Git - Recent commits, open branches, uncommitted changes

### Depth Levels

```bash
tand init [--depth=1|2|3|4]
```

| Depth | Generates |
|-------|-----------|
| 1 | Epics only (from features) |
| 2 | Epics + Stories |
| 3 | Epics + Stories + Tasks |
| 4 | Full breakdown with complexity estimates |

### Init Flow
1. **Discover sources** - Find PRDs, plans, TODOs, git state
2. **Import features** - Link to Praude features or create from plans
3. **Generate epics** - Break features into implementation chunks
4. **Generate stories** (depth >= 2) - User-facing deliverables with acceptance criteria
5. **Generate tasks** (depth >= 3) - Atomic work items
6. **Estimate** (depth >= 4) - Add complexity estimates

## Dependencies

- **Intermute** (separate project) - Agent coordination layer
- **tmux** - Session management for agents
- **SQLite** - Local state storage

## Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| Should Vauxhall control agents? | Yes, Vauxhall directs agents |
| Where is orchestration? | Vauxhall orchestrates, Tandemonium tracks |
| What hierarchy? | 5 levels: PRD > Feature > Epic > Story > Task |
| UI standardization depth? | Unified shell with top-level tabs |
| Integration format? | Filesystem conventions (.praude/, .tandemonium/) |

## Schmux Patterns Applied

Insights from [sergeknystautas/schmux](https://github.com/sergeknystautas/schmux):

| Schmux Pattern | Application |
|----------------|-------------|
| **Daemon + HTTP API** | Vauxhall runs as daemon with REST API, not just TUI. Enables web dashboard and programmatic control |
| **Run targets** | Agents defined as configurable targets with `name`, `type`, `command`. Auto-detect Claude, Codex, Gemini |
| **Session spawning** | `/api/spawn` with targets, prompts, nicknames. Sessions can be spawned on existing workspaces |
| **Workspace isolation** | Git clone/checkout per agent. Overlay local files (.env, config) to new workspaces |
| **NudgeNik states** | Agent status: **Blocked** (needs permission), **Waiting** (needs input), **Working**, **Done** |
| **WebSocket streaming** | `/ws/terminal/{sessionId}` for live terminal output |
| **Session disposal** | Clean teardown of agent sessions and workspaces |

### NudgeNik-Inspired Analysis (for Intermute)

Agent interpretation capabilities:
- **Triage**: Identify which sessions need attention
- **Evaluation**: Verify claimed test execution, assess completion
- **Intelligent prompting**: Flag repetitive failures, mismatched claims
- **Escalation**: Recommend model switches, trigger human intervention

## Gastown Principles Applied

Insights from [Maggie Appleton's Gastown article](https://maggieappleton.com/gastown):

| Gastown Principle | Application |
|-------------------|-------------|
| **Design is the bottleneck** | Praude as design bottleneck - agents churn through implementation, so PRD quality is the limiting factor |
| **Specialized roles with hierarchy** | Clear separation: Praude (vision) → Tandemonium (tracking) → Vauxhall (orchestration) → Intermute (coordination) |
| **Persistent identity, ephemeral sessions** | Agent roles persist in Tandemonium tasks, sessions are disposable via tmux |
| **Continuous work queues** | Tandemonium tasks as work queues, Vauxhall assigns agents to pull from queues |
| **Agent nudging** | Intermute handles periodic prompts to keep agents engaged and detect stalls |
| **Merge conflict management** | Intermute handles conflict resolution with creative reimagining when needed |

## Intermute: Agent Coordination Layer

Successor to MCP Agent Mail. Separate project, used by Vauxhall.

### What MCP Agent Mail Has (Keep)
- Message threading with importance and attachments
- Inbox fetching with pagination
- Acknowledgment and read receipts
- File path reservations with conflict detection
- Contact policies
- Thread summarization with LLM
- Search capabilities

### What's Missing (Intermute Adds)

| Gap | Current State | Intermute Solution |
|-----|---------------|-------------------|
| **Agent discovery** | Must know names upfront | Auto-registration on startup, agent directory |
| **State awareness** | Can't tell agent status | NudgeNik-style detection (Blocked/Waiting/Working/Done) |
| **Delivery guarantee** | AckRequired optional | Guaranteed delivery with configurable retries |
| **Push notifications** | Must poll inbox | WebSocket/SSE for real-time delivery |
| **Global scope** | Per-project DB only | Global agent registry across projects |
| **Agent health** | No heartbeats | Health monitoring, stall detection, nudging |
| **Work coordination** | Only messages/locks | Task queues, assignment, handoff |

### Intermute Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Intermute Server                           │
│                                                                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐   │
│  │ Agent Registry│  │ Message Bus   │  │ NudgeNik Engine   │   │
│  │ - Discovery   │  │ - Delivery    │  │ - State detection │   │
│  │ - Health      │  │ - Threading   │  │ - Stall detection │   │
│  │ - Heartbeats  │  │ - Attachments │  │ - Nudge triggers  │   │
│  └───────────────┘  └───────────────┘  └───────────────────┘   │
│                                                                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐   │
│  │ Reservations  │  │ Task Queues   │  │ Conflict Resolver │   │
│  │ - File locks  │  │ - Assignment  │  │ - Merge detection │   │
│  │ - TTL expiry  │  │ - Handoff     │  │ - Resolution      │   │
│  └───────────────┘  └───────────────┘  └───────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Storage (SQLite/Postgres)              │  │
│  │  agents | messages | reservations | tasks | health       │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Core API (HTTP + WebSocket)

**Agent Lifecycle**
```
POST   /api/agents                    # Register agent
DELETE /api/agents/{id}               # Unregister agent
GET    /api/agents                    # List all agents
GET    /api/agents/{id}               # Get agent details
POST   /api/agents/{id}/heartbeat     # Send heartbeat
```

**Messaging**
```
POST   /api/messages                  # Send message
GET    /api/inbox/{agent}             # Fetch inbox (paginated)
POST   /api/messages/{id}/ack         # Acknowledge message
POST   /api/messages/{id}/read        # Mark as read
WS     /ws/inbox/{agent}              # Real-time message stream
```

**File Coordination**
```
POST   /api/reserve                   # Reserve file paths
DELETE /api/reserve/{agent}           # Release reservations
GET    /api/reservations              # List all reservations
GET    /api/conflicts                 # List active conflicts
```

**NudgeNik Analysis**
```
GET    /api/analyze/{agent}           # Get agent state analysis
POST   /api/nudge/{agent}             # Send nudge prompt
GET    /api/stalls                    # List stalled agents
```

**Task Coordination**
```
POST   /api/tasks                     # Create task in queue
GET    /api/tasks/available           # Get available tasks
POST   /api/tasks/{id}/claim          # Claim task for agent
POST   /api/tasks/{id}/complete       # Mark task complete
POST   /api/tasks/{id}/handoff        # Hand off to another agent
```

### Agent States (NudgeNik)

| State | Meaning | Detection | Action |
|-------|---------|-----------|--------|
| **Working** | Making progress | Recent file changes, terminal output | None |
| **Waiting** | Needs user input | Prompt visible, no progress | Notify user |
| **Blocked** | Needs permission | Permission dialog visible | Auto-approve or escalate |
| **Stalled** | No progress, not waiting | No activity, same output loop | Nudge or escalate |
| **Done** | Completed task | Success message, clean exit | Close session |
| **Error** | Failed | Error output, crash | Alert user |

### Escalation Triggers

- **Stall > 5 min**: Send nudge prompt
- **Stall > 15 min**: Alert user
- **Same error 3x**: Recommend different approach
- **Conflict unresolved 10 min**: Escalate to user
- **Heartbeat missed 2x**: Mark agent unhealthy

### Agent Registration Flow

**Environment Variable Priority:**
1. **Project `.env`** - Highest priority, per-project override
2. **Vauxhall-set** - When Vauxhall spawns the session
3. **Shell profile** - Fallback default (~/.bashrc, ~/.zshrc)

**Config Variables:**
```bash
# Shell profile (~/.bashrc or ~/.zshrc)
export INTERMUTE_URL="http://localhost:7338"
export INTERMUTE_AGENT_NAME="${HOSTNAME}-${USER}"  # Optional default name

# Project .env (overrides shell profile)
INTERMUTE_URL=http://localhost:7338
INTERMUTE_PROJECT=/path/to/project
```

**Registration Sequence (Vauxhall-launched):**

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Vauxhall │     │   tmux   │     │  Agent   │     │ Intermute│
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ spawn session  │                │                │
     │ (sets env vars)│                │                │
     │───────────────>│                │                │
     │                │                │                │
     │                │ start agent    │                │
     │                │───────────────>│                │
     │                │                │                │
     │                │                │ POST /agents   │
     │                │                │ (auto-register)│
     │                │                │───────────────>│
     │                │                │                │
     │                │                │   agent_id     │
     │                │                │<───────────────│
     │                │                │                │
     │                │                │ heartbeat loop │
     │                │                │───────────────>│
     │                │                │                │
```

**Registration Sequence (Pre-existing agent):**

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│  Agent   │     │ Intermute│     │ Vauxhall │
└────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │
     │ POST /agents   │                │
     │ (manual/hook)  │                │
     │───────────────>│                │
     │                │                │
     │   agent_id     │                │
     │<───────────────│                │
     │                │                │
     │                │ GET /agents    │
     │                │ (poll/subscribe│
     │                │<───────────────│
     │                │                │
     │                │ agent appears  │
     │                │───────────────>│
     │                │                │
```

**Agent Registration Payload:**
```json
{
  "name": "claude-shadow-work",
  "type": "claude",
  "project_path": "/root/projects/shadow-work",
  "session_id": "shadow-work-claude",
  "capabilities": ["code", "review", "test"],
  "metadata": {
    "model": "claude-opus-4-5-20251101",
    "started_at": "2026-01-23T12:00:00Z"
  }
}
```

**Response:**
```json
{
  "agent_id": "agent-abc123",
  "name": "claude-shadow-work",
  "inbox_ws": "ws://localhost:7338/ws/inbox/agent-abc123",
  "heartbeat_interval": 30
}
```

**Claude Code Hook for Auto-Registration:**

Create a PreToolUse hook that registers on first tool call:

```bash
# ~/.claude/hooks/pre-tool-use.sh
if [ -z "$INTERMUTE_REGISTERED" ] && [ -n "$INTERMUTE_URL" ]; then
  curl -s -X POST "$INTERMUTE_URL/api/agents" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"claude-$(basename $PWD)\",
      \"type\": \"claude\",
      \"project_path\": \"$PWD\",
      \"session_id\": \"$TMUX_PANE\"
    }" > /dev/null
  export INTERMUTE_REGISTERED=1
fi
```

**Heartbeat:**
- Agents send heartbeat every 30s (configurable)
- Missed 2 consecutive heartbeats = unhealthy
- Missed 5 consecutive heartbeats = presumed dead, session marked for cleanup

### NudgeNik State Detection

**Three-tier detection approach:**

1. **Pattern Matching (Fast, Free)** - Known patterns for common cases
2. **Trained Classifier (Fast, Accurate)** - ML model for nuanced cases
3. **LLM Fallback (Slow, Flexible)** - For novel situations

```
Terminal Buffer
     │
     ▼
┌──────────────────┐
│ Pattern Matching │──── Known pattern? ──── Yes ──► State
└──────────────────┘                                  │
     │ No match                                       │
     ▼                                                │
┌──────────────────┐                                  │
│ Classifier Model │──── High confidence? ─── Yes ──►│
└──────────────────┘                                  │
     │ Low confidence                                 │
     ▼                                                │
┌──────────────────┐                                  │
│  LLM Analysis    │─────────────────────────────────►│
└──────────────────┘                                  │
                                                      ▼
                                                   Result
```

**Pattern Matching (Tier 1):**

Known patterns for immediate classification:

| Agent | Pattern | State |
|-------|---------|-------|
| Claude | `◐ Thinking...` | Working |
| Claude | `? ` (question prompt) | Waiting |
| Claude | `Allow?` / `Approve?` | Blocked |
| Claude | `✓ Done` / task complete | Done |
| Codex | `Generating...` | Working |
| Codex | `Continue?` | Waiting |
| All | Permission denied loop | Blocked |
| All | Same output 3x in 60s | Stalled |
| All | Process exit 0 | Done |
| All | Process exit non-zero | Error |

**Trained Classifier (Tier 2):**

Train a lightweight classifier on labeled terminal sessions:

```python
# Training data format
{
  "buffer": "Recent 500 chars of terminal output...",
  "agent_type": "claude",
  "state": "working",  # Label: working|waiting|blocked|stalled|done|error
  "confidence": 0.95
}
```

**Model options:**
- **Small transformer** (DistilBERT fine-tuned) - ~100MB, runs locally
- **Logistic regression** on TF-IDF features - <1MB, very fast
- **LSTM** on character sequences - ~10MB, good for patterns

**Training data collection:**
- Log all terminal buffers with human-labeled states
- Use LLM to bootstrap labels, human-verify
- Continuously improve from production data

**LLM Fallback (Tier 3):**

For cases where pattern matching misses and classifier has low confidence:

```json
{
  "role": "system",
  "content": "You are analyzing AI agent terminal output to determine operational state."
}
{
  "role": "user",
  "content": "Classify the agent state from this terminal output:\n\n```\n{buffer}\n```\n\nRespond with JSON: {\"state\": \"working|waiting|blocked|stalled|done|error\", \"evidence\": [\"...\"], \"confidence\": 0.0-1.0}"
}
```

**State Definitions:**

| State | Evidence | Human Meaning |
|-------|----------|---------------|
| **Working** | Active file changes, tool calls, thinking indicator | Agent is making progress |
| **Waiting** | Question prompt visible, cursor at input | Needs user to answer question |
| **Blocked** | Permission dialog, approval needed | Needs permission to proceed |
| **Stalled** | No progress, repeated same actions | Stuck, may need nudge or help |
| **Done** | Success message, clean exit, task complete | Finished assigned work |
| **Error** | Exception, crash, error message | Failed, needs attention |

**Confidence Thresholds:**
- Pattern match: Always 1.0 confidence
- Classifier ≥ 0.85: Use classifier result
- Classifier < 0.85: Fall back to LLM
- LLM < 0.7: Mark as "uncertain", alert user

**Nudge Actions by State:**

| State | Action |
|-------|--------|
| Working | None - let it work |
| Waiting | Notify user of pending question |
| Blocked | Auto-approve if safe, else notify user |
| Stalled | Send nudge prompt via agent inbox |
| Done | Close session, update task status |
| Error | Alert user, log for debugging |

### Training Data Collection

**Passive Collection (Always On):**

Intermute continuously collects terminal snapshots for training:

```sql
CREATE TABLE training_samples (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    agent_type TEXT NOT NULL,       -- claude, codex, gemini
    buffer TEXT NOT NULL,           -- Terminal output (last 500 chars)
    buffer_full TEXT,               -- Full context if needed (last 2000 chars)
    detected_state TEXT,            -- Pattern/classifier detection
    detected_confidence REAL,       -- 0.0-1.0
    llm_state TEXT,                 -- LLM fallback result (if used)
    llm_confidence REAL,
    human_label TEXT,               -- Human correction (ground truth)
    labeled_at TEXT,
    collected_at TEXT NOT NULL,
    session_id TEXT,
    project_path TEXT
);

CREATE INDEX idx_training_unlabeled ON training_samples(human_label) WHERE human_label IS NULL;
CREATE INDEX idx_training_by_state ON training_samples(detected_state, detected_confidence);
```

**Collection Strategy:**

| Trigger | What's Captured | Why |
|---------|-----------------|-----|
| State change | Buffer at transition | Learn state boundaries |
| Low confidence (< 0.85) | Buffer + LLM result | Improve weak spots |
| Every 60s (sampled) | 10% of buffers | General coverage |
| User correction | Buffer + correct label | Ground truth |
| Nudge sent | Buffer before/after | Learn what worked |

**Human Labeling Interface:**

Vauxhall includes a labeling UI for training data:

```
┌─────────────────────────────────────────────────────────────────┐
│  Training Data Review              [Skip] [Working] [Stalled]  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Agent: claude-shadow-work                                      │
│  Detected: Working (0.72)  ← Low confidence                     │
│  LLM said: Stalled (0.81)                                       │
│                                                                 │
│  Terminal output:                                               │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │ ◐ Thinking...                                              │ │
│  │                                                            │ │
│  │ I'll try a different approach to fix this test.           │ │
│  │                                                            │ │
│  │ ◐ Thinking...                                              │ │
│  │                                                            │ │
│  │ Let me check the error message again.                      │ │
│  │                                                            │ │
│  │ ◐ Thinking...                                              │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  What state is this agent in?                                   │
│  [Working] [Waiting] [Blocked] [Stalled] [Done] [Error] [Skip] │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Bootstrap Strategy:**

1. **Seed with synthetic data:**
   - Generate examples for each state using templates
   - Include agent-specific patterns

2. **Use LLM for initial labeling:**
   - Run GPT-4/Claude on historical logs
   - Human-verify a sample (10-20%)
   - Train classifier on verified + LLM labels

3. **Active learning loop:**
   - Deploy classifier
   - Surface low-confidence samples for human review
   - Retrain weekly on new labels

4. **Disagreement mining:**
   - When classifier and LLM disagree, prioritize for review
   - These are the most informative samples

**Training Pipeline:**

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Raw Logs    │────▶│  LLM Label   │────▶│  Human       │
│  (passive)   │     │  (bootstrap) │     │  Review      │
└──────────────┘     └──────────────┘     └──────────────┘
                                                │
                                                ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Deploy      │◀────│  Evaluate    │◀────│  Train       │
│  Model       │     │  (holdout)   │     │  Classifier  │
└──────────────┘     └──────────────┘     └──────────────┘
       │
       │ Production feedback
       ▼
┌──────────────┐
│  Active      │──── Low confidence samples ────► Human Review
│  Learning    │
└──────────────┘
```

**Metrics to Track:**

| Metric | Target | Meaning |
|--------|--------|---------|
| Pattern match rate | > 60% | Most cases handled instantly |
| Classifier accuracy | > 90% | When classifier is used |
| LLM fallback rate | < 10% | Expensive calls minimized |
| Human correction rate | < 5% | System is accurate |
| Time to detection | < 2s | Fast feedback loop |

### Conflict Resolution

**Philosophy:** Allow maximum parallelism. Let agents work freely on shared files. Resolve conflicts through a dedicated negotiator agent.

**File Reservation Model:**

- **Shared reads**: Any agent can read any file
- **No write locks**: Agents modify files optimistically
- **Conflict detection at merge**: When agent completes, check for conflicts
- **Negotiator resolves**: Dedicated agent reconciles conflicting changes

**Conflict Detection:**

```
Agent A                     Agent B                     Intermute
   │                           │                           │
   │ start task                │ start task                │
   │──────────────────────────────────────────────────────>│
   │                           │──────────────────────────>│
   │                           │                           │
   │ modify foo.go             │ modify foo.go             │
   │                           │                           │
   │ complete task             │                           │
   │ (push branch)             │                           │
   │──────────────────────────────────────────────────────>│
   │                           │                           │
   │                           │ complete task             │
   │                           │ (push branch)             │
   │                           │──────────────────────────>│
   │                           │                           │
   │                           │        ┌─────────────────┐│
   │                           │        │ Conflict in     ││
   │                           │        │ foo.go detected ││
   │                           │        └─────────────────┘│
   │                           │                           │
   │                           │   spawn negotiator        │
   │                           │──────────────────────────>│
```

**Negotiator Agent:**

A dedicated agent process that specializes in conflict resolution:

**Negotiator Agent Configuration:**

```yaml
# ~/.intermute/config.yaml
negotiator:
  # Agent type and model
  type: claude                    # claude, codex, gpt, custom
  model: claude-3-haiku           # Fast/cheap for routine conflicts
  escalate_model: claude-opus-4   # For complex conflicts

  # Behavior
  max_attempts: 3                 # Retries before escalating
  timeout: 5m                     # Max time per conflict
  run_tests: true                 # Validate resolution with tests
  test_timeout: 2m                # Max time for test validation

  # Escalation rules
  escalate_on:
    - security_files              # Always escalate security-related files
    - database_migrations         # Schema changes need human review
    - confidence_below: 0.7       # Low confidence resolutions
    - test_failures: 2            # After 2 failed test runs

  # File patterns
  always_escalate:
    - "**/.env*"
    - "**/secrets/**"
    - "**/migrations/**"
    - "**/auth/**"

  prefer_agent:
    # When conflicts involve these, prefer the primary agent
    primary: claude
    on_tie: prefer_primary        # prefer_primary, prefer_secondary, escalate

  # Session management
  persistent: false               # Keep negotiator running between conflicts?
  session_prefix: "negotiator-"   # tmux session naming

  # Custom prompts
  prompts:
    analyze: |
      Analyze the conflict between these two changes...
    propose: |
      Propose a resolution that preserves both intents...
    validate: |
      Verify the merged result is correct...
```

**Per-Project Overrides:**

```yaml
# project/.intermute.yaml
negotiator:
  # Override for this project
  model: claude-opus-4            # Use stronger model for this codebase
  run_tests: true
  test_command: "go test ./..."   # Project-specific test command

  # Project-specific escalation
  always_escalate:
    - "internal/billing/**"       # Billing code needs human review
```

**Environment-Based Configuration:**

```bash
# Environment variables (override config)
INTERMUTE_NEGOTIATOR_MODEL=claude-opus-4
INTERMUTE_NEGOTIATOR_TIMEOUT=10m
INTERMUTE_NEGOTIATOR_ESCALATE_ON_SECURITY=true
```

**Multiple Negotiator Strategies:**

Different strategies for different conflict types:

```yaml
negotiator:
  strategies:
    default:
      model: claude-3-haiku
      approach: conservative      # Prefer clean merges, escalate often

    refactoring:
      model: claude-opus-4
      approach: aggressive        # Try harder to combine changes
      match_patterns:
        - "*_test.go"
        - "**/*.test.ts"

    documentation:
      model: claude-3-haiku
      approach: append            # Just concatenate doc changes
      match_patterns:
        - "*.md"
        - "docs/**"

    config_files:
      model: claude-opus-4
      approach: semantic          # Understand config structure
      match_patterns:
        - "*.yaml"
        - "*.json"
        - "*.toml"
```

**Negotiator Modes:**

| Mode | Description | Use Case |
|------|-------------|----------|
| **Conservative** | Prefer escalation over risky merges | Production-critical code |
| **Aggressive** | Try hard to auto-resolve | Fast iteration, low-risk code |
| **Semantic** | Understand file structure, merge intelligently | Config files, schemas |
| **Append** | Just combine changes (for docs, logs) | Documentation, changelogs |
| **Interactive** | Ask agents to revise their changes | Complex conflicts |

**Interactive Mode:**

Instead of just merging, negotiator can ask agents to revise their changes. This is the most collaborative approach.

**When to Use Interactive Mode:**
- Both changes are large and intertwined
- Auto-merge would lose context or intent
- Changes are architecturally significant
- Conflict involves API contracts

**Interactive Negotiation Flow:**

```
┌───────────┐     ┌────────────┐     ┌───────────┐     ┌───────────┐
│  Agent A  │     │ Negotiator │     │  Agent B  │     │ Intermute │
└─────┬─────┘     └──────┬─────┘     └─────┬─────┘     └─────┬─────┘
      │                  │                 │                 │
      │                  │ Conflict detected                 │
      │                  │<────────────────────────────────────│
      │                  │                 │                 │
      │                  │ Analyze both changes              │
      │                  │                 │                 │
      │ Request revision │                 │                 │
      │<─────────────────│                 │                 │
      │                  │                 │                 │
      │ Submit revision  │                 │                 │
      │─────────────────>│                 │                 │
      │                  │                 │                 │
      │                  │ Request revision│                 │
      │                  │────────────────>│                 │
      │                  │                 │                 │
      │                  │ Submit revision │                 │
      │                  │<────────────────│                 │
      │                  │                 │                 │
      │                  │ Verify compatible                 │
      │                  │                 │                 │
      │                  │ Run tests                         │
      │                  │                 │                 │
      │                  │ Merge & commit  │                 │
      │                  │────────────────────────────────────>│
      │                  │                 │                 │
```

**Revision Request Message:**

Negotiator sends via Intermute inbox:

```json
{
  "type": "revision_request",
  "conflict_id": "conflict-abc123",
  "file_path": "internal/api/handler.go",
  "your_changes": {
    "summary": "Added validateInput() function",
    "diff": "..."
  },
  "other_agent": "codex-1",
  "other_changes": {
    "summary": "Refactored input handling to use InputValidator",
    "diff": "..."
  },
  "suggested_revision": {
    "approach": "Integrate your validation into their InputValidator pattern",
    "steps": [
      "Move validateInput() logic into InputValidator.Validate()",
      "Update your callers to use InputValidator",
      "Remove standalone validateInput() function"
    ]
  },
  "constraints": [
    "Preserve all validation rules from your original change",
    "Use the InputValidator interface from their change",
    "Ensure backward compatibility for existing API callers"
  ],
  "deadline": "2026-01-23T12:30:00Z"
}
```

**Agent Revision Response:**

Agent responds with revised changes:

```json
{
  "type": "revision_response",
  "conflict_id": "conflict-abc123",
  "status": "revised",
  "branch": "feature/validate-input-v2",
  "commit": "abc123",
  "summary": "Moved validation logic into InputValidator pattern",
  "changes": [
    {
      "file": "internal/api/handler.go",
      "description": "Removed standalone validateInput()"
    },
    {
      "file": "internal/api/validator.go",
      "description": "Added validation rules to InputValidator.Validate()"
    }
  ],
  "notes": "Preserved all original validation rules. Added test cases."
}
```

**Revision States:**

| State | Meaning |
|-------|---------|
| **pending** | Revision requested, waiting for agent |
| **in_progress** | Agent acknowledged, working on revision |
| **submitted** | Agent submitted revised changes |
| **accepted** | Negotiator accepted revision |
| **rejected** | Negotiator needs more changes |
| **timeout** | Agent didn't respond in time |
| **declined** | Agent can't/won't revise |

**Handling Declined Revisions:**

If agent declines to revise:

```json
{
  "type": "revision_response",
  "conflict_id": "conflict-abc123",
  "status": "declined",
  "reason": "My change is a critical bugfix that must be preserved as-is",
  "suggestion": "Apply my change first, then have Agent B rebase on top"
}
```

Negotiator options:
1. **Accept reasoning**: Apply suggested order
2. **Ask other agent**: Request revision from Agent B instead
3. **Escalate**: Let human decide

**Conversation Thread:**

Multiple rounds of revision are supported:

```
Round 1: Negotiator requests revision from Agent A
         Agent A submits revision
         Negotiator: "Almost there, but you missed the edge case for null input"

Round 2: Negotiator requests follow-up revision
         Agent A submits second revision
         Negotiator: "Looks good, merging"
```

**Interactive Mode Configuration:**

```yaml
negotiator:
  interactive:
    enabled: true
    max_rounds: 3                 # Max revision rounds before escalating
    response_timeout: 10m         # How long to wait for revision
    allow_decline: true           # Agents can refuse to revise
    require_both: false           # Only one agent needs to revise?
    prefer_revision_from: newer   # Ask the agent with newer changes to revise
```

**Dashboard View:**

```
┌─────────────────────────────────────────────────────────────────┐
│  Interactive Negotiation: conflict-abc123                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  File: internal/api/handler.go                                  │
│  Status: AWAITING REVISION from claude-1                        │
│                                                                 │
│  Timeline:                                                      │
│  ├─ 12:00 Conflict detected                                     │
│  ├─ 12:01 Negotiator analyzed changes                           │
│  ├─ 12:02 Revision requested from claude-1                      │
│  └─ 12:02 Waiting... (8m remaining)                             │
│                                                                 │
│  claude-1 changes: Added validateInput() function               │
│  codex-1 changes:  Refactored to use InputValidator             │
│                                                                 │
│  Suggested resolution:                                          │
│  Move claude-1's validation into codex-1's InputValidator       │
│                                                                 │
│  [View Diffs] [Nudge Agent] [Escalate] [Cancel]                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Negotiator Workflow:**

```
┌─────────────────────────────────────────────────────────────────┐
│                     Negotiator Agent                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Receive conflict notification                               │
│     - Branch A changes to foo.go                                │
│     - Branch B changes to foo.go                                │
│     - Conflict regions identified                               │
│                                                                 │
│  2. Analyze intent                                               │
│     - Read original file                                        │
│     - Read both diffs                                           │
│     - Understand what each agent was trying to do               │
│                                                                 │
│  3. Propose resolution                                          │
│     a) Clean merge (changes don't overlap semantically)         │
│     b) Prefer one (one change subsumes the other)               │
│     c) Combine (merge both changes intelligently)               │
│     d) Escalate (can't resolve, need human)                     │
│                                                                 │
│  4. Validate                                                    │
│     - Apply proposed resolution                                 │
│     - Run tests                                                 │
│     - If tests fail, try again or escalate                      │
│                                                                 │
│  5. Complete                                                    │
│     - Commit merged result                                      │
│     - Notify original agents                                    │
│     - Update task status                                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Negotiator Prompt:**

```markdown
You are a Negotiator agent responsible for resolving merge conflicts.

## Conflict Context

File: {file_path}
Original content:
```
{original}
```

Agent A ({agent_a_name}) changes:
```diff
{diff_a}
```
Agent A's task: {task_a_description}

Agent B ({agent_b_name}) changes:
```diff
{diff_b}
```
Agent B's task: {task_b_description}

## Your Task

1. Understand what each agent was trying to accomplish
2. Determine if the changes are compatible
3. Propose a resolution that preserves both intents
4. If changes are incompatible, explain why and recommend which to keep

Respond with:
- resolution_type: "clean_merge" | "prefer_a" | "prefer_b" | "combine" | "escalate"
- proposed_content: The merged file content (if not escalating)
- reasoning: Why this resolution is correct
- tests_to_run: Which tests should verify this change
```

**Escalation Criteria:**

| Condition | Action |
|-----------|--------|
| Negotiator can't resolve in 3 attempts | Escalate to human |
| Tests fail after merge | Try different approach, then escalate |
| Semantic conflict (both changes valid but incompatible) | Escalate to human |
| Security-sensitive file | Always escalate |
| Negotiator confidence < 0.7 | Escalate to human |

**Conflict Queue:**

```sql
CREATE TABLE conflicts (
    id TEXT PRIMARY KEY,
    file_path TEXT NOT NULL,
    branch_a TEXT NOT NULL,
    branch_b TEXT NOT NULL,
    agent_a TEXT NOT NULL,
    agent_b TEXT NOT NULL,
    detected_at TEXT NOT NULL,
    status TEXT NOT NULL,          -- pending, negotiating, resolved, escalated
    negotiator_session TEXT,
    resolution_type TEXT,
    resolved_at TEXT,
    escalated_at TEXT,
    escalated_reason TEXT
);
```

**Integration with Vauxhall:**

Vauxhall shows conflict queue in dashboard:

```
┌─────────────────────────────────────────────────────────────────┐
│  Conflicts (2 pending, 1 negotiating)                           │
├─────────────────────────────────────────────────────────────────┤
│  ● NEGOTIATING  foo.go                                          │
│    claude-1 vs codex-1 | Negotiator: 2m elapsed                 │
│                                                                 │
│  ○ PENDING      bar.go                                          │
│    claude-2 vs claude-3 | Waiting for negotiator                │
│                                                                 │
│  ⚠ ESCALATED    security/auth.go                                │
│    claude-1 vs codex-2 | Needs human review                     │
│    Reason: Security-sensitive file                              │
└─────────────────────────────────────────────────────────────────┘
```

## Additional Patterns from Industry

### From Schmux (Not Yet Covered)

**Workspace Overlays:**
Auto-copy local-only files to new workspaces:
```yaml
overlays:
  - ".env"
  - ".env.local"
  - "config/local.yaml"
  - ".claude/settings.json"
```
When spawning a new workspace, copy these files from template.

**Quick Launch Presets:**
Pre-configured combinations of repo + targets + prompts:
```yaml
presets:
  - name: "shadow-work review"
    repo: "github.com/user/shadow-work"
    targets: ["claude", "codex"]
    prompt: "Review recent changes and suggest improvements"

  - name: "jawncloud feature"
    repo: "github.com/user/jawncloud"
    targets: ["claude"]
    prompt: "Implement the next feature from the backlog"
```

**Tool Auto-Detection:**
Automatically detect available agent CLIs on startup:
- Scan PATH for `claude`, `codex`, `gemini`, `aider`, etc.
- Probe for API keys in environment
- Register detected tools as available run targets

### From Gastown (Not Yet Covered)

**Seancing (Session Resumption):**
Query previous sessions about unfinished work:
```
POST /api/seance/{session_id}
{
  "question": "What were you working on when you stopped?",
  "context_window": 1000  // Last N tokens of session
}
```
Useful when picking up work from a previous agent.

**Maintenance Agents ("Boot the Dog"):**
Background agents that perform cleanup:
- Prune old worktrees
- Archive completed sessions
- Update dependencies
- Run linters/formatters
- Clean up temp files

**Stacked Diffs Workflow:**
Atomic changes merged incrementally to reduce conflicts:
```
main ← stack-1 ← stack-2 ← stack-3
       (done)    (review)  (wip)
```
Each agent works on one stack level. Reduces merge conflicts vs. long-lived branches.

**Context Rot Mitigation:**
Strategies to prevent information decay in long sessions:
- Periodic summarization of conversation
- Key facts extracted and pinned
- Session time limits (force refresh after N hours)
- "Memory consolidation" at checkpoints

**Hooks as Work Pointers:**
Each agent has a "hook" indicating current assigned work:
```yaml
agent: claude-1
hook:
  task_id: "TASK-001"
  file: "internal/api/handler.go"
  line: 42
  context: "Implementing input validation"
```

### From CrewAI

**Agent Training (Human-in-the-Loop):**
Train agents to improve over time:
- Capture successful task completions as examples
- Human corrections become training data
- Periodic fine-tuning on project-specific patterns

**Real-Time Tracing:**
Observe every step of agent workflows:
```json
{
  "trace_id": "trace-123",
  "agent": "claude-1",
  "events": [
    {"type": "task_start", "task": "TASK-001", "ts": "..."},
    {"type": "tool_call", "tool": "Read", "args": {...}, "ts": "..."},
    {"type": "thinking", "content": "...", "ts": "..."},
    {"type": "tool_call", "tool": "Edit", "args": {...}, "ts": "..."},
    {"type": "task_complete", "result": "...", "ts": "..."}
  ]
}
```

**Memory/Knowledge Components:**
- **Short-term memory**: Current conversation context
- **Long-term memory**: Learned patterns, project knowledge
- **Episodic memory**: Past task completions
- **Semantic memory**: Domain knowledge, code patterns

### From Google ADK

**Hierarchical Multi-Agent Structures:**

Agents can spawn sub-agents to handle specific parts of their work. This creates a tree structure:

```
Orchestrator Agent (Vauxhall-launched)
├── Planning Agent
│   └── Research Sub-agents (parallel, ephemeral)
├── Implementation Agent
│   ├── Frontend Agent (long-running)
│   └── Backend Agent (long-running)
└── Review Agent
    ├── Code Review Sub-agent (ephemeral)
    └── Test Review Sub-agent (ephemeral)
```

**Agent Lifecycle Types:**

| Type | Spawned By | Lifespan | Use Case |
|------|------------|----------|----------|
| **Root** | Vauxhall | Long-running | Main work agents (claude-1, codex-1) |
| **Sub-agent** | Parent agent | Task-scoped | Delegate specific work |
| **Ephemeral** | Any agent | Single task | Quick research, validation |
| **Specialist** | Orchestrator | On-demand | Domain experts (frontend, backend) |

**Sub-Agent Spawning:**

Parent agent requests sub-agent via Intermute:

```json
POST /api/agents/spawn
{
  "parent_id": "agent-abc123",
  "name": "research-auth-patterns",
  "type": "claude",
  "model": "claude-3-haiku",  // Cheaper for sub-tasks
  "task": {
    "description": "Research OAuth 2.0 best practices",
    "context": "...",
    "return_to": "agent-abc123"  // Report back to parent
  },
  "ephemeral": true,  // Auto-cleanup when done
  "timeout": "5m"
}
```

**Response:**
```json
{
  "agent_id": "agent-xyz789",
  "parent_id": "agent-abc123",
  "status": "spawned",
  "session_id": "research-auth-patterns-xyz",
  "inbox_ws": "ws://localhost:7338/ws/inbox/agent-xyz789"
}
```

**Sub-Agent Communication:**

Sub-agents report results back to parent via inbox:

```json
{
  "type": "sub_agent_result",
  "from": "agent-xyz789",
  "to": "agent-abc123",
  "task_id": "research-auth-patterns",
  "status": "complete",
  "result": {
    "summary": "OAuth 2.0 with PKCE is recommended for...",
    "findings": [...],
    "recommendations": [...]
  },
  "duration": "3m 42s"
}
```

**Hierarchical Configuration:**

```yaml
# Intermute config
hierarchy:
  max_depth: 3              # Max nesting level
  max_children: 5           # Max sub-agents per parent
  ephemeral_timeout: 10m    # Auto-kill ephemeral agents

  spawn_permissions:
    - parent_type: claude
      allowed_children: ["claude", "haiku"]
    - parent_type: orchestrator
      allowed_children: ["claude", "codex", "gemini"]

  models_by_depth:
    0: claude-opus-4        # Root agents: strongest model
    1: claude-sonnet-4      # First-level sub-agents
    2: claude-3-haiku       # Deeper: cheaper, faster
```

**Parallel Sub-Agent Patterns:**

1. **Fan-out / Fan-in:**
   - Parent spawns N sub-agents in parallel
   - Each handles part of the work
   - Parent waits for all, then synthesizes results

2. **Specialist Delegation:**
   - Parent identifies domain (frontend, backend, database)
   - Spawns specialist sub-agent for that domain
   - Specialist returns when done, parent continues

3. **Research Swarm:**
   - Parent spawns many small research agents
   - Each investigates one aspect
   - Parent aggregates findings

**Vauxhall Hierarchy View:**

```
┌─────────────────────────────────────────────────────────────────┐
│  Agent Hierarchy                                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ● claude-shadow-work (root)                                    │
│  │ Status: Working | Task: TASK-001                             │
│  │                                                              │
│  ├─● research-patterns (ephemeral)                              │
│  │   Status: Working | ETA: 2m                                  │
│  │                                                              │
│  └─● impl-validator (sub-agent)                                 │
│      Status: Waiting | Needs: permission                        │
│                                                                 │
│  ○ codex-shadow-work (root)                                     │
│    Status: Idle | No active task                                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Cleanup and Lifecycle:**

| Event | Action |
|-------|--------|
| Sub-agent completes task | Send result to parent, archive session |
| Sub-agent times out | Notify parent, kill session |
| Parent dies | Orphan sub-agents get new parent (Vauxhall) or killed |
| Ephemeral agent idle 5m | Auto-cleanup |

**Description-Driven Delegation:**
Agents have descriptions that LLM uses for routing:
```yaml
agents:
  - name: frontend-expert
    description: "Handles React, CSS, and UI implementation"
    capabilities: ["react", "css", "tailwind", "accessibility"]

  - name: backend-expert
    description: "Handles Go, API design, and database work"
    capabilities: ["go", "sql", "rest", "grpc"]
```
Orchestrator reads descriptions and routes tasks to appropriate agents.

**Step-by-Step Execution Tracking:**
Visual debugger showing:
- Current step in workflow
- State at each step
- Tool calls with inputs/outputs
- Decision points and reasoning

## Implementation Roadmap

### Phase 1: Core Infrastructure
**Foundation for all other features**

| Feature | Component | Description |
|---------|-----------|-------------|
| Workspace overlays | Vauxhall | Auto-copy .env files to new workspaces |
| Quick launch presets | Vauxhall | Pre-configured repo + targets + prompts |
| Tool auto-detection | Vauxhall | Scan PATH for agent CLIs |
| Real-time tracing | Intermute | Observe every step of workflows |
| Hooks as work pointers | Intermute | Track current assigned work per agent |

### Phase 2: Agent Orchestration
**Multi-agent coordination patterns**

| Feature | Component | Description |
|---------|-----------|-------------|
| Hierarchical agents | Vauxhall | Nested agent trees, sub-agents |
| Description-driven delegation | Intermute | Route tasks by agent capabilities |
| Seancing | Intermute | Query previous sessions about unfinished work |
| Stacked diffs workflow | Tandemonium | Atomic changes merged incrementally |

### Phase 3: Quality & Sustainability
**Long-term quality patterns**

| Feature | Component | Description |
|---------|-----------|-------------|
| Context rot mitigation | Intermute | Summarization, time limits, memory consolidation |
| Maintenance agents | Vauxhall | Background cleanup (prune worktrees, archive sessions) |
| Agent training | Intermute | Human-in-the-loop improvement |
| Memory components | Intermute | Short-term, long-term, episodic, semantic memory |

### Phase 4: Advanced Features
**Future enhancements**

| Feature | Component | Description |
|---------|-----------|-------------|
| Step-by-step debugger | Vauxhall | Visual debugger for agent workflows |
| Multi-host support | Vauxhall | Monitor agents on remote servers |
| Agent marketplace | Vauxhall | Share and discover agent configurations |

## Non-Goals

- Multi-host support (Phase 4 - future work)
- Authentication (assume trusted network for v1)
