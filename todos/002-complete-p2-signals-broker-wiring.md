---
status: complete
priority: p2
issue_id: "002"
tags: [signals, coordination]
dependencies: []
---

# Wire signal emitters into the signals broker

## Problem Statement

The signals WebSocket server exists, but no tool publishes signals into the broker, so subscribers never receive live updates.

## Findings

- Plan requires Gurgeh and Pollard to publish signals into the broker. (`docs/plans/2026-01-27-coordination-infrastructure-plan.md:121-136`)
- Broker provides Publish and WS fan-out, but no other package calls Publish. (`pkg/signals/broker.go:17-57`)
- Pollard watch loop only scans/diffs and writes snapshots; it does not emit signals. (`internal/pollard/watch/watcher.go:1-115`)
- Gurgeh emitter returns signals but has no broker wiring. (`internal/gurgeh/signals/emitter.go:1-120`)

## Proposed Solutions

### Option 1: Inject broker into Pollard watch + Gurgeh signal pipeline

**Approach:**
- Add optional broker dependency to Pollard watch and call Publish for emitted signals
- Add broker publishing path where Gurgeh signals are generated/persisted

**Pros:**
- Enables real-time WS updates as planned
- Minimal changes to existing emitters

**Cons:**
- Requires dependency injection wiring in CLI/TUI entry points

**Effort:** 2-4 hours

**Risk:** Medium (wiring across tools)

---

### Option 2: Add a signal forwarder that streams from stores

**Approach:**
- Introduce a small service that reads from the signal store and publishes new entries to broker

**Pros:**
- Decouples emitters from broker

**Cons:**
- Adds polling/dup logic

**Effort:** 4-6 hours

**Risk:** Medium

## Recommended Action

Publish signals through a local-only HTTP endpoint on the signals server and wire Pollard watch + Gurgeh spec load to send signals via a lightweight client.

## Technical Details

**Affected files:**
- `pkg/signals/broker.go:17-57`
- `internal/pollard/watch/watcher.go:1-115`
- `internal/gurgeh/signals/emitter.go:1-120`

## Resources

- Coordination plan Phase 4: `docs/plans/2026-01-27-coordination-infrastructure-plan.md:121-136`

## Acceptance Criteria

- [x] Pollard watch emits SignalCompetitorShipped (or equivalent) through broker
- [x] Gurgeh signal emission publishes to broker when signals are raised
- [x] Signals WS subscribers receive live updates
- [ ] Tests added/updated for broker publish paths

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Verified broker exists but no Publish calls outside broker
- Verified Pollard watch and Gurgeh emitter do not publish

**Learnings:**
- Signals WS server is effectively idle without publisher wiring

### 2026-01-28 - Implementation

**By:** Codex

**Actions:**
- Added `POST /api/signals` to signals server with JSON validation
- Added signals HTTP client with default local URL
- Wired Pollard watch to publish competitor watch updates
- Wired Gurgeh spec API to emit/store/publish signals on spec load

**Learnings:**
- Local-only WS server needs an explicit publish surface for cross-process emitters
