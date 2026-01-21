package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/spf13/cobra"
)

func AgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent registry",
	}
	cmd.AddCommand(agentEnsureCmd(), agentRegisterCmd(), agentWhoisCmd(), agentHealthCmd())
	return cmd
}

func agentEnsureCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Ensure .tandemonium and agent registry exist",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("agent ensure", err)
				}
			}()
			root, created, err := ensureProjectAndDB()
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"project_root": root,
					"initialized":  true,
					"created":      created,
				}
				return writeJSON(cmd, payload)
			}
			if created {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", root)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Ready %s\n", root)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func agentRegisterCmd() *cobra.Command {
	var name string
	var program string
	var model string
	var task string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register or update an agent",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("agent register", err)
				}
			}()
			if name == "" {
				return fmt.Errorf("agent name required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			profile, err := storage.UpsertAgent(db, storage.AgentProfile{
				Name:            name,
				Program:         program,
				Model:           model,
				TaskDescription: task,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"name":             profile.Name,
					"program":          profile.Program,
					"model":            profile.Model,
					"task_description": profile.TaskDescription,
					"created_ts":       profile.CreatedAt,
					"updated_ts":       profile.UpdatedAt,
					"last_active_ts":   profile.LastActiveAt,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered %s\n", profile.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Agent name")
	cmd.Flags().StringVar(&program, "program", "", "Agent program")
	cmd.Flags().StringVar(&model, "model", "", "Agent model")
	cmd.Flags().StringVar(&task, "task", "", "Task description")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func agentWhoisCmd() *cobra.Command {
	var name string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "whois",
		Short: "Show agent profile",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("agent whois", err)
				}
			}()
			if name == "" {
				return fmt.Errorf("agent name required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			profile, err := storage.GetAgent(db, name)
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"name":             profile.Name,
					"program":          profile.Program,
					"model":            profile.Model,
					"task_description": profile.TaskDescription,
					"created_ts":       profile.CreatedAt,
					"updated_ts":       profile.UpdatedAt,
					"last_active_ts":   profile.LastActiveAt,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s program=%s model=%s\n", profile.Name, profile.Program, profile.Model)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Agent name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func agentHealthCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check agent registry health",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("agent health", err)
				}
			}()
			_, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			ts := time.Now().UTC().Format(time.RFC3339Nano)
			if jsonOut {
				payload := map[string]interface{}{
					"status":    "ok",
					"timestamp": ts,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok %s\n", ts)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func ensureProjectAndDB() (string, bool, error) {
	cwd, _ := os.Getwd()
	root, err := project.FindRoot(cwd)
	created := false
	if err != nil {
		if err := project.Init(cwd); err != nil {
			return "", false, err
		}
		root = cwd
		created = true
	}
	db, err := storage.Open(project.StateDBPath(root))
	if err != nil {
		return "", false, err
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		return "", false, err
	}
	_ = db.Close()
	return root, created, nil
}
