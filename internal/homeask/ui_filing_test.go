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

func TestAttachmentsRejectSymlinksAndNonMedia(t *testing.T) {
	f := &CardFiler{}
	dir := t.TempDir()
	media := filepath.Join(dir, "shot.png")
	os.WriteFile(media, []byte("image"), 0600)
	link := filepath.Join(dir, "linked.png")
	if err := os.Symlink(media, link); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(dir, "notes.txt")
	os.WriteFile(text, []byte("private"), 0600)
	for _, p := range []string{link, text} {
		if err := f.CheckUI(CardRequest{Attachments: []string{p}}); err == nil {
			t.Errorf("accepted %s", p)
		}
	}
}
