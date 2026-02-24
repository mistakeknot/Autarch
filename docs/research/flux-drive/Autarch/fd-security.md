---
agent: fd-security
tier: 1
issues:
  - id: P0-1
    severity: P0
    section: "Loopback Enforcement"
    title: "Bigend web+daemon servers bind 0.0.0.0 without netguard, exposing session/spawn APIs to network"
  - id: P1-1
    severity: P1
    section: "MCP Path Traversal"
    title: "MCP handleGetPRD and handleUpdateTask accept unsanitized IDs enabling directory traversal"
  - id: P1-2
    severity: P1
    section: "API Key Leakage"
    title: "USDA hunter embeds API key directly in URL query string, visible in logs and proxies"
  - id: P2-1
    severity: P2
    section: "Request Body Size"
    title: "HTTP servers decode JSON bodies without size limits, enabling memory exhaustion"
  - id: P2-2
    severity: P2
    section: "Bigend Session Spawn"
    title: "Session name from HTTP request passed unsanitized to tmux exec.Command arguments"
  - id: P2-3
    severity: P2
    section: "Synthesizer Command Execution"
    title: "Synthesizer AgentCmd parsed with strings.Fields allows injection via config"
  - id: P3-1
    severity: P3
    section: "WebSocket Origin"
    title: "WebSocket Accept uses empty AcceptOptions, allowing cross-origin connections"
  - id: P3-2
    severity: P3
    section: "Pagination Limits"
    title: "Pollard and Gurgeh servers accept unbounded limit query parameter"
improvements:
  - id: IMP-1
    title: "Add netguard.EnsureLocalOnly to Bigend daemon and web server"
    section: "Loopback Enforcement"
  - id: IMP-2
    title: "Sanitize MCP ID parameters to reject path separators"
    section: "MCP Path Traversal"
  - id: IMP-3
    title: "Move USDA API key from URL query string to request header"
    section: "API Key Leakage"
  - id: IMP-4
    title: "Add http.MaxBytesReader wrapper to all JSON body decoders"
    section: "Request Body Size"
  - id: IMP-5
    title: "Validate session names against allowlist pattern before tmux exec"
    section: "Bigend Session Spawn"
  - id: IMP-6
    title: "Cap pagination limit parameter to a reasonable maximum (e.g., 200)"
    section: "Pagination Limits"
verdict: needs-changes
---

## Summary

Autarch is a Go monorepo with a principled "local-only by default" security posture. Three of its four HTTP/WS servers (Pollard, Gurgeh, Signals) correctly enforce loopback binding via `pkg/netguard/bind.go`. However, the Bigend web server and daemon bypass this guard entirely, defaulting to `0.0.0.0` and exposing session management (including tmux spawn) to the network. The MCP server (stdio-based, no network) has a path traversal vulnerability in its file-reading handlers. API key handling is generally sound (environment variables), but the USDA hunter leaks its key into URLs. SQLite query construction uses parameterized queries throughout -- no injection risk.

## Section-by-Section Review

### 1. Loopback Enforcement

**What exists:** `pkg/netguard/bind.go` provides `EnsureLocalOnly(addr)` which rejects non-loopback addresses. This is called correctly by:
- `/root/projects/Autarch/pkg/signals/server.go:35` (Signals WS server)
- `/root/projects/Autarch/internal/pollard/server/server.go:63` (Pollard API)
- `/root/projects/Autarch/internal/gurgeh/server/server.go:30` (Gurgeh API)

All three default to `127.0.0.1:PORT` in their CLI flags.

**What is missing:** The Bigend servers do NOT use netguard:

- `/root/projects/Autarch/cmd/bigend/main.go:30-31`: Web server defaults to `host = "0.0.0.0"`, port `8099`. No call to `EnsureLocalOnly`.
- `/root/projects/Autarch/internal/bigend/daemon/server.go:76-81`: Daemon `Start()` calls `ListenAndServe()` directly without netguard. Default daemon addr is `127.0.0.1:8100` (correct default), but no enforcement.
- `/root/projects/Autarch/internal/bigend/web/server.go:102-131`: `ListenAndServe(addr)` uses the addr as-is, no netguard check.

