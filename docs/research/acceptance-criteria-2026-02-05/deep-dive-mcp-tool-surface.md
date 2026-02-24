# Deep-dive MCP tool surface

**Source:** Agent result from session b817bfb0
**Task ID:** a855033
**Date:** 2026-02-05

---

Now I have a thorough understanding of the full codebase. Here is the complete MCP tool surface area design.

---

# Autarch MCP Tool Surface Area Design

## Research Summary

### Existing Infrastructure

Two MCP server implementations exist today:

**1. Go MCP Server** (`/root/projects/Autarch/pkg/mcp/server.go`)
- JSON-RPC 2.0 over stdio
- Tool registration via `Server.RegisterTool(Tool{Name, Description, InputSchema, Handler})`
- Response envelope: `ToolsCallResult{Content: []ContentBlock{Type, Text}, IsError}`
- 8 tools registered: `autarch_list_prds`, `autarch_get_prd`, `autarch_list_tasks`, `autarch_update_task`, `autarch_research`, `autarch_suggest_hunters`, `autarch_project_status`, `autarch_send_message`
- Naming convention: `autarch_` prefix, snake_case verbs

**2. TypeScript MCP Server** (`/root/projects/Autarch/mcp-server/src/server.ts`)
- `@modelcontextprotocol/sdk` with stdio transport
- Zod schema validation
- 4 tools: `list_tasks`, `claim_task`, `update_progress`, `complete_task`
- Task IDs: `tsk_` prefix + 26-char ULID
- Naming convention: snake_case, no prefix

**Entity Types** (from `/root/projects/Autarch/pkg/intermute/types.go`):
- **Spec**: id, project, title, vision, users, problem, status (draft/research/validated/archived/needs_revision), version, timestamps
- **Epic**: id, project, spec_id, title, description, status (open/in_progress/done), version
- **Story**: id, project, epic_id, title, acceptance_criteria[], status (todo/in_progress/review/done), version
- **Task**: id, project, story_id, title, agent, session_id, status (pending/running/blocked/done), version
- **Insight**: id, project, spec_id, source, category, title, body, url, score
- **Session**: id, project, name, agent, task_id, status (running/idle/error), timestamps
- **CriticalUserJourney**: id, spec_id, project, title, persona, priority, entry_point, exit_point, steps[], success_criteria[], error_recovery[], status, version
- **Signal**: id, type, source, spec_id, affected_field, severity, title, detail, dismissed
- **Reservation**: id, agent_id, project, path_pattern, exclusive, reason, expires_at, released_at
- **Agent**: id, session_id, name, project, capabilities[], metadata{}, status, last_seen
- **Message**: id, thread_id, project, from, to[], subject, body, importance, ack_required

**Confidence Scoring** (from `/root/projects/Autarch/internal/gurgeh/arbiter/confidence/calculator.go`):
- 5 axes: Completeness, Consistency, Specificity, Research, Assumptions (all 0.0-1.0)
- Weighted total: 20% completeness + 25% consistency + 20% specificity + 20% research + 15% assumptions
- Blended with scan quality scores (Grounding, Clarity, Completeness, Consistency)

**Sprint Phases**: Vision, Problem, Users, Features+Goals, CUJs, Requirements, Scope+Assumptions, Acceptance Criteria

---

## Complete Tool Catalog

### Naming Convention

Following the existing Go MCP server pattern: `autarch_` prefix + `{verb}_{entity}`. Plural for list operations, singular for CRUD on specific items.

### Entity CRUD Tools (7 entities)

