# Schema Versioning (Soft Policy)

This document defines the compatibility policy for Autarch shared schemas:

- `pkg/contract` (cross-tool entity types)
- `pkg/events` (event spine types + payloads)

Autarch uses **soft versioning**: avoid breaking changes and keep older data readable without migrations whenever possible.

## Principles

1) **Additive changes are preferred**
   - Add new optional fields with `omitempty`
   - Never change field meaning

2) **No renames or type changes**
   - If a name must change, add a new field and keep the old one
   - If a type must change, add a new field with the new type

3) **Events are append-only**
   - New event types are allowed
   - Existing event types must remain compatible with old payloads
   - Never repurpose an event type for a different meaning

4) **Deprecation over deletion**
   - Keep old fields for at least one release cycle
   - Document deprecated fields and provide replacements

5) **Schema changes must be documented**
   - Update `docs/INTEGRATION.md` (event types list + payload notes)
   - Update this document with the change rationale

## When a change is allowed (no migration needed)

- Adding optional fields
- Adding new event types
- Adding new enum values at the end of a list

## When a migration is required

- Removing fields
- Changing field types
- Changing event payload structure in a breaking way

If a migration is required:

1) Add a new event type or field instead of changing existing ones
2) Provide a backfill script or migration plan
3) Update relevant docs

## Checklist for Schema Changes

- [ ] Additive only (or documented migration)
- [ ] `omitempty` used for new fields
- [ ] Event type list updated (`docs/INTEGRATION.md`)
- [ ] Tests updated for new payloads
- [ ] Consumers updated to tolerate missing fields
