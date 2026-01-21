package tui

import "testing"

func TestApproveAdapterImplements(t *testing.T) {
	var _ Approver = (*ApproveAdapter)(nil)
}
