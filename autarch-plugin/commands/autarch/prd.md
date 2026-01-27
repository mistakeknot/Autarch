---
name: autarch:prd
description: Generate a PRD using Gurgeh's interview framework
argument-hint: "[feature description or PRD-ID]"
---

# Generate or Edit PRD

Create or edit a Product Requirements Document using Gurgeh's structured interview framework.

## Usage

```bash
# Create new PRD interactively
/autarch:prd

# Create PRD with initial description
/autarch:prd "User authentication with OAuth support"

# Edit existing PRD
/autarch:prd PRD-001
```

## New PRD Workflow

When creating a new PRD, use the prd-interview skill to guide the conversation:

### Phase 1: Context (2-3 questions)
- **Vision**: "What outcome are you trying to achieve?"
- **Problem**: "What problem does this solve?"
- **Beneficiary**: "Who benefits from this?"

### Phase 2: Requirements (3-5 questions)
- **Must-haves**: "What are the non-negotiables?"
- **Constraints**: "What are the technical/business limits?"
- **Integration**: "What existing systems are involved?"

### Phase 3: Success Criteria (2-3 questions)
- **Metrics**: "How will you measure success?"
- **Failure modes**: "What does failure look like?"

### Phase 4: Goals & Non-Goals
- **Goals**: Measurable outcomes with metrics and targets
- **Non-Goals**: Explicit scope boundaries
- **Assumptions**: Foundational beliefs the PRD relies on

## Steps

1. Check for existing brainstorm in `docs/brainstorms/`
2. If creating new PRD:
   - Run arbiter agent to gather requirements
   - Validate against Gurgeh schema
   - Write to `.gurgeh/specs/PRD-{id}.yaml`
3. If editing existing PRD:
   - Open in editor or present current content
   - Collect changes through conversation
   - Update and validate
4. Run multi-agent review to identify gaps
5. Auto-commit the PRD
6. Suggest next steps: `/autarch:research` or `/autarch:tasks`

## Output

PRD is saved to `.gurgeh/specs/PRD-{id}.yaml` with:
- Title and summary
- Goals, non-goals, assumptions
- Requirements and acceptance criteria
- Critical User Journeys
- Research references (if available)
