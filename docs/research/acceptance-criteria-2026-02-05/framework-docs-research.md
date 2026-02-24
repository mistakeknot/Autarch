# Framework Docs Research

**Source:** Agent result from task a2bf766
**Date:** 2026-02-05

---

Now I have all the information I need. Let me compile the comprehensive research report.

---

# Framework Documentation Research for Autarch Acceptance Criteria Plan

## 1. Bubble Tea (charmbracelet/bubbletea v1.3.10)

### Summary
Bubble Tea is a Go framework for building terminal user interfaces based on The Elm Architecture (Model-View-Update). Autarch uses it extensively across 49+ files for the unified TUI, arbiter sprint views, Coldwine task management, Bigend dashboard, and Pollard research views.

### Version Information
- **Installed:** v1.3.10 (stable v1 series)
- **Available:** v2.0.0-beta.4 exists but is beta; v1.3.10 is correct for production
- **Bubbles:** v0.21.0 (TUI components)
- **Lipgloss:** v1.1.1-0.20250404203927 (pre-release pin)

### Concurrent Message Handling: Cmds vs Synchronous Updates

**Core Pattern:** All state mutation happens synchronously in `Update()`. Asynchronous work is dispatched via `Cmd` functions that return `Msg` values back to `Update()`.

```go
// CORRECT: Dispatch async work via Cmd, receive result as Msg
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Synchronous state update
        m.cursor++
        // Dispatch async work
        return m, fetchDataCmd(m.cursor)
    case DataFetchedMsg:
        // Handle async result synchronously
        m.data = msg.Data
        return m, nil
    }
    return m, nil
}

func fetchDataCmd(id int) tea.Cmd {
    return func() tea.Msg {
        // This runs in a goroutine managed by Bubble Tea
        data, err := fetchFromDB(id)
        if err != nil {
            return DataErrorMsg{Err: err}
        }
        return DataFetchedMsg{Data: data}
    }
}
```

**`tea.Batch` -- Concurrent execution, no ordering guarantees:**
```go
func (m model) Init() tea.Cmd {
    return tea.Batch(
        startPollingCmd,
        fetchInitialDataCmd,
        listenForWebSocketCmd,
    )
}
```

**`tea.Sequence` -- Sequential execution, guaranteed order:**
```go
// v1 uses Sequentially, not Sequence
cmd := tea.Sequentially(
    saveToFileCmd,
    notifyUserCmd,
)
```

**Relevance to AC plan:** The arbiter view at `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go` correctly uses this pattern. The `ArbiterView` dispatches orchestrator calls as Cmds and receives results as Msgs. The plan's concern about ~60 FPS refresh (from `State()` returning deep-copied snapshots per the institutional learnings) is addressed by Bubble Tea's framerate-based renderer, which coalesces multiple state changes into a single render frame.

### Multi-Pane Layouts with Independent Refresh Rates

**Pattern: `tea.Every` for clock-synced ticks, `tea.Tick` for independent intervals:**

```go
// Each pane can have its own tick interval
type TickMsg struct{ PaneID string }

func tickCmd(paneID string, interval time.Duration) tea.Cmd {
    return tea.Tick(interval, func(t time.Time) tea.Msg {
        return TickMsg{PaneID: paneID}
    })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case TickMsg:
        switch msg.PaneID {
        case "log":
            m.logPane.Refresh()
            return m, tickCmd("log", 100*time.Millisecond)   // 10 FPS
        case "sidebar":
            m.sidebar.Refresh()
            return m, tickCmd("sidebar", 2*time.Second)      // 0.5 FPS
        case "doc":
            m.docPane.Refresh()
            return m, tickCmd("doc", 500*time.Millisecond)   // 2 FPS
        }
    }
    return m, nil
}
```

**Relevance to AC plan:** AC-5.3 requires WebSocket updates within 2 seconds. The sidebar badge update (AC-1.3) needs <5s. These can use independent tick intervals per pane rather than a single global refresh, avoiding unnecessary re-renders of static panes.

### Testing TUI Components (teatest)

**Package:** `github.com/charmbracelet/x/exp/teatest` (experimental)

