package specs

import "testing"

func TestValidateSpecMissingTitle(t *testing.T) {
	doc := []byte("id: \"TAND-001\"\nstatus: \"ready\"\n")
	if err := Validate(doc); err == nil {
		t.Fatalf("expected validation error")
	}
}
