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
