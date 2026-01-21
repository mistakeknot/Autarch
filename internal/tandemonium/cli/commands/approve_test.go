package commands

import "testing"

func TestApproveCmd(t *testing.T) {
	if ApproveCmd().Use != "approve" {
		t.Fatal("expected approve")
	}
}