| # | Tool Name | Operation | Entity | Required Params | Optional Params | Auth |
|---|-----------|-----------|--------|-----------------|-----------------|------|
| **Spec** | | | | | | |
| 1 | `autarch_create_spec` | Create | Spec | `title` | `vision`, `users`, `problem`, `project` | lead |
| 2 | `autarch_get_spec` | Read | Spec | `id` | - | any_agent |
| 3 | `autarch_update_spec` | Update | Spec | `id` | `title`, `vision`, `users`, `problem`, `status` | lead |
| 4 | `autarch_delete_spec` | Delete | Spec | `id` | - | lead |
| 5 | `autarch_list_specs` | List | Spec | - | `status`, `project` | any_agent |
| **Insight (Finding)** | | | | | | |
| 6 | `autarch_create_insight` | Create | Insight | `title`, `source` | `spec_id`, `category`, `body`, `url`, `score` | any_agent |
| 7 | `autarch_get_insight` | Read | Insight | `id` | - | any_agent |
| 8 | `autarch_update_insight` | Update | Insight | `id` | `title`, `body`, `score`, `category` | any_agent |
| 9 | `autarch_delete_insight` | Delete | Insight | `id` | - | lead |
| 10 | `autarch_list_insights` | List | Insight | - | `spec_id`, `category`, `source` | any_agent |
| 11 | `autarch_link_insight` | Link | Insight | `insight_id`, `spec_id` | - | any_agent |
| **Task** | | | | | | |
| 12 | `autarch_create_task` | Create | Task | `title` | `story_id`, `agent`, `project` | lead |
| 13 | `autarch_get_task` | Read | Task | `id` | - | any_agent |
| 14 | `autarch_update_task` | Update | Task | `id` | `status`, `agent`, `session_id`, `note` | assigned_or_lead |
| 15 | `autarch_delete_task` | Delete | Task | `id` | - | lead |
| 16 | `autarch_list_tasks` | List | Task | - | `status`, `agent`, `story_id` | any_agent |
| 17 | `autarch_claim_task` | Claim | Task | `task_id`, `agent_id` | - | any_agent |
| 18 | `autarch_complete_task` | Complete | Task | `task_id` | `base_branch` | assigned_or_lead |
| **Reservation** | | | | | | |
| 19 | `autarch_reserve_files` | Create | Reservation | `path_pattern`, `agent_id` | `exclusive`, `reason`, `ttl_minutes` | any_agent |
| 20 | `autarch_release_reservation` | Delete | Reservation | `id` | - | owner_or_lead |
| 21 | `autarch_list_reservations` | List | Reservation | - | `agent_id`, `active_only` | any_agent |
| 22 | `autarch_check_reservation` | Read | Reservation | `path` | - | any_agent |
| **Signal** | | | | | | |
| 23 | `autarch_emit_signal` | Create | Signal | `type`, `spec_id`, `title` | `severity`, `detail`, `affected_field`, `source` | any_agent |
| 24 | `autarch_get_signal` | Read | Signal | `id` | - | any_agent |
| 25 | `autarch_dismiss_signal` | Update | Signal | `id` | `reason` | lead |
| 26 | `autarch_list_signals` | List | Signal | - | `spec_id`, `type`, `severity`, `active_only` | any_agent |
| **Team (Agent)** | | | | | | |
| 27 | `autarch_register_agent` | Create | Agent | `name` | `capabilities[]`, `metadata{}`, `project` | self |
| 28 | `autarch_get_agent` | Read | Agent | `name` or `id` | - | any_agent |
| 29 | `autarch_update_agent` | Update | Agent | `id` | `status`, `capabilities[]`, `metadata{}` | self_or_lead |
| 30 | `autarch_list_agents` | List | Agent | - | `project`, `status` | any_agent |
| 31 | `autarch_heartbeat` | Update | Agent | `agent_id` | `status`, `task_id` | self |
| **Confidence** | | | | | | |
| 32 | `autarch_get_confidence` | Read | Confidence | `spec_id` or `sprint_id` | - | any_agent |
| 33 | `autarch_recalculate_confidence` | Compute | Confidence | `sprint_id` | - | lead |

### Special / Orchestration Tools

| # | Tool Name | Purpose | Required Params | Optional Params | Auth |
|---|-----------|---------|-----------------|-----------------|------|
| 34 | `autarch_refresh_context` | Returns full state snapshot for context refresh during long sessions | - | `sprint_id`, `include` (array of: specs, tasks, signals, confidence, reservations, agents) | any_agent |
| 35 | `autarch_report_status` | Agent proactively reports its status; replaces heuristic stall detection | `agent_id`, `status` | `task_id`, `progress_pct`, `blockers[]`, `message` | self |
| 36 | `autarch_list_capabilities` | Returns all registered MCP tools with descriptions and schemas | - | `filter` (entity name to filter by) | any_agent |
| 37 | `autarch_send_message` | Send a message via Intermute to another tool/agent | `to`, `subject`, `body` | `thread_id`, `importance`, `ack_required` | any_agent |
| 38 | `autarch_fetch_inbox` | Read messages for an agent | `agent_id` | `since_cursor`, `limit` | self_or_lead |

