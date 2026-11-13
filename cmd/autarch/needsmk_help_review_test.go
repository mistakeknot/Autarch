package main

import (
	"strings"
	"testing"
)

func TestNeedsMkHelpExplainsPickedClosedAskKey(t *testing.T) {
	help := needsMkFileCmd().Long
	for _, text := range []string{"picked closed card", "same ask_key exits 5", "new ask needs a new ask_key"} {
		if !strings.Contains(help, text) {
			t.Errorf("help lacks %q", text)
		}
	}
}
