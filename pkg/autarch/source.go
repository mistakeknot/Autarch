package autarch

// DataSource defines read-only access to Autarch domain entities.
// Implemented by HTTPSource (Intermute API) and LocalSource (dot-directory files).
type DataSource interface {
	ListSpecs(status string) ([]Spec, error)
	ListEpics(specID string) ([]Epic, error)
	ListStories(epicID string) ([]Story, error)
	ListTasks(status, agent string) ([]Task, error)
	ListInsights(specID, category string) ([]Insight, error)
}

// WritableDataSource extends DataSource with write operations.
// Implemented by LocalSource for offline writes to .coldwine/state.db.
type WritableDataSource interface {
	DataSource
	CreateEpic(epic Epic) (Epic, error)
	CreateStory(story Story) (Story, error)
	CreateTask(task Task) (Task, error)
}
