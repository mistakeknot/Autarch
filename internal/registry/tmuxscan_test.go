package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// fakeTmux answers the inventory with a fixed reply, so the paths that matter
// most -- an unreachable server and an empty answer -- can be exercised
// without one.
type fakeTmux struct {
	out string
	err error
}

func (f fakeTmux) Run(_ context.Context, _ io.Reader, _ ...string) ([]byte, error) {
	return []byte(f.out), f.err
}

const fakeSocket = "/tmp/registry-test-socket"

func paneLine(sessionID, windowID, paneID string, panePID int64, sessionName, command string) string {
	return strings.Join([]string{
		fakeSocket, "1691", "100", sessionID, windowID, paneID,
		fmt.Sprint(panePID), "0", sessionName, "win", "1000", "/Users/sma/projects", command,
	}, "\x1f")
}

// The transport folds "no server running" into an empty success, so an empty
// inventory reads exactly like an empty estate. It is not one.
func TestAnEmptyInventoryIsARefusalNotAnEmptyEstate(t *testing.T) {
	s := newStore(t)
	res, err := ScanTmuxPanesWith(s, fakeSocket, fakeTmux{out: ""})
	if err == nil {
		t.Fatal("an inventory that returned no panes must be an error, not a complete sweep of nothing")
	}
	if res.Complete {
		t.Error("an unreached server must never produce a complete sweep")
	}
	var status string
	mustScan(t, s.DB(), `SELECT status FROM source WHERE source_id = ?`, &status, SourceTmuxInventory)
	if status != "error" {
		t.Errorf("source status = %q, want error", status)
	}
}