**Key API:**
```go
import "github.com/charmbracelet/x/exp/teatest"

func TestMyView(t *testing.T) {
    m := NewMyModel()
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

    // Send input
    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/gur")})

    // Wait for specific output
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return strings.Contains(string(bts), "Gurgeh")
    }, teatest.WithCheckInterval(100*time.Millisecond),
       teatest.WithDuration(3*time.Second))

    // Get final output after quitting
    tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
    out := tm.FinalOutput(t, teatest.WithFinalTimeout(time.Second))

    // Golden file comparison
    teatest.RequireEqualOutput(t, out)

    // Or check final model state
    finalModel := tm.FinalModel(t)
    myModel := finalModel.(MyModel)
    assert.Equal(t, "expected", myModel.SomeField)
}
```

**Critical caveats:**
1. Set `lipgloss.SetColorProfile(termenv.Ascii)` in test `init()` to avoid CI/CD color profile mismatches
2. Add `*.golden -text` to `.gitattributes` to prevent line ending conversion
3. `FinalOutput` blocks until `tea.Program` exits -- set appropriate timeouts
4. The `teatest` package is experimental (under `x/exp/`)

**Relevance to AC plan:** The existing test pattern at `/root/projects/Autarch/internal/tui/unified_app_test.go` uses direct `Update()` calls with `tea.KeyMsg`, which is lighter-weight than teatest but does not test rendering. For AC criteria requiring visual verification (AC-1.5, AC-X.3), teatest golden files would be appropriate. For logic-only tests (AC-1.9 confidence calculation), direct `Update()` calls are preferred.

### Performance with High-Frequency Updates

Bubble Tea uses a **framerate-based renderer** that coalesces multiple state updates into single render frames. Key facts:

- Default target is approximately 60 FPS
- Multiple `Update()` calls between frames share a single `View()` call
- The renderer uses `sync.Mutex` internally for thread safety
- `View()` should be pure and deterministic -- no side effects

**Pattern for the 60 FPS race concern from institutional learnings:**
```go
// arbiter State() returns deep copy to avoid race with TUI refresh
func (o *Orchestrator) State() *SprintState {
    o.mu.RLock()
    defer o.mu.RUnlock()
    return o.state.DeepCopy()  // Must return copy, not pointer to shared state
}
```

This is already implemented in Autarch per the `arbiter-state-pointer-escape` solution doc. The `-race` flag requirement in test categories is correct.

---

## 2. SQLite with WAL Mode (modernc.org/sqlite v1.43.0)

### Summary
Pure-Go SQLite implementation used as the database/sql driver. Autarch uses it for event storage, Coldwine task state, Gurgeh signal persistence, and session tracking.

### Version Information
- **Installed:** modernc.org/sqlite v1.43.0
- **Driver name:** `"sqlite"` (not `"sqlite3"`)
- **Key difference from mattn/go-sqlite3:** No CGO required, pure Go

### Separate Read/Write Connection Pool Patterns

**Current Autarch pattern** (at `/root/projects/Autarch/pkg/db/open.go`):
```go
db.SetMaxOpenConns(1)  // Single connection for all operations
```

This is the **simplest safe pattern** for SQLite but creates a bottleneck identified in the plan's "Data Integrity Risks" section: "SQLite single-connection bottleneck under Agent Teams. MaxOpenConns(1) serializes all reads and writes."

**Recommended pattern for concurrent agent workloads -- separate read/write pools:**

```go
// OpenReadWrite returns separate pools for reads and writes.
// Writers: 1 connection (SQLite limitation -- only one writer at a time)
// Readers: multiple connections (WAL allows concurrent reads with writes)
func OpenReadWrite(path string) (reader *sql.DB, writer *sql.DB, err error) {
    // Writer pool: exactly 1 connection
    writer, err = sql.Open("sqlite", path)
    if err != nil {
        return nil, nil, fmt.Errorf("open writer: %w", err)
    }
    writer.SetMaxOpenConns(1)
    writer.SetConnMaxLifetime(0)

    // Reader pool: multiple connections allowed in WAL mode
    reader, err = sql.Open("sqlite", path)
    if err != nil {
        writer.Close()
        return nil, nil, fmt.Errorf("open reader: %w", err)
    }
    reader.SetMaxOpenConns(4) // Scale with agent count
    reader.SetConnMaxLifetime(0)

    // Apply PRAGMAs to both pools
    for _, db := range []*sql.DB{writer, reader} {
        pragmas := []string{
            "PRAGMA journal_mode=WAL",
            "PRAGMA synchronous=NORMAL",
            "PRAGMA busy_timeout=5000",
            "PRAGMA foreign_keys=ON",
        }
        for _, p := range pragmas {
            if _, err := db.Exec(p); err != nil {
                writer.Close()
                reader.Close()
                return nil, nil, fmt.Errorf("pragma %q: %w", p, err)
            }
        }
    }

    return reader, writer, nil
}
```

