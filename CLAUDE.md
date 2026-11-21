# Autarch

> See `AGENTS.md` for comprehensive development guide.

## Overview

Unified monorepo for AI agent development tools: Bigend, Gurgeh, Coldwine, Pollard, and the Mycroft bb worker spawner. What each tool does, its CLI and its docs: the Quick Reference table in `AGENTS.md`.

## Quick Commands

Build, test and run commands: `agents/development.md`.

## Workflow Discipline

**Review before implement:** When asked to "review", "look at", or "check" a plan/implementation, do NOT start implementing. First use the requested workflow tools to analyze and discuss. Only implement after explicit approval.

**Verify end-to-end before completion:** After making changes to data flow, exploration, or TUI features, verify the full user-facing result—don't assume success from tool execution alone. Run the actual flow and confirm data displays correctly.

**Pause after major refactoring:** For multi-file refactoring, pause after changes to run the affected flow manually. Confirm data propagates correctly through the pipeline before continuing.

## Execution Mode
codex-first: true

## Concurrency Rules

Test-run and concurrency conventions (including the `-race` flag): `agents/conventions.md`.

Review-only judgement rules (message fall-through, `Clone()` snapshots, chrome dimensions, ANSI-safe truncation) live in `CODING_STANDARDS.md`, which the review sub-agent reads.

## Workflow Rules

- Reproduce bugs before planning fixes. Phase 0 (reproduction + failing test) is mandatory before any multi-phase fix plan. If bug cannot be reproduced, document as could-not-reproduce and close.

## Design Decisions (Do Not Re-Ask)

See [Design Decisions (Do Not Re-Ask) in AGENTS.md](AGENTS.md#design-decisions-do-not-re-ask).
