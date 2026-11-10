package homeask

import (
	"context"
	"path/filepath"
	"strings"
)

// ProjectCheck vets the (project, root) pair a card will carry before anything is created. Home hides
// a card whose project is not the one its task board is bound to, and shows it display-only when the
// root is not the one serve resolves for the project, so a filing that fails it would "succeed" and
// never reach mk.
type ProjectCheck func(project, root string) error

// PrepareAsk is the one pre-filing check every way of filing runs (needs-mk file, decide file, the MCP
// decision tool). It resolves askProject against the Home project tasksProject is bound to, then runs
// f.Check on the resolved project and askRoot. It returns the project the card must carry and a
// non-fatal warning to show the filer (also returned with a Check error, so it is not lost). Nothing
// is created here.
func (f *CardFiler) PrepareAsk(ctx context.Context, tasksProject, askProject, askRoot string) (project, warning string, err error) {
	project, warning, err = f.ResolveAskProject(ctx, tasksProject, askProject)
	if err != nil {
		return "", "", err
	}
	if f.Check != nil {
		if err := f.Check(project, askRoot); err != nil {
			return "", warning, err
		}
	}
	return project, warning, nil
}

// BoundProject is the Home project tasksProject is confirmed bound to, or "" when it is unbound, only
// suggested or rejected, or cannot be read.
func (f *CardFiler) BoundProject(ctx context.Context, tasksProject string) string {
	if f == nil {
		return ""
	}
	if b, ok := f.binding(ctx, tasksProject); ok {
		return b.home
	}
	return ""
}

// DefaultAskProject is BoundProject for the filer's own default tasks board: TasksProject, else the
// only board in TasksProjects. It is "" when the filer has no single board to ask about.
func (f *CardFiler) DefaultAskProject(ctx context.Context) string {
	if f == nil || f.ConfigErr != nil {
		return ""
	}
	board := strings.TrimSpace(f.TasksProject)
	if board == "" && len(f.TasksProjects) == 1 {
		for _, v := range f.TasksProjects {
			board = v
		}
	}
	if board == "" {
		return ""
	}
	return f.BoundProject(ctx, board)
}

// ProjectDefaulter is a filer that knows the Home project its task board is bound to.
type ProjectDefaulter interface {
	DefaultAskProject(ctx context.Context) string
}

// DefaultProject is the project to file under when the caller named none. The task board's binding
// comes first because the folder name is wrong in a bb worktree (Autarch-project-visit, not Autarch);
// the root's folder name is only the last fallback. filer may be nil or any Filer.
func DefaultProject(ctx context.Context, filer any, root string) string {
	if d, ok := filer.(ProjectDefaulter); ok {
		if p := d.DefaultAskProject(ctx); p != "" {
			return p
		}
	}
	return filepath.Base(root)
}

// FolderOrBound is DefaultProject for a caller that already knows its tasks board (needs-mk file).
func (f *CardFiler) FolderOrBound(ctx context.Context, tasksProject, root string) string {
	if p := f.BoundProject(ctx, tasksProject); p != "" {
		return p
	}
	return filepath.Base(root)
}
