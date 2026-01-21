---
status: pending
priority: p3
issue_id: "020"
tags: [database, data-integrity, code-review]
dependencies: []
---

# Add Database Schema Versioning

## Problem Statement

No version tracking or migration system for schema changes. Schema changes could fail silently on existing databases.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/storage/db.go` lines 23-40

```go
func Migrate(db *sql.DB) error {
    _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tasks (...)
// No version table, no migration history
`)
    return err
}
```

**Contrast:** tasks.yml has version tracking (version: 1, rev: 380), but SQLite does not.

**Impact:**
- No rollback for failed migrations
- No detection of schema drift
- Schema changes may fail silently

## Proposed Solutions

### Option 1: Add Version Table (Recommended)
- **Pros:** Track schema versions
- **Cons:** Manual migration management
- **Effort:** Small
- **Risk:** Low

```go
func Migrate(db *sql.DB) error {
    _, _ = db.Exec(`
CREATE TABLE IF NOT EXISTS schema_version (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
`)
    // Check current version, apply migrations
}
```

### Option 2: Use Migration Library
- **Pros:** Automatic up/down migrations
- **Cons:** External dependency
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Implement Option 1 for minimal overhead.

## Technical Details

- **Affected files:** `internal/storage/db.go`
- **Components:** Storage
- **Database changes:** schema_version table

## Acceptance Criteria

- [ ] schema_version table created
- [ ] Current version tracked
- [ ] Migrations applied in order
- [ ] Version checked before migrate

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during data integrity review | Version tracking prevents silent failures |

## Resources

- golang-migrate: https://github.com/golang-migrate/migrate
