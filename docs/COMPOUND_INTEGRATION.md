# Compound Engineering Integration

> How Autarch tools leverage Compound Engineering patterns for enhanced AI-agent workflows

This guide covers the integration between Autarch's tool suite and the Compound Engineering plugin patterns.

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     COMPOUND ENGINEERING PATTERNS                        │
│                                                                          │
│   Multi-Agent Review   │   Knowledge Compounding   │   SpecFlow Analysis │
└──────────────┬─────────────────────┬────────────────────────┬───────────┘
               │                     │                        │
               ▼                     ▼                        ▼
       ┌───────────────┐     ┌───────────────┐     ┌──────────────────┐
       │    GURGEH     │     │   docs/       │     │    SpecFlow      │
       │ PRD Reviewers │     │  solutions/   │     │  Gap Analyzer    │
       └───────────────┘     └───────────────┘     └──────────────────┘
               │                     │                        │
               │     ┌───────────────┼────────────────────────┘
               │     │               │
               ▼     ▼               ▼
       ┌──────────────────────────────────────────────────────────────────┐
       │                     AUTARCH PLUGIN                                │
       │                                                                   │
       │   /autarch:prd   │   /autarch:research   │   /autarch:tasks      │
       └──────────────────────────────────────────────────────────────────┘
```

---

## Patterns Adopted

### 1. Multi-Agent Parallel Review

**Origin:** Compound Engineering's review agent pattern

**Implementation:** `internal/gurgeh/review/` and `internal/pollard/review/`

Both Gurgeh (PRDs) and Pollard (research) now use concurrent reviewer agents that validate quality in parallel:

```go
// Gurgeh PRD review (internal/gurgeh/review/prd_reviewers.go)
result, err := review.RunParallelReview(ctx, spec)
// Runs: CompletenessReviewer, CUJConsistencyReviewer,
//       AcceptanceCriteriaReviewer, ScopeCreepDetector

// Pollard research review (internal/pollard/review/reviewers.go)
result, err := review.RunParallelReview(ctx, insight)
// Runs: SourceCredibilityReviewer, RelevanceReviewer, ContradictionDetector
```

**Benefits:**
- Faster review cycles (parallel execution)
- Specialized reviewers catch specific issues
- Consistent quality scoring (0.0-1.0)
- Aggregated results with severity levels

### 2. Knowledge Compounding (docs/solutions/)

**Origin:** Compound Engineering's `/workflows:compound` pattern

**Implementation:** `docs/solutions/` directory with YAML frontmatter

Solved problems are captured as searchable documentation:

```bash
docs/solutions/
├── gurgeh/           # PRD generation issues
├── coldwine/         # Task orchestration issues
├── pollard/          # Research/hunter issues
├── bigend/           # Aggregation issues
├── integration/      # Cross-tool issues
└── patterns/         # Reusable patterns
```

**Solution file format:**
```yaml
---
module: gurgeh
date: 2026-01-26
problem_type: validation_error
component: prd_reviewers
symptoms:
  - "CUJ validation fails on valid specs"
root_cause: "Missing linked_requirements field check"
severity: medium
tags: [cuj, validation, review]
---

# Problem Title

## Problem Statement
...
```

**Workflow:**
1. Before debugging, search solutions: `grep -r "symptom" docs/solutions/`
2. After fixing, run `/compound` to capture the solution
3. Future sessions benefit from institutional knowledge

### 3. SpecFlow Gap Analysis

**Origin:** Compound Engineering's `spec-flow-analyzer` agent

**Implementation:** `internal/gurgeh/spec/specflow_analyzer.go`

Detects specification gaps in PRDs before implementation:

```go
analyzer := spec.NewSpecFlowAnalyzer()
result := analyzer.Analyze(prdSpec)