**Why this works:** In WAL mode, SQLite supports unlimited concurrent readers alongside a single writer. Readers do not block writers and writers do not block readers. The writer pool stays at MaxOpenConns(1) because SQLite can only have one write transaction at a time. The reader pool can scale to 4+ connections, eliminating the bottleneck for dashboard queries (AC-5.2) while agents are writing task state.

### Per-Connection PRAGMA Enforcement

**Current gap:** The `pkg/db/Open()` function sets PRAGMAs after opening, but if `database/sql` opens a new connection in the pool, that connection will NOT have PRAGMAs applied.

**Best practice using `RegisterConnectionHook`** (available in modernc.org/sqlite):

```go
import "modernc.org/sqlite"

func init() {
    sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
        pragmas := []string{
            "PRAGMA foreign_keys = ON",
            "PRAGMA busy_timeout = 5000",
            "PRAGMA journal_mode = WAL",
            "PRAGMA synchronous = NORMAL",
        }
        for _, p := range pragmas {
            if _, err := conn.ExecContext(context.Background(), p, nil); err != nil {
                return fmt.Errorf("pragma %q: %w", p, err)
            }
        }
        return nil
    })
}
```

This ensures every new connection gets the correct PRAGMAs, regardless of pool recycling. This is especially important for `foreign_keys = ON` which is per-connection, not per-database.

**Relevance to AC plan:** The Coldwine storage at `/root/projects/Autarch/internal/coldwine/storage/db.go` correctly calls `PRAGMA foreign_keys = ON` after opening. However, the Gurgeh signals store at `/root/projects/Autarch/internal/gurgeh/signals/store.go` uses DSN params (`?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)`) which the comment in `pkg/db/open.go` says "modernc.org/sqlite does not support DSN params." This is an inconsistency that should be verified.

### Busy Timeout vs Application-Level Retry

**Current setting:** `busy_timeout=5000` (5 seconds)

**How it works:** When a writer encounters a lock from another connection, SQLite waits up to `busy_timeout` milliseconds before returning `SQLITE_BUSY`. With WAL mode, this primarily affects writer-writer conflicts (rare with MaxOpenConns(1) on the write pool) and checkpoint operations.

**Recommended layered approach for agent workloads:**

```go
// Application-level retry wrapping busy timeout
func withRetry(ctx context.Context, db *sql.DB, fn func() error) error {
    var sqliteErr *sqlite.Error
    for attempt := 0; attempt < 3; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }
        if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_BUSY {
            backoff := time.Duration(attempt+1) * 100 * time.Millisecond
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(backoff):
                continue
            }
        }
        return err // Non-retryable error
    }
    return fmt.Errorf("sqlite busy after 3 retries")
}
```

**Relevance to AC plan:** The "Timing Thresholds Summary" specifies "SQLite write p99 <100ms under 3 concurrent agent sessions." The combination of 5s busy_timeout (SQLite-level) + 3 retries with exponential backoff (application-level) + separate read/write pools should meet this target. The critical metric is actually p99 write latency, not timeout -- if writes regularly approach 5s, something is architecturally wrong.

### Connection Pool Sizing

| Configuration | MaxOpenConns | Use Case |
|--------------|-------------|----------|
| Writer pool | 1 | Mandatory. SQLite supports only one writer. |
| Reader pool (single agent) | 2 | Dashboard + TUI polling |
| Reader pool (3 agents) | 4 | One per agent + dashboard |
| Reader pool (5+ agents) | 8 | Diminishing returns above this |
| ConnMaxLifetime | 0 | No expiry for embedded DB |

---

## 3. nhooyr.io/websocket v1.8.7

### Summary
Minimal WebSocket library used by Bigend for terminal streaming and by the Intermute subscriber for real-time events. **Important:** v1.8.7 is the last version under the `nhooyr.io/websocket` module path. The library has moved to `github.com/coder/websocket` for newer versions.

### Version Information
- **Installed:** nhooyr.io/websocket v1.8.7
- **Current maintainer:** Coder (github.com/coder/websocket)
- **Status:** v1.8.7 is effectively the final release under the nhooyr.io path

### Origin Validation Best Practices

**Current Autarch code** (at `/root/projects/Autarch/internal/bigend/web/server.go` line 440):
```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: []string{"*"}, // SECURITY ISSUE: allows any origin
})
```

