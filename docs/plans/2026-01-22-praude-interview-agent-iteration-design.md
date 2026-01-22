# Praude Interview Iteration + New PRD Flow Design

## Goal
Make `n` create a new PRD and immediately start the interview for that PRD. Make `g` re-interview the selected PRD, preserving existing content. Within each interview step, the user can iterate with the agent (turn-by-turn) to refine the draft before moving on.

## User Flow
- `n`: create template PRD, select it, enter interview mode targeting that PRD.
- `g`: enter interview mode for the selected PRD. If none selected, show a status error.
- In interview: `enter` iterates on the current step by calling the agent. `[`/`]` move between steps. Enter does not advance steps.

## Interview Iteration
Each step maintains:
- `answer`: user-provided text buffer.
- `draft`: latest agent-generated text for the step.

Pressing Enter submits the current answer and context to the agent. The agent returns a revised draft for the step. The draft is displayed and can be refined by editing the answer and pressing Enter again. Navigation restores the per-step answer and draft when moving between steps.

## Agent Integration
Interview iteration uses a real agent call. We write a step brief (under `.praude/briefs/`) containing:
- step question
- current PRD context (title, summary, requirements)
- current answer and prior draft
- required output format

We run the agent synchronously and parse the returned draft from stdout. If the agent cannot run or returns malformed output, keep the previous draft and show a status message.

## Spec Updates
Finalizing the interview writes back to the same PRD file. We merge interview-owned fields (Title, Summary, Requirements, UserStory, StrategicContext, CUJ) into the existing spec while preserving non-interview fields (market, competitive, research, metadata). For re-interview, missing fields are filled; existing fields are overwritten only when the user iterates a step.

## Testing
- `n` creates a PRD and enters interview mode targeting that PRD.
- `g` re-interviews selected PRD and prefills answers.
- Enter triggers agent iteration and updates draft.
- `[`/`]` move steps without losing answer/draft.
- Finalize preserves non-interview fields.
