package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// filerTimeout bounds each bb call; tests shorten it.
var filerTimeout = 10 * time.Second

func newFiler() *homeask.ExecFiler {
	return &homeask.ExecFiler{Timeout: filerTimeout, RecoveryTimeout: filerTimeout}
}

func gitRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return cwd
	}
	return strings.TrimSpace(string(out))
}

// resolveThread applies the thread rules: --thread or $BB_THREAD_ID, and a flag that
// disagrees with a non-empty $BB_THREAD_ID is refused.
func resolveThread(flag string) (string, error) {
	env := os.Getenv("BB_THREAD_ID")
	if flag != "" && env != "" && flag != env {
		return "", &usageError{"thread flag conflicts with BB_THREAD_ID"}
	}
	if flag != "" {
		return flag, nil
	}
	return env, nil
}

// buildAsk fills what the caller's context knows (thread, asker, project root) into
// the JSON on stdin, then validates it as any other filing.
func buildAsk(raw []byte, thread, asker, projectRoot, project string) (homeask.Ask, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return homeask.Ask{}, &usageError{"stdin is not a JSON object: " + err.Error()}
	}
	if asker == "" {
		if s, ok := m["asker"].(string); ok {
			asker = s
		} else {
			asker = "thread"
		}
	}
	m["asker"] = asker
	if asker != "mycroft" {
		if t, ok := m["thread"].(string); ok && t != "" && thread != "" && t != thread {
			return homeask.Ask{}, &usageError{"thread in the request conflicts with the caller's thread"}
		}
		if thread == "" {
			if t, ok := m["thread"].(string); ok {
				thread = t
			}
		}
		if thread == "" {
			return homeask.Ask{}, &usageError{"a thread is required: pass --thread or run inside a bb thread"}
		}
		m["thread"] = thread
	}
	if _, ok := m["project_root"]; !ok {
		if projectRoot == "" {
			projectRoot = gitRoot()
		}
		m["project_root"] = projectRoot
	}
	if _, ok := m["project"]; !ok {
		if project == "" {
			project = filepath.Base(m["project_root"].(string))
		}
		m["project"] = project
	}
	b, err := json.Marshal(m)
	if err != nil {
		return homeask.Ask{}, &usageError{err.Error()}
	}
	a, err := homeask.Parse(b)
	if err != nil {
		return homeask.Ask{}, fmt.Errorf("%w: %v", homeask.ErrInvalid, err)
	}
	return a, nil
}

func decideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "decide",
		Short:         "File asks with mk through Home (no pick: mk rules in Home)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(decideFileCmd(), decideBBPassthrough("list", "List the asks waiting for a ruling", []string{"asker", "project"}), decideBBPassthrough("stats", "Picks, delegation and filing counts", []string{"since"}))
	return cmd
}

func decideFileCmd() *cobra.Command {
	var thread, asker, projectRoot, project string
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File the ask JSON on stdin; prints the decision id",
		Long: `Exit codes: 2 validation, 3 Home down or not filed, 4 outcome unknown
(re-run the same command; it is idempotent), 5 already ruled.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 16*1024+1))
			if err != nil {
				return err
			}
			if len(raw) > 16*1024 {
				return &usageError{"request is over 16 KiB"}
			}
			t, err := resolveThread(thread)
			if err != nil {
				return err
			}
			a, err := buildAsk(raw, t, asker, projectRoot, project)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			id, err := newFiler().File(ctx, a)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), id)
			return nil
		},
	}
	cmd.Flags().StringVar(&thread, "thread", "", "Asking thread (default $BB_THREAD_ID)")
	cmd.Flags().StringVar(&asker, "asker", "", "thread (default) or mycroft")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root (default: the git root)")
	cmd.Flags().StringVar(&project, "project", "", "Project name (default: the root's directory name)")
	return cmd
}

// decideBBPassthrough runs `bb home <verb> --json [flags]` and streams its output.
func decideBBPassthrough(verb, short string, flags []string) *cobra.Command {
	vals := make(map[string]*string, len(flags))
	cmd := &cobra.Command{
		Use:          verb,
		Short:        short,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			argv := []string{"home", verb}
			for _, f := range flags {
				if v := *vals[f]; v != "" {
					argv = append(argv, "--"+f, v)
				}
			}
			argv = append(argv, "--json")
			ctx, cancel := context.WithTimeout(context.Background(), filerTimeout)
			defer cancel()
			c := exec.CommandContext(ctx, "bb", argv...)
			c.WaitDelay = time.Second
			var stderr strings.Builder
			c.Stdout, c.Stderr = cmd.OutOrStdout(), &stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("%w: bb home %s: %v %s", homeask.ErrHomeDown, verb, err, strings.TrimSpace(stderr.String()))
			}
			return nil
		},
	}
	for _, f := range flags {
		vals[f] = new(string)
		cmd.Flags().StringVar(vals[f], f, "", "")
	}
	return cmd
}
