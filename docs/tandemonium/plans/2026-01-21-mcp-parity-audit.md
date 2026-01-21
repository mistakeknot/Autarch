# MCP Agent Mail Parity Audit (Tandemonium)

Date: 2026-01-21

Source of truth: `AGENTS.md` MCP Agent Mail tool list (project instructions).

Scope: Tandemonium CLI + storage implementation in `internal/cli/commands` and
`internal/storage` (no external MCP server).

## Summary

Core messaging, inbox, contact policies, contact requests, attachments, and
reservations are implemented. Remaining gaps are primarily: reply semantics,
reservation renew/force-release, and agent registry/health operations.

## Tool-by-tool Mapping

Legend:
- **Full**: matches behavior and data shape closely
- **Partial**: similar behavior but missing parameters or scope
- **Missing**: no equivalent command/operation
- **Out of scope**: not part of Tandemonium CLI/UX target

| MCP Agent Mail tool | Tandemonium equivalent | Status | Notes |
| --- | --- | --- | --- |
| send_message | `tandemonium mail send` | Full | Supports to/cc/bcc, importance, ack, metadata, attachments persisted. |
| fetch_inbox | `tandemonium mail inbox` | Full | Supports `since`, `urgent-only`, `limit`, `page-token`, JSON output. |
| acknowledge_message | `tandemonium mail ack` | Full | Ack per recipient. |
| mark_message_read | `tandemonium mail read` | Full | Read per recipient. |
| search_messages | `tandemonium mail search` | Full | Supports pagination and JSON output. |
| summarize_thread | `tandemonium mail summarize` | Full | LLM command hook supported, examples optional. |
| set_contact_policy | `tandemonium mail policy set` | Full | Enforced on send. |
| get_contact_policy | `tandemonium mail policy get` | Full | Returns policy. |
| request_contact | `tandemonium mail contact request` | Full | Creates pending request. |
| respond_contact | `tandemonium mail contact respond` | Full | Accept/deny. |
| list_contacts | `tandemonium mail contact list` | Full | Accepted contacts only. |
| file_reservation_paths | `tandemonium lock reserve` | Full | Exclusive/shared, TTL, reason. |
| release_file_reservations | `tandemonium lock release` | Full | Releases by path or reservation id. |
| renew_file_reservations | `tandemonium lock renew` | Full | Extends expiry for active reservations. |
| force_release_file_reservation | `tandemonium lock force-release` | Full | Releases reservation by id. |
| reply_message | `tandemonium mail reply` | Full | Defaults to original sender + thread, supports overrides. |
| ensure_project | `tandemonium agent ensure` | Full | Idempotent project init + DB migrate. |
| register_agent | `tandemonium agent register` | Full | Upserts agent profile. |
| whois | `tandemonium agent whois` | Full | Returns agent profile. |
| health_check | `tandemonium agent health` | Full | DB health + timestamp. |
| list_contacts (project-level) | `tandemonium mail contact list` | Full | Scoped to owner. |

## Out of Scope (MCP macros / orchestration helpers)

The following are MCP Agent Mail workflow helpers that Tandemonium does not
intend to expose as CLI equivalents:

- `macro_*` helpers (session boot, thread alignment, etc.)
- Subagent dispatch or mailbox macros

## Gaps and Recommendations

1) Monitor for MCP Agent Mail schema changes.

## Notes

- Attachments are persisted on send and exposed via inbox JSON payloads.
- Error messages are now consistently wrapped (`mail <cmd> failed: ...`, etc.).