func TestAClaimIsUpgradedInPlaceNotReplaced(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	var beforeID int64
	var beforeKey, beforeBasis string
	mustScan(t, s.DB(), `SELECT binding_id FROM pane_binding`, &beforeID)
	mustScan(t, s.DB(), `SELECT pane_key FROM pane_binding`, &beforeKey)
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &beforeBasis)
	if beforeBasis != "session_file_claim" || beforeKey != ClaimedPaneKey("%67") {
		t.Fatalf("expected a claim before verification, got %s / %s", beforeBasis, beforeKey)
	}

	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten - 74e5", "2.1.278")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	var afterID int64
	var afterKey, afterBasis string
	mustScan(t, s.DB(), `SELECT binding_id FROM pane_binding`, &afterID)
	mustScan(t, s.DB(), `SELECT pane_key FROM pane_binding`, &afterKey)
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &afterBasis)

	if afterID != beforeID {
		t.Errorf("binding id changed %d -> %d: verification invented a discontinuity the agent never experienced", beforeID, afterID)
	}
	if afterBasis != "tmux_inventory" {
		t.Errorf("basis after verification = %q, want tmux_inventory", afterBasis)
	}
	want := fakeSocket + "/1691/100/%67/36230"
	if afterKey != want {
		t.Errorf("pane_key after verification = %q, want %q", afterKey, want)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding`); n != 1 {
		t.Errorf("bindings after verification = %d, want 1 -- the claim was replaced rather than upgraded", n)
	}
}

func TestAFailedTmuxReadDoesNotDemoteVerifiedBindings(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "2.1.278")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	before := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE binding_basis = 'tmux_inventory' AND observed_to_ms IS NULL`)
	if before != 1 {
		t.Fatalf("verified bindings before = %d, want 1", before)
	}

	if _, err := ScanTmuxPanesWith(s, fakeSocket, fakeTmux{err: errors.New("connection refused")}); err == nil {
		t.Fatal("expected the failed read to surface")
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project after a failed read: %v", err)
	}

	if after := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE binding_basis = 'tmux_inventory' AND observed_to_ms IS NULL`); after != before {
		t.Errorf("verified bindings after a failed read = %d, want %d", after, before)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NOT NULL`); n != 0 {
		t.Errorf("%d bindings were closed by a read that failed", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NULL`); n != 1 {
		t.Errorf("open instances after a failed read = %d, want 1", n)
	}
}

// Both occupants of a shared pane are verified, and neither evicts the other.
func TestVerificationKeepsBothOccupantsOfOnePane(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	pane := "iterm[]linsekasten:@67.%67"
	writeRecord(t, dir, 52620, record(52620, "74e5950e", pane, "/Users/sma/projects", "parent", "auto", 1000, 2000))
	writeRecord(t, dir, 37995, record(37995, "90057d0b", pane, "/Users/sma/projects/linsenkasten", "child", "derived", 1100, 2100))
	scanAndProject(t, s, dir)

	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten - 74e5", "2.1.278")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding
		WHERE observed_to_ms IS NULL AND binding_basis = 'tmux_inventory'`); n != 2 {
		t.Errorf("verified bindings on the shared pane = %d, want 2", n)
	}
	// They share a pane_key, which is correct: it identifies the pane, not
	// the occupant. The unique index is per (instance, pane), not per pane.
	if n := count(t, s.DB(), `SELECT COUNT(DISTINCT pane_key) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("distinct pane keys = %d, want 1", n)
	}
}

// A conversation arriving in a pane that has not changed since the last
// inventory must still be verified. The inventory dedupes unchanged panes, so
// waiting for a new pane event would leave it claimed indefinitely -- an
// absence of new data read as an absence of fact.
func TestAClaimInAQuietPaneIsVerifiedFromWhatIsAlreadyKnown(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	// The pane is inventoried first, while nothing is in it.
	if _, err := ScanTmuxPanesWith(s, fakeSocket,
		fakeTmux{out: paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh")}); err != nil {
		t.Fatalf("tmux scan: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}

	// A conversation appears in it. No new pane event will follow, because
	// the pane itself is unchanged.
	writeRecord(t, dir, 81453, record(81453, "e13b1e95", "iterm[autarch - e4bedaf5:@98.%98", "/Users/sma/projects", "child", "derived", 1000, 2000))
	scanAndProject(t, s, dir)
	// The next sweep is what verifies it, and the pane is unchanged, so that
	// sweep emits no observation at all: only the roster can do this.
	if res := sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh")); res.Inserted != 0 {
		t.Fatalf("the pane emitted %d observations; this test is only meaningful when it emits none", res.Inserted)
	}

	var basis, key string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &basis)
	mustScan(t, s.DB(), `SELECT pane_key FROM pane_binding`, &key)
	if basis != "tmux_inventory" {
		t.Errorf("binding basis = %q, want tmux_inventory -- the claim never got verified", basis)
	}
	if key != fakeSocket+"/1691/100/%98/53126" {
		t.Errorf("pane_key = %q, still on a claimed key", key)
	}

	// And a second occupant of that same quiet pane lands on the same key, so
	// "how many conversations are in this pane" is answerable at once.
	writeRecord(t, dir, 55409, record(55409, "e4bedaf5", "tmux-organizer:@98.%98", "/Users/sma/projects", "parent", "auto", 1100, 2100))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))
	if n := count(t, s.DB(), `SELECT COUNT(DISTINCT pane_key) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("distinct pane keys = %d, want 1 -- two occupants of one pane landed on different keys", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 2 {
		t.Errorf("open bindings = %d, want 2", n)
	}
}

// A pane that returns to a state it held before must record the return. A
// content hash over all history suppresses it, and the latest recorded
// observation then stays on the intermediate state -- stale data read as
// current by verifyFromLatestObservation.
func TestAPaneRevertingToAPreviousStateIsRecorded(t *testing.T) {
	s := newStore(t)

	stateA := paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh")
	stateB := paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "2.1.278")

	for i, out := range []string{stateA, stateB, stateA} {
		if _, err := ScanTmuxPanesWith(s, fakeSocket, fakeTmux{out: out}); err != nil {
			t.Fatalf("sweep %d: %v", i, err)
		}
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'pane.observed'`); n != 3 {
		t.Errorf("pane observations = %d, want 3 -- the revert was suppressed", n)
	}

	// An unchanged pane still emits nothing.
	if _, err := ScanTmuxPanesWith(s, fakeSocket, fakeTmux{out: stateA}); err != nil {
		t.Fatalf("fourth sweep: %v", err)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'pane.observed'`); n != 3 {
		t.Errorf("pane observations = %d, want 3 -- an unchanged pane emitted an event", n)
	}

	// And the latest observation is the one that is actually current.
	var payload string
	mustScan(t, s.DB(), `SELECT payload FROM event WHERE kind = 'pane.observed' ORDER BY event_id DESC LIMIT 1`, &payload)
	if !strings.Contains(payload, `"command":"zsh"`) {
		t.Errorf("latest observation is stale: %s", payload)
	}
}

// ---------------------------------------------------------------- B1.5

// sweep runs one tmux inventory over the given pane lines and projects it.
func sweep(t *testing.T, s *Store, lines ...string) ScanResult {
	t.Helper()
	res, err := ScanTmuxPanesWith(s, fakeSocket, fakeTmux{out: strings.Join(lines, "\n")})
	if err != nil {
		t.Fatalf("tmux sweep: %v", err)
	}
	if _, err := Project(s); err != nil {
		t.Fatalf("project: %v", err)
	}
	return res
}

// The headline gap B1 shipped with: nothing recorded that a pane was ABSENT.
// The sweep dedupes unchanged panes, so a pane that vanished emitted no event
// at all, and the only thing that could ever close a binding was its instance
// dying. A pane closed by hand left a binding pointing at somewhere that no
// longer existed, and the registry went on presenting it as where that agent
// could be found.
func TestAPaneThatDisappearsClosesItsBinding(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	doomed := paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh")
	survivor := paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh")

	sweep(t, s, doomed, survivor)
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, doomed, survivor)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE binding_basis = 'tmux_inventory' AND observed_to_ms IS NULL`); n != 1 {
		t.Fatalf("verified open bindings before the pane closes = %d, want 1", n)
	}

	// The pane is killed. Nothing else changes.
	sweep(t, s, survivor)

	var basis string
	mustScan(t, s.DB(), `SELECT end_basis FROM pane_binding WHERE pane_id = '%67'`, &basis)
	if basis != "pane_absent_from_complete_scan" {
		t.Errorf("end_basis = %q, want pane_absent_from_complete_scan", basis)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding
		WHERE pane_id = '%67' AND observed_to_ms IS NOT NULL AND end_event_id IS NOT NULL`); n != 1 {
		t.Error("a closed binding must cite the sweep that closed it")
	}
	// The pane died; the process may not have. Those are separate facts and
	// only one of them was observed.
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM launch_instance WHERE ended_ms IS NOT NULL`); n != 0 {
		t.Error("a pane going away is not evidence that the process in it exited")
	}
}

// A pane the sweep never covered is not a pane the sweep found missing. A
// session_file_claim carries no server identity at all -- that is what makes
// it a claim -- so "absent from the server we swept" and "present on a server
// we did not sweep" are the same observation, and closing on it would be the
// exact inference this registry refuses.
func TestAClaimInAnUnsweptPaneStaysClaimed(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "elsewhere:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// And again, so the sweep has had every chance to act on it.
	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))

	var basis, key string
	var closed sql.NullInt64
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding WHERE pane_id = '%67'`, &basis)
	mustScan(t, s.DB(), `SELECT pane_key FROM pane_binding WHERE pane_id = '%67'`, &key)
	mustScan(t, s.DB(), `SELECT observed_to_ms FROM pane_binding WHERE pane_id = '%67'`, &closed)
	if basis != "session_file_claim" {
		t.Errorf("binding basis = %q, want session_file_claim -- nothing verified this pane", basis)
	}
	if key != "claimed:%67" {
		t.Errorf("pane_key = %q, want claimed:%%67", key)
	}
	if closed.Valid {
		t.Error("a sweep that never covered this pane closed its binding anyway")
	}
}

