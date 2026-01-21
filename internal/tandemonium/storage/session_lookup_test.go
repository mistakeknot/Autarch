package storage

import "testing"

func TestFindSessionByTask(t *testing.T) {
	db, _ := OpenTemp()
	_ = Migrate(db)
	_ = InsertSession(db, Session{ID: "s1", TaskID: "T1", State: "working", Offset: 0})
	s, err := FindSessionByTask(db, "T1")
	if err != nil || s.ID != "s1" {
		t.Fatalf("expected session")
	}
}
