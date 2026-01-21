package tui

type Approver interface {
	Approve(taskID, branch string) error
}

func (m *Model) ApproveTask(a Approver, taskID, branch string) error {
	return a.Approve(taskID, branch)
}