// The reason the roster is load-bearing for verification and not only for
// closure. Before this, "the most recent observation of pane %67" was taken as
// current however old it was -- and because the sweep dedupes, a pane that
// died last week still has a perfectly vivid last observation. A fresh claim
// was verified against it, and the registry asserted that a live agent was
// sitting in a pane that no longer existed.
func TestAClaimIsNotVerifiedAgainstAPaneTheLastSweepDidNotList(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	// %67 existed once and was observed in detail.
	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM event WHERE kind = 'pane.observed'`); n != 1 {
		t.Fatalf("pane observations = %d, want 1", n)
	}
	// It is gone by the next sweep. No new event describes %67; its absence
	// exists only in the roster.
	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))

	// Now an agent claims it.
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	var basis string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding WHERE pane_id = '%67'`, &basis)
	if basis != "session_file_claim" {
		t.Errorf("binding basis = %q, want session_file_claim -- it was verified from an observation of a pane that no longer exists", basis)
	}
}

// The gate: a pane id is unique only within one server. Matching on it alone
// was right by accident on an estate with one socket, and would have bound an
// agent to another machine's pane the moment a second appeared. The claim
// carries no server identity, so the corroboration available is the window id
// the provider wrote in the same breath as the pane id -- measured to agree
// with the live server 11 times out of 11 on 2026-09-19.
func TestAClaimNamingADifferentWindowIsNotVerified(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	// Same pane id, different window: not the pane this record means.
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "somewhere-else:@99.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	var basis, claimedWindow string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &basis)
	mustScan(t, s.DB(), `SELECT claimed_window_id FROM pane_binding`, &claimedWindow)
	if basis != "session_file_claim" {
		t.Errorf("binding basis = %q, want session_file_claim -- a pane id alone verified this binding", basis)
	}
	if claimedWindow != "@99" {
		t.Errorf("claimed_window_id = %q, want @99 -- the disagreement is the evidence and must survive", claimedWindow)
	}

	// A later sweep that does list @99.%67 settles it.
	sweep(t, s, paneLine("$3", "@99", "%67", 36230, "iterm[]linsekasten", "zsh"))
	scanAndProject(t, s, dir)
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &basis)
	if basis != "tmux_inventory" {
		t.Errorf("binding basis = %q, want tmux_inventory once the server agreed", basis)
	}
}

