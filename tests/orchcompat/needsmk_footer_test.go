package orchcompat

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const footerSchema = `
CREATE TABLE settings_kv (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE cards (task_id TEXT PRIMARY KEY, card_key TEXT, title TEXT, state TEXT, deleted_at TEXT);
CREATE TABLE picks (decision_id TEXT PRIMARY KEY, pick_id TEXT, option_id TEXT, picked_at TEXT, reason TEXT, "by" TEXT NOT NULL DEFAULT 'mk');
CREATE TABLE obligations (id TEXT PRIMARY KEY, decision_id TEXT, kind TEXT, state TEXT, recipient TEXT, voided_at TEXT, last_error TEXT);
`

func footerDB(t *testing.T, e *env, stmts ...string) string {
	t.Helper()
	path := filepath.Join(e.root, "home-data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range append([]string{footerSchema}, stmts...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

var footerFixture = []string{
	`INSERT INTO cards VALUES ('t1','ZZ-2','Second','open',NULL),('t2','ZZ-1','First','open',NULL),('t3','ZZ-3','Parked','open',NULL),
		('t4','ZZ-4','Ruled parked','ruled',NULL),('t5','ZZ-5','Gone','open','2026-10-01T00:00:00Z'),('t6','ZZ-6','Closed','closed',NULL),
		('t7','ZZ-7','Ruled','ruled',NULL),('t8','ZZ-8','Auto','ruled',NULL)`,
	`INSERT INTO settings_kv VALUES ('later','{"t3":{"by":"mk"},"t4":{"by":"mk"}}')`,
	`INSERT INTO picks(decision_id,pick_id,option_id,picked_at,reason) VALUES ('card-t7-g1','p1','run','2026-10-10T10:00:00.000Z','line one
line two')`,
	`INSERT INTO picks(decision_id,pick_id,option_id,picked_at,reason,"by") VALUES ('card-t8-g1','p2','merged','2026-10-11T10:00:00.000Z',NULL,'auto')`,
	`INSERT INTO picks(decision_id,pick_id,option_id,picked_at,reason) VALUES ('card-t6-g1','p3','old','2026-10-01T00:00:00.000Z',NULL)`,
	`INSERT INTO obligations VALUES ('o1','card-t7-g1','notice','queued','thr_x',NULL,NULL),('o2','card-t7-g1','wake','done','thr_x',NULL,NULL),
		('o3','card-t7-g1','wake','dismissed','thr_x',NULL,NULL),('o4','card-t7-g1','wake','pending','thr_y','2026-10-02T00:00:00Z',NULL),
		('o5','orphan','ruling-file','pending',NULL,NULL,'boom')`,
}

const footerBody = "To decide (Home): ZZ-1 (First); ZZ-2 (Second)\n" +
	"Parked (Later): ZZ-3 ZZ-4\n"

const footerTail = "Undelivered obligations:\n" +
	"  notice queued -> thr_x (ZZ-7) \n" +
	"  ruling-file pending -> - (orphan) boom\n"

func TestFooterPrintsFourSections(t *testing.T) {
	e := newEnv(t)
	db := footerDB(t, e, footerFixture...)
	cur := filepath.Join(e.root, "cur")
	if err := os.WriteFile(cur, []byte("2026-10-05T00:00:00.000Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := e.autarch("needs-mk", "footer", "--db", db, "--cursor", cur, "--peek")
	wantCode(t, r, 0)
	want := footerBody +
		"New picks since 2026-10-05T00:00:00.000Z:\n" +
		"  ZZ-7 -> run at 2026-10-10T10:00:00.000Z | line one line two\n" +
		"  ZZ-8 -> merged at 2026-10-11T10:00:00.000Z [closed automatically on evidence, not by mk]\n" +
		footerTail
	if r.Stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.Stdout, want)
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q", r.Stderr)
	}
	if b, _ := os.ReadFile(cur); string(b) != "2026-10-05T00:00:00.000Z\n" {
		t.Errorf("--peek moved the cursor: %q", b)
	}
}

func TestFooterWithoutACursorFileShowsEveryPick(t *testing.T) {
	e := newEnv(t)
	db := footerDB(t, e, footerFixture...)
	r := e.autarch("needs-mk", "footer", "--db", db, "--cursor", filepath.Join(e.root, "none"), "--peek")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, "New picks since 1970-01-01T00:00:00Z:\n") || !strings.Contains(r.Stdout, "  ZZ-6 -> old at 2026-10-01T00:00:00.000Z\n") {
		t.Errorf("stdout:\n%s", r.Stdout)
	}
}

func TestFooterEmptyDatabase(t *testing.T) {
	e := newEnv(t)
	db := footerDB(t, e)
	r := e.autarch("needs-mk", "footer", "--db", db, "--peek")
	wantCode(t, r, 0)
	want := "To decide (Home): \nParked (Later): \nNew picks since 1970-01-01T00:00:00Z:\nUndelivered obligations:\n"
	if r.Stdout != want {
		t.Errorf("stdout = %q, want %q", r.Stdout, want)
	}
}

var stampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z\n$`)

func TestFooterMovesThePerCallerCursor(t *testing.T) {
	e := newEnv(t)
	db := footerDB(t, e, footerFixture...)
	cursorFile := filepath.Join(e.state, "autarch", "needs-mk-footer", "thr_a.cursor")

	first := e.autarch("needs-mk", "footer", "--db", db)
	wantCode(t, first, 0)
	if !strings.Contains(first.Stdout, "New picks since 1970-01-01T00:00:00Z:\n") || !strings.Contains(first.Stdout, "  ZZ-7 -> run") {
		t.Errorf("first run:\n%s", first.Stdout)
	}
	b, err := os.ReadFile(cursorFile)
	if err != nil {
		t.Fatalf("cursor file for the thread was not written: %v", err)
	}
	if !stampRe.Match(b) {
		t.Errorf("cursor = %q, want a millisecond UTC stamp and a newline", b)
	}
	if st, _ := os.Stat(cursorFile); st.Mode().Perm()&0o022 != 0 {
		t.Errorf("cursor mode %v is writable by others", st.Mode().Perm())
	}

	second := e.autarch("needs-mk", "footer", "--db", db)
	wantCode(t, second, 0)
	if !strings.Contains(second.Stdout, "New picks since "+strings.TrimSpace(string(b))+":\nUndelivered obligations:\n") {
		t.Errorf("second run still lists picks:\n%s", second.Stdout)
	}
	if !strings.HasPrefix(second.Stdout, footerBody) {
		t.Errorf("second run lost the card sections:\n%s", second.Stdout)
	}
}

func TestFooterCallerSelection(t *testing.T) {
	cursorDir := func(e *env) string { return filepath.Join(e.state, "autarch", "needs-mk-footer") }
	t.Run("--caller names the file", func(t *testing.T) {
		e := newEnv(t)
		db := footerDB(t, e)
		wantCode(t, e.autarch("needs-mk", "footer", "--db", db, "--caller", "vizier.1"), 0)
		if _, err := os.Stat(filepath.Join(cursorDir(e), "vizier.1.cursor")); err != nil {
			t.Error(err)
		}
		if _, err := os.Stat(filepath.Join(cursorDir(e), "thr_a.cursor")); err == nil {
			t.Error("BB_THREAD_ID cursor written although --caller was given")
		}
	})
	t.Run("default when there is no thread", func(t *testing.T) {
		e := newEnv(t)
		e.thread = ""
		db := footerDB(t, e)
		wantCode(t, e.autarch("needs-mk", "footer", "--db", db), 0)
		if _, err := os.Stat(filepath.Join(cursorDir(e), "default.cursor")); err != nil {
			t.Error(err)
		}
	})
	t.Run("two callers do not move each other", func(t *testing.T) {
		e := newEnv(t)
		db := footerDB(t, e, footerFixture...)
		wantCode(t, e.autarch("needs-mk", "footer", "--db", db, "--caller", "a"), 0)
		r := e.autarch("needs-mk", "footer", "--db", db, "--caller", "b")
		if !strings.Contains(r.Stdout, "  ZZ-7 -> run") {
			t.Errorf("caller b did not see the picks:\n%s", r.Stdout)
		}
	})
	t.Run("a bad caller name is a usage error", func(t *testing.T) {
		e := newEnv(t)
		db := footerDB(t, e)
		for _, c := range []string{"a/b", "-x", "a b", ".."} {
			r := e.autarch("needs-mk", "footer", "--db", db, "--caller", c)
			wantCode(t, r, 2, "--caller must be letters, digits, dot, dash or underscore")
			if r.Stdout != "" {
				t.Errorf("caller %q: stdout = %q", c, r.Stdout)
			}
		}
	})
}

func TestFooterDatabaseLookup(t *testing.T) {
	t.Run("AUTARCH_HOME_DB", func(t *testing.T) {
		e := newEnv(t)
		e.vars["AUTARCH_HOME_DB"] = footerDB(t, e, footerFixture...)
		r := e.autarch("needs-mk", "footer", "--peek")
		wantCode(t, r, 0)
		if !strings.HasPrefix(r.Stdout, footerBody) {
			t.Errorf("stdout:\n%s", r.Stdout)
		}
	})
	t.Run("HOME_BB_DATA", func(t *testing.T) {
		e := newEnv(t)
		dir := filepath.Join(e.root, "bbdata", "plugins", "autarch")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		src := footerDB(t, e, footerFixture...)
		raw, _ := os.ReadFile(src)
		if err := os.WriteFile(filepath.Join(dir, "data.db"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		e.vars["HOME_BB_DATA"] = filepath.Join(e.root, "bbdata")
		r := e.autarch("needs-mk", "footer", "--peek")
		wantCode(t, r, 0)
		if !strings.HasPrefix(r.Stdout, footerBody) {
			t.Errorf("stdout:\n%s", r.Stdout)
		}
	})
	t.Run("AUTARCH_HOME_DB wins over HOME_BB_DATA", func(t *testing.T) {
		e := newEnv(t)
		e.vars["AUTARCH_HOME_DB"] = footerDB(t, e)
		e.vars["HOME_BB_DATA"] = filepath.Join(e.root, "nothing")
		wantCode(t, e.autarch("needs-mk", "footer", "--peek"), 0)
	})
	t.Run("--db wins over the environment", func(t *testing.T) {
		e := newEnv(t)
		e.vars["AUTARCH_HOME_DB"] = filepath.Join(e.root, "nothing.db")
		wantCode(t, e.autarch("needs-mk", "footer", "--peek", "--db", footerDB(t, e)), 0)
	})
	t.Run("no database known", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch("needs-mk", "footer", "--peek")
		wantCode(t, r, 2, "no Home data.db known: set AUTARCH_HOME_DB or HOME_BB_DATA, or pass --db")
	})
	t.Run("database file missing", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch("needs-mk", "footer", "--db", filepath.Join(e.root, "nope.db"))
		wantCode(t, r, 1, "footer: home database: stat ", "no such file or directory")
		if _, err := os.Stat(filepath.Join(e.state, "autarch")); err == nil {
			t.Error("a cursor was written although the read failed")
		}
	})
	t.Run("database with the wrong schema", func(t *testing.T) {
		e := newEnv(t)
		p := filepath.Join(e.root, "bad.db")
		db, err := sql.Open("sqlite", p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE x (a TEXT)"); err != nil {
			t.Fatal(err)
		}
		db.Close()
		r := e.autarch("needs-mk", "footer", "--db", p)
		wantCode(t, r, 1, "footer: read ")
	})
}

func TestFooterNeverWritesTheDatabase(t *testing.T) {
	e := newEnv(t)
	db := footerDB(t, e, footerFixture...)
	before, _ := os.ReadFile(db)
	wantCode(t, e.autarch("needs-mk", "footer", "--db", db), 0)
	after, _ := os.ReadFile(db)
	if string(before) != string(after) {
		t.Error("footer changed the database file")
	}
}