The web server at `0.0.0.0:8099` exposes the full htmx+Tailwind dashboard, session management, project enumeration, and WebSocket terminal streaming to any network interface.

**netguard implementation quality:** The guard itself is well-written. It handles `localhost`, empty host (rejected), and IP parsing correctly. The gap is in inconsistent adoption.

### 2. API Key Storage and Handling

**Environment variables (good):**
- `GITHUB_TOKEN` -- read via `os.Getenv()` at `/root/projects/Autarch/internal/pollard/hunters/github.go:58`
- `COURTLISTENER_API_KEY` -- read via `os.Getenv()` at `/root/projects/Autarch/internal/pollard/hunters/legal.go:29`
- `USDA_API_KEY` -- read via `os.Getenv()` at `/root/projects/Autarch/internal/pollard/hunters/usda.go:29`
- `INTERMUTE_API_KEY` -- read at multiple locations

**No hardcoded secrets found.** All keys come from environment.

**URL leakage (bad):** The USDA hunter at `/root/projects/Autarch/internal/pollard/hunters/usda.go:119-124` embeds the API key directly in the URL:
```go
apiURL := fmt.Sprintf(
    "https://api.nal.usda.gov/fdc/v1/foods/search?api_key=%s&query=%s&pageSize=%d",
    h.apiKey,
    url.QueryEscape(query),
    min(maxResults, 200),
)
```
This means the key appears in HTTP access logs, proxy logs, browser history, and any error messages that include the URL. In contrast, the GitHub hunter uses `Authorization` header (`/root/projects/Autarch/internal/pollard/hunters/github.go:272`) and the CourtListener hunter uses `Authorization: Token` header (`/root/projects/Autarch/internal/pollard/hunters/legal.go:131`).

### 3. SQLite Query Safety

**All queries use parameterized statements.** At `/root/projects/Autarch/internal/pollard/state/db.go`, every query uses `?` placeholders:
- Line 96: `INSERT INTO hunter_runs (...) VALUES (?, ?, ?)`
- Line 112: `UPDATE hunter_runs SET ... WHERE id = ?`
- Line 127: `SELECT ... WHERE hunter_name = ? ORDER BY ...`
- Line 157: `SELECT ... LIMIT ?`

The schema migration at line 68-88 uses static DDL strings with no interpolation.

The `pkg/db/open.go` helper executes only hardcoded PRAGMA statements.

**No SQL injection risk.**

### 4. Input Validation on HTTP APIs

**Good practices:**
- Method checks on every handler (405 for wrong method)
- JSON decode with `DisallowUnknownFields()` on signals server (`/root/projects/Autarch/pkg/signals/server.go:105`)
- Required field validation on signal publish (`server.go:92-94`)
- Status whitelist validation in MCP update task handler

**Missing: Request body size limits.** All three HTTP servers decode request bodies with `json.NewDecoder(r.Body).Decode(&req)` without any `http.MaxBytesReader` wrapper:
- `/root/projects/Autarch/internal/pollard/server/server.go:141,179,216`
- `/root/projects/Autarch/pkg/signals/server.go:104`
- `/root/projects/Autarch/internal/bigend/daemon/server.go:174`

A malicious local client could send a multi-gigabyte JSON body to exhaust memory. For local-only servers this is low risk (the attacker already has local access), but for Bigend on `0.0.0.0` it becomes network-exploitable.

The fetcher pipeline does use `io.LimitReader` at `/root/projects/Autarch/internal/pollard/pipeline/fetcher.go:266`, showing awareness of the pattern -- it just was not applied to incoming requests.

### 5. MCP Server Path Traversal

The MCP server communicates via stdin/stdout (no network exposure), so the attacker would need to be the AI agent invoking tools. However, the MCP protocol is designed with the assumption that tool parameters are untrusted.

