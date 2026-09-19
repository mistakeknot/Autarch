package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A pid far above the macOS ceiling, so liveness checks are deterministic:
// nothing in the fixtures is ever accidentally alive.
const deadPID = 4000001

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "registry.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewStore(db, "clavain")
	// A fixed clock, so a replay produces a byte-identical projection.
	var tick int64 = 1_700_000_000_000
	s.now = func() int64 { tick++; return tick }
	// Fixture pids are not real processes, so the default probe would report
	// every one of them dead. Tests that care about closure set their own.
	s.probe = allAlive
	return s
}

// allAlive is the default probe for fixtures: nothing has died.
func allAlive(targets []probeTarget) map[string]string {
	out := make(map[string]string, len(targets))
	for _, t := range targets {
		out[t.instanceID] = ProbeAlive
	}
	return out
}

// deadPIDs is a probe that reports the named pids dead and everything else
// alive, so an absence can be staged precisely.
func deadPIDs(pids ...int64) func([]probeTarget) map[string]string {
	dead := map[int64]bool{}
	for _, p := range pids {
		dead[p] = true
	}
	return func(targets []probeTarget) map[string]string {
		out := make(map[string]string, len(targets))
		for _, t := range targets {
			if dead[t.pid] {
				out[t.instanceID] = ProbeDead
			} else {
				out[t.instanceID] = ProbeAlive
			}
		}
		return out
	}
}

func writeRecord(t *testing.T, dir string, pid int64, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", pid)), []byte(body), 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

func record(pid int64, sessionID, tmux, cwd, name, nameSource string, startedAt, updatedAt int64) string {
	return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":%q,"startedAt":%d,
	  "procStart":"Sat Sep 19 22:25:12 2026","version":"2.1.278","kind":"interactive",
	  "entrypoint":"cli","pidDomain":"darwin","tmux":%q,"messagingSocketPath":"/tmp/cc-socks/x.sock",
	  "name":%q,"nameSource":%q,"nameSince":%d,"status":"busy","updatedAt":%d,"statusUpdatedAt":%d,
	  "bridgeSessionId":"session_x"}`,
		pid, sessionID, cwd, startedAt, tmux, name, nameSource, startedAt, updatedAt, updatedAt)
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %.50s: %v", q, err)
	}
	return n
}

func scanAndProject(t *testing.T, s *Store, dir string) ScanResult {
	t.Helper()
	res, err := ScanClaudeSessions(s, dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}
	return res
}

func TestParseTmuxRefHandlesTheNamesThisEstateActuallyUses(t *testing.T) {
	cases := []struct {
		in                 string
		ok                 bool
		name, window, pane string
	}{
		// A trailing separator inside the name: splitting on the FIRST colon
		// would truncate it, and the name is evidence we keep.
		{"iterm]shadow-workipedia - :@109.%109", true, "iterm]shadow-workipedia - ", "@109", "%109"},
		{"2:@2.%2", true, "2", "@2", "%2"},
		{"tmux-organizer:@98.%98", true, "tmux-organizer", "@98", "%98"},
		{"iterm]infinite-fun-space|afterthem@claude - 39d76846:@63.%63", true,
			"iterm]infinite-fun-space|afterthem@claude - 39d76846", "@63", "%63"},
		{"", false, "", "", ""},
		{"no-location", false, "", "", ""},
		{"name:@12", false, "", "", ""},
		{"name:12.34", false, "", "", ""},
	}
	for _, c := range cases {
		got, ok := ParseTmuxRef(c.in)
		if ok != c.ok {
			t.Errorf("ParseTmuxRef(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && (got.SessionName != c.name || got.WindowID != c.window || got.PaneID != c.pane) {
			t.Errorf("ParseTmuxRef(%q) = %+v, want {%q %q %q}", c.in, got, c.name, c.window, c.pane)
		}
	}
}

func TestScanIsIdempotent(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))

	first := scanAndProject(t, s, dir)
	if first.Inserted != 1 || !first.Complete {
		t.Fatalf("first scan: %+v", first)
	}

	second := scanAndProject(t, s, dir)
	if second.Inserted != 0 || second.Skipped != 1 {
		t.Errorf("re-reading an unchanged record must insert nothing: %+v", second)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'session.observed'`); n != 1 {
		t.Errorf("session events after two scans = %d, want 1", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation`); n != 1 {
		t.Errorf("conversations = %d, want 1", n)
	}
}

// "We could not look" must never render as "there is nothing there."
func TestAFailedReadFreezesRatherThanDemotes(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	before := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`)
	if before != 1 {
		t.Fatalf("open instances before = %d, want 1", before)
	}

	// The directory becomes unreadable.
	if _, err := ScanClaudeSessions(s, filepath.Join(dir, "does-not-exist")); err == nil {
		t.Fatal("expected an error from an unreadable directory")
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project after a failed scan: %v", err)
	}

	if after := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); after != before {
		t.Errorf("open instances after a failed read = %d, want %d -- a failed read demoted the estate", after, before)
	}
	var status string
	var lastErr sql.NullString
	if err := s.DB().QueryRow(`SELECT status, last_error FROM source WHERE source_id = ?`, SourceClaudeSessions).
		Scan(&status, &lastErr); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if status != "error" {
		t.Errorf("source status after a failed read = %q, want error", status)
	}
	if !lastErr.Valid || lastErr.String == "" {
		t.Error("a failed read must leave a reason to show the operator")
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM source_scan WHERE complete = 1`); n != 1 {
		t.Errorf("complete scans = %d, want 1 -- the failed sweep must not count as complete", n)
	}
}

// A record that will not parse is a gap, so the sweep cannot claim to have
// enumerated everything -- and therefore cannot justify closing anything.
func TestAnUnparsableRecordMakesTheSweepIncomplete(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	writeRecord(t, dir, 101, `{"this is": not json`)
	res := scanAndProject(t, s, dir)
	if res.Complete {
		t.Error("a sweep that could not read every record must not claim completeness")
	}
	if len(res.Unparsable) != 1 {
		t.Errorf("unparsable = %v, want one entry", res.Unparsable)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'scan.completed'`); n != 1 {
		t.Errorf("scan.completed events = %d, want 1 (only the first sweep completed)", n)
	}
}