// Returns gaps by category:
// - missing_flow: Requirements without CUJ coverage
// - unclear_criteria: Vague acceptance criteria
// - edge_case: Missing edge case handling
// - error_handling: Missing error scenarios
// - state_transition: Implicit state changes
// - data_validation: Missing validation rules
// - integration_point: Undocumented integrations
```

**CLI Access:**
```bash
gurgeh review PRD-001 --gaps  # Includes SpecFlow analysis
```

### 4. Agent-Native Architecture

**Origin:** Compound Engineering's agent-native principles

**Implementation:** CLI parity + MCP server

All TUI actions are available as CLI commands (Parity principle):

| TUI Action | CLI Equivalent |
|------------|----------------|
| Create PRD | `gurgeh create --title "..." --summary "..."` |
| Approve PRD | `gurgeh approve PRD-001` |
| Review PRD | `gurgeh review PRD-001 --gaps` |
| Assign Task | `coldwine task assign TASK-001 --agent claude` |
| Block Task | `coldwine task block TASK-001 --reason "..."` |

MCP server for AI agent access:
```bash
autarch-mcp --project /path/to/project
# Exposes: autarch_list_prds, autarch_create_prd, autarch_research, etc.
```

---

## Workflow Integration

### Research → PRD Enhancement

Compound's `best-practices-researcher` pattern feeds Pollard's knowledge base:

```
Compound research agents → Pollard .pollard/insights/
                                    │
                                    ▼
                           Gurgeh PRD enrichment
```

### Pollard ↔ Arbiter via Intermute (First-Class Research)

Pollard findings are published as Intermute Insights and linked to the sprint's Spec:

```
                           ┌──────────────────────┐
                           │  Pollard Hunters      │
                           │  (github-scout, HN)   │
                           └──────────┬───────────┘
                                      │ QuickScanner.Scan()
                                      ▼
                           ┌──────────────────────┐
                           │  Arbiter Orchestrator │
                           │  runQuickScan()       │
                           └──────────┬───────────┘
                                      │ ResearchProvider.PublishInsight()
                                      ▼
                           ┌──────────────────────┐
                           │  Intermute Server     │
                           │  Spec ← Insight links │
                           └──────────┬───────────┘
                                      │ FetchLinkedInsights()
                                      ▼
                           ┌──────────────────────┐
                           │  SprintState          │
                           │  .Findings[]          │
                           │  .ResearchCtx         │
                           │  .Confidence.Research  │
                           └──────────────────────┘
```

**Key files:**
- `internal/gurgeh/arbiter/intermute.go` — `ResearchProvider` interface + `ResearchBridge`
- `internal/gurgeh/arbiter/deepscan.go` — Async deep scan handoff
- `internal/gurgeh/arbiter/orchestrator.go` — `researchQuality()` scoring formula
- `internal/pollard/quick/scan.go` — Real Pollard scanner (implements `QuickScanner`)

**Research quality scoring** (30% count + 30% diversity + 40% relevance):
- Count: `min(findingCount / 10, 1.0)` — scales linearly up to 10 findings
- Diversity: `min(sourceTypes / 3, 1.0)` — rewards distinct sources (github, hackernews, arxiv)
- Relevance: average of all finding relevance scores (0.0–1.0)

**Pre-sprint research import:**
```go
// Import existing Pollard findings before starting sprint
state, err := orch.StartWithResearch(ctx, userInput, pollardFindings)
// Each finding → Intermute Insight → linked to sprint Spec
```

### PRD → Implementation Planning

Autarch PRD feeds into Compound's planning workflow:

```
Gurgeh PRD → /compound:deepen-plan → Enhanced implementation plan
                                              │
                                              ▼
                                      Coldwine task generation
```

### Review Pipeline

Multi-agent review at each stage:

```
Gurgeh PRD ────► Gurgeh reviewers ────► Approved PRD
                      │
                      ▼
              Compound review agents
                      │
                      ▼
Coldwine tasks ◄──── Quality code
```

### Recommended Workflow Chains

**Feature Development:**
```bash
/autarch:prd                    # Create PRD with interview
/compound:deepen-plan           # Enhance with research
/autarch:tasks                  # Generate epics/stories
/workflows:work                 # Execute implementation
/autarch:status                 # Monitor progress
```

**Research-Driven PRD:**
```bash
/autarch:research "topic"       # Gather intelligence
/autarch:prd --from-research    # Create PRD from insights
gurgeh review PRD-001 --gaps    # Validate completeness
/autarch:tasks                  # Generate tasks
```

---

## Claude Code Plugin

### Installation

The Autarch plugin (`autarch-plugin/`) provides:

| Component | Purpose |
|-----------|---------|
| Commands | `/autarch:prd`, `/autarch:research`, `/autarch:tasks`, `/autarch:status` |
| Agents | `arbiter` (PRD), `ranger` (research), `forger` (tasks) |
| Skills | `prd-interview`, `research-brief` |

### Agent → Tool Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                        AUTARCH AGENTS                           │
│  (Claude Code plugin - orchestrate user interactions)           │
├─────────────────────────────────────────────────────────────────┤
│  Arbiter ─────────► Gurgeh (PRD tool)                           │
│  Ranger ──────────► Pollard (research tool) ──► Hunters         │
│  Forger ──────────► Coldwine (task tool)                        │
└─────────────────────────────────────────────────────────────────┘

Agents decide WHAT to do. Tools execute HOW to do it.
```

