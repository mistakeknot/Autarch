package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestLoadReviewQueue(t *testing.T) {
	db, _ := storage.OpenTemp()
	defer db.Close()
	_ = storage.Migrate(db)
	_ = storage.AddToReviewQueue(db, "TAND-001")

	ids, err := LoadReviewQueue(db)
	if err != nil || len(ids) != 1 {
		t.Fatal("expected one review item")
	}
}
