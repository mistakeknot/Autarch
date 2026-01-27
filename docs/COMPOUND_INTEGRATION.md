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