**Ranger and Pollard:** Ranger is the orchestrating agent; Pollard is the research tool with hunters (github-scout, hackernews, openalex, etc.) as data sources. See [docs/pollard/HUNTERS.md](pollard/HUNTERS.md).
| MCP | `autarch-mcp` server for tool access |

### Configuration

```json
// .claude-plugin/plugin.json
{
  "name": "autarch",
  "mcp": {
    "servers": [{
      "name": "autarch",
      "command": "autarch-mcp",
      "args": ["--project", "."]
    }]
  }
}
```

### Commands

| Command | Description |
|---------|-------------|
| `/autarch:init` | Initialize Autarch in current project |
| `/autarch:prd [topic]` | Create PRD using interview framework |
| `/autarch:research [topic]` | Run Pollard research |
| `/autarch:tasks [PRD-ID]` | Generate epics/stories from PRD |
| `/autarch:status` | Show project status via Bigend |
| `/autarch:feature-to-ship` | End-to-end workflow |

### Skills

**prd-interview:** Structured interview for PRD creation
- Phase 1: Context (vision, problem, beneficiary)
- Phase 2: Requirements (must-haves, constraints)
- Phase 3: Success criteria (metrics, failure modes)
- Phase 4: Scope boundaries (goals, non-goals, assumptions)

**research-brief:** Research planning and hunter selection
- Topic analysis
- Hunter recommendations (github-scout, openalex, pubmed, context7, etc.)
- Research question generation
- Deliverable specification

---

## MCP Server

The Autarch MCP server (`pkg/mcp/`) exposes tools for AI agents:

### Tools

| Tool | Description |
|------|-------------|
| `autarch_list_prds` | List all PRD specs |
| `autarch_get_prd` | Get specific PRD details |
| `autarch_list_tasks` | List Coldwine tasks |
| `autarch_update_task` | Update task status |
| `autarch_research` | Run Pollard research scan |
| `autarch_suggest_hunters` | Get hunter recommendations |
| `autarch_project_status` | Get Bigend aggregation |
| `autarch_send_message` | Send via Intermute |

### Running

```bash
# Build
go build ./cmd/autarch-mcp

# Run
./autarch-mcp --project /path/to/project

# Or via MCP config
{
  "mcpServers": {
    "autarch": {
      "command": "autarch-mcp",
      "args": ["--project", "."]
    }
  }
}
```

---

## Knowledge Capture Package

The `pkg/compound/` package provides programmatic access to knowledge capture:

```go
import "github.com/mistakeknot/autarch/pkg/compound"

// Capture a solution
solution := compound.Solution{
    Module:      "gurgeh",
    Date:        time.Now().Format("2006-01-02"),
    ProblemType: "validation_error",
    Component:   "prd_reviewers",
    Symptoms:    []string{"CUJ validation fails"},
    RootCause:   "Missing field check",
    Severity:    "medium",
    Tags:        []string{"cuj", "validation"},
}

body := `
## Problem Statement
CUJ validation was failing on valid specs...

## Solution
Added nil check for linked_requirements field...
`

err := compound.Capture(projectPath, solution, body)

// Search solutions
opts := compound.SearchOptions{
    Module: "gurgeh",
    Tags:   []string{"cuj"},
}
solutions, err := compound.Search(projectPath, opts)
```

---

## Testing Integration

### Unit Tests

```bash
# Test Gurgeh review agents
go test ./internal/gurgeh/review -v

# Test Pollard review agents
go test ./internal/pollard/review -v

# Test SpecFlow analyzer
go test ./internal/gurgeh/spec -v

# Test MCP server
go test ./pkg/mcp -v

# Test compound package
go test ./pkg/compound -v
```

### Integration Test

```bash
# 1. Build tools
go build ./cmd/...

# 2. Initialize project
./gurgeh init
./coldwine init
./pollard init

# 3. Create and review PRD
./gurgeh create -i  # Interactive interview
./gurgeh review PRD-001 --gaps

# 4. Generate tasks
./coldwine epic create --prd PRD-001

# 5. Check status
./bigend --tui
```

---

