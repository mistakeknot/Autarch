package main

import (
	"strings"

	"testing"
)

func TestNeedsMkVizierOwnership(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, tc := range []struct {
		name    string
		vizier  bool
		flags   []string
		refused bool
	}{
		{"vizier needs owner", true, nil, true},
		{"vizier own", true, []string{"--vizier-owned"}, false},
		{"ordinary unchanged", false, nil, false},
		{"ordinary cannot mark vizier", false, []string{"--vizier-owned"}, true},
		{"both refused", true, []string{"--vizier-owned", "--owner-thread", "thr_other"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &stubBB{vizier: tc.vizier}
			args := append([]string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", writeAsk(t)}, tc.flags...)
			_, err := runNeedsMk(t, s, args...)
			if tc.refused {
				if err == nil || exitCode(err) != 2 || s.creates != 0 {
					t.Fatalf("err=%v creates=%d", err, s.creates)
				}
				if !strings.Contains(err.Error(), "--vizier-owned") {
					t.Fatal(err)
				}
			} else if err != nil || s.creates != 1 {
				t.Fatalf("err=%v creates=%d", err, s.creates)
			}
		})
	}
}
