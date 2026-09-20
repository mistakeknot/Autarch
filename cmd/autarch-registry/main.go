// Command autarch-registry runs the estate registry's read-only watcher.
//
// It reads the Claude CLI's published session records, appends what it sees to
// an append-only log, and projects that log into the identity tables. It opens
// nothing for writing outside its own database, holds no lock on the provider's
// files, and touches no configuration.
//
// Intended owner: a launchd agent, com.sma.autarch-registry, running `scan`
// every 30 seconds with KeepAlive false -- each sweep is a complete, idempotent
// unit, so a missed run costs one interval and a crashed run costs nothing. The
// watcher never needs to be running for the log to stay correct; it only needs
// to run again.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/mistakeknot/autarch/internal/registry"
)

func main() {
	dbPath := flag.String("db", defaultDB(), "registry database path")
	dir := flag.String("sessions", registry.DefaultSessionDir(), "Claude session record directory")
	socket := flag.String("tmux-socket", registry.DefaultTmuxSocket(), "tmux server socket to inventory (empty to skip)")
	host := flag.String("host", registry.DefaultHost(), "machine these observations belong to")
	flag.Parse()

	cmd := "scan"
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	if err := run(cmd, *dbPath, *dir, *host, *socket); err != nil {
		fmt.Fprintln(os.Stderr, "autarch-registry:", err)
		os.Exit(1)
	}
}

func defaultDB() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "registry.db"
	}
	return filepath.Join(home, ".autarch", "registry.db")
}

func run(cmd, dbPath, dir, host, socket string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return err
	}
	opener := registry.Open
	if cmd == "migrate" {
		opener = registry.OpenForMigration
	}
	db, err := opener(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	// The log holds provider metadata and, later, transcript spans. Nobody
	// but the owner reads it.
	_ = os.Chmod(dbPath, 0o600)

	store := registry.NewStore(db, host)

	switch cmd {
	case "scan":
		// Sessions first, then panes: a claim has to exist before the live
		// server can verify it. A failure in either is reported and neither
		// stops the other, because a half-swept estate is still more than an
		// unswept one -- and neither sweep can close anything it did not see.
		res, scanErr := registry.ScanClaudeSessions(store, dir)
		fmt.Printf("sessions scan %d: %d records, %d new, %d unchanged, complete=%v\n",
			res.ScanID, res.RecordsSeen, res.Inserted, res.Skipped, res.Complete)
		if len(res.Unparsable) > 0 {
			fmt.Printf("  unreadable: %v\n", res.Unparsable)
		}
		// Closures happen in this first pass, so its result is what carries
		// them. Reporting only the second pass printed a constant zero.
		first, projErr := registry.Project(store)
		if projErr != nil {
			return projErr
		}

		var tmuxErr error
		if socket != "" {
			var panes registry.ScanResult
			panes, tmuxErr = registry.ScanTmuxPanes(store, socket)
			if tmuxErr != nil {
				fmt.Printf("tmux scan: %v (nothing demoted)\n", tmuxErr)
			} else {
				fmt.Printf("tmux scan %d: %d panes, %d new, %d unchanged, complete=%v\n",
					panes.ScanID, panes.RecordsSeen, panes.Inserted, panes.Skipped, panes.Complete)
			}
		}

		second, projErr := registry.Project(store)
		fmt.Printf("projected %d events, %d instances closed\n",
			first.Applied+second.Applied, first.Closed+second.Closed)
		// A projector that declines to act on an event must say so here.
		// Silently skipping is how a frozen live list goes on being presented
		// as current.
		for _, r := range append(append([]string{}, first.Refusals...), second.Refusals...) {
			fmt.Printf("  refused: %s\n", r)
		}
		if scanErr != nil {
			return scanErr
		}
		if projErr != nil {
			return projErr
		}
		return tmuxErr

	case "migrate":
		from, proj, err := registry.Migrate(store)
		if err != nil {
			return err
		}
		if from == registry.SchemaVersion {
			fmt.Printf("already at schema v%d; nothing to do\n", from)
			return nil
		}
		fmt.Printf("migrated schema v%d -> v%d by replaying the log: %d events applied\n",
			from, registry.SchemaVersion, proj.Applied)
		return nil

	case "rebuild":
		proj, err := registry.Rebuild(store)
		if err != nil {
			return err
		}
		fmt.Printf("rebuilt from the log: %d events applied\n", proj.Applied)
		return nil

	case "status":
		return status(store)

	default:
		return fmt.Errorf("unknown command %q (scan, status, rebuild, migrate)", cmd)
	}
}

