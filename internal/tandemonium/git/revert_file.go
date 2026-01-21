package git

func RevertFile(r Runner, base, path string) error {
	_, err := r.Run("git", "checkout", base, "--", path)
	return err
}
