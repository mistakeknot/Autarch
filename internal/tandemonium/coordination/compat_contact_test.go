package coordination

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestContactPolicyAndRequestFlow(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	if _, err := SetContactPolicy(db, SetContactPolicyRequest{Owner: "alice", Policy: "contacts_only"}); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	policy, err := GetContactPolicy(db, GetContactPolicyRequest{Owner: "alice"})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if policy.Policy != "contacts_only" {
		t.Fatalf("expected contacts_only")
	}

	if _, err := RequestContact(db, RequestContactRequest{Requester: "bob", Recipient: "alice"}); err != nil {
		t.Fatalf("request contact: %v", err)
	}
	if _, err := RespondContact(db, RespondContactRequest{Requester: "bob", Recipient: "alice", Accept: true}); err != nil {
		t.Fatalf("respond contact: %v", err)
	}
}
