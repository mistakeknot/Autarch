package homefooter

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const schema = `
CREATE TABLE settings_kv (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE cards (task_id TEXT PRIMARY KEY, card_key TEXT, title TEXT, state TEXT, deleted_at TEXT);
CREATE TABLE picks (decision_id TEXT PRIMARY KEY, pick_id TEXT, option_id TEXT, picked_at TEXT, reason TEXT);
CREATE TABLE obligations (id TEXT PRIMARY KEY, decision_id TEXT, kind TEXT, state TEXT, recipient TEXT, voided_at TEXT, last_error TEXT);
`

func fixture(t *testing.T, stmts ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range append([]string{schema}, stmts...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

func run(t *testing.T, o Options) string {
	t.Helper()
	var b bytes.Buffer
	if err := Print(&b, o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSectionsOpenParkedPicksObligations(t *testing.T) {
	long := strings.Repeat("é", 70)
	db := fixture(t,
		`INSERT INTO cards VALUES ('t1','AUTA-2','`+long+`','open',NULL),
			('t2','AUTA-1','Decide A','open',NULL),
			('t3','AUTA-3','Parked one','open',NULL),
			('t4','AUTA-4','Parked ruled','ruled',NULL),
			('t5','AUTA-5','Deleted','open','2026-10-01T00:00:00Z'),
			('t6','AUTA-6','Closed','closed',NULL),
			('t7','AUTA-7','Ruled not parked','ruled',NULL)`,
		`INSERT INTO settings_kv VALUES ('later','{"t3":{"by":"mk"},"t4":{"by":"mk"}}')`,
		`INSERT INTO picks VALUES ('card-t7-g1','p1','run','2026-10-10T10:00:00.000Z','line one
line two')`,
		`INSERT INTO picks VALUES ('card-t6-g1','p2','old','2026-10-01T00:00:00.000Z',NULL)`,
		`INSERT INTO obligations VALUES ('o1','card-t7-g1','notice','queued','thr_x',NULL,NULL),
			('o2','card-t7-g1','wake','done','thr_x',NULL,NULL),
			('o3','card-t7-g1','wake','dismissed','thr_x',NULL,NULL),
			('o4','card-t7-g1','wake','pending','thr_y','2026-10-02T00:00:00Z',NULL),
			('o5','orphan-decision','ruling-file','pending',NULL,NULL,'boom')`,
	)
	cur := filepath.Join(t.TempDir(), "c")
	os.WriteFile(cur, []byte("2026-10-05T00:00:00.000Z\n"), 0o644)
	got := run(t, Options{DB: db, Cursor: cur, Peek: true})
	want := "To decide (Home): AUTA-1 (Decide A); AUTA-2 (" + strings.Repeat("é", 60) + ")\n" +
		"Parked (Later): AUTA-3 AUTA-4\n" +
		"New picks since 2026-10-05T00:00:00.000Z:\n" +
		"  AUTA-7 -> run at 2026-10-10T10:00:00.000Z | line one line two\n" +
		"Undelivered obligations:\n" +
		"  notice queued -> thr_x (AUTA-7) \n" +
		"  ruling-file pending -> - (orphan-decision) boom\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestNoLaterSettingAndEmpty(t *testing.T) {
	got := run(t, Options{DB: fixture(t)})
	want := "To decide (Home): \nParked (Later): \nNew picks since 1970-01-01T00:00:00Z:\nUndelivered obligations:\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestCursorMovesToRunStartAndPeekLeavesIt(t *testing.T) {
	db := fixture(t,
		`INSERT INTO cards VALUES ('t1','AUTA-1','x','ruled',NULL)`,
		`INSERT INTO picks VALUES ('card-t1-g1','p1','a','2026-10-10T10:00:00.000Z',NULL)`)
	cur := filepath.Join(t.TempDir(), "sub", "c")
	clock := func() time.Time { return time.Date(2026, 10, 11, 1, 2, 3, 4_000_000, time.UTC) }

	first := run(t, Options{DB: db, Cursor: cur, Now: clock})
	if !strings.Contains(first, "AUTA-1 -> a") {
		t.Fatalf("first run should show the pick:\n%s", first)
	}
	b, err := os.ReadFile(cur)
	if err != nil || strings.TrimSpace(string(b)) != "2026-10-11T01:02:03.003Z" {
		t.Fatalf("cursor = %q, %v", b, err)
	}
	// A peek shows the same footer and does not move the cursor.
	later := func() time.Time { return clock().Add(time.Hour) }
	peek := run(t, Options{DB: db, Cursor: cur, Peek: true, Now: later})
	if strings.Contains(peek, "AUTA-1 -> a") {
		t.Fatalf("pick is older than the cursor:\n%s", peek)
	}
	if b2, _ := os.ReadFile(cur); string(b2) != string(b) {
		t.Fatalf("peek moved the cursor to %q", b2)
	}
	// Another caller's cursor still sees it.
	other := run(t, Options{DB: db, Cursor: filepath.Join(t.TempDir(), "o"), Peek: true})
	if !strings.Contains(other, "AUTA-1 -> a") {
		t.Fatal("a fresh cursor should see every pick")
	}
}

func TestPickOnALaterCardGenerationIsShown(t *testing.T) {
	db := fixture(t,
		`INSERT INTO cards VALUES ('t1','AUTA-1','x','ruled',NULL)`,
		`INSERT INTO picks VALUES ('card-t1-g2','p1','b','2026-10-10T10:00:00.000Z',NULL)`)
	if got := run(t, Options{DB: db, Peek: true}); !strings.Contains(got, "AUTA-1 -> b") {
		t.Fatalf("generation 2 pick missing:\n%s", got)
	}
}

func TestReadOnlyAndMissingDB(t *testing.T) {
	db := fixture(t, `INSERT INTO cards VALUES ('t1','AUTA-1','x','open',NULL)`)
	before, _ := os.ReadFile(db)
	run(t, Options{DB: db})
	after, _ := os.ReadFile(db)
	if !bytes.Equal(before, after) {
		t.Fatal("footer wrote to the database")
	}
	var b bytes.Buffer
	if err := Print(&b, Options{DB: filepath.Join(t.TempDir(), "nope.db")}); err == nil {
		t.Fatal("a missing database must be an error, not an empty footer")
	}
	if b.Len() != 0 {
		t.Fatalf("printed %q before failing", b.String())
	}
}

func TestDefaultDB(t *testing.T) {
	t.Setenv("AUTARCH_HOME_DB", "/x/y.db")
	if p, _ := DefaultDB(); p != "/x/y.db" {
		t.Fatal(p)
	}
	t.Setenv("AUTARCH_HOME_DB", "")
	t.Setenv("HOME_BB_DATA", "/d")
	if p, _ := DefaultDB(); p != "/d/plugins/autarch/data.db" {
		t.Fatal(p)
	}
	t.Setenv("HOME_BB_DATA", "")
	if _, err := DefaultDB(); err == nil {
		t.Fatal("no database must be an error")
	}
}