// Measured 2026-09-19: across eleven live records the claimed session name
// disagrees with the live server three times -- 'tmux-organizer' against
// 'iterm[autarch - e4be...', 'iterm]' against 'iterm[]', and one trailing
// space. All three are the same pane. Verification used to overwrite the
// claim with the observation, which destroyed the only evidence the two ever
// differed, and left a column that still looked populated.
func TestVerificationPreservesWhatTheRecordClaimed(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch - e4bedaf5", "zsh"))
	writeRecord(t, dir, 55409, record(55409, "e4bedaf5", "tmux-organizer:@98.%98", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch - e4bedaf5", "zsh"))

	var basis, seen, claimed, window, claimedWindow string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding`, &basis)
	mustScan(t, s.DB(), `SELECT session_name_seen FROM pane_binding`, &seen)
	mustScan(t, s.DB(), `SELECT claimed_session_name FROM pane_binding`, &claimed)
	mustScan(t, s.DB(), `SELECT window_id FROM pane_binding`, &window)
	mustScan(t, s.DB(), `SELECT claimed_window_id FROM pane_binding`, &claimedWindow)

	if basis != "tmux_inventory" {
		t.Fatalf("binding basis = %q -- a session name disagreement must not block verification, it disagrees a quarter of the time", basis)
	}
	if seen != "iterm[autarch - e4bedaf5" {
		t.Errorf("session_name_seen = %q, want what the server said", seen)
	}
	if claimed != "tmux-organizer" {
		t.Errorf("claimed_session_name = %q, want what the record claimed", claimed)
	}
	if window != "@98" || claimedWindow != "@98" {
		t.Errorf("window_id = %q / claimed_window_id = %q, want @98 in both", window, claimedWindow)
	}

	// A later record rewrite refreshes the claim without writing over the
	// observation: the record is the sole author of one and no author at all
	// of the other.
	writeRecord(t, dir, 55409, record(55409, "e4bedaf5", "renamed-by-hand:@98.%98", "/Users/sma/projects", "parent", "auto", 1000, 3000))
	scanAndProject(t, s, dir)
	mustScan(t, s.DB(), `SELECT session_name_seen FROM pane_binding`, &seen)
	mustScan(t, s.DB(), `SELECT claimed_session_name FROM pane_binding`, &claimed)
	if seen != "iterm[autarch - e4bedaf5" {
		t.Errorf("session_name_seen = %q; a session record wrote over what the live server observed", seen)
	}
	if claimed != "renamed-by-hand" {
		t.Errorf("claimed_session_name = %q, want the newer claim", claimed)
	}
}

// A pane id that survives while its root process does not is a respawn, not a
// disappearance. The enum has named it since v3 with nothing to write it.
func TestARespawnedPaneClosesAsPanePidChanged(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))

	sweep(t, s, paneLine("$3", "@67", "%67", 99001, "iterm[]linsekasten", "zsh"))

	var basis string
	mustScan(t, s.DB(), `SELECT end_basis FROM pane_binding WHERE observed_to_ms IS NOT NULL`, &basis)
	if basis != "pane_pid_changed" {
		t.Errorf("end_basis = %q, want pane_pid_changed", basis)
	}
}

// The rule the whole registry turns on, at the pane level: an inventory that
// failed is not an inventory that came back empty. A sweep that cannot reach
// the server publishes no roster, so nothing authorises any absence.
func TestAFailedSweepClosesNoBinding(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))

	for _, broken := range []fakeTmux{{out: ""}, {err: errors.New("no server running")}, {out: "garbage\x1fnot-a-pane"}} {
		_, _ = ScanTmuxPanesWith(s, fakeSocket, broken)
		if _, err := Project(s); err != nil {
			t.Fatalf("project: %v", err)
		}
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NOT NULL`); n != 0 {
		t.Error("a sweep that could not look closed a binding anyway")
	}
	var present sql.NullInt64
	mustScan(t, s.DB(), `SELECT last_present_ms FROM pane_binding`, &present)
	if !present.Valid {
		t.Error("the binding lost the presence the last GOOD sweep confirmed")
	}
}

// Presence is recorded only where it was observed, and only for a binding
// that names the server that observed it. A claim has no server, so it has no
// presence -- and saying otherwise would be the registry reporting it had
// confirmed something it never looked at.
func TestOnlyAVerifiedBindingCarriesPresence(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))
	writeRecord(t, dir, 55409, record(55409, "e4bedaf5", "iterm[autarch:@98.%98", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "elsewhere:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"))

	var verified, claimed sql.NullInt64
	mustScan(t, s.DB(), `SELECT last_present_ms FROM pane_binding WHERE pane_id = '%98'`, &verified)
	mustScan(t, s.DB(), `SELECT last_present_ms FROM pane_binding WHERE pane_id = '%67'`, &claimed)
	if !verified.Valid {
		t.Error("a verified binding the sweep listed carries no presence timestamp")
	}
	if claimed.Valid {
		t.Error("a claim nothing has looked at was recorded as present")
	}
}

