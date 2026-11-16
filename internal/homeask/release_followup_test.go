package homeask

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReleaseFlagsArchivedOwnerAndKeepsQueueMoving(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_archived": "archived"}
	f := bb.filer()
	r := testReq()
	r.Key = "a-archived"
	r.Owner = "thr_archived"
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Key = "z-good"
	r.Owner = ""
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	report, e := f.ReleaseDetailed(context.Background())
	if e == nil || report.Failures["a-archived"] == "" || len(report.Groups["other"]) != 1 {
		t.Fatalf("missing failure or later delivery: %+v %v", report, e)
	}
	raw, _ := json.Marshal(report)
	var wire map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	if len(wire["owner_attention"]) == 0 {
		t.Fatalf("archived owner not flagged: %s", raw)
	}
	s, _, done, e := f.openState()
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	v := s["a-archived"]
	if !v.Held || v.Attempted || v.Refunded {
		t.Fatalf("owner repair must remain possible: %+v", v)
	}
}
