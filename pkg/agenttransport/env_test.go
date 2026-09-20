package agenttransport

import (
	"strings"
	"testing"
)

// tmux emits PaneFormat's \x1f separator intact only under a UTF-8 locale.
// Interactive shells always set one, so this was invisible by hand and broke
// the moment the inventory ran under launchd, which passes no locale at all.
func TestChildEnvGuaranteesAUTF8Locale(t *testing.T) {
	has := func(env []string, prefix string) string {
		for _, kv := range env {
			if strings.HasPrefix(kv, prefix) {
				return kv
			}
		}
		return ""
	}

	// The launchd case: no locale at all.
	got := childEnv([]string{"PATH=/usr/bin"})
	if has(got, "LANG=") == "" {
		t.Error("an environment with no locale must be given one, or tmux mangles the field separator")
	}

	// An operator's own locale is never overridden; doing so would be a worse
	// bug than the one this fixes.
	for _, existing := range []string{"LANG=de_DE.UTF-8", "LC_ALL=C.UTF-8"} {
		env := childEnv([]string{"PATH=/usr/bin", existing})
		if len(env) != 2 {
			t.Errorf("childEnv added to an environment that already had %s: %v", existing, env)
		}
	}
}
