package serve

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindRejectsNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "[::]:0"} {
		s, err := New(Config{Addr: addr, TokenPath: filepath.Join(t.TempDir(), "t")})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "refusing to bind non-loopback") {
			t.Fatalf("%s: expected netguard error, got %v", addr, err)
		}
	}
}
