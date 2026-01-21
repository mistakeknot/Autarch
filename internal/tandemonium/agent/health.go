package agent

type HealthRunner interface {
	IsAlive(session string) bool
	Restart(session string) error
}

type HealthMonitor struct {
	runner  HealthRunner
	restart bool
}

func NewHealthMonitor(runner HealthRunner, restart bool) *HealthMonitor {
	return &HealthMonitor{runner: runner, restart: restart}
}

func (h *HealthMonitor) Check(session string) {
	if h.restart && !h.runner.IsAlive(session) {
		_ = h.runner.Restart(session)
	}
}