// status prints coverage before content. A producer's liveness is what makes
// the numbers beneath it mean anything: an unchecked source and an empty
// estate look identical unless the difference is stated first.
func status(s *registry.Store) error {
	db := s.DB()

	fmt.Println("SOURCES")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  id\thost\tstate\tlast success\tscans (complete)")
	rows, err := db.Query(`SELECT s.source_id, s.host, s.status, s.last_success_ms, s.expected_interval_ms,
		(SELECT COUNT(*) FROM source_scan WHERE source_id = s.source_id),
		(SELECT COUNT(*) FROM source_scan WHERE source_id = s.source_id AND complete = 1)
		FROM source s ORDER BY s.source_id`)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for rows.Next() {
		var id, host, st string
		var last, interval sql.NullInt64
		var scans, complete int
		if err := rows.Scan(&id, &host, &st, &last, &interval, &scans, &complete); err != nil {
			rows.Close()
			return err
		}
		// A producer that last succeeded an hour ago is not healthy just
		// because its last run succeeded. Under launchd, a job that stopped
		// firing is the main way a failure goes unnoticed, and without this
		// the row would read ok forever.
		state, age := st, "never"
		if last.Valid {
			ageMs := now - last.Int64
			age = fmt.Sprintf("%ds ago", ageMs/1000)
			if st == "ok" && interval.Valid && interval.Int64 > 0 && ageMs > 3*interval.Int64 {
				state = fmt.Sprintf("stale (%dx interval)", ageMs/interval.Int64)
			}
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%d (%d)\n", id, host, state, age, scans, complete)
	}
	rows.Close()
	w.Flush()

	fmt.Println("\nLIVE CONVERSATIONS")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  pane\tpid\tentry\tname\tsource\tcwd\tproject")
	rows, err = db.Query(`
		SELECT COALESCE(pb.pane_id,'-'), li.pid, li.entrypoint,
		       c.display_name, c.display_name_source, li.launch_cwd,
		       COALESCE((SELECT project_key FROM project_association pa
		                 WHERE pa.conversation_id = c.conversation_id AND pa.retracted_ms IS NULL
		                 ORDER BY pa.confidence DESC LIMIT 1), '#unassigned')
		FROM instance_conversation ic
		JOIN launch_instance li ON li.instance_id = ic.instance_id
		JOIN conversation c ON c.conversation_id = ic.conversation_id
		LEFT JOIN pane_binding pb ON pb.instance_id = li.instance_id AND pb.observed_to_ms IS NULL
		WHERE ic.observed_to_ms IS NULL AND li.ended_ms IS NULL
		ORDER BY pb.pane_id, li.pid`)
	if err != nil {
		return err
	}
	live := 0
	for rows.Next() {
		var pane, entry, name, nameSource, cwd, project string
		var pid int64
		if err := rows.Scan(&pane, &pid, &entry, &name, &nameSource, &cwd, &project); err != nil {
			rows.Close()
			return err
		}
		live++
		fmt.Fprintf(w, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\n", pane, pid, entry, name, nameSource, cwd, project)
	}
	rows.Close()
	w.Flush()
	fmt.Printf("\n  %d live\n", live)

	// "Live" means only "not known to have ended". An instance nobody has
	// been able to probe recently is not a running agent; it is a row the
	// instrument has stopped reaching, and saying so is the difference
	// between a status and a guess.
	var unverified int
	if err := db.QueryRow(`SELECT COUNT(*) FROM launch_instance
		WHERE ended_ms IS NULL AND (last_alive_ms IS NULL OR last_alive_ms < ?)`,
		time.Now().UnixMilli()-3*30_000).Scan(&unverified); err != nil {
		return err
	}
	if unverified > 0 {
		fmt.Printf("  %d of them have no liveness confirmation in the last 3 intervals\n", unverified)
	}

	// Panes carrying more than one conversation at once: the case a single
	// pane-to-agent map cannot represent.
	rows, err = db.Query(`SELECT pane_id, COUNT(*) FROM pane_binding
		WHERE observed_to_ms IS NULL GROUP BY pane_key HAVING COUNT(*) > 1`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var pane string
		var n int
		if err := rows.Scan(&pane, &n); err != nil {
			rows.Close()
			return err
		}
		fmt.Printf("  pane %s holds %d conversations at once\n", pane, n)
	}
	rows.Close()

	// Currently unassigned, not ever-failed: a conversation that was
	// attributed later must stop being counted as a gap.
	var unattributed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversation c
		WHERE NOT EXISTS (SELECT 1 FROM project_association pa
		                  WHERE pa.conversation_id = c.conversation_id AND pa.retracted_ms IS NULL)`).
		Scan(&unattributed); err != nil {
		return err
	}
	fmt.Printf("  %d conversations unassigned", unattributed)
	var withReason int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT conversation_id) FROM event
		WHERE kind = 'attribution.attempted'`).Scan(&withReason); err != nil {
		return err
	}
	fmt.Printf(", %d with a recorded reason\n", withReason)

	var verified, claimed int
	if err := db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN binding_basis = 'tmux_inventory' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN binding_basis = 'session_file_claim' THEN 1 ELSE 0 END),0)
		FROM pane_binding WHERE observed_to_ms IS NULL`).Scan(&verified, &claimed); err != nil {
		return err
	}
	fmt.Printf("  %d pane bindings confirmed by the live server, %d still only claimed\n", verified, claimed)

	// "Verified" says a sweep once identified this pane; it does not say the
	// pane is still there. Those were the same sentence until the roster
	// existed, and a binding verified last Tuesday rendered exactly like one
	// confirmed present thirty seconds ago.
	var stalePresence int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pane_binding
		WHERE observed_to_ms IS NULL AND binding_basis = 'tmux_inventory'
		  AND (last_present_ms IS NULL OR last_present_ms < ?)`,
		time.Now().UnixMilli()-3*30_000).Scan(&stalePresence); err != nil {
		return err
	}
	if stalePresence > 0 {
		fmt.Printf("  %d of them were not listed by a complete sweep in the last 3 intervals\n", stalePresence)
	}

	// How many verified bindings a second instrument agrees with. The window
	// gate cannot tell one server from another -- window ids are per-server
	// counters exactly as pane ids are -- so "verified" alone cannot exclude a
	// claim arriving from a server this registry does not sweep. A parent pid
	// that is this pane's own root process is a different instrument reaching
	// the same answer, and pids are host-unique.
	var corroborated int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pane_binding
		WHERE observed_to_ms IS NULL AND corroborated_by IS NOT NULL`).Scan(&corroborated); err != nil {
		return err
	}
	if verified > 0 {
		fmt.Printf("  %d of the %d verified are corroborated by the process table\n", corroborated, verified)
	}

	// An open claim is either young or stuck, and a count alone cannot say
	// which. Its age in complete sweeps can.
	var oldestClaim sql.NullInt64
	if err := db.QueryRow(`SELECT MIN(pb.first_event_id) FROM pane_binding pb
		WHERE pb.observed_to_ms IS NULL AND pb.binding_basis = 'session_file_claim'`).Scan(&oldestClaim); err != nil {
		return err
	}
	if oldestClaim.Valid {
		sweeps := 0
		if err := db.QueryRow(`SELECT COUNT(*) FROM event
			WHERE kind = 'scan.completed' AND source_id = 'tmux-inventory' AND event_id > ?`,
			oldestClaim.Int64).Scan(&sweeps); err != nil {
			return err
		}
		fmt.Printf("  the oldest open claim has survived %d complete pane sweeps\n", sweeps)
	}

	var refusals int
	var lastRefusal sql.NullString
	if err := db.QueryRow(`SELECT refusal_count, last_refusal FROM projection_state
		WHERE projection = 'registry'`).Scan(&refusals, &lastRefusal); err != nil && err != sql.ErrNoRows {
		return err
	}
	if refusals > 0 {
		fmt.Printf("  projector has refused %d event(s); most recently: %s\n", refusals, lastRefusal.String)
	}

	var closedPanes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pane_binding
		WHERE end_basis IN ('pane_absent_from_complete_scan','pane_pid_changed')`).Scan(&closedPanes); err != nil {
		return err
	}
	if closedPanes > 0 {
		fmt.Printf("  %d binding(s) closed because the pane itself went away\n", closedPanes)
	}

	// Parent capture, reported as what it is FOR. A parent pid on its own
	// says little; what it answers is whether a human started this agent or
	// another process did, which is the distinction that separates the two
	// occupants of a shared pane. Coverage is stated too, because a process
	// whose parent was never read is not a process without one -- and for
	// anything already exited that gap can never be filled.
	var liveInstances, withParent, fromPane int
	if err := db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN li.ppid IS NOT NULL THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN li.ppid IS NOT NULL AND li.ppid = pb.pane_pid THEN 1 ELSE 0 END),0)
		FROM launch_instance li
		LEFT JOIN pane_binding pb ON pb.instance_id = li.instance_id AND pb.observed_to_ms IS NULL
		WHERE li.ended_ms IS NULL`).Scan(&liveInstances, &withParent, &fromPane); err != nil {
		return err
	}
	fmt.Printf("  %d of %d live instances have a parent pid: %d launched from their pane's own shell, %d by something else\n",
		withParent, liveInstances, fromPane, withParent-fromPane)

	// How far the projector is behind the log. A projector wedged on one bad
	// event leaves everything above stale while it still reads current.
	var lag int64
	if err := db.QueryRow(`SELECT COALESCE((SELECT MAX(event_id) FROM event),0)
		- COALESCE((SELECT applied_through_event_id FROM projection_state WHERE projection = 'registry'),0)`).
		Scan(&lag); err != nil {
		return err
	}
	if lag != 0 {
		fmt.Printf("  projector is %d events behind the log\n", lag)
	}

	var reopened int
	if err := db.QueryRow(`SELECT COALESCE(SUM(reopened_count),0) FROM launch_instance`).Scan(&reopened); err != nil {
		return err
	}
	if reopened > 0 {
		fmt.Printf("  %d instance closure(s) were contradicted by later proof of life and reopened\n", reopened)
	}
	return nil
}
