---
name: autarch:status
description: Get Bigend project status aggregation
argument-hint: "[--detailed]"
---

# Project Status

Get an aggregated view of project status across all Autarch tools using Bigend.

## Usage

```bash
# Quick status overview
/autarch:status

# Detailed breakdown
/autarch:status --detailed
```

## What's Included

### PRD Status (Gurgeh)
- Total PRDs by status (draft, review, approved, implemented)
- PRDs pending review
- PRDs with validation issues

### Task Status (Coldwine)
- Total tasks by status (pending, in_progress, blocked, completed)
- Blocked tasks with reasons
- Task velocity (if historical data available)

### Research Status (Pollard)
- Total insights collected
- Recent research briefs
- Pending research requests

### Health Checks
- Tool initialization status
- Intermute connectivity
- Data consistency

## Steps

1. Check which Autarch tools are initialized
2. For each tool, gather status:
   - Count files by status
   - Identify blockers or issues
   - Calculate metrics
3. Aggregate into unified view
4. Display summary (or detailed if requested)

## Output Example

```
Autarch Project Status
======================

PRDs:      3 total (1 draft, 1 review, 1 approved)
Tasks:     12 total (3 pending, 5 in_progress, 1 blocked, 3 completed)
Research:  8 insights

Blockers:
  - TASK-007: Waiting for API credentials

Recent Activity:
  - PRD-003 created (2 hours ago)
  - TASK-009 completed (4 hours ago)
```

## Detailed Mode

With `--detailed`, includes:
- Full PRD list with titles
- All blocked items with reasons
- Research insight summaries
- Inter-tool message queue status