At `/root/projects/Autarch/pkg/mcp/handlers.go:63-90`, `handleGetPRD`:
```go
filename := id
if !strings.HasSuffix(filename, ".yaml") {
    filename = filename + ".yaml"
}
prdPath := filepath.Join(s.projectPath, ".gurgeh", "specs", filename)
data, err := os.ReadFile(prdPath)
```

If `id` is `../../../etc/passwd`, the resulting path becomes `{projectPath}/.gurgeh/specs/../../../etc/passwd.yaml`. The `.yaml` suffix mitigates arbitrary file read somewhat, but `../../etc/shadow.yaml` is a valid traversal attempt, and on systems where `.yaml` files exist at unexpected paths, this allows reading them. The same pattern appears in `handleUpdateTask` at line 172, which also **writes** back to the traversed path.

Similarly, `handleSendMessage` at line 384 uses the validated `to` parameter (whitelisted) but the message `msgID` is timestamp-based and safe.

### 6. WebSocket Security

At `/root/projects/Autarch/pkg/signals/broker.go:69`:
```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
```

Empty `AcceptOptions` means `nhooyr.io/websocket` defaults to accepting all origins. For a loopback-only server this is a non-issue -- cross-origin requests from a browser would still connect to localhost. But if Bigend's `0.0.0.0` binding were combined with a WebSocket endpoint, a malicious web page could make cross-origin WebSocket connections to the user's machine.

The Bigend daemon and web server also use empty `AcceptOptions` at `/root/projects/Autarch/internal/bigend/daemon/server.go:339` and `/root/projects/Autarch/internal/bigend/web/server.go:439`.

### 7. Command Execution Surfaces

**Synthesizer (`/root/projects/Autarch/internal/pollard/pipeline/synthesizer.go:117-128`):**
The `AgentCmd` field is split with `strings.Fields()` and passed to `exec.CommandContext()`. The command comes from YAML config (`pipeline.synthesizer.agent`), not from network input. Since the config file is local and user-controlled, this is the user running their own agent -- not an injection vector.

**Bigend session spawn (`/root/projects/Autarch/internal/bigend/daemon/sessions.go:67`):**
```go
cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", projectPath)
```
The `name` and `projectPath` come from HTTP JSON body (SpawnRequest at `server.go:167-170`). Since `exec.Command` passes arguments as an array (not through a shell), direct shell injection is not possible. However, tmux session names with special characters could cause tmux-level issues or be used for tmux command injection via tmux's command parsing. The `agentCommand()` function at `sessions.go:211-223` whitelists agent types to a fixed set, which is good.

**Agent hunter (`/root/projects/Autarch/internal/pollard/hunters/agent.go:148`):**
```go
cmd := exec.CommandContext(ctx, h.agentCommand, args...)
```
The agent command comes from the `POLLARD_AGENT_COMMAND` environment variable. User-controlled, local-only.

### 8. Colony Detection (/proc Access)

At `/root/projects/Autarch/internal/bigend/colony/detector.go:93-124`, the colony detector reads `/proc` to find processes whose CWD matches project roots. This is read-only, uses `os.Readlink` (not shell commands), and is guarded by `runtime.GOOS != "linux"`. The `sameOrChild` function at line 126-140 uses `filepath.Clean` and `filepath.Rel` correctly to prevent path confusion. No security concern here.

## Issues Found

### P0-1: Bigend Web + Daemon Skip Loopback Guard (Critical)

- **Location:** `/root/projects/Autarch/cmd/bigend/main.go:30` (default `host = "0.0.0.0"`) and `/root/projects/Autarch/internal/bigend/web/server.go:103` (no netguard call)
- **Threat:** The Bigend web server binds to all interfaces by default, exposing the full dashboard, session spawn API (which creates tmux sessions and runs agent commands), project enumeration, and WebSocket terminal streaming to the network. An attacker on the same LAN can spawn arbitrary agent sessions on the user's machine.
- **Likelihood:** High. The default flag value is `0.0.0.0`. Any user who runs `./dev bigend` without explicit `--host` is network-exposed.
- **Mitigation:** (1) Change default `host` flag from `"0.0.0.0"` to `"127.0.0.1"` in `cmd/bigend/main.go:30`. (2) Add `netguard.EnsureLocalOnly(addr)` call in both `internal/bigend/web/server.go:ListenAndServe` and `internal/bigend/daemon/server.go:Start`. (3) For the daemon, verify the daemon-addr default of `127.0.0.1:8100` is enforced via netguard as well.

