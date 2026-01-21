package commands

import "testing"

func TestStatusCommand(t *testing.T) {
	if StatusCmd().Use != "status" {
		t.Fatalf("unexpected Use")
	}
}
