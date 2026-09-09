# Source records and graph ownership

Status: accepted direction; runtime implementation pending.
Authority: [mk's September 9 instruction](2026-09-09-human-ratification.md).

Canonical project files carry guidance. Original conversations, attributed
answers, accepted revisions and review evidence remain recoverable retained
files independent of the Autarch installation. Autarch presents and transcribes
them; Lattice indexes them. Deleting or rebuilding the graph cannot alter or
delete those records.

This explicitly amends the September 2 card's statement that deleting Autarch
loses only preferences. Durable source records must survive removal of the app;
they must not exist only inside a disposable index. Preserve the older ruling
and its rationale in [the card history](../why.md).

Extend the existing local review-record format and Lattice feedback projection.
No estate-wide graph service or graph migration is required for this milestone.
Every projected source, ruling, outcome, plan and evaluation link carries its
source reference, revision, authority state and coverage gaps. Inferred
relationships remain provisional; graph identity confidence cannot confer
human authority.

The projection has one explicit rebuild writer. Bounded map/context queries
read the last complete projection without rebuilding it on view switches.
Unknown context, missing sources, stale revisions and unresolved contradictions
remain explicit. Rebuild verification compares meaning and source bytes.

Separate wire v1 from durable-record v2. Read old v1 records, retain legacy
executions and initialize additive fields. Older binaries must refuse newer
records instead of silently dropping their fields. Rollback retains records and
uses a compatible reader; no destructive downgrade is permitted.

Alternatives rejected: a new graph as primary truth, and a second controller
store for visits. Both create competing authority and unnecessary migration.
