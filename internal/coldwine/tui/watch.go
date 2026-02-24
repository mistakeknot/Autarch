package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/mistakeknot/autarch/internal/coldwine/project"
)

type watchMsg struct{}

func watchCmd() tea.Cmd {
	return func() tea.Msg {
		root, err := project.FindRoot(".")
		if err != nil {
			return nil
		}
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}
		defer watcher.Close()

		specDir := project.SpecsDir(root)
		_ = watcher.Add(specDir)

		stateDir := filepath.Dir(project.StateDBPath(root))
		_ = watcher.Add(stateDir)

		for {
			select {
			case evt, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if evt.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					if shouldReloadPath(evt.Name) {
						return watchMsg{}
					}
				}
			case <-watcher.Errors:
			}
		}
	}
}

func shouldReloadPath(path string) bool {
	slash := filepath.ToSlash(path)
	if strings.HasPrefix(slash, ".coldwine/specs/") {
		return true
	}
	if strings.Contains(slash, "/.coldwine/specs/") {
		return true
	}
	if strings.HasPrefix(slash, ".coldwine/") && strings.HasSuffix(slash, "state.db") {
		return true
	}
	if strings.Contains(slash, "/.coldwine/") && strings.HasSuffix(slash, "state.db") {
		return true
	}
	return false
}
