package door

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// The threads axis' switch and its background read (plan WI-5). Kept beside
// the model rather than inside it so the rows-only door and its tests stay
// untouched, the same way WithBriefing is kept off the constructor.

// WithThreads turns the threads axis on. A registry note that cannot be read
// is a stated error on the threads screen, never a refusal to open.
func (m Model) WithThreads(o ThreadsOptions) Model {
	m.threadsOpts = o
	m.threadsOn = true
	m.threads = ThreadSet{ByRoot: make(map[string][]Thread)}
	m.threadResults = make(chan threadMsg, 64)
	if o.RegistryPath != "" {
		f, err := os.Open(o.RegistryPath)
		if err != nil {
			m.registryErr = err
		} else {
			m.registry, m.registryErr = ParseRegistryNote(f)
			f.Close()
		}
	}
	return m
}

// startThreads reads every seat in the background, streaming each finished
// thread onto threadResults from a worker goroutine. The pending count is set
// by the caller before this command runs, so a fast result cannot outrun it.
func (m Model) startThreads(sessions []TmuxSession) tea.Cmd {
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	roots := m.threadsOpts.Roots
	transcriptsRoot := m.threadsOpts.TranscriptsRoot
	results := m.threadResults
	return func() tea.Msg {
		go ReadThreads(context.Background(), sessions, projects, roots, transcriptsRoot, func(th Thread) {
			results <- threadMsg{t: th}
		})
		return nil
	}
}

func (m Model) waitForThread() tea.Cmd {
	results := m.threadResults
	return func() tea.Msg { return <-results }
}