## Troubleshooting

### Review Agents Not Running

1. Check spec file exists: `ls .gurgeh/specs/`
2. Verify spec format: `gurgeh show PRD-001`
3. Run with verbose: `gurgeh review PRD-001 -v`

### SpecFlow Analysis Empty

1. Check PRD has requirements and CUJs
2. Verify acceptance criteria format
3. Run analyzer directly:
   ```go
   analyzer := spec.NewSpecFlowAnalyzer()
   result := analyzer.Analyze(spec)
   fmt.Printf("Gaps: %d, Coverage: %.1f%%\n", len(result.Gaps), result.Coverage*100)
   ```

### MCP Server Not Responding

1. Check server running: `ps aux | grep autarch-mcp`
2. Verify project path: `autarch-mcp --project . --help`
3. Test with stdin:
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | ./autarch-mcp --project .
   ```

### Solutions Not Found

1. Check directory exists: `ls docs/solutions/`
2. Verify YAML frontmatter format
3. Search with grep: `grep -r "keyword" docs/solutions/`

---

## Arbiter Spec Sprint

**New in Release:** Propose-first PRD creation workflow with integrated research scanning and confidence-driven refinement.

The Arbiter Spec Sprint is Gurgeh's new default mode for creating specifications. Instead of the interview-based approach, users now get AI-generated proposals that they can accept, edit, or replace.

### 6-Section Propose-First Flow

Press `n` (new sprint) in the Gurgeh TUI to start:

```
┌─────────────────────────────────────────────────────────────────┐
│                  ARBITER SPEC SPRINT                            │
│                   Propose-First PRD                             │
├─────────────────────────────────────────────────────────────────┤
│ 1. Problem          ────► AI drafts problem statement           │
│    (Propose)              User accepts / edits / proposes       │
│                                                                  │
│ 2. Users           ────► Ranger scans tech landscape            │
│    (Propose)              AI identifies user personas           │
│                           User reviews + edits                  │
│                                                                  │
│ 3. Features+Goals  ────► AI generates feature list             │
│    (Propose)              User reviews + edits                  │
│                                                                  │
│ 4. Scope+Assumptions ──► AI scopes boundaries                  │
│    (Propose)              User reviews + edits                  │
│                                                                  │
│ 5. CUJs            ────► AI generates critical user journeys   │
│    (Propose)              User reviews + edits                  │
│                                                                  │
│ 6. Acceptance Criteria  ► AI generates AC for each CUJ         │
│    (Propose)              User reviews + edits + finalizes      │
│                                                                  │
│                           ▼                                      │
│                    PRD COMPLETE                                 │
│            (Consistency checked, confidence scored)            │
└─────────────────────────────────────────────────────────────────┘
```

### Integration with Ranger (Quick Scan)

After the **Problem section** is accepted, the workflow automatically triggers a quick Ranger scan:

```
Problem Section ──► Consistency check pass?
    │
    ├─ No ──► Return to Problem editing
    │
    └─ Yes ──► [QUICK SCAN TRIGGERS]

               Ranger scans for:
               - Similar projects / patterns
               - Existing solutions in space
               - Tech landscape shifts
               - Relevant research / case studies

               ▼

               Insights feed into:
               - User Personas (section 2)
               - Feature generation (section 3)
               - Risk/assumption discovery (section 4)
```

**Key:** The quick scan runs automatically after Problem validation. No manual trigger needed.

### Consistency Engine and Confidence Scoring

At each proposal step, Arbiter validates and scores:

#### Consistency Checks

```
┌─────────────────────────────────────────────────────────────────┐
│ CONSISTENCY ENGINE (internal/gurgeh/consistency/)               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│ Problem-to-Users       Users align with problem scope?          │
│ Users-to-Features      Features address user needs?             │
│ Features-to-CUJs       CUJs demonstrate features?               │
│ CUJs-to-AC             AC validates each CUJ end-to-end?        │
│ AC-to-Scope            AC respects scope boundaries?            │
│                                                                  │
│ Result: Pass/Fail + reason                                      │
│ Failure blocks progression to next section                      │
└─────────────────────────────────────────────────────────────────┘
```

**Example Flow:**
```
1. User accepts Problem: "Unified task dashboard for distributed teams"
2. Arbiter proposes Users: Scrum master, dev, manager, observer
3. Consistency check: "Users align with problem scope?"
   └─ YES → continue to Features
   └─ NO  → highlight misalignment, suggest edits

