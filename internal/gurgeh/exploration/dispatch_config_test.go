package exploration

import (
	"context"
	"strings"
	"testing"
)

func TestExploreRejectsConfiguredBackend(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	t.Setenv("AUTARCH_DISPATCH_AGENT", "test-no-launch")
	_, _, err := Explore(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsupported dispatch backend") {
		t.Fatalf("dispatch did not apply env: %v", err)
	}
}
