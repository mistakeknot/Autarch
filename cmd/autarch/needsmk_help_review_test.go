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

func TestNeedsMkPRHelpNamesIntoOptIn(t *testing.T) {
	help := needsMkFileCmd().Flags().Lookup("pr").Usage
	for _, text := range []string{"--into", "non-default", "explicit"} {
		if !strings.Contains(help, text) {
			t.Errorf("--pr help lacks %q: %s", text, help)
		}
	}
}
