# User & Product Review: iv-gax Dashboard File Fallback

**Primary user:** Developer running Autarch TUI on a machine where Intermute is not running (common during local iteration, fresh checkout, or when the service is intentionally stopped). Their job is to browse their specs, epics, tasks, and insights without having to start a separate server first.

**Date:** 2026-02-22
**Reviewer:** flux-drive fd-user-product

---

### Findings Index

| SEVERITY | ID | Section | Title |
|---|---|---|---|
| HIGH | U1 | Badge Clarity | [offline] badge is ambiguous — does not explain what is degraded or how to fix it |
| HIGH | U2 | Write Failures in Fallback | Mutating operations in fallback mode fail silently with a generic network error |
| MEDIUM | U3 | Fallback Activation is Silent | No in-session notification when fallback activates for the first time |
| MEDIUM | U4 | Session-Sticky Fallback Has No Escape | User cannot recover to live data within a session even if Intermute starts |
| LOW | U5 | Badge Styling | [offline] badge is visually indistinguishable from the surrounding help text |
| LOW | U6 | Data Staleness is Unquantified | Badge gives no indication of how old the local data is |

**Verdict: needs-changes**

---

### Summary

The fallback mechanism is structurally sound — lazy activation on ECONNREFUSED only, read-only local files, sticky for session lifetime. The implementation risks are low. The user-experience risks are medium-to-high: users will not understand what "[offline]" means, what data they are looking at, or why write operations fail. The session-sticky design is defensible but needs a deliberate exit path. The most urgent gap is that mutating calls (CreateSpec, UpdateTask, etc.) bypass the fallback guard entirely and will produce opaque "connection refused" errors when users try to act on what they think is real data. Together these create a pattern where the TUI appears functional but any action results in a confusing error with no guidance.

---

### Issues Found

**U1. HIGH: [offline] badge is ambiguous — does not explain what is degraded or how to fix it**

The badge text "[offline]" appended to the footer at `internal/tui/unified_app.go:793` carries no context about which service is unavailable, what data is being shown instead, or what the user should do. "Offline" in TUI tools typically means no network at all. Here it specifically means "Intermute is unreachable; you are seeing local files from `.gurgeh/`, `.tandemonium/`, and `.pollard/`." A developer who has never seen this before will not know that: (a) read operations are working from files, (b) Intermute is the affected service, (c) they can resolve it by starting Intermute. There is no help binding for the offline state and `/help` will not explain it. Contrast with the already-present pattern of specific slash-command affordances — the existing footer text is already very specific about what each binding does, making this vague badge inconsistent with the surrounding design language. Evidence: `internal/tui/unified_app.go:791-794`, `pkg/autarch/client.go:54-58`.

**U2. HIGH: Mutating operations in fallback mode fail silently with a generic network error**

`CreateSpec`, `UpdateSpec`, `DeleteSpec`, `CreateEpic`, `UpdateTask`, `DeleteTask`, `CreateInsight`, `LinkInsight`, and all other write methods in `pkg/autarch/client.go` have no fallback guard and no read-only check. When `fallbackActive` is true, these methods proceed to attempt an HTTP POST/PUT/DELETE against the unreachable Intermute server, receive ECONNREFUSED, and return a raw `fmt.Errorf("POST /api/specs: %w", err)` error. The view layer will display this (or silently drop it, depending on how each view handles errors). In no case does the user receive a message like "Cannot create specs in offline mode — start Intermute to enable writes." The `DataSource` interface (`pkg/autarch/source.go`) is read-only by design, which is correct, but the client's write surface is not protected at all. A user browsing local specs and attempting to promote one, add a task, or accept a phase transition will hit this. Evidence: `pkg/autarch/client.go:218-265` (CreateSpec/UpdateSpec/DeleteSpec have no fallback guard); `pkg/autarch/source.go:1-12` (interface is read-only); `internal/autarch/local/source.go` (no write methods).

**U3. MEDIUM: Fallback activation is silent — no in-session notification**

Fallback activates the first time a List call fails with ECONNREFUSED (`pkg/autarch/client.go:85-87`). At that moment, `fallbackActive` flips to `true` and `[offline]` appears in the footer. However there is no toast, no status message, no log pane event, and no transient modal to tell the user what just happened. The footer badge is in a low-attention zone (end of a dense help string, same color/weight as surrounding text). A user who is actively scrolling a list will see data appear and may not notice the footer changed. The first signal they get that something is wrong may be a failed write (U2), by which point they have already invested time in the session. Evidence: `pkg/autarch/client.go:79-90` (no logging or messaging on activation); `internal/tui/unified_app.go:786-796` (footer only, no other notification path).

