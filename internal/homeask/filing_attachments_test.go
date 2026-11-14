package homeask

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLargeAttachmentHashUsesBoundedMemory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "clip.mp4")
	file, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(32 << 20); err != nil {
		t.Fatal(err)
	}
	file.Close()
	f := &CardFiler{Run: func(_ context.Context, _ []string, args ...string) BBResult {
		if args[2] == "list" {
			return okRes(map[string]any{"attachments": []any{}})
		}
		return okRes(map[string]any{})
	}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err = f.ensureAttachments(context.Background(), CardRequest{Attachments: []string{p}}, "card"); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("hash allocated %d bytes for 32 MiB media", allocated)
	}
}

func TestAttachmentReplayAfterLostUploadAnswer(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "shot.png")
	os.WriteFile(p, []byte("image"), 0600)
	r := testReq()
	r.Attachments = []string{p}
	stored := ""
	uploads := 0
	f := &CardFiler{Run: func(_ context.Context, _ []string, args ...string) BBResult {
		switch strings.Join(args[:3], " ") {
		case "tasks attachment list":
			return okRes(map[string]any{"attachments": []any{map[string]any{"fileName": stored}}})
		case "tasks attachment add":
			uploads++
			stored = argFlag(args, "--name")
			return BBResult{Code: 1}
		}
		t.Fatalf("unexpected %v", args)
		return BBResult{}
	}}
	if err := f.ensureAttachments(ctx, r, "card"); err == nil {
		t.Fatal("lost answer not reported")
	}
	if err := f.ensureAttachments(ctx, r, "card"); err != nil {
		t.Fatal(err)
	}
	if uploads != 1 {
		t.Fatal("duplicate uploads", uploads)
	}
}

func TestForeignReplayCannotAttach(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	r := testReq()
	r.Attachments = []string{"anything.png"}
	_, err := f.replay(context.Background(), r, &found{ID: "card", Comments: []Comment{{Kind: "agent", ThreadID: "thr_foreign", CreatedAt: "1", ID: "1"}}})
	if err == nil || len(bb.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, bb.calls)
	}
}