4. User accepts Users
5. Arbiter proposes Features: [task creation, sprint view, reporting, notifications]
6. Consistency check: "Features address all user needs?"
   └─ YES → continue to Scope
   └─ NO  → suggest missing features
```

#### Confidence Scoring

Each proposal gets a 0.0–1.0 confidence score:

```go
// internal/gurgeh/confidence/scorer.go
Factors:
- Clarity: Proposal text is unambiguous (0.0-1.0)
- Completeness: All required fields populated (0.0-1.0)
- Coherence: Aligns with prior sections (0.0-1.0)
- Feasibility: Technically achievable, realistic scope (0.0-1.0)

Score = Average(Clarity, Completeness, Coherence, Feasibility)
```

**Display:**
```
Proposal: "Unified task dashboard for distributed teams"
Confidence: 0.87 (HIGH)
  ├─ Clarity: 0.92
  ├─ Completeness: 0.85
  ├─ Coherence: 0.83
  └─ Feasibility: 0.88
```

Low-confidence proposals get a warning but don't block progression. Users can accept, edit, or propose alternatives.

### Handoff Options

After the PRD is complete (all 6 sections finalized), users choose a next action:

```
┌─────────────────────────────────────────────────────────────────┐
│                    PRD COMPLETE                                 │
│                  (Confidence: 0.84)                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│ Option 1: RESEARCH                                              │
│           ├─ Full Pollard scan on problem domain               │
│           ├─ Generate landscape report                          │
│           └─ Feed insights back into PRD refinement             │
│                                                                  │
│ Option 2: TASKS                                                 │
│           ├─ Generate epics + stories in Coldwine              │
│           ├─ Map CUJs to acceptance criteria                    │
│           └─ Assign to agents / teams                           │
│                                                                  │
│ Option 3: EXPORT                                                │
│           ├─ Export as markdown                                 │
│           ├─ Export as JSON/YAML spec                           │
│           └─ Ready for external sharing                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**TUI Navigation:**
```
After section 6 accepted:
├─ Press 'R' to trigger Research (full Pollard scan)
├─ Press 'T' to generate Tasks (Coldwine integration)
├─ Press 'E' to Export (markdown / JSON)
└─ Press 'S' to Save & close
```

### Key Implementation Files

| File | Purpose |
|------|---------|
| `internal/gurgeh/arbiter/sprint.go` | Sprint state machine and orchestration |
| `internal/gurgeh/arbiter/proposer.go` | AI proposal generation for each section |
| `internal/gurgeh/consistency/validator.go` | Cross-section consistency checks |
| `internal/gurgeh/confidence/scorer.go` | Proposal confidence scoring (0.0-1.0) |
| `internal/gurgeh/arbiter/quick_scan.go` | Post-Problem Ranger integration |
| `internal/gurgeh/views/sprint_view.go` | TUI rendering of spec sprint |

### CLI Usage

```bash
# Start a new spec sprint (interactive TUI)
gurgeh sprint new

# Create from existing research
gurgeh sprint new --from-research insights.json

# Get sprint status
gurgeh sprint status PRD-001

# Export completed PRD
gurgeh sprint export PRD-001 --format markdown
gurgeh sprint export PRD-001 --format json
```

### Workflow Timing

Typical spec sprint duration:

| Section | Typical Time | Notes |
|---------|--------------|-------|
| Problem | 2-5 min | Refine AI proposal |
| Users (+ quick scan) | 5-10 min | Ranger runs in background |
| Features+Goals | 3-5 min | User adds / removes |
| Scope+Assumptions | 2-3 min | Boundary setting |
| CUJs | 3-5 min | Critical journeys |
| Acceptance Criteria | 5-10 min | Detailed validation |
| **Total** | **20-40 min** | Depends on domain complexity |

---

### Integration with Compound Patterns

**Arbiter Spec Sprint integrates with Compound Engineering via:**

1. **Multi-Agent Review** (post-sprint):
   ```bash
   gurgeh review PRD-001 --gaps  # Includes SpecFlow analysis
   ```
   Runs: Completeness, CUJ Consistency, Acceptance Criteria, Scope Creep reviewers

2. **Knowledge Compounding**:
   - Proposals and user edits are logged to improve future models
   - Low-confidence sections tracked for pattern analysis

3. **Agent-Native Architecture**:
   - All TUI actions available as CLI commands
   - MCP server exposes `autarch_create_prd_sprint` and related tools

---
