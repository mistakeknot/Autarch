---
name: autarch:workflows:feature-to-ship
description: End-to-end workflow from feature idea to shippable code
argument-hint: "<feature description>"
---

# Feature to Ship Workflow

Complete end-to-end workflow that takes a feature idea through PRD generation, research, task creation, and implementation planning.

## Usage

```bash
/autarch:workflows:feature-to-ship "Add user authentication with OAuth"
```

## Workflow Stages

### Stage 1: Brainstorming
1. Use `/compound:workflows:brainstorm` if available
2. Explore requirements through conversation
3. Capture ideas in `docs/brainstorms/`

### Stage 2: Research (Optional)
1. Run `/autarch:research` on key technical decisions
2. Gather competitive landscape if relevant
3. Document findings for PRD reference

### Stage 3: PRD Generation
1. Run `/autarch:prd` with brainstorm context
2. Complete interview phases (Context, Requirements, Success)
3. Define Goals, Non-Goals, Assumptions
4. Run multi-agent review
5. Address any gaps identified

### Stage 4: PRD Approval
1. Present PRD summary for review
2. Run `/gurgeh approve PRD-{id}` when ready
3. Verify all blockers resolved

### Stage 5: Task Generation
1. Run `/autarch:tasks create --from-prd PRD-{id}`
2. Review generated epics and stories
3. Adjust task breakdown if needed

### Stage 6: Implementation Planning
1. Use `/compound:workflows:plan` for detailed implementation plan
2. Create `docs/plans/` with step-by-step instructions
3. Link tasks to plan sections

### Stage 7: Execution Handoff
1. Present completed artifacts:
   - PRD at `.gurgeh/specs/PRD-{id}.yaml`
   - Tasks at `.coldwine/tasks/`
   - Plan at `docs/plans/`
2. Suggest execution approach:
   - Subagent-driven (current session)
   - Parallel session with `/compound:workflows:work`

## Checkpoints

The workflow pauses for user input at:
- After brainstorming (confirm direction)
- After PRD draft (review before approval)
- After task generation (adjust breakdown)
- Before execution (choose approach)

## Integration with Compound Engineering

This workflow chains with Compound Engineering patterns:
- `/autarch:prd` → `/compound:deepen-plan` → Enhanced implementation
- Task execution → `/compound:workflows:review` → Quality code
- Problem resolution → `/compound:workflows:compound` → Documented learning
