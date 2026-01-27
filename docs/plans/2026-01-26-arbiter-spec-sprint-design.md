# Arbiter Spec Sprint Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this design.

**Goal:** Create a 10-minute guided workflow for solo vibecoders to transform messy ideas into validated PRDs that AI coding agents can execute effectively.

**Core Thesis:** Coding agents will only get better; the real bottleneck is having great product sense, taste, and strategy. Autarch IS the planning phase that human developers use but vibecoders skip.

**Target User:** Solo founder/hacker building with AI coding tools who hit the "prompt and pray" wall at ~5,000 lines when AI loses context.

---

## Design Overview

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        ARBITER SPEC SPRINT (~10 min)                         │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   [1. OPEN] ──► [2. PROBLEM] ──► [3. USERS] ──► [4. FEATURES+GOALS]         │
│       │              │               │                  │                    │
│       │              │               │                  │                    │
│       │              ▼               │                  │                    │
│       │        ┌──────────┐          │                  │                    │
│       │        │  RANGER  │          │                  │                    │
│       │        │  QUICK   │──────────┼──────────────────┘                    │
│       │        │  SCAN    │          │                                       │
│       │        │ (30 sec) │          │                                       │
│       │        └──────────┘          │                                       │
│       │                              │                                       │
│       ▼                              ▼                                       │
│   [5. SCOPE+ASSUMPTIONS] ◄───────────┤                                       │
│            │                         │                                       │
│            ▼                         │                                       │
│        [6. CUJs] ◄───────────────────┤                                       │
│            │                         │                                       │
│            ▼                         │                                       │
│   [7. ACCEPTANCE CRITERIA] ◄─────────┘                                       │
│            │                                                                 │
│            ▼                                                                 │
│   ┌────────────────────────────────────────┐                                │
│   │         CONSISTENCY ENGINE             │                                │
│   │  • User-Feature Mismatch               │                                │
│   │  • Goal-Feature Gap                    │                                │
│   │  • Scope Creep Detection               │                                │
│   │  • Assumption Conflicts                │                                │
│   │                                        │                                │
│   │  🔴 Blockers → Must resolve            │                                │
│   │  🟡 Warnings → Can dismiss             │                                │
│   └────────────────────────────────────────┘                                │
│            │                                                                 │
│            ▼                                                                 │
│   ┌────────────────────────────────────────┐                                │
│   │         CONFIDENCE SCORE               │                                │
│   │                                        │                                │
│   │  Completeness ────────── 20%           │                                │
│   │  Consistency ─────────── 25%           │                                │
│   │  Specificity ─────────── 20%           │                                │
│   │  Research Validation ─── 20%           │                                │
│   │  Assumption Risk ──────── 15%          │                                │
│   │                                        │                                │
│   │  Running total: [██████████░░] 85%     │                                │
│   └────────────────────────────────────────┘                                │
│            │                                                                 │
│            ▼                                                                 │
│   ┌────────────────────────────────────────┐                                │
│   │            HANDOFF OPTIONS             │                                │
│   │                                        │                                │
│   │  1. Research & iterate (Recommended)   │                                │
│   │     → Deep dive with Ranger            │                                │
│   │     → Refine based on findings         │                                │
│   │                                        │                                │
│   │  2. Generate tasks → Coldwine          │                                │
│   │     → Epic/story breakdown             │                                │
│   │                                        │                                │
│   │  3. Export for coding agent            │                                │
│   │     → Markdown/YAML/JSON               │                                │
│   └────────────────────────────────────────┘                                │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Key Components

### 1. Opening

**For existing projects:**
- Arbiter reads project context (README, package.json, existing code)
- Generates initial draft based on inferred purpose

**For blank slate:**
- Single question: "Describe your idea"
- No constraints on length or format

### 2. Section-by-Section Flow

| Section | Arbiter Behavior | User Interaction |
|---------|------------------|------------------|
| **Problem** | Drafts problem statement from context/input | Select from options, edit, or ask Arbiter to revise |
| **Users** | Proposes user personas | Same |
| **Features + Goals** | Lists features with measurable goals | Same |
| **Scope + Assumptions** | Defines boundaries and foundational beliefs | Same |
| **CUJs** | Generates Critical User Journeys | Same |
| **Acceptance Criteria** | Creates testable criteria per CUJ | Same |

**Key behavior:** Arbiter flags if subsequent answers invalidate earlier sections.

### 3. Ranger Quick Scan (After Problem)

**Timing:** After Problem section is finalized, before Features
**Duration:** ~30 seconds
**Hunters:** github-scout + hackernews only (fast, no auth)

**Example output:**
```
📊 Quick Scan Results (30 sec)

Found 3 similar OSS projects:
• Bookwyrm (5.2k ★) - Federated reading tracker
• Hardcover (2.1k ★) - Modern Goodreads alternative
• Libib (closed source) - Library management

HN Discussion Themes:
• "People want momentum features, not just logging"
• "Goodreads import is table stakes"
• "Social features are overrated for solo readers"

→ Informing Features section...
```

### 4. Consistency Engine

