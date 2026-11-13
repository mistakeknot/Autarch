package homeask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestVizierResolutionFailureScope(t *testing.T) {
	for _, tc := range []struct {
		title       string
		owned, want bool
	}{
		{"Coordinator", false, false}, {"Masaq' | vizier candidate", false, true}, {"Coordinator", true, true},
	} {
		t.Run(tc.title+fmt.Sprint(tc.owned), func(t *testing.T) {
			f := &CardFiler{Run: func(_ context.Context, _ []string, args ...string) BBResult {
				if strings.Join(args[:2], " ") == "thread list" {
					return *badRes(1, "offline")
				}
				return okRes(map[string]any{"thread": map[string]any{"id": "thr_x", "title": tc.title}})
			}}
			err := f.CheckVizierOwnership(context.Background(), "thr_x", "", tc.owned)
			if errors.Is(err, ErrInvalid) != tc.want {
				t.Fatalf("err=%v want refusal=%v", err, tc.want)
			}
		})
	}
}
