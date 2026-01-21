package agent

import "testing"

type fakeWorktreeCreator struct{ called bool }

type fakeSessionStarter struct{ called bool }

func TestAdaptersImplementInterfaces(t *testing.T) {
	var _ WorktreeCreator = (*GitWorktreeAdapter)(nil)
	var _ SessionStarter = (*TmuxSessionAdapter)(nil)
}
