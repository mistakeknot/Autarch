# MCP Compatibility Notes (Tandemonium)

Date: 2026-01-16

## Goal
Provide a built-in coordination layer with MCP Agent Mail semantics so external
agents can rely on Tandemonium without running a separate MCP server.

## Implemented Surface (Storage + CLI)

Storage tables:
- messages
- mailboxes
- reservations
- events
- attachments
- contact_policies
- contact_requests

Storage operations:
- send_message (messages + mailboxes)
- fetch_inbox
- ack_message
- mark_read
- reserve_paths / release_paths
- search_messages
- summarize_thread
- contact policy (get/set)
- contact request (request/respond/list)
- attachments (persist + list)

CLI commands:
- tand mail send
- tand mail inbox
- tand mail ack
- tand mail read
- tand mail search
- tand mail summarize
- tand mail policy set/get
- tand mail contact request/respond/list
- tand lock reserve
- tand lock release

## Semantics Mapping
- Each message stored once in `messages`, with per-recipient delivery in
  `mailboxes`.
- Thread ID defaults to message ID when not provided.
- Ack marks a mailbox entry; inbox shows ack/read timestamps if present.
- Reservations model exclusive/shared ownership with TTL-based expiry.

## Gaps vs MCP Agent Mail
See `docs/plans/2026-01-21-mcp-parity-audit.md` for the current parity audit and gaps list.

## Recommended Next Steps
1) Monitor for MCP Agent Mail schema changes.
