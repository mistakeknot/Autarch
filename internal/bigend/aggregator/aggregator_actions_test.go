package aggregator

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/bigend/tmux"
)

type fakeTmux struct {
	killed  bool
	created bool
	name    string
	path    string
	cmd     []string
}

func (f *fakeTmux) IsAvailable() bool                                   { return true }
func (f *fakeTmux) ListSessions() ([]tmux.Session, error)               { return nil, nil }
func (f *fakeTmux) DetectStatus(name string) tmux.Status                { return tmux.StatusUnknown }
func (f *fakeTmux) CapturePane(sessionName string, lines int) (string, error) { return "", nil }
func (f *fakeTmux) NewSession(name, path string, cmd []string) error    { f.created = true; f.name = name; f.path = path; f.cmd = cmd; return nil }
func (f *fakeTmux) RenameSession(oldName, newName string) error         { return nil }
func (f *fakeTmux) KillSession(name string) error                       { f.killed = true; return nil }
func (f *fakeTmux) AttachSession(name string) error                     { return nil }

func TestRestartSession(t *testing.T) {
	scanner := discovery.NewScanner(config.DiscoveryConfig{})
	agg := New(scanner, &config.Config{})
	ft := &fakeTmux{}
	agg.tmuxClient = ft

	if err := agg.RestartSession("demo", "/root/projects/demo", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ft.killed || !ft.created {
		t.Fatalf("expected kill and create")
	}
	if len(ft.cmd) != 1 || ft.cmd[0] != "claude" {
		t.Fatalf("expected claude command, got %v", ft.cmd)
	}
}
