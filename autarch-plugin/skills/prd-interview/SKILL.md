---
name: prd-interview
description: This skill guides PRD generation through structured interviews using Gurgeh's framework.
---

# PRD Interview Skill

## When to Use

Use this skill when:
- Starting a new feature
- Formalizing vague requirements
- Creating specifications for implementation
- User says "create a PRD", "write requirements", "spec this out"

## Interview Framework

You are conducting a structured interview to gather requirements. Follow these phases in order.

### Phase 1: Context (2-3 questions)

**Goal**: Understand the big picture before diving into details.

Ask about:
1. **Vision**: "What outcome are you trying to achieve?"
2. **Problem**: "What specific problem does this solve?"
3. **Beneficiary**: "Who are the primary users?"

**Don't skip this phase.** Even if the user provides details upfront, confirm you understand the context.

### Phase 2: Requirements (3-5 questions)

**Goal**: Gather specific, actionable requirements.

Ask about:
1. **Must-haves**: "What are the non-negotiables?"
2. **Nice-to-haves**: "What would be great but not essential for v1?"
3. **Constraints**: "What technical or business limits exist?"
4. **Integration**: "What existing systems are involved?"

**Probe for specifics.** If answers are vague, ask follow-up questions.

### Phase 3: Success Criteria (2-3 questions)

**Goal**: Define measurable outcomes.

Ask about:
1. **Metrics**: "How will you measure success?"
2. **Failure modes**: "What would make this a failure?"
3. **User behavior**: "What user actions indicate success?"

**Insist on measurability.** "Users will be happy" is not measurable.

### Phase 4: Scope Boundaries

**Goal**: Establish clear boundaries to prevent scope creep.

Ask about:
1. **Goals**: 2-3 measurable goals with metrics and targets
2. **Non-Goals**: What is explicitly out of scope and why
3. **Assumptions**: What foundational beliefs does this rely on

## Question Style

- Ask one question at a time
- Wait for the answer before moving on
- Use follow-up questions to clarify vague answers
- Summarize before moving to the next phase

## Output

After gathering all information, produce a PRD:

1. Write to `.gurgeh/specs/PRD-{id}.yaml`
2. Run `gurgeh validate PRD-{id}`
3. Report any issues
4. Suggest next steps

## Example Conversation

```
Assistant: I'll help you create a PRD. Let's start with the big picture.
What outcome are you trying to achieve with this feature?

User: I want users to be able to log in.