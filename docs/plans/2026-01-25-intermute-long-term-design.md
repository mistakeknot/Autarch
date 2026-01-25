# Intermute Long-Term Design

Date: 2026-01-25

## Context
Intermute is the coordination layer for Autarch. It is a standalone server and protocol that provides durable messaging and real-time delivery for agents across Autarch modules. Bigend remains the session orchestrator and owns live session I/O (stdout/stderr, cancel/interrupt). Intermute is not a session manager.

This design focuses on long-term code quality, architecture stability, and evolution. MVP scope is agent registry + heartbeats + messaging (send/inbox/ack/read) with REST + WebSocket transport.

## Goals
- Durable, replayable coordination for agents and subagents.
- Real-time delivery via WebSocket without sacrificing correctness.
- Stable, versioned protocol with backward-compatible evolution.
- Clean separation between core semantics, storage, and transport adapters.
- Autarch integration via a shared Go client.

## Non-Goals (MVP)
- Session log streaming, cancel/interrupt control (belongs to Bigend).
- Task queues, reservations, search, summarization.
- External message bus (NATS/Redis). Designed for later swap.

## Core Architecture
Intermute is built around a **transport-agnostic core** and a **durable append-only event log**.

Components:
- **Domain core**: message model, threading, ack/read states, idempotency, per-agent cursors.
- **Event log + indexes**: append-only events table; derived inbox/thread views.
- **REST API**: authoritative path for durability and replay.
- **WebSocket gateway**: low-latency delivery of new events.
- **Client SDK**: Go client used by Bigend/Gurgeh/Coldwine/Pollard.

Quality principle: WS is a delivery accelerator, not the source of truth.

## Protocol and API (REST + WebSocket)
Protocol uses versioned JSON. REST requires `X-Intermute-Version`, WS includes version in the hello frame.

REST (authoritative):
- `POST /api/agents` register (returns `agent_id`, `session_id`, `cursor`)
- `POST /api/agents/{id}/heartbeat`
- `POST /api/messages` send message (returns `message_id`, `cursor`)
- `GET /api/inbox/{agent}?since_cursor=...&limit=...`
- `POST /api/messages/{id}/ack`
- `POST /api/messages/{id}/read`

WebSocket (real-time):
- `WS /ws/agents/{id}?cursor=...`
- Events: `message.created`, `message.ack`, `message.read`, `agent.heartbeat`, `agent.state`
- On reconnect, clients resync via REST using last seen cursor.

Message schema (core fields):
`id`, `thread_id`, `from`, `to[]`, `created_at`, `body`, `metadata`, `attachments[]`, `importance`, `ack_required`, `status`, `cursor`.

Idempotency: all POSTs accept `Idempotency-Key`. Retries must be safe.

## Storage and Delivery Semantics
Storage is modeled as an append-only event log with secondary indexes.

Tables (conceptual):
- `events`: immutable event rows (created/ack/read/heartbeat)
- `messages`: materialized latest state per message
- `inbox_index`: per-agent cursor ordering
- `agents`: registry + last heartbeat + status

Delivery semantics:
- At-least-once delivery with idempotent writes.
- Ordering is per-agent inbox order; no global ordering.
- A message is delivered when it appears in inbox; ack/read are explicit events.

Compaction is deferred; snapshots can be added later to reduce replay time.

## Agent Lifecycle and State
Agents register once per session and send heartbeats. Intermute keeps minimal state.

- **Register**: `name`, `project`, `capabilities[]`, `metadata`.
- **Heartbeat**: periodic updates; missing heartbeats mark a session stale.
- **Status**: free-form field set by clients/Bigend; Intermute does not infer complex states.

Bigend owns session I/O and any advanced state inference (working/waiting/stalled).

## Autarch Integration
Intermute is used by all modules via a shared Go client.

- **Bigend**: consumes WS stream for dashboards; owns live session I/O and control; may write status fields back.
- **Gurgeh/Coldwine**: subagent workflows use threads; CLI/TUI can poll or subscribe.
- **Pollard**: replaces file inbox with Intermute messages; hunters can be parallelized; report review can be threaded.

Key contracts:
- Task/subagent workflows are threads.
- Idempotency prevents duplicate task dispatch.
- Capability tags support routing decisions.

## Security, Auth, and Versioning
- **Auth**: project-scoped API keys in MVP; optional per-agent tokens later.
- **TLS**: required outside local dev.
- **Versioning**: additive-only within major version; deprecations announced one release ahead.
- **Validation**: server validates payloads; strict schema checks.

## Testing and Observability
Testing priorities:
- Domain logic: idempotency, ack/read transitions.
- Storage: cursor ordering, `since_cursor` correctness, migrations.
- WS: connect/disconnect/reconnect with REST replay to avoid loss.
- Integration: Go client against real server.

Observability:
- Structured logs (request_id, agent_id, thread_id, cursor).
- Metrics: messages/sec, WS connections, heartbeat lag.
- Health endpoints: `/healthz`, `/readyz`.

## Roadmap (Post-MVP)
- Reservations and conflicts API.
- Task queues + claim/handoff.
- Search and summarization.
- External bus backend (Redis Streams / NATS JetStream).
- Richer auth and agent policies.

## Open Questions
- Exact auth token strategy and rotation.
- When to introduce task queues vs keep in Bigend.
- External bus adoption timeline.
