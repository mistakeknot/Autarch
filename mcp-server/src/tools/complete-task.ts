/**
 * complete_task MCP Tool
 *
 * Completes a task by validating readiness and updating status to done.
 *
 * NOTE: In P0, this MCP tool validates task completion readiness and updates
 * task state. The actual PR creation and worktree cleanup are handled by the
 * Tauri backend when the user completes the task through the UI.
 *
 * The workflow is:
 * 1. MCP Server: Validates task is ready (criteria completed, status is review)
 * 2. MCP Server: Updates task status to done
 * 3. Tauri Backend: Creates PR and cleans up worktree (when user triggers in UI)
 */
import { TaskStorage } from "../storage/tasks.js";
import { CompleteTaskInputSchema, CompleteTaskInput } from "../schemas/index.js";
import { ErrorCode, McpError, TaskStatus } from "../types/index.js";

export async function completeTask(
  storage: TaskStorage,
  args: unknown
): Promise<{ content: Array<{ type: string; text: string }> }> {
  // Validate input
  const input = CompleteTaskInputSchema.parse(args) as CompleteTaskInput;

  // Get the task
  const task = await storage.getTask(input.task_id);

  // Validate task is in review status
  if (task.status !== TaskStatus.Review) {
    throw new McpError(
      ErrorCode.INVALID_STATE,
      `Task ${input.task_id} must be in 'review' status to complete. Current status: ${task.status}`
    );
  }

  // Validate all acceptance criteria are completed
  if (task.acceptance_criteria.length > 0) {
    const incompleteCriteria = task.acceptance_criteria.filter((c) => !c.completed);
    if (incompleteCriteria.length > 0) {
      throw new McpError(
        ErrorCode.INVALID_STATE,
        `Task ${input.task_id} has ${incompleteCriteria.length} incomplete acceptance criteria:\n` +
          incompleteCriteria.map((c) => `  - ${c.text}`).join("\n")
      );
    }
  }

  // Update task status to done
  const updatedTask = await storage.updateTask(input.task_id, {
    status: TaskStatus.Done,
  });

  return {
    content: [
      {
        type: "text",
        text: JSON.stringify(
          {
            success: true,
            message: `Task ${input.task_id} marked as done`,
            task: {
              id: updatedTask.id,
              title: updatedTask.title,
              status: updatedTask.status,
              branch: updatedTask.branch,
              worktree: updatedTask.worktree,
            },
            next_steps: [
              "Task is now marked as done in tasks.yml",
              "To create a PR and cleanup worktree, use the Tauri app UI:",
              "  1. Open the task in Tandemonium app",
              "  2. Click 'Create Pull Request'",
              "  3. The app will run preflight checks, create PR, and cleanup worktree",
              "",
              "Or use the Rust backend directly:",
              `  - Branch: ${updatedTask.branch || "not set"}`,
              `  - Base: ${input.base_branch}`,
              "  - Command: Use the Tauri backend's complete_task command",
            ],
            note: "PR creation and worktree cleanup are handled by the Tauri backend for P0",
          },
          null,
          2
        ),
      },
    ],
  };
}
