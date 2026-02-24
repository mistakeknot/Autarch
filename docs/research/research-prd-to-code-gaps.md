# Research: PRD-to-Code Implementation Gaps

**Date:** 2026-02-08  
**Purpose:** Identify what information coding agents need that isn't captured in Gurgeh specs

## Executive Summary

Gurgeh specs excel at **product-level requirements** (what to build, why, for whom) but systematically lack **implementation-level context** that coding agents need. The three critical gaps are:

1. **Technical Architecture & Design Decisions** - Specs don't capture how to build the feature (patterns, modules, dependencies)
2. **Existing Codebase Context** - Briefs lack file paths, module structure, and relevant existing code
3. **Implementation Constraints** - No tech stack, API contracts, or non-functional requirements beyond generic "performance < 200ms"

The architecture strategist exists but isn't wired into the export flow. Evidence from exploration scans gets captured but doesn't flow through to briefs.

---

## 1. Current Brief Format (What Agents Get)

**Source:** `internal/gurgeh/brief/decompose.go`

```go
type Brief struct {
    Title    string   // What to build
    Outcome  string   // What success looks like
    Criteria []string // How to verify it's done
}
```

**What's in the export prompt:**
- Spec summary, requirements (bullet points)
- CUJs (user journeys with steps)
- Acceptance criteria (testable outcomes)

**What's missing:**
- File paths or module names
- Architecture patterns or tech stack
- Existing code references
- Dependencies between features

**Example brief content:**
```markdown
# Add User Authentication

## Outcome
Users can create accounts and log in securely

## Acceptance Criteria
- [ ] Users can register with email/password
- [ ] Users can log in and receive a session token
- [ ] Passwords are hashed with bcrypt
```

**Agent's problem:** Where does this go? What framework? How does it integrate with existing auth patterns?

---

## 2. Spec Schema (What Gets Captured)

**Source:** `internal/gurgeh/specs/schema.go`