This is flagged as **F4 (HIGH)** in the plan's Security Findings. The `*` pattern with `filepath.Match` matches any hostname, allowing CSRF attacks from any website running on the same machine or network.

**Correct pattern for local-only servers:**

```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    // For loopback-only servers, restrict to localhost origins
    OriginPatterns: []string{
        "localhost",
        "localhost:*",
        "127.0.0.1",
        "127.0.0.1:*",
    },
})
```

**How OriginPatterns works internally:**
- Each pattern is matched case-insensitively against the request Origin header's host using `filepath.Match`
- The request host (Host header) is always authorized automatically
- If `OriginPatterns` is nil/empty AND `InsecureSkipVerify` is false, only same-origin requests are accepted
- `InsecureSkipVerify: true` disables all origin checking (use only in tests)
- The library explicitly warns: "Do not use `*` as a pattern to allow any origin, prefer to use `InsecureSkipVerify` instead to bring attention to the danger of such a setting."

**For the Signals broker** at `/root/projects/Autarch/pkg/signals/broker.go` line 61:
```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
// Empty AcceptOptions = same-origin only (correct default)
```

This is correct -- empty `AcceptOptions` restricts to same-origin.

### Concurrent Write Safety

**Thread safety model for nhooyr.io/websocket v1.8.7:**
- `Write()` / `wsjson.Write()`: Safe for concurrent use (internally serialized)
- `Reader()` / `Read()`: NOT safe for concurrent use. Only one reader at a time.
- `Close()`: Safe for concurrent use
- `Ping()`: Safe for concurrent use

**Current Autarch code is correct:** Both the terminal streaming handler (server.go line 501) and the subscriber read loop (websocket.go line 108) use single-reader patterns -- the server only writes, and the subscriber has a single `readLoop` goroutine.

**Pattern for write-only server connections (as in terminal streaming):**

```go
// CloseRead starts a background goroutine to drain reads
// and returns a context that's cancelled when the connection closes
ctx = conn.CloseRead(ctx)

// Now safe to write from multiple goroutines
for {
    select {
    case <-ctx.Done():
        return
    case data := <-updates:
        conn.Write(ctx, websocket.MessageText, data)
    }
}
```

The `CloseRead` pattern is preferred for write-only connections because it properly handles close frames from the client and cleans up the connection.

### Reconnection Patterns

**The library does not provide built-in reconnection.** The subscriber at `/root/projects/Autarch/pkg/autarch/websocket.go` currently has no reconnection logic -- when the connection drops, the `readLoop` just returns.

**Recommended reconnection pattern:**

```go
func (s *Subscriber) connectWithRetry(ctx context.Context) {
    backoff := 100 * time.Millisecond
    maxBackoff := 30 * time.Second

    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        err := s.connect(ctx)
        if err == nil {
            s.readLoop() // Blocks until disconnect
            backoff = 100 * time.Millisecond // Reset on successful connection
        }

        slog.Warn("websocket disconnected, reconnecting",
            "backoff", backoff, "error", err)

        select {
        case <-ctx.Done():
            return
        case <-time.After(backoff):
        }

        backoff = min(backoff*2, maxBackoff)
    }
}
```

**Relevance to AC plan:** AC-5.3 requires WebSocket updates within 2 seconds. Without reconnection, a dropped connection means permanent loss of dashboard updates. The reconnection pattern above with exponential backoff satisfies this requirement.

### CSRF Protection for WebSocket Endpoints

