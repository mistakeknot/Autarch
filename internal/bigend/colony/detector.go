package colony

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mistakeknot/autarch/internal/bigend/discovery"
)

// Detect finds colonies from a list of discovered projects.
func Detect(projects []discovery.Project) []Colony {
	colonies := make([]Colony, 0, len(projects))
	roots := make([]string, 0, len(projects))
	for _, p := range projects {
		roots = append(roots, p.Path)
	}

	procMembers := detectProcMembers(roots)

	for _, p := range projects {
		c := Colony{
			Name: p.Name,
			Root: p.Path,
		}
		c.Markers = append(c.Markers, detectMarkers(p.Path)...)
		c.Worktrees = append(c.Worktrees, detectWorktrees(p.Path)...)
		if members, ok := procMembers[p.Path]; ok {
			c.Members = append(c.Members, members...)
		}
		if len(c.Worktrees) > 0 || len(c.Members) > 0 || len(c.Markers) > 0 {
			colonies = append(colonies, c)
		}
	}
	return colonies
}

func detectMarkers(root string) []string {
	var markers []string
	for _, name := range []string{".colony", ".agents"} {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			markers = append(markers, name)
		}
	}
	return markers
}

func detectWorktrees(root string) []Worktree {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil
	}
	cmd := exec.Command("git", "-C", root, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseWorktrees(out)
}

func parseWorktrees(out []byte) []Worktree {
	var worktrees []Worktree
	var current Worktree
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimSpace(strings.TrimPrefix(line, "worktree "))}
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees
}

func detectProcMembers(roots []string) map[string][]ColonyMember {
	members := make(map[string][]ColonyMember)
	if runtime.GOOS != "linux" {
		return members
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return members
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || cwd == "" {
			continue
		}
		for _, root := range roots {
			if root == "" {
				continue
			}
			if sameOrChild(root, cwd) {
				members[root] = append(members[root], ColonyMember{PID: pid, CWD: cwd, Source: "proc"})
			}
		}
	}
	return members
}

func sameOrChild(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if strings.HasPrefix(rel, "..") {
		return false
	}
	return rel != "."
}
