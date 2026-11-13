package homefooter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPickExport(t *testing.T) {
	path := fixture(t,
		`CREATE TABLE decisions (id TEXT PRIMARY KEY, task_id TEXT, source TEXT, body_json TEXT, kind TEXT, generation INTEGER, card_fp TEXT);
 INSERT INTO cards VALUES ('merge','CARD-1','Merge','closed',NULL),('question','CARD-2','Question','ruled',NULL),('multi','CARD-3','Multi','ruled',NULL);
 INSERT INTO decisions VALUES
 ('m1','merge','card','{"recommendation":"a","options":[{"id":"a","approval":{"kind":"merge","identity":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}','decide',1,'fp1'),
 ('m2','merge','card','{"recommendation":"b","options":[{"id":"b","approval":{"kind":"merge","identity":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}]}','decide',2,'fp2'),
 ('q1','question','card','{"recommendation":"a","question":"Choose"}','decide',1,'fp3'),
 ('u1','multi','card','{"questions":[{"id":"one","recommendation":"a"},{"id":"two","recommendation":"b"}]}','decide',1,'fp4');
 INSERT INTO picks VALUES ('m1','p1','a','2026-11-07T00:00:00.000Z',NULL,'mk'),('m2','p2','b','2026-11-08T00:00:00.000Z',NULL,'mk'),('q1','p3','other','2026-11-09T00:00:00.000Z','My own words','mk'),('u1','p4','answers','2026-11-10T00:00:00.000Z','one: Option A; two: Option A (changed; recommended was Option B)','mk');
 INSERT INTO decisions VALUES ('old','merge','card','{}','decide',3,'fp2'),('bot','merge','card','{}','decide',4,'fp2');
 INSERT INTO picks VALUES ('old','p5','a','2026-11-01T00:00:00.000Z',NULL,'mk'),('bot','p6','a','2026-11-11T00:00:00.000Z',NULL,'vizier');`)
	before, _ := os.ReadFile(path)
	var out bytes.Buffer
	err := ExportPicks(&out, ExportOptions{DB: path, Days: 7, Now: func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("want all four historical mk picks, got %s", out.String())
	}
	var rows []map[string]any
	for _, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"id", "time", "state", "pick", "title", "recommendation_id", "matched_recommendation", "question_recommendations", "kind", "pr_head", "round", "auto_close", "revisions"} {
			if _, ok := row[key]; !ok {
				t.Errorf("missing %s: %s", key, line)
			}
		}
		for _, key := range []string{"round", "auto_close", "revisions"} {
			if row[key] != nil {
				t.Errorf("%s must be null", key)
			}
		}
		rows = append(rows, row)
	}
	if rows[0]["pr_head"] != strings.Repeat("a", 40) || rows[1]["pr_head"] != strings.Repeat("b", 40) {
		t.Errorf("heads must belong to each picked snapshot: %s", out.String())
	}
	if rows[0]["recommendation_id"] != "a" || rows[0]["matched_recommendation"] != true || rows[2]["matched_recommendation"] != false {
		t.Errorf("match results: %s", out.String())
	}
	if rows[2]["pr_head"] != nil || rows[3]["recommendation_id"] != nil {
		t.Errorf("unavailable fields guessed: %s", out.String())
	}
	qs := rows[3]["question_recommendations"].([]any)
	if len(qs) != 2 || qs[0].(map[string]any)["recommendation_id"] != "a" || qs[1].(map[string]any)["recommendation_id"] != "b" || qs[0].(map[string]any)["matched_recommendation"] != nil || qs[1].(map[string]any)["pick"] != nil {
		t.Errorf("per-question match: %v", qs)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("export changed database")
	}
}

func TestPickExportMissingDataAndWindow(t *testing.T) {
	path := fixture(t, `CREATE TABLE decisions (id TEXT, task_id TEXT, source TEXT, body_json TEXT, kind TEXT);
 INSERT INTO cards VALUES ('legacy','CARD-1','Legacy','ruled',NULL);
 INSERT INTO decisions VALUES ('legacy','legacy','card','{}','decide');
 INSERT INTO picks VALUES ('legacy','legacy','a','2026-11-06T00:00:00.000Z',NULL,'mk');`)
	clock := func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }
	var out bytes.Buffer
	if err := ExportPicks(&out, ExportOptions{DB: path, Days: 7, Now: clock}); err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(out.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"recommendation_id", "matched_recommendation", "pr_head", "question_recommendations", "round", "auto_close", "revisions"} {
		if row[key] != nil {
			t.Errorf("%s guessed: %v", key, row[key])
		}
	}
	out.Reset()
	if err := ExportPicks(&out, ExportOptions{DB: path, Days: 6, Now: clock}); err != nil || out.Len() != 0 {
		t.Fatalf("outside window: %v %s", err, out.String())
	}
	for _, days := range []int{-1, 0, 366} {
		if err := ExportPicks(&out, ExportOptions{DB: path, Days: days, Now: clock}); err == nil {
			t.Errorf("accepted days %d", days)
		}
	}
	missing := path + "-missing"
	if err := ExportPicks(&out, ExportOptions{DB: missing, Days: 7}); err == nil {
		t.Fatal("accepted missing store")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("created missing store")
	}
}