// Two live agents routinely share one pane: a parent and the child it
// dispatched. Neither may evict the other.
func TestTwoConversationsInOnePaneAreBothRetained(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	pane := "iterm[]linsekasten - 74e5950e:@67.%67"
	writeRecord(t, dir, 52620, record(52620, "74e5950e", pane, "/Users/sma/projects", "parent", "auto", 1000, 2000))
	// The child's record names the pane by its PARENT's uuid: the embedded id
	// identifies the pane's first occupant, not the process reading it.
	writeRecord(t, dir, 37995, record(37995, "90057d0b", pane, "/Users/sma/projects/linsenkasten", "child", "derived", 1100, 2100))
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation`); n != 2 {
		t.Errorf("conversations = %d, want 2", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE pane_id = '%67' AND observed_to_ms IS NULL`); n != 2 {
		t.Errorf("open bindings on the shared pane = %d, want 2", n)
	}
	// And they are distinguishable, which is the point of keeping both.
	var childCWD string
	if err := s.DB().QueryRow(`SELECT launch_cwd FROM launch_instance WHERE pid = 37995`).Scan(&childCWD); err != nil {
		t.Fatalf("read child: %v", err)
	}
	if childCWD != "/Users/sma/projects/linsenkasten" {
		t.Errorf("child cwd = %q; the two conversations in one pane were merged", childCWD)
	}
}

