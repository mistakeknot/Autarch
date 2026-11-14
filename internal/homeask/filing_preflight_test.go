package homeask

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestFilingPreflightAggregates(t *testing.T) {
	f := newFake(t, registryAbsent).filer()
	f.Check = func(string, string) error { return fmt.Errorf("project/root mismatch: choose the registered root") }
	r := testReq()
	r.Type = "design"
	r.Ask["options"].([]any)[0].(map[string]any)["id"] = "YES"
	r.Ask["options"].([]any)[0].(map[string]any)["label"] = strings.Repeat("x", 81)
	err := f.Preflight(context.Background(), r)
	for _, want := range []string{"lowercase", "80", "three", "project/root", "owner-thread"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}
