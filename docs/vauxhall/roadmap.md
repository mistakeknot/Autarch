# Vauxhall Roadmap

> Multi-project AI agent mission control dashboard

## Vision

Vauxhall provides a unified web interface for monitoring and coordinating multiple AI coding agents working across different projects. It aggregates data from Praude (PRDs), Tandemonium (tasks), MCP Agent Mail (coordination), and tmux (sessions) into a single dashboard that shows:

- **What agents are active** and what they're working on
- **Project health** across all your repositories
- **Live terminal output** from agent sessions
- **Communication threads** between agents
- **File reservations** and potential conflicts

Think of it as "mission control" for your AI engineering team.

---

## Milestones

### M0: Foundation (Done ✅)

**Goal:** Basic project scaffolding and discovery

| Task | Status | Notes |
|------|--------|-------|
| Project structure | ✅ | Go modules, internal packages |
| Configuration | ✅ | TOML config, CLI flags |
| Project discovery | ✅ | Scans for .praude/, .tandemonium/ |
| Web server | ✅ | net/http + htmx + Tailwind |
| Basic templates | ✅ | Dashboard, projects, agents, sessions |
| Initial commit | ✅ | Compiles and runs |

**Deliverable:** `./vauxhall --scan-root ~/projects` starts a web server showing discovered projects.

---

### M1: tmux Integration

**Goal:** See all tmux sessions and identify which ones have agents

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| List sessions | P0 | Low | Parse `tmux list-sessions` output |
| Session metadata | P0 | Low | Created time, window count, attached status |
| Capture pane | P1 | Medium | Get recent terminal output via `tmux capture-pane` |
| Agent detection | P1 | Medium | Heuristics to identify claude-code/codex sessions |
| Session → Project linking | P2 | Medium | Match session CWD to project paths |

**Agent Detection Heuristics:**
- Session name patterns: `*-claude`, `*-codex`, `claude-*`, `codex-*`
- Window title contains "claude" or "codex"
- Recent output contains agent signatures (thinking indicators, tool calls)
- CWD matches a known project with .tandemonium/ or MCP Agent Mail registration

**Deliverable:** Dashboard shows all tmux sessions with agent indicators and project associations.

---

### M2: Praude Integration

