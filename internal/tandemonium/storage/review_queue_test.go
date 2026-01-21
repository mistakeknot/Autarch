package storage

import "testing"

func TestRemoveFromReviewQueue(t *testing.T) {
	db, _ := OpenTemp()
	defer db.Close()
	_ = Migrate(db)
	_ = AddToReviewQueue(db, "TAND-001")
	_ = RemoveFromReviewQueue(db, "TAND-001")
	ids, _ := ListReviewQueue(db)
	if len(ids) != 0 {
		t.Fatal("expected empty")
	}
}