For loopback-only servers (Autarch's default), CSRF is lower risk but still relevant because any website opened in a browser on the same machine could connect to `ws://localhost:8099/ws/terminal/...`.

**Defense in depth:**
1. Restrict `OriginPatterns` to localhost (shown above)
2. Add a session token or per-connection nonce:

```go
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
    // Verify a short-lived token from the HTML page
    token := r.URL.Query().Get("token")
    if !s.verifyWSToken(token) {
        http.Error(w, "invalid token", http.StatusForbidden)
        return
    }
    // ... proceed with Accept
}
```

---

## 4. gopkg.in/yaml.v3 v3.0.1

### Summary
YAML parsing library used extensively across Autarch for spec persistence, sprint state, task definitions, feedback logs, hunter configs, and more. Used in 56+ files.

### Version Information
- **Installed:** gopkg.in/yaml.v3 v3.0.1
- **Known CVE:** CVE-2022-28948 (DoS via crafted Unmarshal input) -- fixed in v3.0.1
- **YAML spec:** YAML 1.2 compliant

### YAML Bomb Protection (Billion Laughs)

**Built-in protection in yaml.v3:** The library was modified after the Kubernetes CVE-2019-11253 incident to **fail parsing if the result object becomes too large** during anchor/alias expansion. This provides basic protection against exponential expansion attacks.

**However, the protection is not configurable** -- there is no knob to set maximum expansion depth or object size limits. For defense in depth, especially for the feedback YAML files that the plan identifies as a poisoning risk (F5 MEDIUM):

```go
// Defense in depth: limit file size before parsing
const maxFeedbackFileSize = 1 * 1024 * 1024 // 1 MB

func loadFeedback(path string) (*Feedback, error) {
    info, err := os.Stat(path)
    if err != nil {
        return nil, err
    }
    if info.Size() > maxFeedbackFileSize {
        return nil, fmt.Errorf("feedback file %s exceeds %d bytes", path, maxFeedbackFileSize)
    }

    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var fb Feedback
    dec := yaml.NewDecoder(io.LimitReader(f, maxFeedbackFileSize))
    if err := dec.Decode(&fb); err != nil {
        return nil, fmt.Errorf("decode feedback: %w", err)
    }
    return &fb, nil
}
```

**Relevance to AC plan:** The plan identifies feedback YAML poisoning (F5) as a MEDIUM risk. The `io.LimitReader` pattern is already used in Autarch for HTTP responses (`internal/pollard/pipeline/fetcher.go` line 266) but not for any YAML file loading. All current YAML loading uses `os.ReadFile()` + `yaml.Unmarshal()` with no size limits.

### KnownFields Strict Parsing

**Purpose:** Rejects YAML keys that do not map to struct fields, catching typos and injection of unexpected keys.

```go
func loadSpecStrict(path string) (*Spec, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var spec Spec
    dec := yaml.NewDecoder(f)
    dec.KnownFields(true)  // Reject unknown keys
    if err := dec.Decode(&spec); err != nil {
        return nil, fmt.Errorf("strict decode %s: %w", path, err)
    }
    return &spec, nil
}
```

**Known limitation:** `KnownFields` does NOT work with `Node.Decode()` -- the node internally creates a new decoder without the option. This is a [documented issue](https://github.com/go-yaml/yaml/issues/460).

**Current Autarch gap:** No YAML loading in Autarch uses `KnownFields`. All 56 files use `yaml.Unmarshal()` which does not support this option. This means:
- A feedback.yaml with `preference_poisoning: true` would be silently ignored
- Spec files with typos in field names would be silently dropped
- No validation of expected schema on load

**Recommended approach:** Use `KnownFields(true)` for critical files (specs, feedback, task definitions) and standard `Unmarshal` for more lenient files (configs, hunter results).

### Atomic File Write Patterns (Temp+Rename)

**Current Autarch pattern** (non-atomic, identified as Gap in the plan):

```go
// From /root/projects/Autarch/internal/gurgeh/specs/evolution.go
// SaveRevision -- TWO non-atomic writes:
os.WriteFile(snapPath, data, 0644)   // Step 1: write snapshot
os.WriteFile(revPath, revData, 0644) // Step 2: write revision metadata
// If crash between step 1 and 2: orphaned snapshot with no revision record
```

**Recommended atomic write pattern:**

```go
// atomicWriteFile writes data to path atomically via temp+rename.
// On Unix, rename is atomic within the same filesystem.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    
    tmp, err := os.CreateTemp(dir, ".tmp-*")
    if err != nil {
        return fmt.Errorf("create temp: %w", err)
    }
    tmpPath := tmp.Name()
    
    // Clean up temp file on any error
    defer func() {
        if err != nil {
            os.Remove(tmpPath)
        }
    }()
    
    if _, err = tmp.Write(data); err != nil {
        tmp.Close()
        return fmt.Errorf("write temp: %w", err)
    }
    
    // Sync to disk before rename
    if err = tmp.Sync(); err != nil {
        tmp.Close()
        return fmt.Errorf("sync temp: %w", err)
    }
    
    if err = tmp.Close(); err != nil {
        return fmt.Errorf("close temp: %w", err)
    }
    
    if err = os.Rename(tmpPath, path); err != nil {
        return fmt.Errorf("rename: %w", err)
    }
    
    return nil
}
```

**For SaveRevision (multi-file atomic operation):**

```go
func SaveRevision(root string, spec *Spec, author, trigger string, changes []Change) (*SpecRevision, error) {
    // Do NOT mutate spec.Version until both writes succeed
    version := spec.Version + 1
    
    // Prepare data without mutating input
    specCopy := *spec
    specCopy.Version = version
    
    snapData, err := yaml.Marshal(&specCopy)
    if err != nil {
        return nil, err
    }
    
    rev := &SpecRevision{
        ID:      fmt.Sprintf("%s_v%d", spec.ID, version),
        Version: version,
        // ...
    }
    revData, err := yaml.Marshal(rev)
    if err != nil {
        return nil, err
    }
    
    // Write both atomically
    snapPath := filepath.Join(dir, fmt.Sprintf("%s_v%d.yaml", spec.ID, version))
    if err := atomicWriteFile(snapPath, snapData, 0644); err != nil {
        return nil, err
    }
    
    revPath := filepath.Join(dir, fmt.Sprintf("%s_v%d_rev.yaml", spec.ID, version))
    if err := atomicWriteFile(revPath, revData, 0644); err != nil {
        // Clean up the snapshot if revision write fails
        os.Remove(snapPath)
        return nil, err
    }
    
    // Only mutate input after both writes succeed
    spec.Version = version
    
    return rev, nil
}
```

**Key fix:** The current `SaveRevision` at `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` line 47 mutates `spec.Version` as a side effect BEFORE writing files. If either write fails, the caller's spec has an incremented version that was never persisted. The fix above defers the mutation until after successful writes.

---

## 5. Claude Code Agent Teams (Experimental)

### Summary
Claude Code's multi-agent orchestration system (also called "Swarm Mode") allows a lead agent to spawn teammate agents for parallel development. Autarch's Coldwine tool is designed to bridge between Agent Teams (agent lifecycle) and Intermute (file reservation enforcement).

### Version / Feature Flag
- **Feature flag:** `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` (must be enabled)
- **Status:** Experimental. Anthropic's official documentation uses the term "subagents" rather than "teammates"
- **Availability:** Released early 2026 alongside Claude Sonnet 5

### File Structure at ~/.claude/

```
~/.claude/
  teams/{team-name}/
    config.json              # Team metadata + member list
    inboxes/
      team-lead.json         # Leader's inbox
      worker-1.json          # Teammate inboxes
      worker-2.json
  tasks/{team-name}/
    1.json                   # Individual task files
    2.json
    3.json
```

**Team config.json structure:**
```json
{
  "team_name": "feature-auth",
  "description": "Implementing OAuth2 authentication",
  "members": [
    {
      "agentId": "lead@feature-auth",
      "name": "lead",
      "agentType": "Lead",
      "color": "#4A90D9",
      "backendType": "tmux",
      "tmuxPaneId": "%12",
      "cwd": "/root/projects/MyApp"
    },
    {
      "agentId": "worker-1@feature-auth",
      "name": "worker-1",
      "agentType": "Explore",
      "color": "#50C878",
      "backendType": "tmux",
      "tmuxPaneId": "%13",
      "cwd": "/root/projects/MyApp"
    }
  ]
}
```

### Task List Format

**Location:** `~/.claude/tasks/{team-name}/N.json`

```json
{
  "id": "1",
  "subject": "Implement JWT validation middleware",
  "description": "Create middleware in internal/auth/jwt.go...",
  "status": "pending",
  "owner": "",
  "activeForm": "",
  "blockedBy": [],
  "blocks": ["3"],
  "createdAt": 1706000000000,
  "updatedAt": 1706000001000
}
```

**Status values:** `pending`, `in_progress`, `completed`

**Task claiming:** Teammates call `TaskUpdate()` to set `owner` and `status: "in_progress"`. This is a simple file write -- no locking mechanism. The task list is a flat structure; Coldwine's Initiative/Epic/Story/Task hierarchy must be flattened to leaf-level Tasks with context in the `description` field.

**Important constraint:** Teammates cannot call `TaskCreate()` -- only the leader can create tasks.

### TeammateTool Operations (13 total)

| Operation | Who Can Call | Purpose |
|-----------|-------------|---------|
| `spawnTeam` | Leader | Create team, become leader |
| `discoverTeams` | Any | List available teams |
| `requestJoin` | Teammate | Request to join a team |
| `approveJoin` | Leader | Accept join request |
| `rejectJoin` | Leader | Decline join request |
| `write` | Any | Send message to one teammate |
| `broadcast` | Any | Message all teammates |
| `requestShutdown` | Leader | Ask teammate to exit |
| `approveShutdown` | Teammate | Accept shutdown request |
| `rejectShutdown` | Teammate | Decline shutdown request |
| `approvePlan` | Leader | Approve teammate's plan |
| `rejectPlan` | Leader | Reject plan with feedback |
| `cleanup` | Leader | Remove team resources |

### Teammate Lifecycle

**Spawn phase:**
1. Leader calls `spawnTeam` with team_name and description
2. Leader creates tasks via task system
3. Leader spawns teammates (each gets env vars: `CLAUDE_CODE_TEAM_NAME`, `CLAUDE_CODE_AGENT_ID`, `CLAUDE_CODE_AGENT_NAME`, `CLAUDE_CODE_AGENT_TYPE`, `CLAUDE_CODE_PLAN_MODE_REQUIRED`)

**Active phase:**
- Teammates self-claim unblocked tasks from the shared task list
- **5-minute heartbeat timeout** for crash detection
- Communication via inbox-based messaging (JSON files)
- Broadcasting is expensive: sends N separate messages for N teammates

**Plan approval gating** (when `CLAUDE_CODE_PLAN_MODE_REQUIRED=true`):
1. Teammate enters plan mode before implementation
2. Teammate sends `plan_approval_request` message to leader
3. Leader reviews and calls `approvePlan` or `rejectPlan`
4. Teammate proceeds with implementation only after approval

**Shutdown phase:**
1. Leader calls `requestShutdown` for each teammate
2. Teammate receives `shutdown_request` message
3. Teammate calls `approveShutdown`
4. Process terminates
5. Leader calls `cleanup` to remove team resources

### Limitations and Experimental Status

1. **No session resumption:** Teammates are lost on `/resume` -- they cannot be restarted or re-attached
2. **Single leader per team:** One leader; teammates cannot spawn sub-teams (no nesting)
3. **Leader is fixed:** Cannot transfer leadership
4. **One team per session:** A single Claude Code session can lead one team
5. **In-process teammates die if leader exits:** Use tmux/iTerm2 backend for persistence
6. **No real-time output visibility** with in-process backend (use tmux for debugging)
7. **Broadcasting is O(N):** Sends N separate file writes
8. **Cannot cleanup while active teammates exist:** Must shutdown all first
9. **Task claiming has no lock:** File-based, possible race condition if two teammates claim simultaneously
10. **Token consumption:** 3-5x tokens compared to single-session development
11. **Heartbeat timeout is fixed at 5 minutes** -- not configurable

**Relevance to AC plan:**

For **Coldwine ↔ Agent Teams bridge** (Gap 2 in the plan): The task claiming mechanism is file-based with no events. Coldwine must either:
- **Poll** `~/.claude/tasks/{team-name}/*.json` at 1-2 second intervals using `fsnotify` or periodic reads
- **Wrap** the task claiming call so Coldwine intercepts before the file write

The plan's recommended `AgentTeamsClient` interface should abstract this:

```go
// AgentTeamsClient abstracts Agent Teams operations for Coldwine and Bigend.
// Implementations: FileSystemClient (reads ~/.claude/teams/), MockClient (for tests).
type AgentTeamsClient interface {
    // ListTeams returns all active teams
    ListTeams() ([]Team, error)
    
    // GetTeam returns team config and members
    GetTeam(name string) (*TeamConfig, error)
    
    // ListTasks returns all tasks for a team
    ListTasks(teamName string) ([]Task, error)
    
    // WatchTaskClaims emits events when task ownership changes
    WatchTaskClaims(ctx context.Context, teamName string) (<-chan TaskClaimEvent, error)
    
    // GetTeammateStatus returns heartbeat-based status for a teammate
    GetTeammateStatus(agentID string) (TeammateStatus, error)
}
```

For **Bigend reading team config** (AC-5.5, AC-5.7): Bigend should read `~/.claude/teams/{team-name}/config.json` for authoritative member lists and `~/.claude/tasks/{team-name}/*.json` for task assignments.

---

## Cross-Cutting Findings

### File Paths Referenced in This Report

| Path | Relevance |
|------|-----------|
| `/root/projects/Autarch/pkg/db/open.go` | SQLite connection setup -- needs read/write pool split |
| `/root/projects/Autarch/pkg/db/open_test.go` | PRAGMA verification tests |
| `/root/projects/Autarch/internal/coldwine/storage/db.go` | Coldwine SQLite with foreign_keys, shared DB pool |
| `/root/projects/Autarch/internal/gurgeh/signals/store.go` | Signal store -- DSN param inconsistency |
| `/root/projects/Autarch/internal/bigend/web/server.go` | WebSocket origin `*` vulnerability (line 440) |
| `/root/projects/Autarch/pkg/autarch/websocket.go` | WebSocket subscriber -- no reconnection logic |
| `/root/projects/Autarch/pkg/signals/broker.go` | Signal broker -- correct empty AcceptOptions, silent drop on full buffer |
| `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` | SaveRevision -- non-atomic writes, version mutation side effect |
| `/root/projects/Autarch/internal/gurgeh/specs/load.go` | YAML loading -- no KnownFields, no size limits |
| `/root/projects/Autarch/internal/gurgeh/arbiter/persistence.go` | Sprint state persistence -- non-atomic os.WriteFile |
| `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go` | Bubble Tea TUI component -- correct Cmd pattern |
| `/root/projects/Autarch/internal/tui/unified_app_test.go` | Existing TUI test pattern -- direct Update() calls |

### Priority Recommendations by AC

| Priority | Issue | Affected ACs |
|----------|-------|-------------|
| **P0** | WebSocket origin `*` must be restricted to localhost | AC-X.1, F4 |
| **P0** | SaveRevision non-atomic writes + version mutation | AC-1.15, race condition testing |
| **P1** | SQLite MaxOpenConns(1) bottleneck for agent workloads | AC-2.7, AC-4.x, timing thresholds |
| **P1** | No WebSocket reconnection in subscriber | AC-5.3 |
| **P1** | AgentTeamsClient interface needed for testability | AC-5.5, AC-5.7, AC-X.9 |
| **P2** | YAML KnownFields not used anywhere | AC-3.6, AC-3.9 (feedback integrity) |
| **P2** | No file size limits on YAML loading | F5 (feedback poisoning) |
| **P2** | RegisterConnectionHook for per-connection PRAGMAs | AC-2.7 data integrity |
| **P3** | teatest not used; golden file testing would help | AC-1.5, AC-X.3 |

### References

**Bubble Tea:**
- [GitHub - charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [Go Package Docs](https://pkg.go.dev/github.com/charmbracelet/bubbletea)
- [Writing Bubble Tea Tests (Charm blog)](https://charm.land/blog/teatest/)
- [Testing Bubble Tea Interfaces (Pattern Matched)](https://patternmatched.substack.com/p/testing-bubble-tea-interfaces)

**SQLite / modernc.org/sqlite:**
- [SQLite WAL Documentation](https://sqlite.org/wal.html)
- [SQLite Isolation](https://sqlite.org/isolation.html)
- [modernc.org/sqlite Go Package](https://pkg.go.dev/modernc.org/sqlite)
- [SQLite Optimizations for Ultra High-Performance (PowerSync)](https://www.powersync.com/blog/sqlite-optimizations-for-ultra-high-performance)

**nhooyr.io/websocket:**
- [Go Package Docs](https://pkg.go.dev/nhooyr.io/websocket)
- [coder/websocket GitHub (current maintainer)](https://github.com/coder/websocket)
- [accept.go source (v1.8.7)](https://github.com/coder/websocket/blob/v1.8.7/accept.go)
- [OriginPatterns usage references](https://ref.gotd.dev/use/nhooyr.io/websocket..AcceptOptions.OriginPatterns.html)

**gopkg.in/yaml.v3:**
- [Go Package Docs](https://pkg.go.dev/gopkg.in/yaml.v3)
- [GitHub - go-yaml/yaml](https://github.com/go-yaml/yaml)
- [CVE-2022-28948 (Snyk)](https://security.snyk.io/vuln/SNYK-GOLANG-GOPKGINYAMLV3-2841557)
- [KnownFields issue #460](https://github.com/go-yaml/yaml/issues/460)

**Claude Code Agent Teams:**
- [Official Anthropic Subagents Docs](https://docs.anthropic.com/en/docs/claude-code/sub-agents)
- [SDK Subagents Guide](https://docs.anthropic.com/en/docs/claude-code/sdk/subagents)
- [Swarm Orchestration Technical Reference (Gist)](https://gist.github.com/kieranklaassen/4f2aba89594a4aea4ad64d753984b2ea)
- [Claude Code's Hidden Multi-Agent System (paddo.dev)](https://paddo.dev/blog/claude-code-hidden-swarm/)
- [Swarm Mode Complete Guide (Apiyi)](https://help.apiyi.com/en/claude-code-swarm-mode-multi-agent-guide-en.html)