**Total: 38 tools** (33 entity CRUD + 5 orchestration)

---

## JSON Schema Examples for 3 Key Tools

### 1. `autarch_create_spec`

```json
{
  "name": "autarch_create_spec",
  "description": "Create a new product specification (PRD). Returns the created spec with generated ID. Status defaults to 'draft'. Use autarch_update_spec to advance status through the lifecycle.",
  "inputSchema": {
    "type": "object",
    "required": ["title"],
    "properties": {
      "title": {
        "type": "string",
        "description": "Spec title (max 200 chars)",
        "maxLength": 200
      },
      "vision": {
        "type": "string",
        "description": "Product vision statement"
      },
      "users": {
        "type": "string",
        "description": "Target user personas and their needs"
      },
      "problem": {
        "type": "string",
        "description": "Problem statement this spec addresses"
      },
      "project": {
        "type": "string",
        "description": "Project scope (defaults to server's configured project)"
      }
    }
  }
}
```

**Response envelope:**
```json
{
  "ok": true,
  "data": {
    "id": "spec_01J7X...",
    "project": "autarch",
    "title": "Agent-native MCP tool surface",
    "vision": "",
    "users": "",
    "problem": "",
    "status": "draft",
    "version": 1,
    "created_at": "2026-02-05T10:30:00Z",
    "updated_at": "2026-02-05T10:30:00Z"
  },
  "meta": {
    "tool": "autarch_create_spec",
    "duration_ms": 42
  }
}
```

### 2. `autarch_refresh_context`

```json
{
  "name": "autarch_refresh_context",
  "description": "Returns a comprehensive state snapshot for context refresh during long-running sessions. Agents should call this every 10-15 minutes or when resuming after idle. Includes only requested sections to minimize token usage.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "sprint_id": {
        "type": "string",
        "description": "Active sprint ID to include sprint state. Omit for project-level context only."
      },
      "include": {
        "type": "array",
        "items": {
          "type": "string",
          "enum": ["specs", "tasks", "signals", "confidence", "reservations", "agents", "sprint", "inbox"]
        },
        "description": "Sections to include in snapshot. Defaults to all if omitted.",
        "default": ["specs", "tasks", "signals", "confidence", "reservations", "agents"]
      }
    }
  }
}
```

**Response envelope:**
```json
{
  "ok": true,
  "data": {
    "timestamp": "2026-02-05T10:45:00Z",
    "project": "autarch",
    "specs": {
      "total": 3,
      "by_status": {"draft": 1, "validated": 2},
      "items": [
        {"id": "spec_01J7X...", "title": "Agent-native MCP tools", "status": "draft"}
      ]
    },
    "tasks": {
      "total": 12,
      "by_status": {"pending": 3, "running": 4, "blocked": 1, "done": 4},
      "my_tasks": [
        {"id": "tsk_01J8Y...", "title": "Implement spec CRUD", "status": "running", "progress": 60}
      ]
    },
    "signals": {
      "active_count": 2,
      "critical": [
        {"id": "sig_01...", "type": "assumption_decayed", "spec_id": "spec_01J7X...", "title": "Market size assumption stale"}
      ]
    },
    "confidence": {
      "spec_01J7X...": {
        "completeness": 0.5,
        "consistency": 0.85,
        "specificity": 0.7,
        "research": 0.3,
        "assumptions": 0.5,
        "total": 0.58
      }
    },
    "reservations": {
      "active": [
        {"id": "rsv_01...", "agent_id": "agent-claude-1", "path_pattern": "internal/gurgeh/**", "exclusive": true, "expires_at": "2026-02-05T11:00:00Z"}
      ]
    },
    "agents": {
      "online": 3,
      "items": [
        {"name": "gurgeh-lead", "status": "running", "task_id": "tsk_01J8Y...", "last_seen": "2026-02-05T10:44:30Z"}
      ]
    },
    "sprint": {
      "id": "abc123...",
      "phase": "Features + Goals",
      "phase_index": 3,
      "total_phases": 8,
      "confidence_total": 0.58,
      "conflicts": [],
      "sections": {
        "Vision": {"status": "accepted"},
        "Problem": {"status": "accepted"},
        "Users": {"status": "accepted"},
        "Features + Goals": {"status": "proposed"}
      }
    }
  },
  "meta": {
    "tool": "autarch_refresh_context",
    "duration_ms": 87,
    "next_refresh_hint": "600s"
  }
}
```

