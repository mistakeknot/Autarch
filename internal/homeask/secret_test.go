package homeask

import (
	"context"
	"strings"
	"testing"
)

func fakeSecretMetadata() map[string]any {
	return map[string]any{"thread": "thr_23456789ab", "names": []string{"FAKE_TEST_TOKEN"}, "path": "/fake-test/credentials.env", "project": "fake-test-project"}
}

func TestSecretCardRoundTripAndIdentity(t *testing.T) {
	r := testReq()
	base, err := RequestIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Secret = fakeSecretMetadata()
	desc, identity, err := r.Description()
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCard(desc)
	if err != nil || c.Secret == nil || !secretEqual(c.Secret, r.Secret) {
		t.Fatalf("metadata roundtrip failed: %v", err)
	}
	if identity == base {
		t.Fatal("secret metadata missing from request identity")
	}
	if _, err = ParseCard(desc + "\n```home-secret\n{}\n```\n"); err == nil {
		t.Fatal("duplicate block accepted")
	}
	for _, key := range []string{"value", "values", "token", "password", "extra", "Thread"} {
		f := newFake(t, registryOnCreate)
		r.Secret = fakeSecretMetadata()
		r.Secret[key] = "fake-test-value-not-a-secret"
		_, err = f.filer().FileCard(context.Background(), r)
		if err == nil || len(f.tasks) != 0 {
			t.Fatalf("invalid metadata reached tasks: %s", key)
		}
		if strings.Contains(err.Error(), "fake-test-value-not-a-secret") {
			t.Fatal("error echoed rejected content")
		}
	}
}
