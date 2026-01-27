---
name: autarch:tasks
description: View and manage Coldwine tasks
argument-hint: "[list|create|assign|block] [args]"
---

# Task Management

View and manage tasks using Coldwine's task orchestration system.

## Usage

```bash
# List all tasks
/autarch:tasks

# List tasks by status
/autarch:tasks list --status pending
/autarch:tasks list --status blocked

# Create epic from PRD
/autarch:tasks create --from-prd PRD-001

# Assign task to agent
/autarch:tasks assign TASK-001 --agent claude

# Block a task
/autarch:tasks block TASK-001 --reason "Waiting for API access"

# Complete a task
/autarch:tasks complete TASK-001
```

## Task States

| Status | Description |
|--------|-------------|
| `pending` | Ready to be worked on |
| `in_progress` | Currently being worked on |
| `blocked` | Waiting on external dependency |
| `completed` | Work finished |

## Creating Tasks from PRD

When creating tasks from a PRD:
1. Each Critical User Journey becomes an epic
2. Each requirement becomes a story
3. Acceptance criteria become task verification steps
4. Dependencies are inferred from requirement ordering

## Steps

### For `list`:
1. Read tasks from `.coldwine/tasks/` or `.tandemonium/specs/`
2. Apply filters (status, assignee)
3. Display task summary with status indicators

### For `create --from-prd`:
1. Load the specified PRD
2. Generate epic structure from CUJs
3. Create story tasks from requirements
4. Write to task directory
5. Display created tasks

### For `assign`:
1. Load the specified task
2. Update assignee field
3. Set status to `in_progress` if pending
4. Save and confirm

### For `block`:
1. Load the specified task
2. Set status to `blocked`
3. Record block reason and timestamp
4. Save and confirm

## Integration with Intermute

Tasks can send messages to other Autarch tools:
- Notify Gurgeh when requirements need clarification
- Request Pollard research for blocked items
- Report status to Bigend for project overview