**Goal:** Read PRD specs and show roadmap/requirements context

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| YAML spec reader | P0 | Low | Parse .praude/specs/*.yaml files |
| PRD list endpoint | P0 | Low | API to list PRDs for a project |
| PRD detail view | P1 | Medium | Show full PRD with CUJs, requirements |
| Research artifacts | P2 | Low | List .praude/research/*.md files |
| Suggestion status | P2 | Medium | Show pending suggestions |

**Data Model:**
```go
type PRD struct {
    ID           string       // PRD-001
    Title        string
    Status       string       // draft, approved, in_progress, done
    Summary      string
    Requirements []Requirement
    CUJs         []CUJ
    FilesToModify []FileAction
    Complexity   string       // low, medium, high
    Priority     string
}
```

**Deliverable:** Project detail page shows PRD list with status, clicking shows full PRD content.

---

### M3: Tandemonium Integration

**Goal:** Read tasks and show work in progress

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| SQLite reader | P0 | Low | Read-only connection to state.db |
| Task list endpoint | P0 | Low | API to list tasks for a project |
| Task detail view | P1 | Medium | Show task spec, acceptance criteria |
| Task → PRD linking | P1 | Medium | Link tasks to source PRDs |
| Worktree status | P2 | Medium | Show which worktrees are active |

**Data Model:**
```go
type Task struct {
    ID              string    // TAND-001
    Title           string
    Status          string    // todo, in_progress, review, done, blocked
    PRDRef          string    // PRD-001 if linked
    AssignedAgent   string    // agent name if assigned
    WorktreePath    string    // if isolated
    AcceptanceCriteria []string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

**Deliverable:** Project detail page shows task board with status columns.

---

### M4: MCP Agent Mail Integration

**Goal:** Read agent registrations and message threads

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| Agent registry reader | P0 | Medium | Query registered agents from DB |
| Inbox reader | P0 | Medium | Fetch messages for agents |
| Agent profile view | P1 | Low | Show agent details, program, model |
| Message thread view | P1 | Medium | Show conversation threads |
| File reservations | P1 | Medium | Show active file locks |
| Cross-project view | P2 | High | Aggregate agents across projects |

**Data Sources:**
- Tandemonium built-in: `.tandemonium/state.db` (agents, messages, reservations tables)
- Standalone MCP Agent Mail: `~/.agent_mail/` or project `.agent_mail/`

**Deliverable:** Agents page shows all registered agents with inbox counts, clicking shows message history.

---

### M5: Live Terminal Streaming

**Goal:** Watch agent terminal output in real-time

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| WebSocket endpoint | P0 | Medium | `/ws/terminal/:session` |
| tmux streaming | P0 | Medium | Continuous capture-pane polling |
| xterm.js integration | P1 | Medium | Terminal renderer in browser |
| Output buffering | P1 | Medium | Handle fast output, scrollback |
| Multiple terminals | P2 | Medium | Tab interface for multiple sessions |
| Input forwarding | P3 | High | Optional: send keystrokes to tmux |

**Implementation Notes:**
- Use `tmux capture-pane -p -t session:window.pane` for output
- Poll every 100-200ms for responsive feel
- Buffer last N lines to avoid overwhelming browser
- Consider ANSI color preservation

**Deliverable:** Click on a session to see live terminal output in browser.

---

### M6: Activity Feed

**Goal:** Unified timeline of all agent activity

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| Activity aggregator | P0 | Medium | Combine events from all sources |
| Git commit watcher | P1 | Medium | Watch for new commits in projects |
| Message notifications | P1 | Low | New messages in threads |
| Reservation events | P1 | Low | File lock/unlock events |
| Task state changes | P1 | Low | Task moved to review, done, etc. |
| Real-time updates | P2 | Medium | WebSocket push for new activity |

**Activity Types:**
```go
type Activity struct {
    Time        time.Time
    Type        string    // commit, message, reservation, task_update, agent_start, agent_stop
    AgentName   string
    ProjectPath string
    Summary     string
    Details     any       // type-specific payload
}
```

**Deliverable:** Dashboard shows live activity feed, filterable by project/agent/type.

---

### M7: Agent Control (Future)

**Goal:** Take actions on agents, not just observe

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| Start agent | P1 | High | Launch new agent session for task |
| Stop agent | P1 | Medium | Kill tmux session gracefully |
| Send message | P1 | Medium | Compose message to agent via UI |
| Assign task | P2 | Medium | Assign task to idle agent |
| Redirect agent | P3 | High | Interrupt and give new instructions |

**Considerations:**
- Security: who can control agents?
- Confirmation dialogs for destructive actions
- Agent-specific launch configs (codex vs claude)

**Deliverable:** UI buttons to start/stop agents, send messages.

---

### M8: Multi-Host Support (Future)

**Goal:** Monitor agents on remote servers

| Task | Priority | Complexity | Description |
|------|----------|------------|-------------|
| SSH tunnel support | P1 | High | Connect to remote tmux/DBs |
| Host configuration | P1 | Medium | Define remote hosts in config |
| Unified view | P1 | Medium | Merge data from multiple hosts |
| Latency handling | P2 | Medium | Graceful degradation for slow links |

**Config Example:**
```toml
[[hosts]]
name = "local"
scan_roots = ["~/projects"]

[[hosts]]
name = "ethics-gradient"
ssh = "ethics-gradient"
scan_roots = ["/root/projects"]
```

**Deliverable:** Single dashboard showing agents across local and remote machines.

---

## Implementation Order

```
M0 ──► M1 ──► M2 ──► M3 ──► M4 ──► M5 ──► M6
       │      │      │      │
       │      └──────┴──────┘
       │             │
       │      (can parallelize)
       │
       └──► M7 (after M4)
            │
            └──► M8 (after M7)
```

**Recommended sequence:**
1. **M1 (tmux)** - Most visible impact, shows sessions immediately
2. **M2-M4 (data readers)** - Can be done in parallel by different agents
3. **M5 (terminal streaming)** - High value, depends on M1
4. **M6 (activity feed)** - Depends on M2-M4
5. **M7-M8 (control, multi-host)** - Future enhancements

---

## Technical Decisions

### Why Go?
- Matches Praude/Tandemonium stack (team familiarity)
- Single binary deployment
- Excellent concurrency for WebSockets
- html/template + embed for self-contained binary

### Why htmx over React/Vue?
- Minimal JavaScript complexity
- Server-rendered HTML is simpler to maintain
- htmx handles dynamic updates elegantly
- Tailwind provides styling without build step

### Why SQLite read-only?
- Tandemonium/Agent Mail already use SQLite
- No need to duplicate data
- Read-only prevents accidental corruption
- Can query directly without API layer

### Why not extend Tandemonium instead?
- Tandemonium is single-project focused
- Vauxhall needs cross-project aggregation
- Separate tool = cleaner separation of concerns
- Can run Vauxhall without Tandemonium

---

## File Structure (Target)

```
Vauxhall/
├── cmd/
│   └── vauxhall/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── discovery/
│   │   └── scanner.go
│   ├── praude/
│   │   ├── reader.go
│   │   └── types.go
│   ├── tandemonium/
│   │   ├── db.go
│   │   └── types.go
│   ├── agentmail/
│   │   ├── db.go
│   │   └── types.go
│   ├── tmux/
│   │   ├── client.go
│   │   ├── detector.go
│   │   └── streamer.go
│   ├── aggregator/
│   │   ├── aggregator.go
│   │   └── activity.go
│   └── web/
│       ├── server.go
│       ├── handlers.go
│       ├── websocket.go
│       └── templates/
│           ├── layout.html
│           ├── dashboard.html
│           ├── project_detail.html
│           ├── agent_detail.html
│           ├── task_board.html
│           ├── terminal.html
│           └── partials/
│               ├── activity_item.html
│               ├── agent_card.html
│               └── task_card.html
├── static/
│   ├── css/
│   │   └── app.css
│   └── js/
│       ├── terminal.js
│       └── websocket.js
├── docs/
│   └── roadmap.md
├── go.mod
├── go.sum
├── CLAUDE.md
└── AGENTS.md
```

---

## Success Metrics

### M1 Success
- [ ] All tmux sessions appear in UI within 5 seconds of creation
- [ ] Agent detection correctly identifies 90%+ of claude-code/codex sessions
- [ ] Session-to-project linking works for projects in scan roots

### M5 Success
- [ ] Terminal output appears in browser within 200ms of real output
- [ ] Can view 3+ terminal streams simultaneously without lag
- [ ] Scrollback works for at least 1000 lines

### Overall Success
- [ ] Single developer can monitor 5+ concurrent agents effectively
- [ ] Context switching between projects takes <2 seconds
- [ ] Activity feed catches all significant events within 30 seconds

---

## Open Questions

1. **Agent naming**: Should Vauxhall assign its own names, or rely on MCP Agent Mail names?
   - *Leaning:* Use MCP Agent Mail names when available, generate for unregistered sessions

2. **Persistence**: Should Vauxhall have its own DB for activity history?
   - *Leaning:* Yes, for cross-project activity timeline and faster queries

3. **Authentication**: Should there be any auth for the web UI?
   - *Leaning:* No for v1, assume trusted network (localhost/Tailscale)

4. **Mobile**: Should the UI be mobile-responsive for phone monitoring?
   - *Leaning:* Nice to have, not critical for v1

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-20 | Initial roadmap created |
| 2026-01-20 | M0 completed (foundation) |
