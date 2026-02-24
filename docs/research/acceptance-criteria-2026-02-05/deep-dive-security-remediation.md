# Deep-dive security remediation

**Source:** Agent result from session b817bfb0
**Task ID:** a5c733d
**Date:** 2026-02-05

---

I now have a complete picture of all six findings. Here is the comprehensive security remediation report.

---

# Security Remediation Report: Autarch/Intermute Codebases

## F1 (CRITICAL): No Glob Overlap Check in Intermute `Reserve()`

### Affected File
`/root/projects/Intermute/internal/storage/sqlite/sqlite.go` -- lines 813-838

### Current Vulnerable Code

```go
func (s *Store) Reserve(r core.Reservation) (*core.Reservation, error) {
    if r.ID == "" {
        r.ID = uuid.NewString()
    }
    now := time.Now().UTC()
    r.CreatedAt = now
    if r.TTL == 0 {
        r.TTL = 30 * time.Minute
    }
    r.ExpiresAt = now.Add(r.TTL)

    exclusive := 0
    if r.Exclusive {
        exclusive = 1
    }

    _, err := s.db.Exec(
        `INSERT INTO file_reservations (id, agent_id, project, path_pattern, exclusive, reason, created_at, expires_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        r.ID, r.AgentID, r.Project, r.PathPattern, exclusive, r.Reason,
        r.CreatedAt.Format(time.RFC3339Nano), r.ExpiresAt.Format(time.RFC3339Nano),
    )
    if err != nil {
        return nil, fmt.Errorf("insert reservation: %w", err)
    }
    return &r, nil
}
```

### Vulnerability

The `Reserve()` function performs a blind INSERT without checking whether any existing active reservations have overlapping glob patterns. This means:

1. Agent A can reserve `pkg/events/*.go` exclusively.
2. Agent B can then reserve `pkg/events/reconcile.go` exclusively -- no conflict check.
3. Both agents believe they have exclusive access to the same file, leading to data races and overwrites.

There is also no `filepath.Match`-based overlap detection anywhere in the Intermute codebase (confirmed by grep).

### Fix Sketch

```go
func (s *Store) Reserve(r core.Reservation) (*core.Reservation, error) {
    if r.ID == "" {
        r.ID = uuid.NewString()
    }
    now := time.Now().UTC()
    r.CreatedAt = now
    if r.TTL == 0 {
        r.TTL = 30 * time.Minute
    }
    r.ExpiresAt = now.Add(r.TTL)

    exclusive := 0
    if r.Exclusive {
        exclusive = 1
    }

    // Use a transaction for atomic check-then-insert
    tx, err := s.db.Begin()
    if err != nil {
        return nil, fmt.Errorf("begin reserve tx: %w", err)
    }
    defer tx.Rollback()

    // Fetch all active reservations in the same project
    nowStr := now.Format(time.RFC3339Nano)
    rows, err := tx.Query(
        `SELECT id, agent_id, path_pattern, exclusive
         FROM file_reservations
         WHERE project = ? AND released_at IS NULL AND expires_at > ?`,
        r.Project, nowStr,
    )
    if err != nil {
        return nil, fmt.Errorf("query active reservations: %w", err)
    }
    defer rows.Close()

    for rows.Next() {
        var existingID, existingAgent, existingPattern string
        var existingExcl int
        if err := rows.Scan(&existingID, &existingAgent, &existingPattern, &existingExcl); err != nil {
            return nil, fmt.Errorf("scan reservation: %w", err)
        }
        // Skip same agent's own reservations (agent can stack non-conflicting)
        // But still check if either side is exclusive
        if existingExcl == 1 || r.Exclusive {
            if globsOverlap(existingPattern, r.PathPattern) {
                return nil, fmt.Errorf("conflict: pattern %q overlaps with existing reservation %s (pattern %q, agent %s)",
                    r.PathPattern, existingID, existingPattern, existingAgent)
            }
        }
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate reservations: %w", err)
    }

    _, err = tx.Exec(
        `INSERT INTO file_reservations (id, agent_id, project, path_pattern, exclusive, reason, created_at, expires_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        r.ID, r.AgentID, r.Project, r.PathPattern, exclusive, r.Reason,
        r.CreatedAt.Format(time.RFC3339Nano), r.ExpiresAt.Format(time.RFC3339Nano),
    )
    if err != nil {
        return nil, fmt.Errorf("insert reservation: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("commit reserve tx: %w", err)
    }
    return &r, nil
}

// globsOverlap returns true if two glob patterns could match the same path.
// Conservative: if pattern A matches a representative set of paths from pattern B
// or vice versa, they overlap. For exact correctness, we check both directions.
func globsOverlap(a, b string) bool {
    // Direct match: one pattern matches the other literally
    if matchedAB, _ := filepath.Match(a, b); matchedAB {
        return true
    }
    if matchedBA, _ := filepath.Match(b, a); matchedBA {
        return true
    }
    // Prefix overlap: check if the non-wildcard prefixes share a common directory
    // e.g., "pkg/events/*.go" and "pkg/events/reconcile.go"
    aDir := extractGlobPrefix(a)
    bDir := extractGlobPrefix(b)
    if !strings.HasPrefix(aDir, bDir) && !strings.HasPrefix(bDir, aDir) {
        return false // Different directory trees, cannot overlap
    }
    // Same directory tree -- if either contains wildcards, conservatively assume overlap
    return containsGlobMeta(a) || containsGlobMeta(b)
}

func extractGlobPrefix(pattern string) string {
    // Return the longest prefix without glob metacharacters
    for i, c := range pattern {
        if c == '*' || c == '?' || c == '[' {
            return pattern[:i]
        }
    }
    return pattern
}

func containsGlobMeta(s string) bool {
    return strings.ContainsAny(s, "*?[")
}
```

### Acceptance Criteria

**AC-F1:** Given an active exclusive reservation for pattern `pkg/events/*.go` by agent-A in project P, when agent-B attempts to `Reserve()` pattern `pkg/events/reconcile.go` (exclusive) in project P, then `Reserve()` returns a non-nil error containing the word "conflict" and no new row is inserted into `file_reservations`.

### Test Cases

1. **Test_Reserve_ExclusiveConflict_ExactSamePattern**: Reserve `internal/*.go` exclusive, then attempt the same pattern exclusive. Expect error.
2. **Test_Reserve_ExclusiveConflict_ChildPattern**: Reserve `pkg/events/*.go` exclusive, then attempt `pkg/events/reconcile.go` exclusive. Expect error.
3. **Test_Reserve_ExclusiveConflict_ParentPattern**: Reserve `pkg/events/reconcile.go` exclusive, then attempt `pkg/events/*.go` exclusive. Expect error.
4. **Test_Reserve_SharedNonConflicting**: Reserve `pkg/events/*.go` shared (exclusive=false), then attempt `pkg/events/reconcile.go` shared. Expect success (shared+shared = OK).
5. **Test_Reserve_ExpiredDoesNotBlock**: Reserve `pkg/*.go` exclusive, manually expire it, then attempt same pattern. Expect success.
6. **Test_Reserve_DifferentProjectNoConflict**: Reserve `pkg/*.go` exclusive in project-A, then same pattern in project-B. Expect success.
7. **Test_Reserve_ReleasedDoesNotBlock**: Reserve, release, then re-reserve the same pattern. Expect success.
8. **Test_Reserve_DisjointPaths**: Reserve `cmd/*.go` exclusive, then `pkg/*.go` exclusive. Expect success (no overlap).

---

## F2 (HIGH): No Agent Identity Verification for Reservations

### Affected File
`/root/projects/Intermute/internal/http/handlers_reservations.go` -- lines 169-179

### Current Vulnerable Code

```go
func (s *Service) releaseReservation(w http.ResponseWriter, r *http.Request, id string) {
    if err := s.store.ReleaseReservation(id); err != nil {
        if strings.Contains(err.Error(), "not found") {
            w.WriteHeader(http.StatusNotFound)
        } else {
            w.WriteHeader(http.StatusInternalServerError)
        }
        return
    }
    w.WriteHeader(http.StatusOK)
}
```

Also the storage layer at `/root/projects/Intermute/internal/storage/sqlite/sqlite.go` lines 842-856:

```go
func (s *Store) ReleaseReservation(id string) error {
    now := time.Now().UTC().Format(time.RFC3339Nano)
    res, err := s.db.Exec(
        `UPDATE file_reservations SET released_at = ? WHERE id = ? AND released_at IS NULL`,
        now, id,
    )
    // ... no agent_id check
}
```

### Vulnerability

1. **No ownership check on release**: Any agent (or any caller) that knows a reservation ID can release any other agent's reservation. There is no verification that the caller is the owner.
2. **No agent_id derivation from auth context**: The `createReservation` handler accepts `agent_id` from the request body (line 98-101), meaning callers can claim to be any agent. In API key mode, auth context carries `project` but the handler does not map the key to a specific agent identity.
3. **No ownership check on create either**: In `createReservation` (lines 92-133), `req.AgentID` is taken from the request body without validation against the authenticated identity.

### Fix Sketch

```go
// In handlers_reservations.go -- releaseReservation
func (s *Service) releaseReservation(w http.ResponseWriter, r *http.Request, id string) {
    info, _ := auth.FromContext(r.Context())

    // Fetch reservation to verify ownership
    reservation, err := s.store.GetReservation(id)
    if err != nil {
        w.WriteHeader(http.StatusNotFound)
        return
    }

    // In API key mode, verify the caller's project owns this reservation
    // and the agent_id from the request (or auth) matches
    if info.Mode == auth.ModeAPIKey {
        if reservation.Project != info.Project {
            w.WriteHeader(http.StatusForbidden)
            return
        }
    }

    // For additional security, require agent_id in the request body or
    // query param and verify it matches the reservation owner
    callerAgent := r.URL.Query().Get("agent_id")
    if callerAgent != "" && callerAgent != reservation.AgentID {
        w.WriteHeader(http.StatusForbidden)
        return
    }

    if err := s.store.ReleaseReservation(id); err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
}

// In createReservation -- validate agent_id against auth context
func (s *Service) createReservation(w http.ResponseWriter, r *http.Request) {
    // ... existing decode ...
    info, _ := auth.FromContext(r.Context())

    // In API key mode, the agent_id must belong to the authenticated project
    if info.Mode == auth.ModeAPIKey {
        // Verify agent exists and belongs to the project
        agents, _ := s.store.ListAgents(info.Project)
        agentBelongs := false
        for _, a := range agents {
            if a.ID == req.AgentID || a.Name == req.AgentID {
                agentBelongs = true
                break
            }
        }
        if !agentBelongs {
            w.WriteHeader(http.StatusForbidden)
            return
        }
    }
    // ... rest of function ...
}

// New method needed on Store interface and sqlite implementation:
// GetReservation(id string) (*core.Reservation, error)
func (s *Store) GetReservation(id string) (*core.Reservation, error) {
    row := s.db.QueryRow(
        `SELECT id, agent_id, project, path_pattern, exclusive, reason, created_at, expires_at, released_at
         FROM file_reservations WHERE id = ?`, id,
    )
    // ... scan fields and return ...
}
```

### Acceptance Criteria

**AC-F2:** Given a reservation R owned by agent-A in project P, when agent-B (authenticated via API key for project P) sends DELETE `/api/reservations/{R.id}`, then the server returns 403 Forbidden and the reservation remains active.

### Test Cases

1. **Test_Release_OwnerCanRelease**: Agent-A creates reservation, Agent-A releases it. Expect 200 OK.
2. **Test_Release_NonOwnerBlocked**: Agent-A creates reservation, Agent-B attempts release. Expect 403.
3. **Test_Release_WrongProjectBlocked**: Agent from project-X attempts to release reservation from project-Y. Expect 403.
4. **Test_Create_AgentIDMustBelongToProject**: In API key mode, attempt to create reservation with agent_id not registered in the project. Expect 403.
5. **Test_Release_LocalhostStillChecksOwnership**: Even in localhost mode, verify that the agent_id ownership check is enforced (defense in depth).

---

## F3 (HIGH): MCP Tools Have No Access Control

### Affected Files
- `/root/projects/Autarch/pkg/mcp/server.go` -- `handleToolsCall()` at lines 292-334
- `/root/projects/Autarch/pkg/mcp/handlers.go` -- state-modifying handlers

### Current Vulnerable Code

In `server.go`, `handleToolsCall` dispatches directly to tool handlers with no caller identity check:

```go
func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) {
    var params ToolsCallParams
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, -32602, "Invalid params", err.Error())
        return
    }
    s.mu.RLock()
    tool, ok := s.tools[params.Name]
    s.mu.RUnlock()
    if !ok {
        s.sendError(req.ID, -32602, "Unknown tool", params.Name)
        return
    }
    result, err := tool.Handler(ctx, params.Arguments)
    // ... no identity check, no scope validation ...
}
```

State-modifying handlers in `handlers.go`:
- **`handleUpdateTask`** (line 148): Writes YAML files to disk via `os.WriteFile` with no authorization.
- **`handleSendMessage`** (line 359): Writes message files to `.intermute/queues/` with sender hardcoded as `"mcp-agent"`, no identity verification.
- **`handleGetPRD`** (line 63): Path traversal potential -- the `id` param is appended to a filepath with only `.yaml` suffix check, meaning `../../../etc/passwd` could be attempted (mitigated only by `.yaml` suffix).

### Vulnerability

1. **No caller identity**: MCP protocol communicates over stdin/stdout. The `ToolHandler` signature `func(ctx context.Context, params map[string]interface{})` carries no caller identity. Any process that can write to the MCP server's stdin can invoke any tool.
2. **No scope limitation**: All tools (read-only and mutating) are equally accessible. `autarch_update_task` and `autarch_send_message` modify state but have no more protection than `autarch_list_prds`.
3. **Path traversal in handleGetPRD**: The `id` parameter at line 70-75 constructs a file path without sanitizing directory traversal sequences.

### Fix Sketch

```go
// 1. Add caller identity to context
type callerKey struct{}

type CallerInfo struct {
    AgentID string
    Scopes  []string // e.g., ["read", "write", "admin"]
}

func WithCaller(ctx context.Context, caller CallerInfo) context.Context {
    return context.WithValue(ctx, callerKey{}, caller)
}

func CallerFromContext(ctx context.Context) (CallerInfo, bool) {
    v, ok := ctx.Value(callerKey{}).(CallerInfo)
    return v, ok
}

// 2. Add scope to Tool definition
type Tool struct {
    Name         string
    Description  string
    InputSchema  map[string]interface{}
    Handler      ToolHandler
    RequiredScope string // "read" or "write"
}

// 3. Enforce scope in handleToolsCall
func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) {
    var params ToolsCallParams
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, -32602, "Invalid params", err.Error())
        return
    }

    s.mu.RLock()
    tool, ok := s.tools[params.Name]
    s.mu.RUnlock()

    if !ok {
        s.sendError(req.ID, -32602, "Unknown tool", params.Name)
        return
    }

    // Enforce scope if caller info is present
    caller, hasCaller := CallerFromContext(ctx)
    if tool.RequiredScope != "" && hasCaller {
        if !hasScope(caller.Scopes, tool.RequiredScope) {
            s.sendError(req.ID, -32603, "Forbidden",
                fmt.Sprintf("tool %s requires scope %q", params.Name, tool.RequiredScope))
            return
        }
    }

    result, err := tool.Handler(ctx, params.Arguments)
    // ... existing result handling ...
}

// 4. Fix path traversal in handleGetPRD
func (s *Server) handleGetPRD(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    id, ok := params["id"].(string)
    if !ok || id == "" {
        return nil, fmt.Errorf("id parameter is required")
    }

    // Sanitize: strip directory components, allow only alphanumeric + dash + underscore
    baseName := filepath.Base(id) // Remove any path components
    if baseName != id {
        return nil, fmt.Errorf("invalid PRD ID: must not contain path separators")
    }

    if !strings.HasSuffix(baseName, ".yaml") {
        baseName = baseName + ".yaml"
    }

    prdPath := filepath.Join(s.projectPath, ".gurgeh", "specs", baseName)

    // Verify resolved path is still under specs directory
    specsDir := filepath.Join(s.projectPath, ".gurgeh", "specs")
    resolved, err := filepath.Abs(prdPath)
    if err != nil || !strings.HasPrefix(resolved, specsDir) {
        return nil, fmt.Errorf("invalid PRD path")
    }

    // ... rest of handler ...
}

// 5. Tag tools with scopes during registration
s.RegisterTool(Tool{
    Name: "autarch_list_prds",
    RequiredScope: "read",
    // ...
})
s.RegisterTool(Tool{
    Name: "autarch_update_task",
    RequiredScope: "write",
    // ...
})
s.RegisterTool(Tool{
    Name: "autarch_send_message",
    RequiredScope: "write",
    // ...
})
```

### Acceptance Criteria

**AC-F3a:** Given an MCP caller with scope `["read"]`, when the caller invokes `autarch_update_task`, then the server returns a JSON-RPC error with code -32603 and message containing "Forbidden".

**AC-F3b:** Given an MCP `autarch_get_prd` call with `id` set to `"../../etc/passwd"`, then the handler returns an error and does not read files outside `.gurgeh/specs/`.

### Test Cases

1. **Test_MCP_WriteToolRequiresWriteScope**: Set context with read-only caller, call `autarch_update_task`. Expect error.
2. **Test_MCP_ReadToolAllowedWithReadScope**: Set context with read-only caller, call `autarch_list_prds`. Expect success.
3. **Test_MCP_NoCallerInfoDefaultBehavior**: No caller info in context (backward compat for local usage). Define policy: allow or deny. Test accordingly.
4. **Test_MCP_PathTraversal_GetPRD**: Call `handleGetPRD` with `id: "../../../etc/passwd"`. Expect error, not file contents.
5. **Test_MCP_PathTraversal_UpdateTask**: Call `handleUpdateTask` with `id: "../../malicious"`. Expect error.
6. **Test_MCP_SendMessage_CallerIdentity**: Verify `handleSendMessage` uses caller identity from context, not hardcoded `"mcp-agent"`.

---

## F4 (HIGH): WebSocket Wildcard Origins

### Affected File
`/root/projects/Autarch/internal/bigend/web/server.go` -- line 440

### Current Vulnerable Code

```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: []string{"*"}, // Allow all origins for local development
})
```

Additionally, all mutating HTTP endpoints (`handleSessionNew`, `handleSessionAction`, `handleRefresh`, `handleProjectMCPAction`) lack CSRF protection.

### Vulnerability

1. **WebSocket cross-origin hijacking**: The wildcard origin policy `{"*"}` means any website a user visits can open a WebSocket to the Bigend server and stream terminal output. If the server is bound to `127.0.0.1`, a malicious page at `evil.com` could make the browser connect to `ws://127.0.0.1:PORT/ws/terminal/SESSION_NAME` and exfiltrate terminal content.

2. **CSRF on mutating endpoints**: The POST endpoints for session management (`/api/sessions/new`, `/api/sessions/{name}/restart`, `/api/sessions/{name}/fork`, `/api/sessions/{name}/rename`, `/api/projects/.../mcp/start`) accept both JSON and form-encoded bodies (lines 272-285, 333-344). A malicious page could submit a form to `http://localhost:PORT/api/sessions/new` and create or restart agent sessions.

### Fix Sketch

```go
// 1. Restrict WebSocket origins to localhost
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: []string{
        "http://localhost:*",
        "http://127.0.0.1:*",
        "http://[::1]:*",
    },
})

// 2. Add CSRF middleware for mutating endpoints
func csrfProtect(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
            origin := r.Header.Get("Origin")
            if origin == "" {
                origin = r.Header.Get("Referer")
            }
            if origin != "" && !isLocalhostOrigin(origin) {
                http.Error(w, "CSRF: origin not allowed", http.StatusForbidden)
                return
            }
        }
        next(w, r)
    }
}

func isLocalhostOrigin(origin string) bool {
    u, err := url.Parse(origin)
    if err != nil {
        return false
    }
    host := u.Hostname()
    return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// 3. Apply to mux registration
mux.HandleFunc("/api/sessions/new", csrfProtect(s.handleSessionNew))
mux.HandleFunc("/api/sessions/", csrfProtect(s.handleSessionAction))
mux.HandleFunc("/api/projects/", csrfProtect(s.handleProjectMCPAction))
mux.HandleFunc("/api/refresh", csrfProtect(s.handleRefresh))
```

### Acceptance Criteria

**AC-F4a:** Given a WebSocket upgrade request to `/ws/terminal/{session}` with `Origin: https://evil.com`, then the server rejects the connection (HTTP 403 or WebSocket handshake failure).

**AC-F4b:** Given a POST request to `/api/sessions/new` with `Origin: https://evil.com`, then the server returns 403 Forbidden.

### Test Cases

1. **Test_WS_LocalhostOriginAllowed**: WebSocket request with `Origin: http://localhost:8080`. Expect successful upgrade.
2. **Test_WS_127001OriginAllowed**: WebSocket request with `Origin: http://127.0.0.1:8080`. Expect success.
3. **Test_WS_ExternalOriginRejected**: WebSocket request with `Origin: https://attacker.com`. Expect rejection.
4. **Test_WS_NoOriginRejected**: WebSocket request with no Origin header. Expect rejection (defense in depth).
5. **Test_CSRF_PostWithExternalOrigin**: POST to `/api/sessions/new` with `Origin: https://evil.com`. Expect 403.
6. **Test_CSRF_PostWithLocalhostOrigin**: POST to `/api/sessions/new` with `Origin: http://localhost:8080`. Expect pass-through.
7. **Test_CSRF_GetNotAffected**: GET requests pass through CSRF check regardless of origin.
8. **Test_CSRF_HtmxRequestAllowed**: POST with `HX-Request: true` header and localhost origin. Expect pass-through.

---

## F5 (MEDIUM): Feedback YAML Poisoning

### Affected Files
Searched `/root/projects/Autarch/internal/pollard/` for `feedback.yaml` or `feedback.yml` -- **no dedicated feedback YAML parsing found**. However, the broader codebase has 47 files using `yaml.Unmarshal` (confirmed by grep), and the MCP handlers at `/root/projects/Autarch/pkg/mcp/handlers.go` parse arbitrary YAML from disk.

### Current Vulnerable Code Pattern

In `handlers.go`, multiple functions use unbounded `yaml.Unmarshal`:

```go
// handleGetPRD -- line 85
var prd map[string]interface{}
if err := yaml.Unmarshal(data, &prd); err != nil {
    return nil, fmt.Errorf("failed to parse PRD: %w", err)
}

// handleUpdateTask -- line 183
var task map[string]interface{}
if err := yaml.Unmarshal(data, &task); err != nil {
    return nil, fmt.Errorf("failed to parse task: %w", err)
}
```

The `weaver/weaver.go` file defines structures with `yaml` tags (lines 47-96) that would be deserialized from YAML files.

### Vulnerability

1. **Billion Laughs / YAML bomb**: `gopkg.in/yaml.v3` will expand YAML aliases and anchors. A crafted YAML file with exponential expansion (e.g., `&a [*a, *a]` nested) could cause OOM.
2. **Arbitrary type instantiation**: Deserializing into `map[string]interface{}` allows any YAML structure. While Go's `yaml.v3` is safer than Python's `yaml.load()`, extremely deep nesting can cause stack overflow.
3. **No file size limits**: `os.ReadFile` (used in `handleGetPRD`, `handleUpdateTask`, `readPRDSummary`, etc.) reads the entire file into memory with no size cap.
4. **No schema validation**: Deserialized YAML is used directly without validating expected fields/types.

### Fix Sketch

```go
// 1. Add a safe YAML reader with size limits
const maxYAMLFileSize = 1 * 1024 * 1024 // 1 MB

func safeReadYAML(path string, out interface{}) error {
    // Check file size before reading
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    if info.Size() > maxYAMLFileSize {
        return fmt.Errorf("YAML file too large: %d bytes (max %d)", info.Size(), maxYAMLFileSize)
    }

    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }

    // Use yaml.v3 Decoder with KnownFields for schema enforcement
    dec := yaml.NewDecoder(bytes.NewReader(data))
    // Note: yaml.v3's Decoder does not have a depth limit config,
    // but we can set a deadline on the decoding operation
    if err := dec.Decode(out); err != nil {
        return fmt.Errorf("decode YAML: %w", err)
    }
    return nil
}

// 2. For typed structs, use KnownFields to reject unexpected keys
func safeReadYAMLStrict(path string, out interface{}) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    if info.Size() > maxYAMLFileSize {
        return fmt.Errorf("YAML file too large: %d bytes", info.Size())
    }
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    dec := yaml.NewDecoder(bytes.NewReader(data))
    dec.KnownFields(true) // Reject unknown fields
    return dec.Decode(out)
}

// 3. Check file permissions before reading
func verifyFileOwnership(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    // Warn if world-writable
    if info.Mode().Perm()&0002 != 0 {
        return fmt.Errorf("YAML file %s is world-writable, refusing to parse", path)
    }
    return nil
}

// 4. Apply to handleGetPRD, handleUpdateTask, readPRDSummary, readTaskSummary
// Replace: data, err := os.ReadFile(prdPath)
// With:    if err := verifyFileOwnership(prdPath); err != nil { return nil, err }
//          var prd map[string]interface{}
//          if err := safeReadYAML(prdPath, &prd); err != nil { return nil, err }
```

### Acceptance Criteria

**AC-F5a:** Given a YAML file larger than 1 MB in `.gurgeh/specs/`, when `autarch_get_prd` reads it, then the handler returns an error containing "too large" and does not load the file into memory.

**AC-F5b:** Given a YAML file with deeply nested aliases (YAML bomb), when parsed by `safeReadYAML`, then the operation either returns an error or completes within 5 seconds without exceeding 50 MB RSS.

### Test Cases

1. **Test_YAML_FileSizeLimit**: Create a 2 MB YAML file, attempt to read via `safeReadYAML`. Expect error.
2. **Test_YAML_NormalFilePasses**: Create a 10 KB valid YAML file. Expect successful parse.
3. **Test_YAML_BillionLaughs**: Create a YAML file with recursive anchors. Verify parsing does not OOM (run with memory limit or timeout).
4. **Test_YAML_WorldWritableRejected**: Create a file with `0666` perms, call `verifyFileOwnership`. Expect error.
5. **Test_YAML_StrictModeRejectsUnknown**: Use `safeReadYAMLStrict` with a typed struct, provide YAML with extra fields. Expect error.
6. **Test_YAML_ValidSchemaAccepted**: Use `safeReadYAMLStrict` with correct fields. Expect success.

---

## F6 (MEDIUM): X-Forwarded-For Spoofing

### Affected File
`/root/projects/Intermute/internal/auth/middleware.go` -- lines 74-101

### Current Vulnerable Code

```go
func isLocalRequest(r *http.Request) bool {
    if ip := forwardedFor(r.Header.Get("X-Forwarded-For")); ip != "" {
        if parsed := net.ParseIP(ip); parsed != nil {
            return parsed.IsLoopback()
        }
        if strings.EqualFold(ip, "localhost") {
            return true
        }
    }
    host := r.RemoteAddr
    if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
        host = h
    }
    // ...
}

func forwardedFor(v string) string {
    if v == "" {
        return ""
    }
    parts := strings.Split(v, ",")
    return strings.TrimSpace(parts[0])
}
```

### Vulnerability

The `X-Forwarded-For` header is **checked before `RemoteAddr`** and is **unconditionally trusted**. Any external client can send:

```
GET /api/inbox HTTP/1.1
X-Forwarded-For: 127.0.0.1
```

This causes `isLocalRequest()` to return `true`, bypassing API key authentication entirely. The `Middleware` function (line 37) then grants `ModeLocalhost` access with no auth required.

This is exploitable when:
- Intermute is exposed beyond localhost (e.g., bound to `0.0.0.0` or accessible via Tailscale/VPN)
- No reverse proxy is in front that strips/rewrites `X-Forwarded-For`

### Fix Sketch

```go
// Option 1: Default to RemoteAddr only (simplest, most secure)
func isLocalRequest(r *http.Request) bool {
    host := r.RemoteAddr
    if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
        host = h
    }
    host = strings.TrimSpace(host)
    if strings.EqualFold(host, "localhost") {
        return true
    }
    parsed := net.ParseIP(host)
    return parsed != nil && parsed.IsLoopback()
}

// Option 2: Configurable trusted proxies (for reverse proxy deployments)
type ProxyConfig struct {
    TrustedProxies []net.IP     // e.g., [127.0.0.1, ::1]
    TrustedCIDRs   []*net.IPNet // e.g., [10.0.0.0/8]
}

func isLocalRequestWithProxy(r *http.Request, proxyCfg *ProxyConfig) bool {
    // First check RemoteAddr directly
    remoteHost := r.RemoteAddr
    if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
        remoteHost = h
    }
    remoteIP := net.ParseIP(strings.TrimSpace(remoteHost))

    // If RemoteAddr is loopback, it's local regardless
    if remoteIP != nil && remoteIP.IsLoopback() {
        return true
    }

    // Only trust X-Forwarded-For if RemoteAddr is a trusted proxy
    if proxyCfg != nil && remoteIP != nil && isTrustedProxy(remoteIP, proxyCfg) {
        if xff := forwardedFor(r.Header.Get("X-Forwarded-For")); xff != "" {
            if parsed := net.ParseIP(xff); parsed != nil {
                return parsed.IsLoopback()
            }
        }
    }

    return false
}

func isTrustedProxy(ip net.IP, cfg *ProxyConfig) bool {
    for _, trusted := range cfg.TrustedProxies {
        if trusted.Equal(ip) {
            return true
        }
    }
    for _, cidr := range cfg.TrustedCIDRs {
        if cidr.Contains(ip) {
            return true
        }
    }
    return false
}

// Update Keyring or Middleware to accept ProxyConfig
func Middleware(ring *Keyring, proxyCfg *ProxyConfig) func(http.Handler) http.Handler {
    // ... pass proxyCfg to isLocalRequestWithProxy ...
}
```

### Acceptance Criteria

**AC-F6:** Given Intermute bound to `0.0.0.0:8080` with default configuration (no trusted proxies), when a request arrives from `RemoteAddr: 203.0.113.10:9999` with header `X-Forwarded-For: 127.0.0.1`, then `isLocalRequest()` returns `false` and the middleware requires API key authentication.

### Test Cases

1. **Test_XFF_Ignored_ByDefault**: Request from external IP with `X-Forwarded-For: 127.0.0.1`. Expect `isLocalRequest` = false.
2. **Test_RemoteAddr_Loopback_StillWorks**: Request from `127.0.0.1:1234` with no XFF. Expect true.
3. **Test_RemoteAddr_Loopback_IPv6**: Request from `[::1]:1234`. Expect true.
4. **Test_XFF_Trusted_Proxy**: Configure trusted proxy `10.0.0.1`. Request from `10.0.0.1:5000` with `X-Forwarded-For: 127.0.0.1`. Expect true.
5. **Test_XFF_Untrusted_Proxy**: Configure trusted proxy `10.0.0.1`. Request from `10.0.0.2:5000` with `X-Forwarded-For: 127.0.0.1`. Expect false.
6. **Test_Middleware_Integration_XFF_Spoofing**: Full middleware test: external IP + spoofed XFF + no bearer token. Expect 401 Unauthorized.
7. **Test_Backward_Compat_Localhost**: Ensure existing `TestLocalhostBypass` from `/root/projects/Intermute/internal/auth/middleware_test.go` still passes (RemoteAddr `127.0.0.1:1234` continues to work).

---

## Summary Table

| ID | Severity | Location | Root Cause | Fix | Effort |
|----|----------|----------|------------|-----|--------|
| F1 | CRITICAL | `Intermute/internal/storage/sqlite/sqlite.go:Reserve()` | Blind INSERT, no overlap check | Atomic TX with glob overlap detection | Medium |
| F2 | HIGH | `Intermute/internal/http/handlers_reservations.go:releaseReservation()` | No ownership verification | Add `GetReservation()` + ownership check against auth context | Low |
| F3 | HIGH | `Autarch/pkg/mcp/server.go:handleToolsCall()` + `handlers.go` | No caller identity, no scope enforcement, path traversal | Scope-based gating, `CallerInfo` context, `filepath.Base` sanitization | Medium |
| F4 | HIGH | `Autarch/internal/bigend/web/server.go:handleTerminalWS()` | Wildcard `OriginPatterns`, no CSRF | Restrict to localhost origins, add origin-check CSRF middleware | Low |
| F5 | MEDIUM | `Autarch/pkg/mcp/handlers.go` + all YAML consumers | No file size limit, no schema validation, no depth protection | `safeReadYAML()` wrapper with size cap + ownership check | Low-Medium |
| F6 | MEDIUM | `Intermute/internal/auth/middleware.go:isLocalRequest()` | X-Forwarded-For trusted unconditionally | Default to RemoteAddr only; optional trusted-proxy allowlist | Low |

### Recommended Priority Order

1. **F6** first -- lowest effort, immediate auth bypass risk if server is network-reachable
2. **F1** next -- critical data integrity for file reservations
3. **F2** alongside F1 -- same codebase, related reservation security
4. **F4** next -- low effort, prevents cross-origin attacks on the web UI
5. **F3** then -- medium effort but important for MCP security posture
6. **F5** last -- medium risk, defensive hardening