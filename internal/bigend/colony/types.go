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
