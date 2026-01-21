package agent

import (
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
)

type GitWorktreeAdapter struct{}

func (g *GitWorktreeAdapter) Create(repo, path, branch string) error {
	return git.CreateWorktree(repo, path, branch)
}

type TmuxSessionAdapter struct{ Runner tmux.Runner }

func (t *TmuxSessionAdapter) Start(id, workdir, logPath string) error {
	return tmux.StartSession(t.Runner, tmux.Session{ID: id, Workdir: workdir, LogPath: logPath})
}