### Present in Spec:
- ✅ **Goals** (ID, description, metric, target)
- ✅ **Requirements** (Given/When/Then with constraints)
- ✅ **Assumptions** (what we're betting on, confidence decay)
- ✅ **Hypotheses** (falsifiable predictions with metrics)
- ✅ **CUJs** (user journeys with steps, priority, success criteria)
- ✅ **Acceptance Criteria** (ID + description)
- ✅ **FilesToModify** (action, path, description)

### FilesToModify Field:
```go
type FileChange struct {
    Action      string // "create", "modify", "delete"
    Path        string // e.g., "internal/auth/handler.go"
    Description string // What to change
}
```

**Status:** Present in schema, but:
1. **Not populated during spec creation** - Arbiter/exploration doesn't generate this
2. **Not surfaced in briefs** - `buildPrompt()` doesn't include FilesToModify
3. **No validation** - Nothing checks if paths exist or are accurate

### Missing from Spec:
- ❌ **Architecture patterns** (monolith, microservices, layered)
- ❌ **Tech stack** (frameworks, libraries, databases)
- ❌ **Module boundaries** (package structure, dependencies)
- ❌ **API contracts** (REST endpoints, GraphQL schema, gRPC services)
- ❌ **Data models** (database schema, entity relationships)
- ❌ **Existing code references** (relevant files, functions, patterns to follow)

---

## 3. Coldwine Task Schema (What Orchestration Tracks)

**Source:** `internal/coldwine/storage/task.go`

```go
type Task struct {
    ID     string
    Title  string
    Status string
}
```

**Gap analysis:** Coldwine is a minimal SQLite wrapper. It doesn't:
- Track which files are being worked on
- Link tasks to spec requirements
- Capture technical decisions or blockers
- Store implementation artifacts (branch names, PRs, test results)

**Implication:** The orchestration layer can't inform the spec layer about implementation reality. No feedback loop from "code in progress" to "spec validity."

---

## 4. Exploration & Scan Artifacts (Codebase Context Capture)

**Sources:**
- `internal/gurgeh/exploration/explore.go` - Runs Claude Code to explore codebase
- `internal/gurgeh/arbiter/scan/artifacts.go` - Structured scan results

### What Exploration Captures:
```go
type PhaseData struct {
    Summary           string          // Human-readable summary
    Evidence          []EvidenceItem  // Code quotes with file paths
    ResolvedQuestions []ResolvedQuestion
    Quality           QualityScores
}

type EvidenceItem struct {
    Type       string  // "readme", "package", "code"
    FilePath   string  // Actual file path
    Quote      string  // Verbatim code quote
    Confidence float64
}
```

**Exploration prompt extracts:**
- Project name, vision, problem
- Features, CUJs, requirements (from existing code/docs)
- **Tech stack** (frameworks, architecture patterns)
- **Risks** (technical challenges)

**Example evidence:**
```json
{
  "tech": {
    "summary": "Go monolith using Bubble Tea for TUI",
    "evidence": [
      {"quote": "tea.Program", "source": "internal/tui/model.go:45"}
    ]
  }
}
```

### Gap Between Exploration and Briefs:
1. **Evidence is captured but not exported** - `buildPrompt()` in decompose.go doesn't include scan artifacts
2. **File paths exist but aren't surfaced** - EvidenceItem.FilePath never reaches briefs
3. **Tech stack identified but discarded** - Exploration's "tech" section doesn't persist in Spec schema

---

## 5. Architecture Strategist (Exists But Disconnected)

**Source:** `internal/gurgeh/architecture/strategist.go`

### What It Analyzes:
```go
type ArchitectureStrategy struct {
    Pattern          ArchitecturePattern  // monolith, microservices, serverless, event-driven
    DataStrategy     DataStrategy         // relational, document, graph, time-series
    CachingApproach  CachingApproach      // none, distributed, CDN, multi-tier
    APIStyle         APIStyle             // REST, GraphQL, gRPC
    ScalingStrategy  ScalingStrategy      // vertical, horizontal, auto
    Recommendations  []ArchitectureRecommendation
    Technologies     []TechnologySuggestion
}
```

### Example Recommendations:
- "Start with a monolith, split later"
- "Use PostgreSQL for ACID guarantees"
- "REST API for universal client support"
- "Redis for distributed caching"

### Gap:
- **Not called during spec creation** - Arbiter doesn't invoke strategist
- **Not stored in Spec** - No field for ArchitectureStrategy
- **Not exported to briefs** - Recommendations never reach agents

**Implication:** High-quality architectural analysis exists but is siloed. Agents get "build user auth" without "use JWT tokens with Redis session store because we're doing REST + distributed caching."

---

## 6. Generator Context (What LLM Sees During Spec Creation)

**Source:** `internal/gurgeh/arbiter/generator.go`

### ProjectContext Fed to LLM:
```go
type ProjectContext struct {
    HasReadme      bool
    ReadmeSnippet  string
    HasPackageJSON bool
    PackageName    string
    Dependencies   []string
    MainFiles      []string
}
```

### Evidence Injection:
The generator does format scan evidence for prompt context:
```go
func formatEvidenceContext(pd *scan.PhaseData) string {
    // Renders Evidence quotes as <evidence> blocks
    // Renders ResolvedQuestions as Q&A pairs
}
```

**Gap:** This evidence enriches the spec content (narrative sections), but doesn't populate structured fields like FilesToModify or TechnicalDesign.

---

## 7. What Coding Agents Actually Need

Based on analysis of the Brief → Claude Code flow and typical agent requirements:

### Tier 1: Critical (Agents fail without this)
1. **File paths for entry points** - "Modify `internal/auth/handler.go`" not "add authentication"
2. **Module/package structure** - Where does new code live? What's the import path?
3. **Existing patterns to follow** - "Auth uses JWT middleware in `pkg/auth/jwt.go`"
4. **Tech stack constraints** - "Use Bubble Tea for TUI, lipgloss for styling"

### Tier 2: High Value (Agents guess poorly without this)
5. **Architecture pattern** - Monolith vs microservices affects where code goes
6. **API contracts** - REST endpoint signatures, GraphQL schema, gRPC protos
7. **Data models** - Database schema, entity relationships
8. **Non-functional requirements** - Latency budgets, security requirements, error handling

### Tier 3: Nice to Have (Improves quality)
9. **Similar implementations** - "See how task creation works in `internal/coldwine/`"
10. **Testing patterns** - What test frameworks, where do tests live
11. **Dependency constraints** - Can I add new libraries? Which ones are approved?
12. **Architectural decision records** - Why did we choose this approach?

---

## 8. Comparison: What Flows vs What's Needed

| Information Type | Captured in Spec? | Exported to Brief? | Agent Needs It? |
|-----------------|-------------------|-------------------|----------------|
| **Product Requirements** | ✅ Yes | ✅ Yes | ✅ Yes |
| Requirements (Given/When/Then) | ✅ Yes | ✅ Yes | ✅ Yes |
| Critical User Journeys | ✅ Yes | ✅ Yes | ✅ Yes |
| Acceptance Criteria | ✅ Yes | ✅ Yes | ✅ Yes |
| Goals & Hypotheses | ✅ Yes | ✅ Yes | 🟡 Sometimes |
| **Technical Context** |  |  |  |
| FilesToModify | ✅ Schema only | ❌ No | ✅ Critical |
| Architecture Pattern | ❌ No (strategist exists) | ❌ No | ✅ Critical |
| Tech Stack | 🟡 Exploration only | ❌ No | ✅ Critical |
| Module Structure | 🟡 Evidence only | ❌ No | ✅ Critical |
| Existing Code References | 🟡 Evidence only | ❌ No | ✅ High Value |
| API Contracts | ❌ No | ❌ No | ✅ High Value |
| Data Models | ❌ No | ❌ No | ✅ High Value |
| NFRs (detailed) | 🟡 Constraints only | 🟡 Generic | ✅ High Value |
| Testing Patterns | ❌ No | ❌ No | 🟡 Nice to Have |
| Similar Code Examples | 🟡 Evidence only | ❌ No | 🟡 Nice to Have |

**Legend:**
- ✅ Yes: Captured/exported/needed
- 🟡 Partial: Exists somewhere but not consistently
- ❌ No: Missing entirely

---

## 9. Root Cause Analysis

### Why These Gaps Exist:

1. **Spec is Product-Focused, Not Implementation-Focused**
   - Gurgeh's design goal: Generate PRDs (what to build, why, for whom)
   - PRDs traditionally don't include technical design
   - Separation of concerns: PM writes PRD, engineers design implementation

2. **Exploration Evidence Gets Discarded**
   - Scan captures file paths, tech stack, code patterns
   - Evidence enriches LLM prompts during spec creation
   - But evidence isn't structured into exportable fields
   - Brief export ignores scan artifacts entirely

3. **Architecture Strategist Is Decoupled**
   - Strategist exists as a separate analysis tool
   - Not integrated into Arbiter workflow
   - Recommendations never persist in Spec schema
   - No path from strategist output to brief export

4. **FilesToModify Exists But Isn't Populated**
   - Schema has the field (FileChange with action, path, description)
   - No code populates it during spec creation
   - Arbiter would need to generate file change plans
   - Requires deeper codebase analysis than current scan provides

5. **Brief Format Is Minimal By Design**
   - 3 fields: Title, Outcome, Criteria
   - Designed for human Claude Code users ("read the brief, figure out the details")
   - Assumes agent will explore codebase independently
   - No structured technical context

---

## 10. Impact on Agent Success

### Scenario: "Add User Authentication" Brief

**What agent gets:**
```markdown
# Add User Authentication

## Outcome
Users can create accounts and log in securely

## Acceptance Criteria
- [ ] Email/password registration
- [ ] JWT token-based sessions
- [ ] bcrypt password hashing
```

**What agent needs to guess:**
1. Where to create auth handler? (`internal/auth/`? `pkg/auth/`? `cmd/api/handlers/`?)
2. What framework? (Standard lib? Gin? Echo? Chi?)
3. Where's the database code? (GORM? sqlx? raw SQL?)
4. How do existing endpoints work? (Middleware pattern? Manual checks?)
5. What's the user model? (Existing struct? Database schema?)
6. Where do tests live? (`*_test.go` alongside? Separate `tests/` dir?)

**Failure modes:**
- ❌ Creates code in wrong package structure
- ❌ Doesn't follow existing auth patterns (if any)
- ❌ Reinvents database abstractions
- ❌ Mismatches API style (adds GraphQL to REST app)
- ❌ Skips tests because test patterns unclear

**Workaround:** Agent runs exploration phase every time, duplicating work already done in spec creation.

---

## 11. Recommendations

### Short-Term Fixes (High Impact, Low Effort)

1. **Include Scan Evidence in Briefs**
   - Modify `buildPrompt()` in `internal/gurgeh/brief/decompose.go`
   - Append "## Technical Context" section with:
     - File paths from Evidence items
     - Tech stack summary from exploration
     - Relevant code patterns
   - Example:
     ```markdown
     ## Technical Context
     **Tech Stack:** Go, Bubble Tea (TUI), SQLite
     **Relevant Files:**
     - `internal/coldwine/storage/task.go` (similar CRUD pattern)
     - `pkg/tui/model.go` (TUI integration example)
     ```

2. **Wire Architecture Strategist into Arbiter**
   - Call `Strategist.Strategize()` after exploration phase
   - Store recommendations in SprintState
   - Export to Spec (add `TechnicalStrategy` field)
   - Include in brief export

3. **Populate FilesToModify During Spec Creation**
   - Arbiter's final review phase generates file change plan
   - Use exploration evidence to identify relevant modules
   - Store in Spec.FilesToModify
   - Export as "## Implementation Plan" in briefs

### Medium-Term Improvements

4. **Add TechnicalDesign Section to Spec Schema**
   ```go
   type TechnicalDesign struct {
       ArchitecturePattern string          // from strategist
       TechStack           []string        // from exploration
       ModuleStructure     []ModuleRef     // package paths
       APIContracts        []APIEndpoint   // REST/GraphQL/gRPC
       DataModels          []Entity        // DB schema
       ImplementationNotes string          // free-form guidance
   }
   ```

5. **Enhance Evidence Capture**
   - Exploration prompt extracts more structured data
   - Separate evidence types: code patterns, file structure, APIs, data models
   - Store evidence in Spec.TechnicalEvidence field

6. **Add "Technical Context" Phase to Arbiter**
   - New phase after AcceptanceCriteria
   - Generates implementation guidance using exploration + strategist
   - User reviews/edits technical recommendations
   - Persists as structured TechnicalDesign

### Long-Term Vision

7. **Bidirectional Spec-Code Sync**
   - Coldwine tracks which files implement which specs
   - Git commits linked to spec IDs
   - Spec shows "implemented in X, Y, Z" with links
   - Agents can navigate from spec to existing implementation

8. **Codebase Diff Awareness**
   - Exploration compares current code to spec FilesToModify
   - Detects drift: "Spec says auth in pkg/auth, but code is in internal/auth"
   - Suggests spec updates or code refactoring

9. **Agent-Facing API**
   - Specs expose JSON API: `/specs/{id}/technical-context`
   - Returns file paths, patterns, tech stack, examples
   - Agents query API instead of parsing markdown

---

## 12. Appendix: Code References

### Key Files Analyzed:
- **Brief export:** `internal/gurgeh/brief/decompose.go` (lines 91-149)
- **Spec schema:** `internal/gurgeh/specs/schema.go` (lines 1-178)
- **Coldwine tasks:** `internal/coldwine/storage/task.go` (lines 5-28)
- **Scan artifacts:** `internal/gurgeh/arbiter/scan/artifacts.go` (lines 9-77)
- **Exploration:** `internal/gurgeh/exploration/explore.go` (lines 18-746)
- **Strategist:** `internal/gurgeh/architecture/strategist.go` (lines 1-708)
- **Generator context:** `internal/gurgeh/arbiter/generator.go` (lines 13-293)
- **Sprint state:** `internal/gurgeh/arbiter/types.go` (lines 119-141)

### FilesToModify Field Usage:
- **Schema definition:** `specs/schema.go:143`
- **PRD feature field:** `specs/prd.go:65`
- **Migration:** `specs/prd.go:176` (spec → feature)
- **Not populated by:** Arbiter, exploration, or any generator
- **Not exported to:** Brief format

### Evidence Flow:
1. **Capture:** `exploration/explore.go` calls Claude Code with codebase scan prompt
2. **Structure:** `arbiter/scan/artifacts.go` PhaseData holds Evidence items with file paths
3. **LLM injection:** `arbiter/generator.go:98-128` formats evidence for prompt context
4. **Storage:** Evidence persists in SprintState.ScanArtifacts
5. **Export:** `arbiter/export.go` converts SprintState → Spec (evidence lost)
6. **Brief:** `brief/decompose.go` converts Spec → Brief (no evidence)

---

## Conclusion

Gurgeh captures rich technical context during spec creation (exploration scans, architecture analysis) but systematically discards it before briefs reach coding agents. The three critical gaps are:

1. **FilesToModify exists in schema but is never populated** - No code generates file change plans
2. **Exploration evidence (file paths, tech stack, patterns) doesn't persist** - Lost between scan → spec → brief
3. **Architecture strategist recommendations exist but aren't wired in** - High-quality analysis goes unused

**Minimal viable fix:** Modify `brief/decompose.go` to include scan evidence and strategist recommendations in exported briefs. This requires no schema changes, just plumbing existing data through the export pipeline.

**Full solution:** Add TechnicalDesign section to Spec schema, populate it from exploration + strategist during Arbiter flow, export it as structured context in briefs. Enables agents to implement features without re-exploring the codebase.