// Measured on Clavain: /clear replaces sessionId in place, same pid and same
// startedAt. The previous conversation must be retained, not overwritten.
func TestClearKeepsBothConversationsOnOneProcess(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 43066, record(43066, "5d183345", "clrtest:@114.%114", "/private/tmp", "tmp-80", "derived", 1789858817805, 1789858817730))
	scanAndProject(t, s, dir)

	writeRecord(t, dir, 43066, record(43066, "e20f9bd7", "clrtest:@114.%114", "/private/tmp", "tmp-80", "derived", 1789858817805, 1789858834363))
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation`); n != 2 {
		t.Errorf("conversations after /clear = %d, want 2 -- the first was overwritten", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance`); n != 1 {
		t.Errorf("instances after /clear = %d, want 1 -- same pid, same start", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM instance_conversation WHERE instance_id = ?`,
		InstanceID("clavain", "darwin", 43066, 1789858817805)); n != 2 {
		t.Errorf("conversation links on one process = %d, want 2", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM instance_conversation WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open links = %d, want 1 -- a process runs one conversation at a time", n)
	}
	var basis string
	if err := s.DB().QueryRow(`SELECT end_basis FROM instance_conversation WHERE observed_to_ms IS NOT NULL`).Scan(&basis); err != nil {
		t.Fatalf("read closed link: %v", err)
	}
	if basis != "session_id_changed" {
		t.Errorf("closing basis = %q, want session_id_changed", basis)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation_lineage WHERE relation = 'clear'`); n != 1 {
		t.Errorf("clear lineage edges = %d, want 1 -- without it the succession is unrecoverable", n)
	}
}

func TestAnAbsentRecordClosesOnlyWithACompleteSweep(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "stays", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	writeRecord(t, dir, deadPID, record(deadPID, "goes", "proj:@2.%2", "/Users/sma/projects/autarch", "b", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 2 {
		t.Fatalf("open instances = %d, want 2", n)
	}

	s.probe = deadPIDs(deadPID)
	os.Remove(filepath.Join(dir, fmt.Sprintf("%d.json", deadPID)))
	// An incomplete sweep sees the same absence and must NOT act on it.
	writeRecord(t, dir, 102, `{"broken`)
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 2 {
		t.Errorf("open instances after an incomplete sweep = %d, want 2", n)
	}

	os.Remove(filepath.Join(dir, "102.json"))
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Errorf("open instances after a complete sweep = %d, want 1", n)
	}
	var basis string
	var endEvent sql.NullInt64
	if err := s.DB().QueryRow(`SELECT end_basis, end_event_id FROM launch_instance WHERE ended_ms IS NOT NULL`).
		Scan(&basis, &endEvent); err != nil {
		t.Fatalf("read closed instance: %v", err)
	}
	if basis != "absent_from_complete_scan" || !endEvent.Valid {
		t.Errorf("closure = %q citing event %v; both are required", basis, endEvent)
	}
	// Its bindings close with it rather than dangling open.
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open bindings after the instance ended = %d, want 1", n)
	}
}

