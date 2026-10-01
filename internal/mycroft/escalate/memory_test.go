package escalate

import (
	"context"
	"fmt"
	"sync"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// MemoryFiler is Home in memory: it files pull asks (idempotent per RequestID, like the real
// filer) and lists them back. It is a test double and stays in _test files.
type MemoryFiler struct {
	mu       sync.Mutex
	Asks     []homeask.Ask
	Rows     []homeask.ListRow
	FileErr  error
	ListErr  error
	CardView homeask.CardView
	Cards    []string
}

var (
	_ PullFiler          = (*MemoryFiler)(nil)
	_ homeask.CardLister = (*MemoryFiler)(nil)
)

func (m *MemoryFiler) FileForPull(_ context.Context, a homeask.Ask) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FileErr != nil {
		return "", m.FileErr
	}
	for i, p := range m.Asks {
		if p.RequestID == a.RequestID {
			return fmt.Sprintf("T%d", i+1), nil
		}
	}
	m.Asks = append(m.Asks, a)
	id := fmt.Sprintf("T%d", len(m.Asks))
	m.Rows = append(m.Rows, homeask.ListRow{TaskID: id, ID: id, Subject: a.Subject, Project: a.Project, Asker: "mycroft", Priority: 2})
	return id, nil
}

func (m *MemoryFiler) ListPull(context.Context) ([]homeask.ListRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return append([]homeask.ListRow(nil), m.Rows...), nil
}

func (m *MemoryFiler) Card(_ context.Context, id string) (homeask.CardView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Cards = append(m.Cards, id)
	v := m.CardView
	v.TaskID = id
	return v, nil
}
