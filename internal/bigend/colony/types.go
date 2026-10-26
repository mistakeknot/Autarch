package colony

// Colony represents a detected agent colony for a project.
type Colony struct {
	Name      string         `json:"name"`
	Root      string         `json:"root"`
	Worktrees []Worktree     `json:"worktrees"`
	Members   []ColonyMember `json:"members"`
	Markers   []string       `json:"markers,omitempty"`
}

// Worktree represents a git worktree entry.
type Worktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
}

// ColonyMember represents a detected agent process.
type ColonyMember struct {
	PID    int    `json:"pid"`
	CWD    string `json:"cwd"`
	Source string `json:"source"` // proc
}

// Clone returns a deep copy of the colony.
func (c Colony) Clone() Colony {
	c.Worktrees = cloneSlice(c.Worktrees)
	c.Members = cloneSlice(c.Members)
	c.Markers = cloneSlice(c.Markers)
	return c
}

func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append(make([]T, 0, len(s)), s...)
}
