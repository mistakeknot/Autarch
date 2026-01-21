package tmux

import "strings"

type Runner interface {
	Run(name string, args ...string) error
}

type Session struct {
	ID      string
	Workdir string
	LogPath string
}

func StartSession(r Runner, s Session) error {
	if err := r.Run("tmux", "new-session", "-d", "-s", s.ID, "-c", s.Workdir); err != nil {
		return err
	}
	cmd := "cat >> " + shellQuote(s.LogPath)
	return r.Run("tmux", "pipe-pane", "-t", s.ID, "-o", cmd)
}

func StopSession(r Runner, id string) error {
	return r.Run("tmux", "kill-session", "-t", id)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
