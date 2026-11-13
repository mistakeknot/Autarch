package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeedsMkEvidenceImport(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	t.Setenv("AUTARCH_HOME_EVIDENCE", filepath.Join(t.TempDir(), "store"))
	image := filepath.Join(t.TempDir(), "picture.bin")
	os.WriteFile(image, []byte("GIF89a"), 0600)
	p := writeAsk(t)
	raw, _ := os.ReadFile(p)
	var ask map[string]any
	json.Unmarshal(raw, &ask)
	ask["ask_key"] = "same-ask"
	ask["evidence"] = []any{map[string]any{"caption": "Picture", "image_path": image}}
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	s := &stubBB{}
	args := []string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", p}
	if _, err := runNeedsMk(t, s, args...); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.desc, image) || strings.Contains(s.desc, "image_path") || !strings.Contains(s.desc, "image/gif") {
		t.Fatal("stored ask did not replace path with image reference")
	}
}
