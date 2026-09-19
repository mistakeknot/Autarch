package registry

import (
	"context"
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
	if n := count(t, s.DB(), `SELECT COUNT(DISTINCT pane_key) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 1 {
		t.Errorf("distinct pane keys = %d, want 1 -- two occupants of one pane landed on different keys", n)
	}
	if n := count(t, s.DB(), `SELECT COUNT(*) FROM pane_binding WHERE observed_to_ms IS NULL`); n != 2 {
		t.Errorf("open bindings = %d, want 2", n)
	}
}
