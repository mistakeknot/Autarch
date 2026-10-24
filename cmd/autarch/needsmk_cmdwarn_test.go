package main

import "testing"

func TestCommandWarning(t *testing.T) {
	inline := "Run this: scp user@host:/srv/x.sh /tmp/ then bash /tmp/x.sh. sha256 abc"
	ownLine := "Run this check:\n\nscp user@host:/srv/x.sh /tmp/\nbash /tmp/x.sh\n\nThen answer."
	prompt := "Run:\n$ bash /tmp/x.sh --check"
	listed := "Steps:\n(1) ssh user@host uptime"
	for name, c := range map[string]struct {
		q    string
		warn bool
	}{
		"inline command warns":     {inline, true},
		"own-line commands do not": {ownLine, false},
		"prompt line does not":     {prompt, false},
		"numbered line does not":   {listed, false},
		"no command mention":       {"Collapse per project or per day?", false},
		"the word bash alone":      {"Is bash the right shell?", false},
	} {
		if got := commandWarning(c.q) != ""; got != c.warn {
			t.Errorf("%s: warn=%v want %v", name, got, c.warn)
		}
	}
}
