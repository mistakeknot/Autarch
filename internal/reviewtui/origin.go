package reviewtui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

type originCommand func(context.Context, string, ...string) ([]byte, error)

func terminalOrigin(ctx context.Context) *review.TerminalContext {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cwd, _ := os.Getwd()
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.WaitDelay = 100 * time.Millisecond
		return cmd.Output()
	}
	origin := resolveTerminalOrigin(ctx, os.Getpid(), os.Getenv("TMUX_PANE"), cwd, run)
	if tmux := os.Getenv("TMUX"); origin.Pane != "" && tmux != "" {
		if end := strings.LastIndex(tmux, ","); end > 0 {
			if split := strings.LastIndex(tmux[:end], ","); split > 0 {
				origin.TmuxSocket = tmux[:split]
			}
		}
	}
	// Query the already-installed companion executable before the controller
	// opens its window. This read-only mode exits before constructing any UI.
	if exe, err := os.Executable(); err == nil && len(origin.ProcessIDs) > 0 {
		ids := make([]string, len(origin.ProcessIDs))
		for i, pid := range origin.ProcessIDs {
			ids[i] = strconv.Itoa(pid)
		}
		data, err := run(ctx, filepath.Join(filepath.Dir(exe), "AutarchCapture.app", "Contents", "MacOS", "AutarchCapture"), "--capture-origin", strings.Join(ids, ","))
		if err == nil {
			var result struct {
				WindowID uint32 `json:"window_id"`
			}
			if json.Unmarshal(data, &result) == nil {
				origin.WindowID = result.WindowID
			}
		}
	}
	return origin
}

func resolveTerminalOrigin(ctx context.Context, pid int, pane, cwd string, run originCommand) *review.TerminalContext {
	origin := &review.TerminalContext{PID: pid, Pane: pane, Cwd: cwd}
	roots := []int{pid}
	if pane != "" {
		// Never follow the tmux server's ancestry: it may have been started from
		// a different terminal days ago. Only clients displaying this pane count.
		roots = nil
		info, err := run(ctx, "tmux", "display-message", "-p", "-t", pane, "#{session_id}\t#{window_id}\t#{pane_id}\t#{pane_current_path}")
		if err != nil {
			return origin
		}
		fields := strings.SplitN(strings.TrimSuffix(string(info), "\n"), "\t", 4)
		if len(fields) != 4 || fields[2] != pane {
			return origin
		}
		origin.Session, origin.Window, origin.Cwd = fields[0], fields[1], fields[3]
		clients, err := run(ctx, "tmux", "list-clients", "-F", "#{client_pid}\t#{pane_id}")
		if err != nil {
			return origin
		}
		for _, line := range strings.Split(string(clients), "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[1] == pane {
				if client, err := strconv.Atoi(parts[0]); err == nil && client > 1 {
					roots = append(roots, client)
				}
			}
		}
		origin.ClientPIDs = append([]int(nil), roots...)
	}
	if len(roots) == 0 {
		return origin
	}
	processes, err := run(ctx, "ps", "-axo", "pid=,ppid=")
	if err != nil {
		return origin
	}
	parents := map[int]int{}
	for _, line := range strings.Split(string(processes), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		child, _ := strconv.Atoi(parts[0])
		parent, _ := strconv.Atoi(parts[1])
		parents[child] = parent
	}
	seen := map[int]bool{}
	for _, root := range roots {
		for p := root; p > 1 && !seen[p]; p = parents[p] {
			seen[p] = true
			origin.ProcessIDs = append(origin.ProcessIDs, p)
		}
	}
	return origin
}
