# Plan Status Report — 2026-01-31

> Generated from todos, git history, and codebase evidence. Status labels are heuristics unless backed by a todo.

## Method

- Map legacy paths to current modules: vauxhall→bigend, praude→gurgeh, tandemonium→coldwine; .praude→.gurgeh; .tandemonium→.coldwine; cmd/vauxhall→cmd/bigend.
- Scan . and /root/projects/Intermute (for Intermute plan paths).
- Status precedence: todo > derived evidence > commit evidence > preexisting > none.

## Summary

- Todo-tracked: 5
- Derived evidence: 1
- Commit evidence: 45
- Preexisting paths (no git evidence): 1
- No evidence: 2

## Status Legend

- todo:<status>: authoritative via todos/
- derived: implemented by concrete subsystems even if plan has no path list
- commit: referenced paths updated on/after plan date
- preexisting: referenced paths exist but no commit evidence
- none: no referenced paths found

## Plan Status

| Plan | Status | Evidence |
| --- | --- | --- |
| 2026-01-22-agent-targets-implementation-plan.md | commit | paths:6 latest:2026-01-28 |
| 2026-01-22-praude-archive-delete-plan.md | commit | paths:8 latest:2026-01-24 |
| 2026-01-22-praude-interview-agent-iteration-plan.md | commit | paths:1 latest:2026-01-24 |
| 2026-01-22-praude-status-list-ux-plan.md | commit | paths:11 latest:2026-01-27 |
| 2026-01-22-praude-vauxhall-style-parity-plan.md | commit | paths:4 latest:2026-01-30 |
| 2026-01-22-tandemonium-apply-detection-atomic-plan.md | commit | paths:2 latest:2026-01-24 |
| 2026-01-22-tandemonium-atomic-yaml-writes-plan.md | commit | paths:7 latest:2026-01-24 |
| 2026-01-22-tandemonium-foreign-keys-implementation-plan.md | commit | paths:1 latest:2026-01-24 |
| 2026-01-22-tandemonium-reject-task-fix-plan.md | commit | paths:1 latest:2026-01-24 |
| 2026-01-22-tandemonium-reviewstate-extraction-plan.md | commit | paths:3 latest:2026-01-30 |
| 2026-01-22-tandemonium-tui-db-reuse-plan.md | commit | paths:2 latest:2026-01-24 |
| 2026-01-22-vauxhall-m1b-parity-implementation-plan.md | commit | paths:6 latest:2026-01-28 |
| 2026-01-22-vauxhall-search-filters-plan.md | commit | paths:2 latest:2026-01-29 |
| 2026-01-22-vauxhall-tui-two-pane-layout-implementation-plan.md | commit | paths:2 latest:2026-01-29 |
| 2026-01-23-praude-interview-layout-init-answers-plan.md | commit | paths:5 latest:2026-01-30 |
| 2026-01-23-praude-interview-polish-implementation-plan.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-23-tandemonium-init-epics-implementation-plan.md | commit | paths:7 latest:2026-01-27 |
| 2026-01-23-tandemonium-init-epics-validation-plan.md | commit | paths:3 latest:2026-01-27 |
| 2026-01-23-tandemonium-tui-vauxhall-port-plan.md | commit | paths:4 latest:2026-01-30 |
| 2026-01-23-vauxhall-focus-pane-plan.md | commit | paths:2 latest:2026-01-29 |
| 2026-01-23-vauxhall-grouping-plan.md | commit | paths:2 latest:2026-01-29 |
| 2026-01-23-vauxhall-web-search-filters-plan.md | commit | paths:4 latest:2026-01-26 |
| 2026-01-25-intermute-mvp-implementation-plan.md | commit | paths:16 latest:2026-01-26 |
| 2026-01-26-arbiter-spec-sprint-implementation.md | commit | paths:11 latest:2026-01-31 |
| 2026-01-26-feat-cursor-style-unified-shell-layout-plan.md | commit | paths:1 latest:2026-01-26 |
| 2026-01-26-feat-pollard-first-class-research-input-plan.md | commit | paths:8 latest:2026-01-31 |
| 2026-01-27-coordination-infrastructure-plan.md | todo:complete | 001-complete-p2-coordination-doc-updates.md |
| 2026-01-27-feat-agent-runner-abstraction-plan.md | todo:complete | 004-complete-p2-coldwine-agent-runner-integration.md |
| 2026-01-27-feat-unified-ui-grammar-plan.md | todo:pending | 005-pending-p2-unified-ui-grammar-migration.md |
| 2026-01-27-feat-vision-spec-lifecycle-plan.md | none | no referenced paths found |
| 2026-01-27-task-performance-reliability-guardrails-plan.md | todo:pending | 006-pending-p2-guardrails-context-timeouts.md |
| 2026-01-28-agent-model-selector-implementation-plan.md | commit | paths:7 latest:2026-01-30 |
| 2026-01-28-agent-panel-streaming-diff-plan.md | commit | paths:5 latest:2026-01-30 |
| 2026-01-28-feat-coordination-api-foundation-plan.md | derived | pkg/httpapi/envelope.go, internal/pollard/server/server.go, pkg/jobs/jobs.go, internal/pollard/server/cache.go, pkg/netguard/bind.go, internal/gurgeh/server/server.go, internal/signals/cli/serve.go, pkg/signals/server.go |
| 2026-01-28-feat-gurgeh-readonly-spec-api-plan.md | commit | paths:5 latest:2026-01-30 |
| 2026-01-28-fix-guardrails-followthrough-plan.md | commit | paths:6 latest:2026-01-30 |
| 2026-01-28-hide-system-labels-chat-panel-plan.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-28-kickoff-chat-initial-system-messages-plan.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-28-kickoff-doc-template-copy-plan.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-28-refactor-extract-shared-async-jobs-package-plan.md | commit | paths:6 latest:2026-01-31 |
| 2026-01-29-feat-unified-tui-non-printable-shortcuts-plan.md | commit | paths:8 latest:2026-01-31 |
| 2026-01-29-interview-breadcrumb-scan-nav.md | commit | paths:3 latest:2026-01-30 |
| 2026-01-29-scan-artifact-ui-display.md | commit | paths:4 latest:2026-01-31 |
| 2026-01-29-scan-artifact-validation.md | commit | paths:14 latest:2026-01-29 |
| 2026-01-29-scan-open-questions-ui.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-29-scan-signoff-breadcrumb-plan.md | commit | paths:3 latest:2026-01-30 |
| 2026-01-29-scan-validation-wiring.md | commit | paths:3 latest:2026-01-31 |
| 2026-01-29-structured-scan-output.md | commit | paths:3 latest:2026-01-29 |
| 2026-01-30-chat-panel-mouse-scroll.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-30-coordination-reconciliation-plan.md | todo:ready | 007-ready-p1-coordination-reconciliation-mvp.md |
| 2026-01-30-open-questions-chat-resolution.md | preexisting | paths:3 |
| 2026-01-30-scan-progress-chatpane.md | commit | paths:2 latest:2026-01-30 |
| 2026-01-31-feat-unified-chat-spec-flow-plan.md | none | no referenced paths found |
| 2026-01-31-plan-status-precommit-hook.md | commit | paths:7 latest:2026-01-31 |

## Derived Evidence Details

- 2026-01-28-feat-coordination-api-foundation-plan.md
  - pkg/httpapi/envelope.go
  - internal/pollard/server/server.go
  - pkg/jobs/jobs.go
  - internal/pollard/server/cache.go
  - pkg/netguard/bind.go
  - internal/gurgeh/server/server.go
  - internal/signals/cli/serve.go
  - pkg/signals/server.go