func TestAttributionRecordsWhyItFailed(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	// The shape of ten of twelve live agents: the umbrella cwd and a session
	// name following no project convention.
	writeRecord(t, dir, 100, record(100, "unattributable", "2:@2.%2", "/Users/sma/projects", "projects-14", "derived", 1000, 2000))
	writeRecord(t, dir, 101, record(101, "attributable", "iterm]jawnomicon|ui - x:@66.%66", "/Users/sma/projects", "ui", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM project_association
		WHERE conversation_id = ? AND project_key = 'jawnomicon' AND basis = 'tmux_session_name'`,
		ConversationID("claude", "clavain", "attributable")); n != 1 {
		t.Error("a session name carrying a project must produce an association")
	}

	var payload string
	if err := s.DB().QueryRow(`SELECT payload FROM event WHERE kind = 'attribution.attempted'`).Scan(&payload); err != nil {
		t.Fatalf("an unattributable conversation must leave a reason: %v", err)
	}
	for _, want := range []string{"umbrella directory", "no project convention"} {
		if !strings.Contains(payload, want) {
			t.Errorf("reason %q does not mention %q", payload, want)
		}
	}
	// The reason is keyed by content, so an unchanged failure does not grow
	// the log on every sweep.
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'attribution.attempted'`); n != 1 {
		t.Errorf("attribution events after two sweeps = %d, want 1", n)
	}
}

// The acceptance test for the whole no-foreign-key-into-a-projection rule:
// drop every projection, replay, and the result must be identical.
func TestRebuildFromTheLogIsIdentical(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 43066, record(43066, "5d183345", "clrtest:@114.%114", "/private/tmp", "tmp-80", "derived", 1789858817805, 1000))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1100, 1100))
	writeRecord(t, dir, 37995, record(37995, "90057d0b", "iterm[]linsekasten:@67.%67", "/Users/sma/projects/linsenkasten", "child", "derived", 1200, 1200))
	scanAndProject(t, s, dir)

	// Verification, so the replay has to reproduce an in-place upgrade.
	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "2.1.278") + "\n" +
			paneLine("$4", "@114", "%114", 40000, "clrtest", "2.1.278")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	// A record that changes AFTER its binding was verified -- the sweep that
	// crashed the projector before the lookup was fixed.
	writeRecord(t, dir, 43066, record(43066, "e20f9bd7", "clrtest:@114.%114", "/private/tmp", "tmp-80", "derived", 1789858817805, 2000))
	scanAndProject(t, s, dir)

	// And a closure, so the replay has to reproduce an ending too.
	s.probe = deadPIDs(37995)
	os.Remove(filepath.Join(dir, "37995.json"))
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NOT NULL`); n != 1 {
		t.Fatalf("closed instances = %d, want 1", n)
	}

	// A durable row naming a conversation must survive the rebuild pointing
	// at the same thing.
	conv := ConversationID("claude", "clavain", "5d183345")
	ev := count(t, s.DB(), `SELECT MIN(event_id) FROM event`)
	mustExec(t, s.DB(), `INSERT INTO item (item_id, conversation_id, kind, created_ms, created_event_id)
		VALUES ('item-1', ?, 'ruling', 1, ?)`, conv, ev)

	before := snapshotProjections(t, s.DB())
	events := count(t, s.DB(), `SELECT COUNT(*) FROM event`)

	if _, err := Rebuild(s); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	if after := count(t, s.DB(), `SELECT COUNT(*) FROM event`); after != events {
		t.Errorf("events after a rebuild = %d, want %d -- a replay must not append", after, events)
	}
	after := snapshotProjections(t, s.DB())
	if before != after {
		t.Errorf("rebuilt projections differ from the originals.\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}

	var got string
	if err := s.DB().QueryRow(`SELECT conversation_id FROM item WHERE item_id = 'item-1'`).Scan(&got); err != nil {
		t.Fatalf("durable row lost: %v", err)
	}
	if got != conv {
		t.Errorf("item now names %q, want %q", got, conv)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation WHERE conversation_id = ?`, got); n != 1 {
		t.Error("the rebuilt conversation does not answer to the id the durable row kept")
	}
}

// snapshotProjections renders every projection row as sorted text, so a
// comparison is insensitive to row order but sensitive to every value.
func snapshotProjections(t *testing.T, db *sql.DB) string {
	t.Helper()
	var out strings.Builder
	for _, table := range ProjectionTables() {
		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			t.Fatalf("select %s: %v", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("columns %s: %v", table, err)
		}
		var lines []string
		for rows.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			var parts []string
			for i, c := range cells {
				parts = append(parts, fmt.Sprintf("%s=%v", cols[i], c))
			}
			lines = append(lines, strings.Join(parts, " "))
		}
		rows.Close()
		sort.Strings(lines)
		out.WriteString("## " + table + "\n")
		for _, l := range lines {
			out.WriteString(l + "\n")
		}
	}
	return out.String()
}

