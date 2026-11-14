package homeask

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUIRequiresAttachment(t *testing.T) {
	for _, typ := range []string{"play", "merge"} {
		r := testReq()
		r.Type = typ
		r.UI = true
		f := newFake(t, registryAbsent).filer()
		if err := f.CheckUI(r); err == nil {
			t.Fatal("accepted UI without proof")
		}
		p := filepath.Join(t.TempDir(), "flow.webm")
		os.WriteFile(p, nil, 0600)
		r.Attachments = []string{p}
		if err := f.CheckUI(r); err == nil {
			t.Fatal("accepted empty video")
		}
		os.WriteFile(p, []byte("video"), 0600)
		if err := f.CheckUI(r); err != nil {
			t.Fatal(err)
		}
		r.Owner = "thr_a"
		if err := f.Preflight(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
}
