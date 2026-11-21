package orchcompat

import (
	"runtime"
	"strings"
	"testing"
)

// Cancelled readbacks can leave a fake-bb process saving while the harness
// reads state. Readers must see a complete snapshot throughout each save.
func TestFakeBBSaveOverlappingRead(t *testing.T) {
	dir := t.TempDir()
	desc := strings.Repeat("snapshot", 8192)
	s := &fbState{Tasks: []*fbTask{{ID: "T1", Desc: desc}}}
	if err := fbSave(dir, s); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan struct{})
	var saveErr error
	go func() {
		defer close(done)
		<-start
		for i := 1; i <= 64; i++ {
			s.Seq = i
			if err := fbSave(dir, s); err != nil {
				saveErr = err
				return
			}
		}
	}()
	// Always join the writer, including on a read failure, before TempDir cleanup.
	defer func() {
		<-done
		if saveErr != nil {
			t.Error(saveErr)
		}
	}()
	close(start)
	for {
		back, err := fbLoad(dir)
		if err != nil {
			t.Fatalf("overlapping read: %v", err)
		}
		if len(back.Tasks) != 1 || back.Tasks[0].ID != "T1" || back.Tasks[0].Desc != desc {
			t.Fatal("overlapping read returned an incomplete snapshot")
		}
		select {
		case <-done:
			return
		default:
		}
		runtime.Gosched()
	}
}