// The crash Fable reproduced on the fourth live sweep: once a binding is
// verified its generated key is the real one, so a lookup by the claimed key
// misses, inserts a second claim, and then collides when that claim is
// verified onto the key the first row already holds.
func TestReobservingAVerifiedInstanceNeitherCrashesNorDuplicates(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 17120, record(17120, "d58d5e63", "iterm]jawnomicon - x:@65.%65", "/Users/sma/projects", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@65", "%65", 36128, "iterm]jawnomicon - x", "2.1.278")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	// The record changes, so a fresh session.observed lands on an instance
	// whose binding is already verified. This is the sweep that crashed.
	for i, updated := range []int64{3000, 4000, 5000} {
		writeRecord(t, dir, 17120, record(17120, "d58d5e63", "iterm]jawnomicon - x:@65.%65", "/Users/sma/projects", "a", "auto", 1000, updated))
		res, err := ScanClaudeSessions(s, dir)
		if err != nil {
			t.Fatalf("sweep %d: %v", i, err)
		}
		if res.Inserted != 1 {
			t.Fatalf("sweep %d inserted %d, expected the changed record", i, res.Inserted)
		}
		if _, err := Project(s); err != nil {
			t.Fatalf("project after sweep %d: %v", i, err)
		}
	}

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open bindings = %d, want 1 -- a duplicate claim was left behind", n)
	}
	var basis string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &basis)
	if basis != "tmux_inventory" {
		t.Errorf("basis = %q, want tmux_inventory -- verification was undone by re-observation", basis)
	}
}

