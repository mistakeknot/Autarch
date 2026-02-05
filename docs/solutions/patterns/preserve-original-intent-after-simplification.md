---
title: "Preserve Original Intent After Plan Simplification"
category: patterns
tags: [planning, yagni, workflows, skills, documentation]
module: workflows
symptom: "Valuable research and vision lost when plans are simplified"
root_cause: "No standard process for preserving deferred context during YAGNI simplification"
---

# Preserve Original Intent After Plan Simplification

## Problem

When reviewers recommend simplifying a plan (YAGNI, over-engineering, premature abstraction), the research and vision that informed the original design is often lost. This creates problems when:

1. The v1 limitations emerge and developers don't know what was already considered
2. Future iterations reinvent wheels that were already designed
3. The "expansion triggers" (when to add complexity) are not documented

## Solution

Created a global Claude Code skill at `~/.claude/skills/preserve-original-intent/SKILL.md` that:

1. Triggers when plans are being simplified significantly
2. Captures the cut content in an "Original Intent" section
3. Creates an "Expansion Path" table with trigger→feature mappings
4. Keeps the simplified v1 implementation as the primary content

### Key Pattern: Trigger-Based Expansion

Instead of deleting research, convert it to a trigger table:

```markdown
### Expansion Path

| Trigger | Add |
|---------|-----|
| Need typed access to evidence | Define `ExplorationResult` struct |
| Adding caching | Security sanitization filter |
| Exploration takes > 30s | Streaming with progress callbacks |
```

This tells future developers WHEN to add complexity, not just WHAT to add.

## Example

**Before simplification:** 530-line plan with interfaces, abstractions, 4 phases

**After with preserved intent:** 144-line plan with:
- 25 lines of v1 code (front and center)
- "Original Intent" section with architecture diagram, key decisions, research insights
- "Expansion Path" table with 5 trigger→feature mappings

## Files

- Skill: `~/.claude/skills/preserve-original-intent/SKILL.md`
- Example plan: `docs/plans/2026-02-04-feat-iterative-codebase-exploration-plan.md`

## Integration

The skill is designed to work with `/plan_review`:

1. Reviewers recommend simplification
2. Accept the simplification
3. Skill triggers to preserve the cut content
4. Result: minimal v1 + preserved vision

## Lesson

**Ship minimal, preserve context.** The simplest code that works is the best code, but the research that led to the original design is valuable institutional knowledge. Don't delete it—restructure it into an "Original Intent" section with clear expansion triggers.
