# Tasks plugin RPC spike (okek16), 2026-09-30

Method: isolated bb 0.44.0+aleph.0.5.1 server (127.0.0.1:52055, scratch data dir), builtin tasks@0.1.2, scratch plugin `needsmk` with a thread-panel action, driven by Playwright. Screenshots in /tmp/okek16-spike/.

| Capability | Works stock? | Evidence | Fork change? |
|---|---|---|---|
| Q1 plugin frontend calls tasks RPC | Yes: `useSdk().plugins.callRpc({pluginId:"tasks",...})` (not `useRpc`, which is own-plugin only) | q1-panel-cross-project.png: rows from two projects; createLabel/createTask/createComment worked (PROJ-2). listTasks with labelIds from several projects and no projectId returns all | No |
| Q2 server-side access | Yes: `bb.sdk.plugins.callRpc` and loopback POST /api/v1/plugins/tasks/rpc/<m> (no token) | listProjects returned data from the plugin server | No. Do not spawn the `bb` CLI from a plugin server: its env lacks BB_SERVER_URL and it hit the default (live) server |
| Q3 live refresh of tasks | No | useRealtime on tasks:changed, comments:changed, projects:changed and guessed prefixes got nothing for panel and CLI writes; own-channel "ping" arrived. Frontend useRealtime has no pluginId arg | Yes for push. Workarounds untested: poll, or server-side poller re-publishing on own channel |
| Q4 `blocks` | Description line `Blocks: bead:.. thread:.. project:..` survives and parses; `needs-mk` label filter works via labelIds | Task schema has no custom fields. Labels are per project (different ids), createLabel needed per project; CLI create fails on missing label; `bb tasks list --label` needs --project when ambiguous. listTasks returns label ids not names | Structured field needs a fork change; description convention works now |
| Q5 comment routing | Notify goes to the thread of the latest AGENT comment, not the attached thread | Attach thr_q863 to PROJ-1; notify comment before any agent comment: notifiedCount 0. After a comment authored from the thread (kind=agent): notifiedCount 1, thread received "New comment on task PROJ-1 from cli..." | Yes if routing to an attached thread is wanted |
| Q6 fork gaps | See above | Upstream is get-bb/bb; no structured fields or cross-plugin realtime in tasks plugin source | Cross-plugin realtime subscription; structured blocks; label-name filter and names in listTasks; notify-to-attached |
| Q7 Run-as-root stub | Not built | Prototype denied by the auto-mode classifier (RCE surface); not worked around | Needs mk's decision |

## Q7 design only (no code)
- Card carries a fenced `root-run` block: path, sha256, timeout.
- Panel shows the tuple, re-checks the file's real sha256 at hand-off, and renders only the item JSON ({run_as:"operator", script, script_sha256, owner_thread, run_timeout_s, label}) and the paste command for Aleph's `todo-add --from-card` adapter (see 2026-09-30-aleph-runner-interface.md).
- Never execs todo-run, never writes status, never marks approved. Approval is slice B only. Card state is untrusted.

## Not proven
- Poller-based refresh workaround; core events (thread:changed) as refresh triggers.
- Q4 option (c) home-side table.
- Q5 behavior with side-chat threads (read from code only).
- Disclosure: a plugin-server probe ran `bb tasks list --json` against the LIVE server (read-only; saw PROJ-12). Removed.