### 3. `autarch_report_status`

```json
{
  "name": "autarch_report_status",
  "description": "Proactive status report from an agent. Replaces heuristic stall detection with explicit signals. Agents should call this at regular intervals (every 2-5 minutes during active work) or when status changes. A missing heartbeat after 10 minutes triggers stall detection in the dashboard.",
  "inputSchema": {
    "type": "object",
    "required": ["agent_id", "status"],
    "properties": {
      "agent_id": {
        "type": "string",
        "description": "The reporting agent's ID"
      },
      "status": {
        "type": "string",
        "enum": ["working", "waiting", "blocked", "reviewing", "idle", "completing"],
        "description": "Current agent status"
      },
      "task_id": {
        "type": "string",
        "description": "Task currently being worked on (if any)"
      },
      "progress_pct": {
        "type": "number",
        "minimum": 0,
        "maximum": 100,
        "description": "Estimated progress percentage on current task"
      },
      "blockers": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "enum": ["dependency", "reservation_conflict", "review_needed", "question", "external"]
            },
            "description": {
              "type": "string"
            },
            "blocking_entity_id": {
              "type": "string",
              "description": "ID of the entity causing the block (task ID, reservation ID, etc.)"
            }
          },
          "required": ["type", "description"]
        },
        "description": "List of current blockers preventing progress"
      },
      "message": {
        "type": "string",
        "description": "Free-form status message for the dashboard (max 500 chars)",
        "maxLength": 500
      }
    }
  }
}
```

**Response envelope:**
```json
{
  "ok": true,
  "data": {
    "acknowledged": true,
    "agent_id": "agent-claude-1",
    "recorded_at": "2026-02-05T10:42:00Z",
    "stall_threshold": "600s",
    "directives": [
      {
        "type": "reservation_expiring",
        "message": "Your reservation on internal/gurgeh/** expires in 5 minutes. Call autarch_reserve_files to renew.",
        "entity_id": "rsv_01..."
      }
    ]
  },
  "meta": {
    "tool": "autarch_report_status",
    "duration_ms": 12
  }
}
```

---

## Authorization Matrix

| Role | Description | Can Invoke |
|------|-------------|------------|
| **lead** | The human operator or lead agent orchestrating the sprint | All tools. Only role that can: delete specs, delete insights, dismiss signals, delete tasks, recalculate confidence |
| **assigned_or_lead** | The agent assigned to a specific task, or the lead | `update_task` (on own tasks), `complete_task` (on own tasks), plus everything `any_agent` can do |
| **self** | The agent acting on its own identity | `register_agent`, `update_agent` (own record), `heartbeat`, `report_status`, `fetch_inbox` (own inbox) |
| **self_or_lead** | Agent acting on own identity or the lead | `update_agent`, `fetch_inbox` |
| **owner_or_lead** | The agent that created a reservation, or the lead | `release_reservation` |
| **any_agent** | Any registered agent | All read/list operations, `create_insight`, `link_insight`, `emit_signal`, `reserve_files`, `check_reservation`, `send_message`, `refresh_context`, `list_capabilities`, `claim_task` |

**Enforcement mechanism**: Each tool call includes the caller's `agent_id` in the MCP session context (set during `initialize`). The server validates against the authorization rule before executing. Failed authorization returns error code `FORBIDDEN` (-32603).

---

## Context Injection Design

### Session Start System Prompt

Injected into the agent's system prompt when an Autarch MCP session initializes:

