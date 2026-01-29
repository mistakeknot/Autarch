date: 2026-01-28
topic: shared-async-jobs

# Shared Async Jobs (Pollard → Autarch)

## What We're Building
Introduce a shared, minimal async job package in Autarch so multiple tools can reuse a common job store and status model without committing to shared HTTP APIs yet. The immediate source is Pollard's in-memory JobStore, which already supports async execution, cancellation, and retention. We will extract the core job types and store into a new shared package (e.g., `pkg/jobs`) while leaving Pollard's HTTP endpoints intact.

## Why This Approach
Pollard already has a working async job system with lifecycle statuses and retention. Generalizing the *core* into a shared package provides quick reuse while avoiding premature standardization of job HTTP semantics. This aligns with local-only coordination and avoids Intermute schema churn until a second concrete consumer exists.

## Key Decisions
- **Scope is minimal**: shared package includes job status enum, job struct, in-memory JobStore, retention (TTL/max) behavior.
- **No shared HTTP handlers yet**: keep Pollard endpoints in `internal/pollard/server` and avoid locking API shapes too early.
- **Defer Intermute generalization**: Intermute remains task/session-focused; revisit only when 2+ tools need async jobs.

## Open Questions
- Package location/name: `pkg/jobs` vs `pkg/coordjobs`.
- Should retention defaults live in Pollard (caller) or in the shared package?
- Do we need a shared error type (e.g., job not found, already complete) now or later?

## Next Steps
- If accepted, proceed to `/workflows:plan` to define extraction steps and tests.
