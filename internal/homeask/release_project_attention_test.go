package homeask

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMissingProjectAttentionForUnownedPull(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.StateDir = t.TempDir()
	r := testReq()
	r.Key = "unowned"
	r.Pull, r.NonBlocking = true, true
	r.Owner, r.Thread = "", ""
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 3 && strings.Join(args[:3], " ") == "tasks label list" {
			return BBResult{Code: 1, Stderr: []byte("project not found: P1")}
		}
		return bb.run(ctx, env, args...)
	}
	report, err := f.ReleaseDetailed(context.Background())
	var entries *ReleaseEntryErrors
	if !errors.As(err, &entries) || !entries.DataOnly() {
		t.Fatalf("expected request fault: %v", err)
	}
	attention := report.OwnerAttention[r.Key]
	if !strings.Contains(attention, "owner unowned request needs attention") || !strings.Contains(attention, "run needs-mk cancel-held "+r.Key) {
		t.Fatalf("unclear attention: %q", attention)
	}
	var footer bytes.Buffer
	if err := f.PrintReleaseOwnerAttention(&footer); err != nil || !strings.Contains(footer.String(), attention) {
		t.Fatalf("footer=%s err=%v", &footer, err)
	}
}