```
--- AUTARCH CONTEXT (auto-injected at session start) ---

You are operating as agent "{agent_name}" (id: {agent_id}) in project "{project}".

## Your Assignment
- Task: {task_title} ({task_id})
- Status: {task_status}
- Story: {story_title} ({story_id})
- Epic: {epic_title} ({epic_id})
- Spec: {spec_title} ({spec_id})

## Acceptance Criteria
{foreach criterion}
- [ ] {criterion.text}
{end}

## File Scope
- Reserved paths: {reservation.path_pattern} (exclusive: {reservation.exclusive}, expires: {reservation.expires_at})
- Shared with: {reservation.shared_with[]}

## Active Signals (requiring attention)
{foreach signal where active && spec_id matches}
- [{signal.severity}] {signal.title}: {signal.detail}
{end}

## Current Confidence ({spec_id})
- Completeness: {confidence.completeness} | Consistency: {confidence.consistency}
- Specificity: {confidence.specificity} | Research: {confidence.research}
- Total: {confidence.total}

## Sprint State (if active)
- Phase: {sprint.phase} ({sprint.phase_index}/{sprint.total_phases})
- Sprint ID: {sprint.id}

## Available Tools
Call `autarch_list_capabilities` for the full tool catalog. Key tools:
- `autarch_report_status` - Report your status every 2-5 min (REQUIRED)
- `autarch_refresh_context` - Refresh this context when stale (>10 min)
- `autarch_update_task` - Update task status/progress
- `autarch_complete_task` - Signal task completion
- `autarch_reserve_files` - Lock files before editing
- `autarch_check_reservation` - Check if a path is reserved by another agent
- `autarch_emit_signal` - Raise alerts about spec quality issues

## Protocol
1. Call `autarch_report_status` with status="working" at start
2. Check `autarch_check_reservation` before editing files outside your scope
3. Call `autarch_report_status` every 5 minutes during active work
4. When blocked, call `autarch_report_status` with status="blocked" and blockers[]
5. When done, call `autarch_complete_task` then `autarch_report_status` with status="completing"

--- END AUTARCH CONTEXT ---
```

### Context Refresh Strategy (Sessions >15 min)

The context injection stays fresh through three mechanisms:

**1. Periodic agent-driven refresh (primary)**
- The agent is instructed to call `autarch_refresh_context` every 10-15 minutes
- The `meta.next_refresh_hint` field in the response tells the agent when to refresh next
- Include only the sections relevant to the agent's current work to minimize token usage

**2. Directive piggyback (reactive)**
- Every `autarch_report_status` response includes a `directives[]` array
- Directives carry urgent context changes that cannot wait for the next refresh:
  - `reservation_expiring`: Agent's file reservation is about to expire
  - `signal_raised`: New critical signal affecting the agent's spec
  - `task_reassigned`: Agent's task was reassigned
  - `phase_advanced`: Sprint phase changed (relevant for Gurgeh agents)
  - `conflict_detected`: New blocker-level consistency conflict

**3. WebSocket push (optional, for connected agents)**
- Agents that call `Connect()` on the Intermute client receive real-time domain events
- The MCP server can subscribe on the agent's behalf during `initialize`
- Events are surfaced as MCP notifications (not tool calls) using the `notifications/message` method
- This is optional -- agents that only use stdio still get freshness via mechanisms 1 and 2

### Token Budget

The initial context injection is designed to fit within approximately 1,500-2,000 tokens. The `autarch_refresh_context` response with all sections is approximately 2,000-3,000 tokens. With selective `include` filtering, an incremental refresh can be as small as 300-500 tokens (e.g., just `["tasks", "signals"]`).

---

## Response Envelope Standard

All tools use a consistent envelope, extending the existing Go MCP server's pattern:

```json
{
  "ok": true|false,
  "data": { ... },
  "meta": {
    "tool": "autarch_<tool_name>",
    "duration_ms": 42,
    "version": "0.2.0"
  },
  "error": {
    "code": "NOT_FOUND|FORBIDDEN|INVALID_STATE|BLOCKED|CONFLICT|INTERNAL",
    "message": "Human-readable error description",
    "details": { ... }
  }
}
```

