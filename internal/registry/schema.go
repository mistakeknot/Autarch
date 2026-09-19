// Package registry is Autarch's estate registry: the durable record of which
// conversations exist, where they are running, and what has been asked of the
// operator.
//
// Three rules shape every table here, and each is enforced by a constraint or
// a trigger rather than left to a caller's good intentions:
//
//   - Nothing is removed by absence of evidence. Closing a record -- an
//     instance, a binding, an item -- requires naming both the basis, drawn
//     from a closed set of positive observations, and the event that carries
//     it. A failed read has neither, so "we could not look" can never render
//     as "there is nothing there."
//   - Absence is structural, never a label. There is no `ignored` outcome and
//     no `unassigned` project. Both are derived from which rows are missing,
//     because the two absences -- never shown, and shown but not acted on --
//     are different facts that cannot be recovered once merged.
//   - Identity is three things, not one, and they nest loosely. A conversation
//     outlives the process running it; a process outlives -- and, via /clear,
//     outlives several of -- the conversations inside it; and a pane holds
//     more than one conversation at once.
//
// The tables fall into three classes, and the boundary between them is load
// bearing: see Tables and the package's replay contract in
// docs/registry-ddl.md.
package registry

import (
	"database/sql"
	"fmt"

	autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

// SchemaVersion is written to PRAGMA user_version on a fresh database. Open
// refuses a database stamped newer than this build and never restamps an older
// one, because a silent restamp is a migration that did not happen.
const SchemaVersion = 3

const schema = `
-- ============================================================ SPINE
-- Append-only, never dropped, and the sole authority. Every projection below
-- is derived from these tables and can be dropped and rebuilt from them.

-- A named producer with its own liveness. Every count a view renders must be
-- read together with its source row: last_success_ms against expected_interval_ms
-- is what separates "zero" from "unchecked" from "stale".
CREATE TABLE IF NOT EXISTS source (
  source_id        TEXT PRIMARY KEY,
  host             TEXT NOT NULL,
  kind             TEXT NOT NULL,
  locator          TEXT NOT NULL DEFAULT '',
  -- Opaque to the registry; the producer defines its own resumption point
  -- (a byte offset, an inode plus size, a max mtime).
  replay_cursor    TEXT NOT NULL DEFAULT '',
  -- 'degraded' is a sweep that completed while one of its own instruments
  -- did not answer. It is neither ok nor an outright failure, and collapsing
  -- it into either loses the distinction an operator needs.
  status           TEXT NOT NULL DEFAULT 'unchecked'
                     CHECK (status IN ('unchecked','ok','degraded','error')),
  -- How often this producer is meant to run, so a watcher that died three
  -- hours ago stops reading as healthy just because its last run succeeded.
  expected_interval_ms INTEGER,
  last_attempt_ms  INTEGER,
  last_success_ms  INTEGER,
  last_error       TEXT,
  -- The observed shape of an undocumented upstream format, so a silent
  -- provider change surfaces as a diff rather than as missing fields.
  format_note      TEXT NOT NULL DEFAULT '',
  -- A producer is identified by what it watches, not by a nickname. Without
  -- this the same id on two hosts silently merges two estates.
  UNIQUE (host, kind, locator),
  CHECK (status <> 'ok' OR last_success_ms IS NOT NULL)
);

-- One row per sweep. Without it, "nothing happened between 14:00 and 15:00"
-- and "the watcher was down between 14:00 and 15:00" read identically -- and
-- that distinction decides whether a missed ruling was the router's fault.
-- It is also the only honest basis for closing a record: a session file that
-- vanished can be cited as absent from complete scan N, which needs N to have
-- an identity.
CREATE TABLE IF NOT EXISTS source_scan (
  scan_id      INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id    TEXT NOT NULL REFERENCES source(source_id),
  started_ms   INTEGER NOT NULL,
  finished_ms  INTEGER,
  -- Complete means every record in scope was enumerated. A partial sweep can
  -- add facts but can never support an absence claim.
  complete     INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0,1)),
  records_seen INTEGER,
  error        TEXT,
  CHECK (complete = 0 OR (finished_ms IS NOT NULL AND error IS NULL))
);
CREATE INDEX IF NOT EXISTS source_scan_by_source ON source_scan(source_id, started_ms);

-- The log. AUTOINCREMENT, not a bare INTEGER PRIMARY KEY: without it SQLite
-- reuses the id of a deleted max row, and a reused id silently rewinds every
-- replay cursor that had passed it.
--
-- conversation_id and instance_id are deliberately NOT foreign keys. They hold
-- deterministic ids (see ids.go) computed from natural keys carried in the
-- payload, so an event can be written before any projection exists, and every
-- projection can be dropped without the log noticing. A foreign key here makes
-- the spine depend on its own output, and replay becomes impossible.
CREATE TABLE IF NOT EXISTS event (
  event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id     TEXT NOT NULL REFERENCES source(source_id),
  scan_id       INTEGER REFERENCES source_scan(scan_id),
  -- Derived from content, not clock, so re-reading the whole sessions
  -- directory at startup inserts nothing new. Scoped per source: a global key
  -- lets a collision between two producers drop a row without a sound.
  dedupe_key    TEXT NOT NULL,
  source_seq    TEXT NOT NULL DEFAULT '',
  kind          TEXT NOT NULL,
  -- occurred_ms is when the world changed, as the provider reports it, and may
  -- be absent or out of order. observed_ms is when we saw it. event_id is the
  -- only total order; ordering by wall clock loses to clock skew and to a
  -- provider that rewrites a file with an older timestamp.
  occurred_ms   INTEGER,
  observed_ms   INTEGER NOT NULL,
  conversation_id TEXT,
  instance_id   TEXT,
  -- The raw provider record, in full. The projections are a lossy read of it,
  -- so anything not captured here is lost for good once the process exits.
  -- Structured provider metadata only: free text belongs in evidence.body,
  -- which is redaction-gated. See docs/registry-ddl.md.
  payload       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
  UNIQUE (source_id, dedupe_key)
);
CREATE INDEX IF NOT EXISTS event_conversation ON event(conversation_id, event_id);
CREATE INDEX IF NOT EXISTS event_instance ON event(instance_id, event_id);
CREATE INDEX IF NOT EXISTS event_kind ON event(kind, event_id);
CREATE INDEX IF NOT EXISTS event_observed ON event(observed_ms);

-- Raw observation backing a claim. Separate from event so retention can drop
-- bodies without losing the fact that the observation happened.
CREATE TABLE IF NOT EXISTS evidence (
  evidence_id     INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id        INTEGER NOT NULL REFERENCES event(event_id),
  kind            TEXT NOT NULL
                    CHECK (kind IN ('session_file','tmux_inventory','transcript_span',
                                    'pane_capture','detector','operator_input')),
  source_id       TEXT NOT NULL REFERENCES source(source_id),
  locator         TEXT NOT NULL DEFAULT '',
  -- Byte offsets, so a transcript tail is read forward from a cursor instead
  -- of reparsed whole on every poll.
  byte_from       INTEGER,
  byte_to         INTEGER,
  content_sha256  TEXT NOT NULL,
  body            TEXT,
  body_redacted   INTEGER NOT NULL DEFAULT 0 CHECK (body_redacted IN (0,1)),
  captured_ms     INTEGER NOT NULL,
  retain_until_ms INTEGER,
  CHECK (byte_from IS NULL OR byte_to IS NULL OR byte_to >= byte_from),
  -- Transcripts and pane captures carry secrets and are about to be copied
  -- into a new database. A body may exist only if it has been through
  -- redaction; raw text cannot be inserted by accident.
  CHECK (body IS NULL OR body_redacted = 1)
);
CREATE INDEX IF NOT EXISTS evidence_event ON evidence(event_id);
CREATE INDEX IF NOT EXISTS evidence_retention ON evidence(retain_until_ms)
  WHERE retain_until_ms IS NOT NULL;

-- ============================================================ PROJECTIONS
-- Derived from the log, droppable, rebuilt by replay. Nothing outside this
-- class may hold a foreign key into it -- that is what keeps a rebuild
-- possible, and TestNothingForeignKeysIntoAProjection enforces it.

-- A conversation outlives every process that runs it. host is part of the key
-- because the estate spans two machines: without it a synced copy of another
-- host's record merges into this one's history.
CREATE TABLE IF NOT EXISTS conversation (
  conversation_id     TEXT PRIMARY KEY,
  provider            TEXT NOT NULL,
  host                TEXT NOT NULL,
  provider_session_id TEXT NOT NULL,
  profile             TEXT NOT NULL DEFAULT '',
  -- The provider's own title and how it got there. 'auto' names are genuine
  -- topic summaries and are good channel evidence; 'derived' names are
  -- placeholders ("projects-14") carrying no topic at all. Storing which is
  -- which is what stops a placeholder being read as a subject.
  display_name        TEXT NOT NULL DEFAULT '',
  display_name_source TEXT NOT NULL DEFAULT '',
  display_name_since_ms INTEGER,
  first_seen_ms       INTEGER NOT NULL,
  last_seen_ms        INTEGER NOT NULL,
  first_event_id      INTEGER NOT NULL REFERENCES event(event_id),
  last_event_id       INTEGER NOT NULL REFERENCES event(event_id),
  UNIQUE (provider, host, provider_session_id)
);

-- /clear, compact, resume and SDK spawn each relate two conversations. basis
-- and confidence are required because the grades differ sharply: a resume
-- pointer is a fact, while a spawn inferred from pane co-tenancy is a guess.
CREATE TABLE IF NOT EXISTS conversation_lineage (
  parent_conversation_id TEXT NOT NULL REFERENCES conversation(conversation_id),
  child_conversation_id  TEXT NOT NULL REFERENCES conversation(conversation_id),
  relation    TEXT NOT NULL CHECK (relation IN ('resume','compact','clear','fork','spawn')),
  basis       TEXT NOT NULL
                CHECK (basis IN ('same_process_session_change','provider_resume_pointer',
                                 'process_parentage','pane_cotenancy','operator_assertion')),
  confidence  REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
  observed_ms INTEGER NOT NULL,
  event_id    INTEGER NOT NULL REFERENCES event(event_id),
  PRIMARY KEY (parent_conversation_id, child_conversation_id, relation),
  CHECK (parent_conversation_id <> child_conversation_id)
);
CREATE INDEX IF NOT EXISTS lineage_child ON conversation_lineage(child_conversation_id);

-- One row per process launch. pid alone is not identity: pids recur after a
-- reboot, and this directory keeps no tombstones -- it still holds orphaned
-- key files from a month ago.
--
-- There is no conversation_id here. Measured 2026-09-19: /clear replaces
-- sessionId in place, same pid and same startedAt, so one process carries
-- several conversations over its life. The link is time-bounded in
-- instance_conversation below.
CREATE TABLE IF NOT EXISTS launch_instance (
  instance_id       TEXT PRIMARY KEY,
  host              TEXT NOT NULL,
  -- Part of the key: a pid means nothing without the namespace it was issued
  -- in, and a container or a remote host reuses the same small integers.
  pid_domain        TEXT NOT NULL,
  pid               INTEGER NOT NULL,
  ppid              INTEGER,
  -- Only observable while the process lives, and the dispatch shim in between
  -- means it is not always the conversation's parent. Recorded raw; any
  -- inference lands in conversation_lineage with its own basis.
  parent_instance_id TEXT,
  -- started_ms is the provider's own millisecond epoch and is the canonical
  -- instance timestamp. proc_start_raw is the provider's human string, kept as
  -- evidence and never parsed: it has one-second resolution and no timezone,
  -- and it disagrees with the epoch field by over a second.
  started_ms        INTEGER NOT NULL,
  proc_start_raw    TEXT NOT NULL DEFAULT '',
  -- The cwd this process was launched or resumed in. Not the project: on this
  -- estate ten of twelve live agents share one cwd. A transcript path is a
  -- locator, never a resume cwd.
  launch_cwd        TEXT NOT NULL DEFAULT '',
  -- 'cli' is a human launch; 'sdk-cli' is a process another agent spawned.
  -- This is what separates a parent from its dispatched child in one pane.
  entrypoint        TEXT NOT NULL DEFAULT '',
  kind              TEXT NOT NULL DEFAULT '',
  agent_version     TEXT NOT NULL DEFAULT '',
  messaging_socket  TEXT NOT NULL DEFAULT '',
  bridge_session_id TEXT NOT NULL DEFAULT '',
  ended_ms          INTEGER,
  end_basis         TEXT
                      CHECK (end_basis IS NULL OR end_basis IN
                        ('process_exited','absent_from_complete_scan','superseded_by_restart',
                         'operator_action')),
  end_event_id      INTEGER REFERENCES event(event_id),
  -- A closure that a later observation contradicts is undone, not ignored.
  -- The count is kept because a reopen means something judged this process
  -- dead while it was running, and that is worth being able to see.
  reopened_count    INTEGER NOT NULL DEFAULT 0,
  last_reopen_event_id INTEGER REFERENCES event(event_id),
  -- When a probe last positively saw this process. An open instance is only
  -- "not known to have ended"; without this, a row nobody has been able to
  -- probe for hours is indistinguishable from one confirmed alive a moment
  -- ago, and status would call both of them live.
  last_alive_ms     INTEGER,
  first_event_id    INTEGER NOT NULL REFERENCES event(event_id),
  last_event_id     INTEGER NOT NULL REFERENCES event(event_id),
  UNIQUE (host, pid_domain, pid, started_ms),
  -- Ending requires a basis from the closed set above AND the event carrying
  -- it. A failed read can supply neither, so it cannot close a live record.
  CHECK (ended_ms IS NULL OR (end_basis IS NOT NULL AND end_event_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS instance_live ON launch_instance(host, pid) WHERE ended_ms IS NULL;

-- Which conversation a process was running, and when. One at a time, many over
-- a lifetime -- the shape /clear forces.
CREATE TABLE IF NOT EXISTS instance_conversation (
  instance_id      TEXT NOT NULL REFERENCES launch_instance(instance_id),
  conversation_id  TEXT NOT NULL REFERENCES conversation(conversation_id),
  observed_from_ms INTEGER NOT NULL,
  observed_to_ms   INTEGER,
  end_basis        TEXT
                     CHECK (end_basis IS NULL OR end_basis IN
                       ('session_id_changed','instance_ended','operator_action')),
  end_event_id     INTEGER REFERENCES event(event_id),
  first_event_id   INTEGER NOT NULL REFERENCES event(event_id),
  last_event_id    INTEGER NOT NULL REFERENCES event(event_id),
  CHECK (observed_to_ms IS NULL OR (end_basis IS NOT NULL AND end_event_id IS NOT NULL))
);
-- A process runs one conversation at a time.
CREATE UNIQUE INDEX IF NOT EXISTS instance_conversation_open
  ON instance_conversation(instance_id) WHERE observed_to_ms IS NULL;
CREATE INDEX IF NOT EXISTS instance_conversation_by_conversation
  ON instance_conversation(conversation_id, observed_from_ms);

-- Time-bounded, and deliberately many-to-one against a pane: a parent and the
-- child it dispatched share one pane routinely on this estate.
--
-- The tmux session NAME is an observation, not a key. Measured 2026-09-19: a
-- record claimed session "tmux-organizer" for the pane the live server called
-- "iterm[autarch - e4bedaf5..."; two records sharing a pane disagreed about
-- its name by one bracket. pane_key follows SamePane() instead.
CREATE TABLE IF NOT EXISTS pane_binding (
  binding_id        INTEGER PRIMARY KEY AUTOINCREMENT,
  instance_id       TEXT NOT NULL REFERENCES launch_instance(instance_id),
  -- Populated only from a successful tmux inventory. socket must be
  -- canonicalised first: tmux reports /private/tmp while a literal path says
  -- /tmp, and mixing the two forks one pane into two.
  socket            TEXT,
  server_pid        INTEGER,
  server_started    INTEGER,
  -- The pane's root process, not the agent's: it survives shell-to-agent
  -- transitions and changes only when the pane is respawned.
  pane_pid          INTEGER,
  tmux_session_id   TEXT NOT NULL DEFAULT '',
  window_id         TEXT NOT NULL DEFAULT '',
  pane_id           TEXT NOT NULL DEFAULT '',
  -- Dated evidence, kept because project attribution and lineage both read it.
  -- Display resolves the live name and falls back to this with its age.
  session_name_seen TEXT NOT NULL DEFAULT '',
  binding_basis     TEXT NOT NULL
                      CHECK (binding_basis IN ('tmux_inventory','session_file_claim')),
  -- Generated by the database, not the writer, so the key cannot drift from
  -- agenttransport.Target.PaneKey(), which is pinned to it by test.
  --
  -- The unverified branch uses the pane id alone. It must not include the
  -- session name or window id: both change under a rename, a join-pane or a
  -- break-pane, and a changing key lets one instance hold two open bindings
  -- on one pane -- which the unique index below could not catch.
  pane_key          TEXT GENERATED ALWAYS AS (
                      CASE WHEN socket IS NOT NULL AND server_pid IS NOT NULL
                                AND server_started IS NOT NULL AND pane_pid IS NOT NULL
                           THEN socket || '/' || server_pid || '/' || server_started
                                || '/' || pane_id || '/' || pane_pid
                           ELSE 'claimed:' || pane_id
                      END) STORED,
  observed_from_ms  INTEGER NOT NULL,
  observed_to_ms    INTEGER,
  end_basis         TEXT
                      CHECK (end_basis IS NULL OR end_basis IN
                        ('pane_absent_from_complete_scan','pane_pid_changed',
                         'instance_ended','operator_action')),
  end_event_id      INTEGER REFERENCES event(event_id),
  first_event_id    INTEGER NOT NULL REFERENCES event(event_id),
  last_event_id     INTEGER NOT NULL REFERENCES event(event_id),
  CHECK (binding_basis <> 'tmux_inventory' OR (socket IS NOT NULL AND server_pid IS NOT NULL
         AND server_started IS NOT NULL AND pane_pid IS NOT NULL)),
  CHECK (observed_to_ms IS NULL OR (end_basis IS NOT NULL AND end_event_id IS NOT NULL))
);
-- At most one open binding per (instance, pane) -- re-observation updates in
-- place -- while different instances stay free to bind one pane at once.
CREATE UNIQUE INDEX IF NOT EXISTS pane_binding_open
  ON pane_binding(instance_id, pane_key) WHERE observed_to_ms IS NULL;
CREATE INDEX IF NOT EXISTS pane_binding_by_pane
  ON pane_binding(pane_key, observed_from_ms);

-- ============================================================ DURABLE
-- Operator-facing and never dropped. These survive a projection rebuild, which
-- is why they reference conversations by deterministic id and not by foreign
-- key: the id is stable across a rebuild, the row is not.

-- Many-to-many with evidence and confidence, never a scalar. A conversation
-- with no row here is unassigned; the REASON lives in an 'attribution.attempted'
-- event listing which bases were tried and what each returned, so #unassigned
-- states a reason instead of shrugging.
--
-- stance exists because an operator saying "this is NOT project X" is a fact a
-- positive-only table cannot hold, and automatic re-assertion would overwrite
-- it on the next sweep.
CREATE TABLE IF NOT EXISTS project_association (
  conversation_id TEXT NOT NULL,
  project_key     TEXT NOT NULL,
  basis           TEXT NOT NULL
                    CHECK (basis IN ('launch_cwd','tmux_session_name','provider_name',
                                     'transcript_topic','operator_override')),
  stance          TEXT NOT NULL DEFAULT 'is' CHECK (stance IN ('is','is_not')),
  confidence      REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
  observed_ms     INTEGER NOT NULL,
  evidence_id     INTEGER REFERENCES evidence(evidence_id),
  event_id        INTEGER NOT NULL REFERENCES event(event_id),
  retracted_ms    INTEGER,
  retracted_basis TEXT,
  PRIMARY KEY (conversation_id, project_key, basis),
  CHECK (retracted_ms IS NULL OR retracted_basis IS NOT NULL),
  -- Only a person may assert a negative.
  CHECK (stance = 'is' OR basis = 'operator_override')
);
CREATE INDEX IF NOT EXISTS association_project ON project_association(project_key)
  WHERE retracted_ms IS NULL;

-- A thing wanting the operator. Created by any of three independent sources;
-- removable by none of them. Written from Phase C; defined now because the
-- absence fields below cannot be backfilled.
CREATE TABLE IF NOT EXISTS item (
  item_id           TEXT PRIMARY KEY,
  conversation_id   TEXT,
  kind              TEXT NOT NULL,
  summary           TEXT NOT NULL DEFAULT '',
  -- What this item is about, in the terms of whichever source saw it first --
  -- an instance plus a status timestamp, or a tool-use id. Without it, a
  -- second sweep creates a second item and the primary metric double-counts.
  anchor_key        TEXT,
  -- Two sources describing one ruling in incompatible terms cannot share an
  -- anchor. Merging points the duplicate at the survivor; it never deletes.
  merged_into_item_id TEXT REFERENCES item(item_id),
  created_ms        INTEGER NOT NULL,
  created_event_id  INTEGER NOT NULL REFERENCES event(event_id),
  resolved_ms       INTEGER,
  resolved_basis    TEXT
                      CHECK (resolved_basis IS NULL OR resolved_basis IN
                        ('tool_answer','operator_action','superseded','conversation_ended')),
  resolved_event_id INTEGER REFERENCES event(event_id),
  -- Resolution needs positive evidence, from a closed set. Nothing resolves
  -- because a detector stopped seeing it.
  CHECK (resolved_ms IS NULL OR (resolved_basis IS NOT NULL AND resolved_event_id IS NOT NULL)),
  CHECK (merged_into_item_id IS NULL OR merged_into_item_id <> item_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS item_anchor ON item(anchor_key) WHERE anchor_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS item_open ON item(conversation_id) WHERE resolved_ms IS NULL;

-- Which sources have independently attested to an item. Insert-only by
-- construction and by trigger: there is no column a source could clear to
-- withdraw, and no delete it could issue.
CREATE TABLE IF NOT EXISTS item_source (
  item_id     TEXT NOT NULL REFERENCES item(item_id),
  source_kind TEXT NOT NULL
                CHECK (source_kind IN ('transcript','wait_state','detector','operator')),
  event_id    INTEGER NOT NULL REFERENCES event(event_id),
  observed_ms INTEGER NOT NULL,
  PRIMARY KEY (item_id, source_kind, event_id)
);

-- We showed it. No row means it was never shown, which is a different fact
-- from having been shown and not acted on.
CREATE TABLE IF NOT EXISTS exposure (
  exposure_id   INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id       TEXT NOT NULL REFERENCES item(item_id),
  surface       TEXT NOT NULL,
  -- How it was shown. 'listed' among others, 'focused' as the current item, or
  -- 'notified' as an interrupt. Rank is its position at that moment; neither
  -- can be reconstructed later, and both decide whether a miss was ours.
  mode          TEXT NOT NULL CHECK (mode IN ('listed','focused','notified')),
  rank          INTEGER,
  shown_from_ms INTEGER NOT NULL,
  shown_to_ms   INTEGER,
  end_basis     TEXT
                  CHECK (end_basis IS NULL OR end_basis IN
                    ('navigated_away','item_resolved','surface_closed','surface_crashed')),
  event_id      INTEGER NOT NULL REFERENCES event(event_id),
  CHECK (shown_to_ms IS NULL OR end_basis IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS exposure_item ON exposure(item_id, shown_from_ms);
-- Parent key for the composite reference from outcome.
CREATE UNIQUE INDEX IF NOT EXISTS exposure_item_pair ON exposure(exposure_id, item_id);

-- They acted. channel says how the action reached us; exposure_id says which
-- showing it answered, and is NULL when there was none -- an answer typed
-- straight into the pane is a real action with no exposure, and forcing a join
-- would either drop it or invent one.
--
-- There is deliberately no 'ignored' action. Ignoring is derived from an
-- exposure with no outcome; never shown is derived from no exposure at all;
-- missed is derived from neither. One label would merge absences that no later
-- pass can separate.
CREATE TABLE IF NOT EXISTS outcome (
  outcome_id  INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     TEXT NOT NULL REFERENCES item(item_id),
  exposure_id INTEGER,
  channel     TEXT NOT NULL CHECK (channel IN ('router','pane_direct','unknown')),
  action      TEXT NOT NULL
                CHECK (action IN ('answered','dismissed','snoozed','opened')),
  occurred_ms INTEGER NOT NULL,
  event_id    INTEGER NOT NULL REFERENCES event(event_id),
  -- Composite, so an outcome cannot cite another item's exposure.
  FOREIGN KEY (exposure_id, item_id) REFERENCES exposure(exposure_id, item_id),
  -- An action that came through the router must name the showing it answered.
  CHECK (channel <> 'router' OR exposure_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS outcome_item ON outcome(item_id, occurred_ms);

-- A ruling taken, by whom, and why.
CREATE TABLE IF NOT EXISTS decision (
  decision_id INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     TEXT NOT NULL REFERENCES item(item_id),
  actor       TEXT NOT NULL CHECK (actor IN ('operator','router','agent')),
  verdict     TEXT NOT NULL,
  rationale   TEXT NOT NULL DEFAULT '',
  occurred_ms INTEGER NOT NULL,
  event_id    INTEGER NOT NULL REFERENCES event(event_id)
);
CREATE INDEX IF NOT EXISTS decision_item ON decision(item_id, occurred_ms);

-- ============================================================ replay state
CREATE TABLE IF NOT EXISTS projection_state (
  projection               TEXT PRIMARY KEY,
  applied_through_event_id INTEGER NOT NULL DEFAULT 0,
  updated_ms               INTEGER NOT NULL
);

-- ============================================================ enforcement
-- Append-only and insert-only are claims until something refuses the write.

CREATE TRIGGER IF NOT EXISTS event_no_update BEFORE UPDATE ON event
BEGIN SELECT RAISE(ABORT, 'event is append-only'); END;

CREATE TRIGGER IF NOT EXISTS event_no_delete BEFORE DELETE ON event
BEGIN SELECT RAISE(ABORT, 'event is append-only'); END;

CREATE TRIGGER IF NOT EXISTS item_source_no_update BEFORE UPDATE ON item_source
BEGIN SELECT RAISE(ABORT, 'an attestation cannot be rewritten'); END;

CREATE TRIGGER IF NOT EXISTS item_source_no_delete BEFORE DELETE ON item_source
BEGIN SELECT RAISE(ABORT, 'an attestation cannot be withdrawn'); END;

CREATE TRIGGER IF NOT EXISTS evidence_no_delete BEFORE DELETE ON evidence
BEGIN SELECT RAISE(ABORT, 'evidence is never deleted; retention drops the body'); END;

-- Retention is the only legal update: the body goes, the fact that we looked
-- stays.
CREATE TRIGGER IF NOT EXISTS evidence_retention_only BEFORE UPDATE ON evidence
WHEN NEW.body IS NOT NULL
  OR NEW.event_id <> OLD.event_id
  OR NEW.content_sha256 <> OLD.content_sha256
  OR NEW.kind <> OLD.kind
  OR NEW.captured_ms <> OLD.captured_ms
BEGIN SELECT RAISE(ABORT, 'evidence may only be updated to drop its body'); END;
`

// Open opens or creates the registry database.
//
// Foreign keys are enabled through the DSN rather than executed on one
// connection: every non-suppression guarantee here depends on references
// resolving, and an Exec-set pragma is lost the first time the pool replaces a
// dropped connection.
func Open(path string) (*sql.DB, error) { return open(path, false) }

// OpenForMigration opens a database stamped at an older schema version, so
// Migrate can bring it forward. Ordinary Open refuses one, deliberately: a
// silent restamp is a migration that did not happen.
func OpenForMigration(path string) (*sql.DB, error) { return open(path, true) }

func open(path string, allowOlder bool) (*sql.DB, error) {
	db, err := autarchdb.OpenWith(path, "foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open registry db: %w", err)
	}

	// Version is read BEFORE the DDL runs. An older build must refuse a newer
	// database without first executing its own CREATE statements against it.
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("read schema version: %w", err)
	}
	if version > SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("registry db at %s is schema v%d, newer than this build's v%d", path, version, SchemaVersion)
	}
	if version != 0 && version < SchemaVersion && !allowOlder {
		db.Close()
		return nil, fmt.Errorf("registry db at %s is schema v%d and this build expects v%d: run `autarch-registry migrate`, which replays the log into fresh projections. Do not delete the file: it holds the event log, which is the only authority and cannot be re-observed", path, version, SchemaVersion)
	}

	// On a migration the DDL is applied by Migrate, inside its transaction.
	if !allowOlder || version == 0 {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, fmt.Errorf("init registry schema: %w", err)
		}
	}

	// Stamped only on a fresh database. Stamping an existing one is a
	// migration that did not happen.
	if version == 0 {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion)); err != nil {
			db.Close()
			return nil, fmt.Errorf("stamp schema version: %w", err)
		}
	}

	return db, nil
}