| Check Type | Severity | Example |
|------------|----------|---------|
| **User-Feature Mismatch** | 🔴 Blocker | "Feature requires enterprise admin, but target is solo founders" |
| **Goal-Feature Gap** | 🟡 Warning | "Goal mentions 'fast onboarding' but no onboarding features listed" |
| **Scope Creep** | 🔴 Blocker | "Feature 'AI assistant' conflicts with non-goal 'no AI features'" |
| **Assumption Conflict** | 🟡 Warning | "Assumes 'users have accounts' but no signup feature" |

**Blockers (🔴):** Must be resolved before proceeding
**Warnings (🟡):** Can be dismissed with acknowledgment

### 5. Confidence Score

Running percentage displayed throughout:

```
┌─────────────────────────────────────────────────┐
│ PRD Confidence: 72%                             │
├─────────────────────────────────────────────────┤
│ Completeness      ████████░░░░░░░░░░░░  16/20  │
│ Consistency       ██████████████████░░  22/25  │
│ Specificity       ██████████░░░░░░░░░░  10/20  │
│ Research Valid.   ████████████░░░░░░░░  12/20  │
│ Assumption Risk   ████████████████████  12/15  │
└─────────────────────────────────────────────────┘
```

**Weights:**
- Completeness: 20% (all sections filled)
- Consistency: 25% (no conflicts)
- Specificity: 20% (measurable criteria)
- Research Validation: 20% (findings support decisions)
- Assumption Risk: 15% (assumptions are reasonable)

### 6. Handoff Options

After PRD completion, offer three paths:

1. **Research & iterate (Recommended for first PRD)**
   - Run Ranger deep scan (5-10 min)
   - Review findings with user
   - Update PRD based on research
   - Repeat until satisfied

2. **Generate tasks → Coldwine (Forger)**
   - Create epics from CUJs
   - Break into stories
   - Estimate complexity
   - Identify dependencies

3. **Export for coding agent**
   - Markdown (human-readable)
   - YAML (structured)
   - JSON (API-friendly)

---

## Interaction Model

**Propose-first, not ask-first:**

```
┌─ ARBITER ─────────────────────────────────────────────────────┐
│                                                               │
│ Based on your description, here's my draft for the Problem:   │
│                                                               │
│ ┌─────────────────────────────────────────────────────────┐   │
│ │ Solo vibecoders hit a wall at ~5,000 lines when AI      │   │
│ │ coding agents lose context. Without a planning phase,   │   │
│ │ they're stuck in "prompt and pray" mode, rebuilding     │   │
│ │ the same features repeatedly.                           │   │
│ └─────────────────────────────────────────────────────────┘   │
│                                                               │
│ Options:                                                      │
│ [A] Accept as-is                                              │
│ [B] "Make it more specific to AI context limits"              │
│ [C] "Focus on the business cost, not technical"               │
│ [D] Edit directly                                             │
│ [E] Tell me what to change                                    │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

---

## Time Budget

| Phase | Duration |
|-------|----------|
| Opening | 1 min |
| Problem | 1-2 min |
| Ranger Quick Scan | 0.5 min |
| Users | 1 min |
| Features + Goals | 2-3 min |
| Scope + Assumptions | 1-2 min |
| CUJs | 1-2 min |
| Acceptance Criteria | 1-2 min |
| **Total (without deep research)** | **~10 min** |
| **With Research & Iterate** | **+5-15 min** |

---

## Design Principles

1. **Propose, don't ask** - AI drafts, user reacts
2. **Full PRD, not simplified** - The value IS the complete thinking
3. **Section-by-section** - Manageable chunks, consistency checks between
4. **Research-informed** - Quick scan before features, deep dive available
5. **Confidence transparency** - Running score shows PRD strength
6. **Blocker-enforced quality** - Can't skip critical conflicts

---

## Files to Create/Modify

| File | Purpose |
|------|---------|
| `internal/gurgeh/arbiter/` | Core Arbiter agent logic |
| `internal/gurgeh/consistency/` | Consistency checking engine |
| `internal/gurgeh/confidence/` | Confidence scoring |
| `internal/pollard/quick_scan.go` | Ranger quick scan mode |
| `autarch-plugin/skills/spec-sprint/SKILL.md` | Claude Code skill |
| `autarch-plugin/agents/prd/arbiter.md` | Update with new flow |

---

## Success Metrics

**Aha moment:** "I articulated my messy idea into a clear spec fast, AND my AI coding agent actually built what I wanted"

**Leading indicators:**
- Time from idea → validated PRD < 15 min
- Consistency engine catches 80%+ of conflicts
- Users who research & iterate have 40% fewer implementation issues
- Confidence score correlates with implementation success

---

## Related Documents

- [COMPOUND_INTEGRATION.md](../COMPOUND_INTEGRATION.md) - Agent relationships
- [docs/pollard/HUNTERS.md](../pollard/HUNTERS.md) - Hunter reference
- [autarch-plugin/agents/prd/arbiter.md](../../autarch-plugin/agents/prd/arbiter.md) - Current Arbiter spec
- [autarch-plugin/agents/research/ranger.md](../../autarch-plugin/agents/research/ranger.md) - Ranger spec
