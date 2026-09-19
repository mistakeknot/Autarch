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
	db, err := registry.Open(dbPath)
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
		if scanErr != nil {
			return scanErr
		}
		if projErr != nil {
			return projErr
		}
		return tmuxErr

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
		return fmt.Errorf("unknown command %q (scan, rebuild, status)", cmd)
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
	return nil
}
