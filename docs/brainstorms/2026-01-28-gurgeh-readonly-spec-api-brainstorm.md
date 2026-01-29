date: 2026-01-28
topic: gurgeh-readonly-spec-api

# Gurgeh Read-only Spec API

## What We're Building
A local-only, read-only HTTP API for Gurgeh specs so external agents can query specs, requirements, CUJs, hypotheses, and history without reading YAML files directly. The API mirrors existing Gurgeh spec data and keeps the response shape stable for agent consumers.

## Why This Approach
External agents/colonies are the primary consumer, and they need a simple, stable interface to spec data. Bigend and the local TUI already read from disk directly, so the API adds value primarily for agents. Local-only enforcement prevents accidental exposure and keeps security assumptions simple.

## Key Decisions
- **Primary consumer:** external agents/colonies.
- **Endpoint set:** full v1 set (list, detail, requirements, CUJs, hypotheses, history).
- **Pagination:** offset/limit for list endpoint.
- **Response shape:** standard `pkg/httpapi` envelope for consistency with Pollard.
- **Bind policy:** strict local-only (reject non-loopback).

## Open Questions
- Should list endpoint include archived specs by default or require `include_archived=true`?
- What default `limit` should we use if not provided?
- Do we return full spec for `/api/specs/{id}` or omit heavy sections by default?

## Next Steps
- Proceed to `/workflows:plan` to define the server implementation, routes, and tests.
