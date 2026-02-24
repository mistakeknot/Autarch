# Compound-Engineering Parallel Multi-Agent Skills Analysis

## Summary
This document analyzes how many parallel agents are launched by major skills in the compound-engineering plugin (version 2.30.0).

---

## 1. `/deepen-plan` Command

**File:** `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/deepen-plan.md`

**Purpose:** Enhance a plan with parallel research agents for each section to add depth, best practices, and implementation details.

### Parallel Agents Launched

#### Phase 1: Skill-Based Sub-Agents (Section 2)
- **Count:** Dynamically discovered from ALL sources
  - Project-local skills: `./.claude/skills/`
  - User global skills: `~/.claude/skills/`
  - compound-engineering plugin skills
  - ALL other installed plugins
- **Type:** Each matched skill gets `Task general-purpose`
- **Instructions:** "Spawn ALL skill sub-agents in PARALLEL" and "No limit on skill sub-agents. Spawn one for every skill that could possibly be relevant."
- **Estimated Count:** 10-30+ parallel skill agents (unlimited in documentation)

#### Phase 2: Learning/Solutions Sub-Agents (Section 3)
- **Count:** Variable based on project learnings in `docs/solutions/`
- **Type:** `Task general-purpose`
- **Instructions:** "SPAWN sub-agents in PARALLEL for all filtered learnings"
- **Filtering:** Only spawn for learnings with relevant tags/categories
- **Estimated Count:** 5-15 parallel learning agents

#### Phase 3: Per-Section Research Agents (Section 4)
- **Count:** One per major plan section identified
- **Type:** `Task Explore`
- **Instructions:** "For each identified section, launch parallel research"
- **Estimated Count:** 4-8 parallel research agents

#### Phase 4: Review Agents (Section 5)
- **Count:** ALL available agents from ALL sources (review/, research/, design/, docs/)
- **Type:** Mixed (subagent_type varies by agent)
- **Instructions:** "Launch ALL agents in parallel" and "Do NOT filter agents by 'relevance' - run them ALL" and "20, 30, 40 parallel agents is fine - use everything"
- **Agent Sources:**
  - Project `.claude/agents/`
  - User's `~/.claude/agents/`
  - compound-engineering plugin (review, research, design, docs - NOT workflow)
  - ALL other installed plugins
- **Estimated Count:** 20-40+ parallel review agents (unlimited)

### Total Agents for `/deepen-plan`
**Estimated: 40-100+ parallel agents in a single execution**

**Agent Types:**
- general-purpose (for skills and learnings)
- Explore (for research)
- Specialized reviewers (security-sentinel, performance-oracle, architecture-strategist, etc.)
- Research agents (best-practices-researcher, framework-docs-researcher, git-history-analyzer, etc.)

**Trigger Keywords in Transcript:**
- "deepen-plan"
- "Skill-based sub-agents"
- "Learnings/Solutions sub-agents"
- "Per-Section Research Agents"
- "Review Agents"
- "CRITICAL: For EACH skill that matches, spawn a separate sub-agent"
- "All run simultaneously"

---

## 2. `/workflows:review` Command (part of `/plan_review`)

**File:** `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/workflows/review.md`

**Purpose:** Perform exhaustive code reviews using multi-agent analysis with deep inspection.

### Parallel Agents Launched

#### Base Review Agents (Parallel Tasks)
Always run these in parallel:

1. `kieran-rails-reviewer`
2. `dhh-rails-reviewer`
3. `git-history-analyzer`
4. `dependency-detective`
5. `pattern-recognition-specialist`
6. `architecture-strategist`
7. `code-philosopher`
8. `security-sentinel`
9. `performance-oracle`
10. `devops-harmony-analyst`
11. `data-integrity-guardian`
12. `agent-native-reviewer`

**Base Count: 12 parallel agents**

#### Conditional Agents (database migrations)
When PR contains migrations:
13. `data-migration-expert`
14. `deployment-verification-agent`

**Conditional Count: +2 agents if applicable**

#### Additional Agents (Optional)
- `code-simplicity-reviewer` (mentioned in parallel_tasks section)

### Total Agents for `/workflows:review`
**Estimated: 12-14 parallel review agents minimum** (up to 15+ if all conditions apply)

