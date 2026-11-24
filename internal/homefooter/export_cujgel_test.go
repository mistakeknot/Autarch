package homefooter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPickExportCujgelSnapshot(t *testing.T) {
	raw, err := os.ReadFile("../homeask/testdata/cujgel/citadel-link.json")
	if err != nil {
		t.Fatal(err)
	}
	link := strings.ReplaceAll(string(raw), "'", "''")
	newer := strings.Replace(link, strings.Repeat("a", 64), strings.Repeat("b", 64), 1)
	path := fixture(t, `CREATE TABLE decisions(id TEXT PRIMARY KEY,task_id TEXT,source TEXT,body_json TEXT,kind TEXT,ask_key TEXT);
CREATE TABLE decision_cujgel(decision_id TEXT PRIMARY KEY,body_json TEXT);
INSERT INTO cards VALUES ('fixture','fixture-card','Compare','closed',NULL);
INSERT INTO decisions VALUES ('g1','fixture','card','{}','decide','cujgel:proj_fixture:comparison_fixture'),('g2','fixture','card','{}','decide','cujgel:proj_fixture:comparison_newer');
INSERT INTO decision_cujgel VALUES ('g1','`+link+`'),('g2','`+newer+`');
INSERT INTO picks VALUES ('g1','p1','option-a','2026-11-12T00:00:00Z',NULL,'mk'),('g2','p2','option-b','2026-11-12T01:00:00Z',NULL,'mk');`)
	var out bytes.Buffer
	if err := ExportPicks(&out, ExportOptions{DB: path, Days: 7, Now: func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("wrong rows: %s", out.String())
	}
	for i, line := range lines {
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if len(row["cujgel"]) == 0 || len(row["ask_key"]) == 0 {
			t.Fatalf("missing linked fields: %s", line)
		}
		var got, want any
		json.Unmarshal(row["cujgel"], &got)
		expected := link
		key := "cujgel:proj_fixture:comparison_fixture"
		pick := "option-a"
		if i == 1 {
			expected = newer
			key = "cujgel:proj_fixture:comparison_newer"
			pick = "option-b"
		}
		json.Unmarshal([]byte(expected), &want)
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(want)
		if !bytes.Equal(gb, wb) {
			t.Fatalf("wrong generation snapshot: %s", line)
		}
		if string(row["ask_key"]) != `"`+key+`"` || string(row["pick"]) != `"`+pick+`"` {
			t.Fatalf("wrong mapping fields: %s", line)
		}
	}
}

func TestPickExportUnlinkedBytesUnchanged(t *testing.T) {
	for _, snapshotTable := range []bool{false, true} {
		t.Run(map[bool]string{false: "previous schema", true: "expanded schema"}[snapshotTable], func(t *testing.T) {
			extra := ""
			if snapshotTable {
				extra = `CREATE TABLE decision_cujgel(decision_id TEXT PRIMARY KEY,body_json TEXT);`
			}
			path := fixture(t, `CREATE TABLE decisions(id TEXT,task_id TEXT,source TEXT,body_json TEXT,kind TEXT,ask_key TEXT);`+extra+`
INSERT INTO cards VALUES ('fixture','fixture-card','Compare','ruled',NULL);
INSERT INTO decisions VALUES ('g1','fixture','card','{}','decide','cujgel:unlinked');
INSERT INTO picks VALUES ('g1','p1','option-a','2026-11-12T00:00:00Z',NULL,'mk');`)
			var out bytes.Buffer
			if err := ExportPicks(&out, ExportOptions{DB: path, Days: 7, Now: func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }}); err != nil {
				t.Fatal(err)
			}
			const want = `{"id":"fixture-card","time":"2026-11-12T00:00:00Z","state":"ruled","pick":"option-a","title":"Compare","recommendation_id":null,"matched_recommendation":null,"question_recommendations":null,"kind":"decide","pr_head":null,"round":null,"auto_close":null,"revisions":null,"decision_id":"g1","pick_id":"p1"}` + "\n"
			if out.String() != want {
				t.Fatalf("unlinked bytes changed:\n%s", out.String())
			}
		})
	}
}