### P1-1: MCP Path Traversal in File Handlers (High)

- **Location:** `/root/projects/Autarch/pkg/mcp/handlers.go:70-75` (handleGetPRD) and lines 168-172 (handleUpdateTask)
- **Threat:** An AI agent (or a compromised MCP client) can pass `id` values containing `../` sequences to read or overwrite files outside the intended `.gurgeh/specs/` and `.coldwine/tasks/` directories. The `handleUpdateTask` handler is particularly dangerous because it writes data back to the traversed path.
- **Likelihood:** Medium. The MCP server runs via stdin/stdout, so the attacker is the AI agent itself. MCP tools are designed to handle untrusted parameters from agents.
- **Mitigation:** Validate the `id` parameter: reject if it contains `/`, `\`, or `..`. Alternatively, after `filepath.Join`, verify the result is still under the expected base directory:
  ```go
  absPath := filepath.Join(s.projectPath, ".gurgeh", "specs", filename)
  if !strings.HasPrefix(absPath, filepath.Join(s.projectPath, ".gurgeh", "specs")) {
      return nil, fmt.Errorf("invalid spec ID")
  }
  ```

### P1-2: USDA API Key in URL Query String (High)

- **Location:** `/root/projects/Autarch/internal/pollard/hunters/usda.go:119-124`
- **Threat:** The API key is embedded in the URL as `?api_key=...`. This means it appears in HTTP access logs (on any proxy between client and server), Go's default request logging, error messages containing the URL, and potentially cached in HTTP intermediaries.
- **Likelihood:** Medium. The USDA API is free-tier, but key leakage enables abuse that could exhaust the user's rate limits or get their key revoked.
- **Mitigation:** The USDA FoodData Central API supports passing the key via the `X-Api-Key` header. Change to:
  ```go
  req.Header.Set("X-Api-Key", h.apiKey)
  ```
  and remove `api_key` from the URL.

### P2-1: No Request Body Size Limits (Medium)

- **Location:** All HTTP handler `json.NewDecoder(r.Body).Decode()` calls across Pollard server, Signals server, and Bigend daemon
- **Threat:** A client can send an arbitrarily large JSON body, causing the server to allocate unbounded memory attempting to decode it. For loopback-only servers the risk is low (attacker already has shell access). For Bigend on `0.0.0.0` this is network-exploitable denial of service.
- **Likelihood:** Low for loopback servers, Medium for Bigend (currently exposed).
- **Mitigation:** Wrap request bodies with `http.MaxBytesReader`:
  ```go
  r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
  ```
  Apply consistently across all POST handlers.

### P2-2: Unsanitized Session Name in tmux exec (Medium)

- **Location:** `/root/projects/Autarch/internal/bigend/daemon/sessions.go:67`
- **Threat:** The session `name` from the HTTP SpawnRequest is passed directly to `exec.Command("tmux", "new-session", "-d", "-s", name, ...)`. While Go's `exec.Command` does not use a shell (so shell metacharacters are safe), tmux session names with special characters (colons, dots, periods) can cause tmux to misinterpret arguments. Names starting with `-` could be parsed as flags. Combined with the Bigend `0.0.0.0` binding, this is network-reachable.
- **Likelihood:** Low. Exploitation requires understanding tmux's internal name parsing.
- **Mitigation:** Validate session names against `^[a-zA-Z0-9_-]+$` before passing to exec. The `agentCommand()` whitelist at line 211-223 is already a good pattern -- apply similar validation to session names.

### P2-3: Synthesizer Command from Config (Medium)

- **Location:** `/root/projects/Autarch/internal/pollard/pipeline/synthesizer.go:117`
- **Threat:** `AgentCmd` is split with `strings.Fields()` and the first token becomes the executable. If a user places a malicious `.pollard/config.yaml` in a project, the synthesizer would execute arbitrary commands. This is "confused deputy" -- the user trusts their config, but a cloned repo could contain a `.pollard/` directory.
- **Likelihood:** Low. Requires the user to clone a repo with a crafted `.pollard/config.yaml` and then run `pollard scan --mode deep` with synthesis enabled.
- **Mitigation:** Document this risk. Optionally, validate `AgentCmd` against a whitelist of known agent commands (claude, codex, aider, cursor). The existing `agentCommand()` whitelist in sessions.go shows the pattern.

### P3-1: WebSocket Cross-Origin (Low)

- **Location:** `/root/projects/Autarch/pkg/signals/broker.go:69`, `/root/projects/Autarch/internal/bigend/daemon/server.go:339`, `/root/projects/Autarch/internal/bigend/web/server.go:439`
- **Threat:** Empty `websocket.AcceptOptions{}` means any web page can open a WebSocket to the server. For loopback servers this is essentially a non-issue. For Bigend on `0.0.0.0`, a malicious website visited by the user could connect to the terminal WebSocket.
- **Likelihood:** Very low for loopback, Low for Bigend (requires knowing the host IP).
- **Mitigation:** Once Bigend's bind address is fixed (P0-1), this becomes moot. For defense-in-depth, set `OriginPatterns: []string{"localhost:*", "127.0.0.1:*"}` in AcceptOptions.

### P3-2: Unbounded Pagination Limit (Low)

- **Location:** `/root/projects/Autarch/internal/pollard/server/server.go:348` and `/root/projects/Autarch/internal/gurgeh/server/server.go:185`
- **Threat:** The `limit` query parameter is parsed with no upper bound. A request with `?limit=999999999` could cause the server to serialize an enormous JSON response.
- **Likelihood:** Very low. Local-only servers, limited by actual data volume.
- **Mitigation:** Cap `limit` to a maximum (e.g., `if limit > 200 { limit = 200 }`).

## What Was NOT Flagged

- **SQLite injection:** All queries use parameterized statements. This is safe by construction.
- **Local file writes to `.pollard/`, `.gurgeh/`, `.coldwine/`:** These are intended operations within the project directory.
- **Colony /proc detection:** Read-only, Linux-guarded, path-safe.
- **Agent execution from environment variables:** The user configures their own agents. This is expected behavior, not a vulnerability.
- **Missing auth on local-only APIs:** The design explicitly defers authentication until remote/multi-host support is added. For loopback-only servers, the OS provides access control.

## Overall Assessment

**Real risk level: Medium** (elevated to High by P0-1)

The project's security architecture is well-thought-out: `netguard` is a clean abstraction, SQLite queries are parameterized, API keys come from environment variables, and the "local-only by default" principle is documented prominently. The main problems are:

1. **P0-1 is a must-fix.** The Bigend server's `0.0.0.0` default directly contradicts the project's stated security principle. This was likely an oversight from early development before netguard was introduced, since the other three servers all adopted it correctly.

2. **P1-1 and P1-2 are must-fix.** Path traversal in MCP handlers and API key URL leakage are real issues with straightforward fixes.

3. **P2-* are recommended.** Body size limits and session name validation are defense-in-depth that become more important once P0-1 is fixed (or if remote access is ever added).

4. **P3-* are nice-to-have.** These only matter if the threat model changes.

### Must-Fix Items
- P0-1: Change Bigend default host to `127.0.0.1`, add netguard enforcement
- P1-1: Sanitize MCP ID parameters against path traversal
- P1-2: Move USDA API key from URL to header

### Nice-to-Have Hardening
- P2-1: Add `http.MaxBytesReader` to all HTTP POST handlers
- P2-2: Validate tmux session names
- P2-3: Whitelist synthesizer agent commands
- P3-1: Restrict WebSocket origins
- P3-2: Cap pagination limits
