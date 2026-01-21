package git

func MergeBranch(r Runner, branch string) error {
	_, err := r.Run("git", "merge", branch)
	return err
}
