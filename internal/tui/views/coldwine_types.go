package views

// ColdwineMode represents the active mode of the Coldwine view.
type ColdwineMode int

const (
	// ModeEpics shows the epic/story/task list (default).
	ModeEpics ColdwineMode = iota
	// ModeRuns shows the sprint run list with detail panel.
	ModeRuns
)

// LayoutMode represents the user's layout preference for Coldwine.
type LayoutMode int

const (
	// LayoutToggle switches sidebar between Epics and Runs lists (default).
	LayoutToggle LayoutMode = iota
	// LayoutInline shows sprint detail inline below tasks in Epics mode.
	LayoutInline
	// LayoutSplit shows epic and sprint detail side-by-side.
	LayoutSplit
)
