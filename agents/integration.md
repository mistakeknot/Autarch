# Integration Overview

**Key Integrations:**
- Gurgeh → Coldwine: PRDs generate tasks (via Briefs or direct import)
- Gurgeh → Pollard: Research enriches PRDs
- Coldwine → Pollard: Research informs implementation
- Coldwine → Clavain/Intercore: Sprint lifecycle, phase gates, dispatch tracking
- Bigend → All: Read-only aggregation
- Intermute: Cross-tool messaging and coordination

## Brief vs Task (Concept Clarification)

| Aspect | Gurgeh Brief | Coldwine Task |
|--------|--------------|---------------|
| **Purpose** | Agent instruction | Orchestration tracking |
| **Fields** | 3 (title, outcome, criteria) | 12+ (status, assignee, worktree, session...) |
| **Storage** | Markdown in `.gurgeh/briefs/` | SQLite in `.coldwine/` |
| **Lifecycle** | Stateless (create → execute → discard) | Stateful (pending → in_progress → completed) |

**Mental model:**
> **Brief** = "What Claude Code should do" (instruction)
> **Task** = "How to track what Claude Code is doing" (orchestration state)
