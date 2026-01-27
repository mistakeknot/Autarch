package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// TaskCmd provides task management commands.
func TaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(
		taskListCmd(),
		taskShowCmd(),
		taskAssignCmd(),
		taskBlockCmd(),
		taskUnblockCmd(),
		taskCompleteCmd(),
	)
	return cmd
}

func taskListCmd() *cobra.Command {
	var status string
	var assignee string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			tasks, err := loadTasks(root)
			if err != nil {
				return err
			}

			// Filter
			var filtered []map[string]interface{}
			for _, t := range tasks {
				if status != "" {
					if s, ok := t["status"].(string); ok && s != status {
						continue
					}
				}
				if assignee != "" {
					if a, ok := t["assignee"].(string); ok && a != assignee {
						continue
					}
				}
				filtered = append(filtered, t)
			}

			if jsonOut {
				return writeJSON(cmd, map[string]interface{}{
					"tasks": filtered,
					"count": len(filtered),
				})
			}

			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tasks found.")
				return nil
			}

			for _, t := range filtered {
				id := t["id"]
				title := t["title"]
				taskStatus := t["status"]
				taskAssignee := t["assignee"]
				fmt.Fprintf(cmd.OutOrStdout(), "%s  [%s]  %s  (assignee: %v)\n",
					id, taskStatus, title, taskAssignee)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <TASK-ID>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			taskID := args[0]
			task, err := loadTask(root, taskID)
			if err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(cmd, task)
			}

			// Print task details
			fmt.Fprintf(cmd.OutOrStdout(), "ID:       %v\n", task["id"])
			fmt.Fprintf(cmd.OutOrStdout(), "Title:    %v\n", task["title"])
			fmt.Fprintf(cmd.OutOrStdout(), "Status:   %v\n", task["status"])
			fmt.Fprintf(cmd.OutOrStdout(), "Assignee: %v\n", task["assignee"])
			if desc, ok := task["description"].(string); ok && desc != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDescription:\n%s\n", desc)
			}
			if reason, ok := task["block_reason"].(string); ok && reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nBlocked: %s\n", reason)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskAssignCmd() *cobra.Command {
	var agent string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "assign <TASK-ID>",
		Short: "Assign a task to an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if agent == "" {
				return fmt.Errorf("--agent is required")
			}

			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			taskID := args[0]
			task, taskPath, err := loadTaskWithPath(root, taskID)
			if err != nil {
				return err
			}

			oldAssignee := task["assignee"]
			task["assignee"] = agent
			task["assigned_at"] = time.Now().Format(time.RFC3339)

			// Update status to in_progress if pending
			if task["status"] == "pending" {
				task["status"] = "in_progress"
			}

			if err := saveTask(taskPath, task); err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(cmd, map[string]interface{}{
					"success":      true,
					"task_id":      taskID,
					"agent":        agent,
					"old_assignee": oldAssignee,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Assigned %s to %s\n", taskID, agent)
			return nil
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "", "Agent to assign (e.g., claude)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskBlockCmd() *cobra.Command {
	var reason string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "block <TASK-ID>",
		Short: "Mark a task as blocked",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reason == "" {
				return fmt.Errorf("--reason is required")
			}

			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			taskID := args[0]
			task, taskPath, err := loadTaskWithPath(root, taskID)
			if err != nil {
				return err
			}

			oldStatus := task["status"]
			task["status"] = "blocked"
			task["block_reason"] = reason
			task["blocked_at"] = time.Now().Format(time.RFC3339)

			if err := saveTask(taskPath, task); err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(cmd, map[string]interface{}{
					"success":    true,
					"task_id":    taskID,
					"old_status": oldStatus,
					"new_status": "blocked",
					"reason":     reason,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Blocked %s: %s\n", taskID, reason)
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for blocking")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskUnblockCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "unblock <TASK-ID>",
		Short: "Remove blocked status from a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			taskID := args[0]
			task, taskPath, err := loadTaskWithPath(root, taskID)
			if err != nil {
				return err
			}

			if task["status"] != "blocked" {
				return fmt.Errorf("task %s is not blocked", taskID)
			}

			task["status"] = "in_progress"
			delete(task, "block_reason")
			delete(task, "blocked_at")

			if err := saveTask(taskPath, task); err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(cmd, map[string]interface{}{
					"success":    true,
					"task_id":    taskID,
					"new_status": "in_progress",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unblocked %s\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskCompleteCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "complete <TASK-ID>",
		Short: "Mark a task as completed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}

			taskID := args[0]
			task, taskPath, err := loadTaskWithPath(root, taskID)
			if err != nil {
				return err
			}

			oldStatus := task["status"]
			task["status"] = "completed"
			task["completed_at"] = time.Now().Format(time.RFC3339)

			if err := saveTask(taskPath, task); err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(cmd, map[string]interface{}{
					"success":    true,
					"task_id":    taskID,
					"old_status": oldStatus,
					"new_status": "completed",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Completed %s\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

// Helper functions

func loadTasks(root string) ([]map[string]interface{}, error) {
	tasksDir := filepath.Join(root, ".coldwine", "tasks")
	if _, err := os.Stat(tasksDir); os.IsNotExist(err) {
		// Also check .tandemonium for backward compatibility
		tasksDir = filepath.Join(root, ".tandemonium", "specs")
	}

	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(tasksDir, entry.Name()))
		if err != nil {
			continue
		}

		var task map[string]interface{}
		if err := yaml.Unmarshal(data, &task); err != nil {
			continue
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

func loadTask(root, taskID string) (map[string]interface{}, error) {
	task, _, err := loadTaskWithPath(root, taskID)
	return task, err
}

func loadTaskWithPath(root, taskID string) (map[string]interface{}, string, error) {
	// Normalize ID
	if !strings.HasSuffix(taskID, ".yaml") {
		taskID = taskID + ".yaml"
	}

	// Try .coldwine first, then .tandemonium
	tasksDir := filepath.Join(root, ".coldwine", "tasks")
	taskPath := filepath.Join(tasksDir, taskID)

	if _, err := os.Stat(taskPath); os.IsNotExist(err) {
		tasksDir = filepath.Join(root, ".tandemonium", "specs")
		taskPath = filepath.Join(tasksDir, taskID)
	}

	data, err := os.ReadFile(taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("task not found: %s", taskID)
		}
		return nil, "", err
	}

	var task map[string]interface{}
	if err := yaml.Unmarshal(data, &task); err != nil {
		return nil, "", err
	}

	return task, taskPath, nil
}

func saveTask(path string, task map[string]interface{}) error {
	data, err := yaml.Marshal(task)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