Error codes (extending the TypeScript server's `ErrorCode` enum):
- `NOT_FOUND` - Entity does not exist
- `FORBIDDEN` - Agent does not have permission for this operation
- `INVALID_STATE` - State transition not allowed (e.g., completing a non-review task)
- `BLOCKED` - Operation blocked by dependency or reservation conflict
- `CONFLICT` - Optimistic locking failure (version mismatch)
- `ALREADY_CLAIMED` - Task already assigned to another agent
- `INTERNAL` - Unexpected server error

---

## Key Design Decisions

**1. Unified Go server, deprecate TypeScript server.** The Go MCP server already has the Intermute client integration, the confidence calculator, and the arbiter orchestrator. The TypeScript server's 4 tools become part of the unified 38-tool surface. The Go server already handles JSON-RPC 2.0 over stdio.

**2. Entity naming: "Insight" not "Finding".** The Intermute type is `Insight` with a well-defined schema. The arbiter's `ResearchFinding` is an internal conversion type. MCP tools should use the canonical Intermute name.

**3. No separate CUJ CRUD in the 7 entities.** CUJs already have full CRUD on the Intermute client (`CreateCUJ`, `GetCUJ`, `ListCUJs`, `UpdateCUJ`, `DeleteCUJ`, `LinkCUJToFeature`). They could be added as an 8th entity group (6 more tools), but the request specified 7 entities. CUJ tools are a natural extension.

**4. Reservation uses `check` not `get`.** The primary agent use case for reservations is "can I edit this file?" not "give me reservation details by ID." `autarch_check_reservation` takes a `path` and returns whether it is reserved and by whom, which is the ergonomic operation for agents about to modify files.

**5. `autarch_report_status` carries directives back.** This is the key innovation for stall detection. Instead of the system guessing whether an agent is stalled by monitoring output frequency, the agent explicitly reports its state. The response carries directives, making every heartbeat an opportunity to push urgent context changes without a separate notification channel.

**6. Confidence is read-only for non-leads.** Confidence scores are computed automatically during sprint phase transitions (`updateConfidence` in `orchestrator.go`). The `recalculate_confidence` tool exists for the lead to force a recomputation after manual edits, but agents cannot manipulate their own confidence scores.

### Files Referenced

- `/root/projects/Autarch/pkg/mcp/server.go` - Go MCP server, tool registration pattern
- `/root/projects/Autarch/pkg/mcp/handlers.go` - Existing 8 tool handler implementations
- `/root/projects/Autarch/pkg/mcp/types.go` - JSON-RPC and MCP protocol types
- `/root/projects/Autarch/mcp-server/src/server.ts` - TypeScript MCP server (4 task tools)
- `/root/projects/Autarch/mcp-server/src/types/index.ts` - Task/Progress/FileScope types
- `/root/projects/Autarch/mcp-server/src/schemas/index.ts` - Zod validation schemas
- `/root/projects/Autarch/mcp-server/src/tools/claim-task.ts` - Idempotent task claiming
- `/root/projects/Autarch/mcp-server/src/tools/complete-task.ts` - Task completion with validation
- `/root/projects/Autarch/mcp-server/src/tools/update-progress.ts` - Progress/criteria tracking
- `/root/projects/Autarch/mcp-server/src/tools/list-tasks.ts` - Task listing with filters
- `/root/projects/Autarch/pkg/intermute/types.go` - All entity types: Spec, Epic, Story, Task, Insight, Session, CUJ, Agent, Message, Reservation
- `/root/projects/Autarch/pkg/intermute/client.go` - Full Intermute client with CRUD for all entities
- `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` - Sprint state, phases, confidence, conflicts, drafts
- `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` - Sprint flow, phase transitions, confidence updates
- `/root/projects/Autarch/internal/gurgeh/arbiter/confidence/calculator.go` - 5-axis confidence scoring
- `/root/projects/Autarch/internal/gurgeh/arbiter/intermute.go` - ResearchProvider interface and ResearchBridge
- `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` - Phase-specific research configs
- `/root/projects/Autarch/pkg/signals/signal.go` - Signal types, severities
- `/root/projects/Autarch/pkg/signals/broker.go` - Pub/sub signal distribution
- `/root/projects/Autarch/internal/coldwine/storage/schema.go` - Epic/Story/Task/AgentSession/Worktree schemas