---
module: {gurgeh|coldwine|pollard|bigend|integration}
date: YYYY-MM-DD
problem_type: {validation_error|integration_issue|performance|config|ui_bug|data_corruption|concurrency}
component: {specific_component}
symptoms:
  - "Symptom 1"
  - "Symptom 2"
root_cause: "Technical explanation of why this happened"
severity: {low|medium|high|critical}
tags: [tag1, tag2, tag3]
---

# {Problem Title}

## Problem Statement

[Clear description of what went wrong, including any error messages or unexpected behavior]

## Investigation

[What was tried during debugging, what failed, what led to the root cause]

## Root Cause

[Technical explanation of the underlying issue]

## Solution

[Code/config changes that fixed it, with before/after examples]

```go
// BEFORE (incorrect):
// ...

// AFTER (correct):
// ...
```

## Files Changed

- `path/to/file1.go` (lines X-Y)
- `path/to/file2.go` (lines A-B)

## Prevention

### Detection - Catch Early
- [How to detect this issue before it becomes a problem]

### Best Practices
- [Guidelines to follow to avoid this issue]

### Testing
- [Test cases that would catch this]

## Key Insight

[The one thing to remember about this issue]

## Related

- [Link to related documentation]
- [Link to related solutions]