// ---------------------------------------------------------------- review B1.5

// Verification happened exactly once, when a claim was first inserted. The
// update branch never retried, so a claim refused on a window disagreement
// stayed refused after the record corrected itself -- the pane had not
// changed, so no observation was coming to retrigger anything, and a quiet
// pane is where a waiting agent sits.
func TestAStuckClaimIsRetriedAgainstALaterRoster(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	pane := paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh")

	sweep(t, s, pane)
	// The record names the wrong window, so the claim is refused.
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@99.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, pane)

	var basis string
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding WHERE pane_id = '%67'`, &basis)
	if basis != "session_file_claim" {
		t.Fatalf("binding basis = %q; a claim naming another window must not verify", basis)
	}

	// The record corrects itself. This takes claimPane's UPDATE branch, which
	// never verified anything, and the pane is unchanged so no observation
	// follows. Only a roster-driven retry can reach it.
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 3000))
	scanAndProject(t, s, dir)
	res := sweep(t, s, pane)
	if res.Inserted != 0 {
		t.Fatalf("the pane emitted %d observations; this test is only meaningful when it emits none", res.Inserted)
	}
	mustScan(t, s.DB(), `SELECT binding_basis FROM pane_binding WHERE pane_id = '%67'`, &basis)
	if basis != "tmux_inventory" {
		t.Errorf("binding basis = %q, want tmux_inventory -- the claim had one chance and missed it", basis)
	}
}

// A verified binding froze at the moment of verification, because verifyPane
// only ever updated rows still claiming. After a pane moves window the row
// kept the old one indefinitely while the roster went on refreshing
// last_present_ms -- "confirmed present 30 seconds ago" beside a window the
// pane left days ago, and the window is what an exposure would join on.
func TestAVerifiedBindingFollowsItsPaneToANewWindow(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))

	// break-pane: same pane, same root process, new window and session.
	sweep(t, s, paneLine("$8", "@200", "%67", 36230, "broken-out", "zsh"))

	var window, session, claimed string
	mustScan(t, s.DB(), `SELECT window_id FROM pane_binding WHERE binding_basis = 'tmux_inventory'`, &window)
	mustScan(t, s.DB(), `SELECT tmux_session_id FROM pane_binding WHERE binding_basis = 'tmux_inventory'`, &session)
	mustScan(t, s.DB(), `SELECT claimed_window_id FROM pane_binding WHERE binding_basis = 'tmux_inventory'`, &claimed)
	if window != "@200" || session != "$8" {
		t.Errorf("window/session = %q/%q, want @200/$8 -- the row froze where it was verified", window, session)
	}
	if claimed != "@67" {
		t.Errorf("claimed_window_id = %q, want @67 -- what the record said is not corrected by the pane moving", claimed)
	}
}

// The ninth instance of the recurring bug, found in review: measuring that the
// window gate admits true claims (11 of 11 on 2026-09-19) says nothing about
// how often it admits false ones, and an estate with one socket cannot sample
// that at all. Window ids are per-server counters exactly as pane ids are.
//
// The process table is a second instrument, and pids are host-unique. Its
// agreement is recorded; its disagreement refuses, because a parent that is
// some other pane's root process is positive evidence of a mismatch.
func TestTheProcessTableCorroboratesAndContradicts(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	here := InstanceID("clavain", "darwin", 55409, 1000)
	elsewhere := InstanceID("clavain", "darwin", 81453, 1100)
	// 53126 is %98's root process; 36230 is %67's. The second agent claims
	// %98, but the process table puts its parent in %67.
	s.probe = aliveWithParents(map[string]int64{here: 53126, elsewhere: 36230})

	sweep(t, s,
		paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"),
		paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 55409, record(55409, "e4bedaf5", "iterm[autarch:@98.%98", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	writeRecord(t, dir, 81453, record(81453, "e13b1e95", "iterm[autarch:@98.%98", "/Users/sma/projects", "impostor", "auto", 1100, 2100))
	// Twice: the first sweep is what gives the probe a target, so parent pids
	// are not known until the second.
	scanAndProject(t, s, dir)
	sweep(t, s,
		paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh"),
		paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))

	var trueBasis, trueCorrob string
	mustScan(t, s.DB(), `SELECT pb.binding_basis FROM pane_binding pb WHERE pb.instance_id = ?`, &trueBasis, here)
	mustScan(t, s.DB(), `SELECT COALESCE(pb.corroborated_by,'') FROM pane_binding pb WHERE pb.instance_id = ?`, &trueCorrob, here)
	if trueBasis != "tmux_inventory" || trueCorrob != "parent_pid" {
		t.Errorf("the true claim = %q/%q, want tmux_inventory/parent_pid", trueBasis, trueCorrob)
	}

	var falseBasis string
	mustScan(t, s.DB(), `SELECT pb.binding_basis FROM pane_binding pb WHERE pb.instance_id = ?`, &falseBasis, elsewhere)
	if falseBasis != "session_file_claim" {
		t.Errorf("a claim whose parent is another pane's root process verified anyway (%q)", falseBasis)
	}
}

// A claim refused on a window disagreement was the quietest of the four
// silences: claimPane discarded the count, so on the path that produces most
// of them nothing was said anywhere. And every successful pass overwrote
// source.last_error, while one sweep projects twice -- so a refusal from the
// first pass survived for milliseconds.
func TestARefusalOutlivesThePassThatMadeIt(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "somewhere-else:@99.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))

	if n := count(t, s.DB(), `SELECT refusal_count FROM projection_state WHERE projection = 'registry'`); n == 0 {
		t.Fatal("the refusal was not recorded anywhere that outlives the pass")
	}
	var last string
	mustScan(t, s.DB(), `SELECT COALESCE(last_refusal,'') FROM projection_state WHERE projection = 'registry'`, &last)
	if !strings.Contains(last, "different window") {
		t.Errorf("last_refusal = %q, want the window disagreement", last)
	}

	// Several more clean passes. The count must not be erased by success.
	for i := 0; i < 3; i++ {
		sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	}
	if n := count(t, s.DB(), `SELECT refusal_count FROM projection_state WHERE projection = 'registry'`); n == 0 {
		t.Error("a later successful pass erased the record that the projector had refused something")
	}
}

// After a pane was watched to vanish, every rewrite of the record opened a
// fresh claim on it, and that claim stayed open until the instance ended -- a
// row saying an agent is in a pane the registry had itself recorded as gone.
func TestAVanishedPaneIsNotReclaimed(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	doomed := paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh")
	survivor := paneLine("$3", "@98", "%98", 53126, "iterm[autarch", "zsh")

	sweep(t, s, doomed, survivor)
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)
	sweep(t, s, doomed, survivor)
	sweep(t, s, survivor)

	// The agent rewrites its record, still naming the pane it was started in.
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 3000))
	res := scanAndProject(t, s, dir)
	_ = res
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE pane_id = '%67' AND observed_to_ms IS NULL`); n != 0 {
		t.Errorf("open bindings on the vanished pane = %d, want 0", n)
	}
	if n := count(t, s.DB(), `SELECT refusal_count FROM projection_state WHERE projection = 'registry'`); n == 0 {
		t.Error("refusing to reclaim a vanished pane was not recorded")
	}

	// And if the pane comes back, it is a place again.
	sweep(t, s, doomed, survivor)
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 4000))
	scanAndProject(t, s, dir)
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE pane_id = '%67' AND observed_to_ms IS NULL`); n != 1 {
		t.Errorf("open bindings after the pane returned = %d, want 1", n)
	}
}

// remain-on-exit keeps a pane listed at the pid of the process that exited.
// Present, and not a place anything is running. Recorded as its own fact:
// folding it into "absent" would close a binding on a pane that is still
// there, and folding it into "present" is what makes a dead pane read as live.
func TestADeadPaneIsRecordedAsDeadAndStillPresent(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	sweep(t, s, paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"))
	writeRecord(t, dir, 52620, record(52620, "74e5950e", "iterm[]linsekasten:@67.%67", "/Users/sma/projects", "parent", "auto", 1000, 2000))
	scanAndProject(t, s, dir)

	// The same line with the dead flag set.
	dead := strings.Replace(paneLine("$3", "@67", "%67", 36230, "iterm[]linsekasten", "zsh"),
		"\x1f0\x1f", "\x1f1\x1f", 1)
	sweep(t, s, dead)

	var payload string
	mustScan(t, s.DB(), `SELECT payload FROM event WHERE kind = 'scan.completed'
		AND source_id = 'tmux-inventory' ORDER BY event_id DESC LIMIT 1`, &payload)
	if !strings.Contains(payload, `"%67:36230:dead"`) {
		t.Errorf("roster does not record the pane as dead: %s", payload)
	}
	var closed sql.NullInt64
	mustScan(t, s.DB(), `SELECT observed_to_ms FROM pane_binding WHERE pane_id = '%67'`, &closed)
	if closed.Valid {
		t.Error("a pane that is still listed, merely dead, had its binding closed")
	}
}
