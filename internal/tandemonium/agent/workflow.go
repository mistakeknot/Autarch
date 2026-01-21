package agent

type WorktreeCreator interface {
	Create(repo, path, branch string) error
}

type SessionStarter interface {
	Start(id, workdir, logPath string) error
}

func StartTask(w WorktreeCreator, s SessionStarter, taskID, repo, worktree, logPath string) error {
	if err := w.Create(repo, worktree, "feature/"+taskID); err != nil {
		return err
	}
	return s.Start(SessionID(taskID), worktree, logPath)
}
