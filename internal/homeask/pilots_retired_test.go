package homeask

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetPilotRetiredDaycapPreservesState(t *testing.T) {
	for _, raw := range []string{"", `{"daycap":{"enabled":true},"unknown":{"enabled":false}}`} {
		dir := t.TempDir()
		path := filepath.Join(dir, "pilots.json")
		if raw != "" {
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}
		f := &CardFiler{StateDir: dir, LockDir: t.TempDir()}
		for _, enabled := range []bool{true, false} {
			if err := f.SetPilot("daycap", enabled); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if raw == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("created state: %s, %v", got, err)
				}
			} else if err != nil || !bytes.Equal(got, []byte(raw)) {
				t.Fatalf("changed state: %s, %v", got, err)
			}
		}
	}
}

func TestSetPilotInvalidNamesListsRetiredDaycap(t *testing.T) {
	f := &CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir()}
	err := f.SetPilot("unknown", true)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "coldread or linkcheck") || !strings.Contains(err.Error(), "daycap is retired") {
		t.Fatalf("error: %v", err)
	}
}
