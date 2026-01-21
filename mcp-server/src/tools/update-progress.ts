/**
 * update_progress MCP Tool
 *
 * Updates task status, progress value, or marks acceptance criteria as completed.
 * Supports atomic updates to tasks.yml with proper validation.
 */
import { TaskStorage } from "../storage/tasks.js";
import { UpdateProgressInputSchema, UpdateProgressInput } from "../schemas/index.js";
import { ErrorCode, McpError, Task, TaskStatus, ProgressMode } from "../types/index.js";

export async function updateProgress(
  storage: TaskStorage,
  args: unknown
): Promise<{ content: Array<{ type: string; text: string }> }> {
  // Validate input
  const input = UpdateProgressInputSchema.parse(args) as UpdateProgressInput;

  // Get the task
  const task = await storage.getTask(input.task_id);

  // Build updates object
  const updates: Partial<Task> = {};

  // Update status if provided
  if (input.status) {
    // Validate transition
    if (!storage.canTransitionTo(task.status, input.status)) {
      throw new McpError(
        ErrorCode.INVALID_STATE,
        `Task ${input.task_id} cannot transition from ${task.status} to ${input.status}`
      );
    }

    // Check if transitioning to blocked requires dependencies
    if (input.status === TaskStatus.Blocked) {
      const isBlocked = await storage.isBlockedByDependencies(input.task_id);
      if (!isBlocked && task.depends_on.length > 0) {
        throw new McpError(
          ErrorCode.INVALID_STATE,
          `Cannot mark task as blocked - all dependencies are completed`
        );
      }
    }

    updates.status = input.status as TaskStatus;
  }

  // Update progress value if provided
  if (input.progress_value !== undefined) {
    if (task.progress.mode !== "manual") {
      throw new McpError(
        ErrorCode.INVALID_STATE,
        `Task ${input.task_id} has automatic progress mode. Cannot set manual progress value.`
      );
    }
    updates.progress = {
      ...task.progress,
      value: input.progress_value,
    };
  }

  // Update completed criteria if provided
  if (input.completed_criteria && input.completed_criteria.length > 0) {
    const criteriaCount = task.acceptance_criteria.length;

    // Validate indices
    for (const index of input.completed_criteria) {
      if (index < 0 || index >= criteriaCount) {
        throw new McpError(
          ErrorCode.INVALID_STATE,
          `Invalid criteria index: ${index}. Task has ${criteriaCount} criteria (indices 0-${criteriaCount - 1})`
        );
      }
    }

    // Update criteria
    const updatedCriteria = task.acceptance_criteria.map((criteria, index) => {
      if (input.completed_criteria!.includes(index)) {
        return { ...criteria, completed: true };
      }
      return criteria;
    });

    updates.acceptance_criteria = updatedCriteria;

    // Auto-update progress if mode is automatic
    if (task.progress.mode === "automatic") {
      const completedCount = updatedCriteria.filter((c) => c.completed).length;
      const progressValue = criteriaCount > 0
        ? Math.round((completedCount / criteriaCount) * 100)
        : 0;

      updates.progress = {
        mode: ProgressMode.Automatic,
        value: progressValue,
      };
    }
  }

  // Apply updates
  const updatedTask = await storage.updateTask(input.task_id, updates);

  // Calculate completion stats
  const completedCriteria = updatedTask.acceptance_criteria.filter((c) => c.completed).length;
  const totalCriteria = updatedTask.acceptance_criteria.length;

  return {
    content: [
      {
        type: "text",
        text: JSON.stringify(
          {
            success: true,
            message: `Task ${input.task_id} progress updated`,
            task: {
              id: updatedTask.id,
              title: updatedTask.title,
              status: updatedTask.status,
              progress: updatedTask.progress,
              acceptance_criteria_completed: `${completedCriteria}/${totalCriteria}`,
              updated_at: updatedTask.updated_at,
            },
            changes: {
              status: input.status ? `${task.status} → ${input.status}` : undefined,
              progress_value: input.progress_value !== undefined
                ? `${task.progress.value} → ${updatedTask.progress.value}`
                : undefined,
              criteria_completed: input.completed_criteria
                ? `Marked ${input.completed_criteria.length} criteria as completed`
                : undefined,
            },
          },
          null,
          2
        ),
      },
    ],
  };
}
