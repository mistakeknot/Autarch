package storage

import "testing"

func TestListContacts(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	if err := RequestContact(db, "alice", "bob"); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := RespondContact(db, "alice", "bob", true); err != nil {
		t.Fatalf("respond: %v", err)
	}

	contacts, err := ListContacts(db, "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(contacts) != 1 || contacts[0] != "bob" {
		t.Fatalf("expected bob")
	}
}