**U4. MEDIUM: Session-sticky fallback has no escape hatch — user cannot reconnect**

Once `fallbackActive = true`, `tryFallback()` returns early (`c.fallback == nil || c.fallbackActive` guard at line 82 of `client.go`), and every List call short-circuits to the local source. There is no way for the user to trigger a re-probe within the session — not via `/refresh`, not via `ctrl+r`, and not by starting Intermute after launch. This is a deliberate design choice documented in the code comment, but the rationale (avoiding repeated probes against a slow server) does not account for the case where the user starts Intermute after seeing the offline badge. The user has no way to know they must restart the TUI. The expected recovery path (start Intermute, use TUI normally) is broken without a restart. The product implication: if Intermute startup is slow and a user launches the TUI before it finishes, they are stuck in fallback for the whole session even though Intermute becomes available seconds later. Evidence: `pkg/autarch/client.go:81-83` (sticky with no reset path); no reconnect command exists in `pkg/tui/command_picker.go` or `internal/tui/unified_app.go`.

**U5. LOW: [offline] badge is visually indistinguishable from surrounding footer text**

The badge is appended as a plain string `"  |  [offline]"` to the existing help string and rendered via `FooterStyle` which uses `ColorMuted` foreground (`pkg/tui/styles.go:47-50`). There is no color differentiation, no bold, and no lipgloss inline style applied to the badge itself. The Tokyo Night palette has warning-appropriate colors (amber, red) that are used elsewhere in the TUI. The badge will be easy to miss in a terminal where the footer is already dense. Conventions in this codebase (e.g., confidence warnings in the Arbiter view) use distinct colors to draw attention to degraded states. Evidence: `internal/tui/unified_app.go:792-794` (plain string append); `pkg/tui/styles.go:47-50` (FooterStyle uses ColorMuted uniformly).

**U6. LOW: Data staleness is unquantified in the badge**

The badge reads "[offline]" but the local data in `.gurgeh/specs/` and `.tandemonium/state.db` could be seconds or months old. The user has no indication of when the local data was last synchronized. For a spec browser, showing a stale spec list with no age information could lead to decisions based on outdated data. The `mapPRDToSpec()` function in `internal/autarch/local/source.go:252-267` does parse `UpdatedAt` from PRD files, so the information is available — it is just not surfaced. Evidence: `internal/autarch/local/source.go:258-267` (timestamps available); `internal/tui/unified_app.go:793` (badge has no timestamp).

---

### Improvements

**I1. Replace "[offline]" with "[offline: local files]" and add a help entry**
A more specific label like `[offline: local files]` or `[local data]` immediately communicates what the user is seeing. Add a corresponding `/help` entry explaining what this state means and that restarting the TUI will re-probe. Rationale: the existing footer already names specific commands — vague badges are inconsistent with the established pattern.

**I2. Guard all write methods with an early-return ErrOffline**
Add a `checkWritable()` helper to `pkg/autarch/client.go` that returns a typed `ErrFallbackReadOnly` sentinel when `fallbackActive` is true, and call it at the top of every Create/Update/Delete/Link/Assign method. Views that receive this error can display "Read-only in offline mode — start Intermute to enable writes" instead of a raw network error. Rationale: write failures are the highest-friction moment in this flow; catching them early with a targeted message is the minimum viable fix for U2.

**I3. Emit a log pane entry or status toast when fallback activates**
In `tryFallback()`, after setting `fallbackActive = true`, send a `slog.Warn("intermute unreachable — switched to local files", ...)` using the existing log handler so it surfaces in the log pane. Optionally emit a `tea.Msg` that the app can display as a transient status. Rationale: passive badge discovery is too weak for a mode change that affects all subsequent writes; a one-time explicit notification matches what users expect from service degradation.

**I4. Add a /reconnect command (or honor ctrl+r) to re-probe Intermute**
Implement a deliberate reconnect path: reset `fallbackActive = false`, attempt one List call, re-activate fallback if it fails or stay online if it succeeds. Expose this as `/reconnect` in the slash command picker and honor the existing `ctrl+r` binding. Rationale: the session-sticky design is safe by default, but the absence of any recovery path within a session makes the feature feel broken when Intermute starts after TUI launch.

**I5. Style the offline badge with a warning color**
Apply an inline lipgloss style using the existing warning/amber color from Tokyo Night to the badge string before appending it to the footer. This does not require changing the footer's base style. Rationale: the badge must be noticeable; footer real estate is dense and ColorMuted actively hides it.