// The projector must be a function of the log. If it probes during replay, a
// closure that was once withheld is applied retroactively -- and a still-live
// agent acquires an ended parent it can never be reopened from.
func TestReplayUsesTheRecordedProbeNotThePresent(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// The record vanishes, but the sweep observed the process still alive, so
	// the closure is withheld.
	os.Remove(filepath.Join(dir, fmt.Sprintf("%d.json", deadPID)))
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Fatalf("open instances = %d, want 1 -- the closure should have been withheld", n)
	}

	before := snapshotProjections(t, s.DB())

	// By the time of the rebuild the process really is gone. The replay must
	// not notice: it is replaying what was observed then, not what is true now.
	s.probe = deadPIDs(deadPID)
	if _, err := Rebuild(s); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Error("the rebuild closed an instance the original pass had deliberately left open")
	}
	if after := snapshotProjections(t, s.DB()); after != before {
		t.Errorf("replay is not a function of the log.\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// A record whose process died still sits on disk: the provider leaves no
// tombstone. Without a positive death it would read as live forever.
func TestAnOrphanedRecordClosesAsProcessExited(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// The record is still present in every sweep, unchanged; only the process
	// is gone. It must be unchanged: a record rewritten during the sweep means
	// something was alive to write it, and that contradiction withholds.
	s.probe = deadPIDs(deadPID)
	scanAndProject(t, s, dir)

	var basis string
	mustScan(t, s.DB(), `SELECT end_basis FROM launch_instance WHERE ended_ms IS NOT NULL`, &basis)
	if basis != "process_exited" {
		t.Errorf("basis = %q, want process_exited -- a record on disk is not a running agent", basis)
	}
}

// A sweep with no roster must refuse rather than conclude the estate emptied.
func TestAScanEventWithoutARosterIsRefused(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	mustExec(t, s.DB(), `INSERT INTO event (source_id, dedupe_key, kind, observed_ms, payload)
		VALUES (?, 'hand-written', 'scan.completed', 9999, '{"records_seen":0,"source_id":"claude-sessions","host":"clavain"}')`,
		SourceClaudeSessions)
	res, err := Project(s)
	if err != nil {
		t.Fatalf("a malformed event must not abort the batch: %v", err)
	}
	if len(res.Refusals) != 1 || !strings.Contains(res.Refusals[0], "empty estate") {
		t.Errorf("refusals = %v, want one saying what it refused to conclude", res.Refusals)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Errorf("open instances = %d, want 1 -- the refused event closed something", n)
	}
	// The cursor advances past it. Wedging there instead would freeze the
	// live list while every later pass failed at the same event, and the
	// frozen list would still be presented as current.
	var through, maxEvent int64
	mustScan(t, s.DB(), `SELECT applied_through_event_id FROM projection_state`, &through)
	mustScan(t, s.DB(), `SELECT MAX(event_id) FROM event`, &maxEvent)
	if through != maxEvent {
		t.Errorf("cursor at %d, log at %d -- one malformed event wedged the projector", through, maxEvent)
	}
	// And the refusal is visible on the projector's own source.
	var lastErr sql.NullString
	mustScan(t, s.DB(), `SELECT last_error FROM source WHERE source_id = ?`, &lastErr, SourceAttribution)
	if !lastErr.Valid || !strings.Contains(lastErr.String, "empty estate") {
		t.Errorf("projector last_error = %v, want the refusal where an operator would see it", lastErr)
	}
}

// A sweep has standing only over what it watches. Another source's agents are
// not absent merely because they were never in scope.
func TestASweepDoesNotJudgeAnotherSourcesInstances(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "mine", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// A second producer, with an agent of its own.
	mustExec(t, s.DB(), `INSERT INTO source (source_id, host, kind, locator, status, last_success_ms)
		VALUES ('codex-rollouts','clavain','session_file','~/.codex/sessions','ok',1)`)
	res := mustExec(t, s.DB(), `INSERT INTO event (source_id, dedupe_key, kind, observed_ms)
		VALUES ('codex-rollouts','c1','session.observed',1)`)
	ev, _ := res.LastInsertId()
	other := InstanceID("clavain", "darwin", 777, 777)
	mustExec(t, s.DB(), `INSERT INTO launch_instance
		(instance_id, host, pid_domain, pid, started_ms, first_event_id, last_event_id)
		VALUES (?, 'clavain', 'darwin', 777, 777, ?, ?)`, other, ev, ev)

	// A complete Claude sweep that has never heard of it, with everything it
	// does know reported dead.
	s.probe = deadPIDs(100, 777)
	os.Remove(filepath.Join(dir, "100.json"))
	scanAndProject(t, s, dir)

	var ended sql.NullInt64
	mustScan(t, s.DB(), `SELECT ended_ms FROM launch_instance WHERE instance_id = ?`, &ended, other)
	if ended.Valid {
		t.Error("a Claude sweep closed a Codex agent it never had in scope")
	}
}

// A false closure of a QUIET agent must be recoverable. Recovery used to
// depend on the agent rewriting its record -- and an unchanged record is
// deduped, emits no event, and a closed instance was dropped from the probe
// set entirely. The quietest agent on this estate is the one waiting for an
// answer, which is the exact agent this system exists to surface.
func TestAQuietAgentRecoversFromAFalseClosure(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// A false death. The record stays exactly as it is from here on: the
	// agent is alive, and simply has nothing to say.
	s.probe = deadPIDs(deadPID)
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NOT NULL`); n != 1 {
		t.Fatalf("expected a closure to recover from")
	}

	// The probe starts working again. Nothing else changes.
	s.probe = allAlive
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Error("a quiet agent never recovered: recovery still waits on it writing something")
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM instance_conversation WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open conversation links = %d, want 1 -- the retracted closure still holds them shut", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open bindings = %d, want 1", n)
	}
}

// A /clear that happens while an instance is wrongly closed must still record
// its lineage. It used to be lost: with no open link to close, the succession
// edge was never written, and no later pass could recover it.
func TestLineageSurvivesAClearDuringAFalseClosure(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	s.probe = deadPIDs(deadPID)
	scanAndProject(t, s, dir)

	// The agent was alive all along, and has just been /clear'ed.
	s.probe = allAlive
	writeRecord(t, dir, deadPID, record(deadPID, "sess-b", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 3000))
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation_lineage WHERE relation = 'clear'`); n != 1 {
		t.Errorf("clear lineage edges = %d, want 1 -- the succession was lost to a retracted closure", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM instance_conversation WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open links = %d, want 1", n)
	}
}

// A sweep whose liveness instrument did not answer is degraded, not healthy.
// All-unknown from a working probe and all-unknown from one that never ran are
// different facts, and the second used to be invisible.
func TestASweepWithADeadProbeIsDegraded(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, deadPID, record(deadPID, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	s.probe = func(targets []probeTarget) map[string]string {
		out := map[string]string{}
		for _, t := range targets {
			out[t.instanceID] = ProbeUnknown
		}
		return out
	}
	res := scanAndProject(t, s, dir)
	if !res.Complete {
		t.Error("the sweep itself completed; only its probe did not")
	}
	if res.ProbeOK {
		t.Error("ProbeOK should be false when nothing came back")
	}
	var status string
	var lastErr sql.NullString
	mustScan(t, s.DB(), `SELECT status FROM source WHERE source_id = ?`, &status, SourceClaudeSessions)
	mustScan(t, s.DB(), `SELECT last_error FROM source WHERE source_id = ?`, &lastErr, SourceClaudeSessions)
	if status != "degraded" {
		t.Errorf("source status = %q, want degraded", status)
	}
	if !lastErr.Valid || !strings.Contains(lastErr.String, "probe") {
		t.Errorf("last_error = %v, want it to name the instrument that failed", lastErr)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Error("a degraded sweep closed something")
	}
}

// A projection-only version bump must not force an operator to delete the
// file, which is where the only authority lives.
func TestMigrateReplaysRatherThanDiscarding(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 100, record(100, "sess-a", "proj:@1.%1", "/Users/sma/projects/autarch", "a", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	before := snapshotProjections(t, s.DB())
	events := count(t, s.DB(), `SELECT COUNT(*) FROM event`)

	// Pretend this database was written by the previous build.
	mustExec(t, s.DB(), fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion-1))

	from, res, err := Migrate(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if from != SchemaVersion-1 {
		t.Errorf("migrated from v%d, want v%d", from, SchemaVersion-1)
	}
	if res.Applied == 0 {
		t.Error("migration applied no events; it did not replay")
	}
	if after := count(t, s.DB(), `SELECT COUNT(*) FROM event`); after != events {
		t.Errorf("events after migration = %d, want %d -- the log was touched", after, events)
	}
	if after := snapshotProjections(t, s.DB()); after != before {
		t.Errorf("projections differ after a projection-only migration:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	var version int
	mustScan(t, s.DB(), "PRAGMA user_version", &version)
	if version != SchemaVersion {
		t.Errorf("version after migration = %d, want %d", version, SchemaVersion)
	}
}

// Measured on Clavain 2026-09-19: `claude --continue` starts a new process
// with a new pid and a new startedAt, carrying the SAME sessionId. So a
// resume is a new launch instance under a continuing conversation -- which is
// the case the three-identity split exists for, and the reason a conversation
// cannot be keyed on a process.
//
// It also settles the pid-reuse worry: because the pid changes too, the old
// instance is never held open by a living process wearing its number.
func TestAResumeIsANewInstanceOnTheSameConversation(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	const session = "e20f9bd7-2eb9-4054-9c36-3900936cebfe"

	writeRecord(t, dir, 43066, record(43066, session, "clrtest:@114.%114", "/private/tmp", "tmp-80", "derived", 1789858817805, 2000))
	scanAndProject(t, s, dir)

	// The process exits and is resumed in a new one.
	s.probe = deadPIDs(43066)
	os.Remove(filepath.Join(dir, "43066.json"))
	writeRecord(t, dir, 40039, record(40039, session, "restest:@116.%116", "/private/tmp", "tmp-80", "derived", 1789861998634, 3000))
	scanAndProject(t, s, dir)

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM conversation`); n != 1 {
		t.Errorf("conversations = %d, want 1 -- a resume forked the conversation", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance`); n != 2 {
		t.Errorf("instances = %d, want 2 -- a resume is a new process", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Errorf("open instances = %d, want 1 -- the old process was observed dead", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM instance_conversation WHERE conversation_id = ?`,
		ConversationID("claude", "clavain", session)); n != 2 {
		t.Error("the conversation should be linked to both processes that ran it")
	}
	// The pane moved with it, and the old binding closed with its instance.
	var pane string
	mustScan(t, s.DB(), `SELECT pane_id FROM pane_binding WHERE observed_to_ms IS NULL`, &pane)
	if pane != "%116" {
		t.Errorf("open binding on pane %s, want %%116", pane)
	}
}
