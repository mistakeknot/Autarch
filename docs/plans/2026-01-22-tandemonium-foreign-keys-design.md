# Tandemonium Foreign Key Constraints (New DBs Only) Design

## Goal
Add foreign key constraints to `review_queue` and `sessions` for new databases, enforcing referential integrity and cascade deletes, without migrating existing DBs.

## Context
SQLite foreign keys are currently missing from the schema. We only want enforcement for **new DBs** going forward. Existing DBs should remain untouched.

## Decision
- Add `REFERENCES tasks(id) ON DELETE CASCADE` for `review_queue.task_id` and `sessions.task_id` in the migration schema.
- Enable `PRAGMA foreign_keys = ON` on every new connection to ensure FK enforcement in SQLite.
- Do **not** rebuild or migrate existing databases.

## Approach
- Update `Migrate` schema (new DBs) to include FK constraints.
- Add a small helper in `Open` (or `OpenShared`) to execute `PRAGMA foreign_keys = ON` once per connection.
- Add tests:
  1. Reject insert into `review_queue` or `sessions` with non-existent `task_id`.
  2. Cascade delete: deleting a task removes associated `review_queue` and `sessions` rows.

## Testing
- Unit tests in `internal/tandemonium/storage/db_test.go` (or a new test file) for FK enforcement and cascade behavior.

## Risks
Low. The changes only affect new DB schemas. Existing DBs are not migrated.

## Success Criteria
- FK constraints present in new schema.
- PRAGMA foreign_keys enabled on new connections.
- Tests verify enforcement and cascade behavior.