**Agent Types:**
- All are specialized reviewers from compound-engineering:review/*
- Each has specific expertise (security, performance, architecture, etc.)

**Trigger Keywords in Transcript:**
- "workflows:review"
- "Run ALL or most of these agents at the same time"
- "Parallel Agents"
- "Conditional Agents"
- "kieran-rails-reviewer"
- "security-sentinel"
- "performance-oracle"

---

## 3. Simple `/plan_review` Command

**File:** `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/plan_review.md`

**Purpose:** Have multiple specialized agents review a plan in parallel.

### Parallel Agents Launched

The command references these agents:
- `@agent-dhh-rails-reviewer`
- `@agent-kieran-rails-reviewer`
- `@agent-code-simplicity-reviewer`

**Count: 3 parallel agents** (minimum documented)

**Note:** This is a simplified wrapper that may call other review agents or `/workflows:review` internally.

### Total Agents for `/plan_review`
**Estimated: 3+ parallel agents**

**Trigger Keywords in Transcript:**
- "plan_review"
- "dhh-rails-reviewer"
- "kieran-rails-reviewer"
- "code-simplicity-reviewer"

---

## 4. Additional Large-Scale Parallel Skills Found

### `/agent-native-audit` Command

**File:** `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/agent-native-audit.md`

**Purpose:** Comprehensive agent-native architecture review with 8 scored principles.

#### Parallel Sub-Agents
Launches 8 parallel Explore agents, one for each principle:

1. **Action Parity Audit** - `Task Explore`
2. **Tools as Primitives** - `Task Explore`
3. **Context Injection** - `Task Explore`
4. **Shared Workspace** - `Task Explore`
5. **CRUD Completeness** - `Task Explore`
6. **UI Integration** - `Task Explore`
7. **Capability Discovery** - `Task Explore`
8. **Prompt-Native Features** - `Task Explore`

**Count: 8 parallel Explore agents**

**Note:** Also spawns sub-agents within each agent for specific analysis tasks.

**Trigger Keywords in Transcript:**
- "agent-native-audit"
- "Launch 8 parallel sub-agents"
- "Action Parity Audit"
- "Tools as Primitives"
- "Context Injection"

---

### `/slfg` Command (Swarm LFG)

**File:** `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/slfg.md`

**Purpose:** Full autonomous engineering workflow using swarm mode for parallel execution.

#### Parallel Phases
1. Sequential: `/workflows:plan` + `/deepen-plan` + `/workflows:work` (with swarm)
2. **Parallel Phase (Swarm):**
   - `/workflows:review` (as background Task)
   - `/test-browser` (as background Task)
3. Sequential: `/resolve_todo_parallel` + `/feature-video`

**Count:** Uses `/deepen-plan` (40-100+ agents) + `/workflows:review` (12-14 agents) + `/workflows:work` swarm (variable)

**Trigger Keywords in Transcript:**
- "slfg"
- "Swarm-enabled"
- "parallel execution"

---

### `/resolve_parallel`, `/resolve_pr_parallel`, `/resolve_todo_parallel` Commands

**Files:**
- `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/resolve_parallel.md`
- `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/resolve_pr_parallel.md`
- `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/commands/resolve_todo_parallel.md`

**Purpose:** Resolve TODO/PR comments in parallel.

#### Parallel Agents
For each unresolved item/comment:
- Spawn `Task pr-comment-resolver` (one per comment/todo)

**Count:** Variable (1 per unresolved item)
- Example: 3 comments → 3 parallel agents
- Example: 15 todos → 15 parallel agents

**Trigger Keywords in Transcript:**
- "resolve_parallel"
- "resolve_pr_parallel"
- "resolve_todo_parallel"
- "Spawn a pr-comment-resolver agent for each unresolved item in parallel"

---

## Summary Table

| Skill | Min Parallel Agents | Max Parallel Agents | Agent Types |
|-------|-------------------|-------------------|------------|
| `/deepen-plan` | 40 | 100+ | general-purpose, Explore, Specialized reviewers |
| `/workflows:review` | 12 | 15+ | Specialized reviewers only |
| `/plan_review` | 3 | 3+ | Specialized reviewers |
| `/agent-native-audit` | 8 | 8+ | Explore agents |
| `/slfg` | 50+ | 150+ | All types (calls multiple commands) |
| `/resolve_parallel` | 1 | N (per item) | pr-comment-resolver |
| `/resolve_pr_parallel` | 1 | N (per comment) | pr-comment-resolver |
| `/resolve_todo_parallel` | 1 | N (per todo) | pr-comment-resolver |

---

## Other Skills in compound-engineering Launching >3 Parallel Agents

The following skills/commands are NOT explicitly documented to launch >3 parallel agents, but use single or sequential agents:

- `brainstorming` - 1 agent
- `compound-docs` - 1 agent
- `file-todos` - Single tool
- `frontend-design` - 1 agent
- `git-worktree` - 1 agent
- `orchestrating-swarms` - Reference documentation (not a command)
- `agent-browser` - Single tool
- etc.

---

## Key Architectural Insights

1. **No Limit on Parallel Agents:** The `/deepen-plan` command explicitly states:
   - "No limit on skill sub-agents. Spawn one for every skill that could possibly be relevant."
   - "20, 30, 40 parallel agents is fine - use everything"
   - "CRITICAL RULES: Do NOT filter agents by 'relevance' - run them ALL"

2. **Nested Parallelization:** `/slfg` calls `/deepen-plan` which calls `/workflows:review` - creating cascading parallel agent swarms.

3. **Dynamic Agent Discovery:** Both `/deepen-plan` and `/workflows:review` discover agents from:
   - Project-local `.claude/agents/` directories
   - User's global `~/.claude/agents/`
   - All installed plugins
   - Not just compound-engineering

4. **Agent Types Used:**
   - `Explore` - Read-only research agents
   - `general-purpose` - Full access agents
   - `Plan` - Architecture/planning agents
   - Specialized reviewers - Domain-specific (security, performance, rails, etc.)
   - `pr-comment-resolver` - Comment/todo resolution agents

5. **Failure Resilience:** The system is designed to handle agents being spawned in parallel without coordination, allowing for:
   - Independent research on different topics
   - Redundant discovery of solutions
   - Parallel processing of large PR reviews

---

## Performance Characteristics

- **Fastest to Complete:** `/plan_review` with 3 agents (~2-3 minutes)
- **Moderate Parallelization:** `/workflows:review` with 12-14 agents (~5-10 minutes)
- **Massive Parallelization:** `/deepen-plan` with 40-100+ agents (~10-30 minutes)
- **Full Pipeline:** `/slfg` with cascading parallel phases (~30-60 minutes total)

