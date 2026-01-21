/**
 * claim_task MCP Tool
 *
 * Claims a task by transitioning it to in_progress and assigning it to an agent.
 * This is an idempotent operation - if the task is already claimed by the same agent,
 * it returns success.
 *
 * NOTE: In P0, worktree creation is handled by the Tauri backend when the user
 * starts working on the task in the UI. The MCP server only manages task state.
 */
import { TaskStorage } from "../storage/tasks.js";
import { ClaimTaskInputSchema, ClaimTaskInput } from "../schemas/index.js";
import { ErrorCode, McpError, TaskStatus } from "../types/index.js";

export async function claimTask(
  storage: TaskStorage,
  args: unknown
): Promise<{ content: Array<{ type: string; text: string }> }> {
  // Validate input
  const input = ClaimTaskInputSchema.parse(args) as ClaimTaskInput;

  // Get the task
  const task = await storage.getTask(input.task_id);

  // Check for idempotency: if already claimed by this agent, return success
  if (task.status === TaskStatus.InProgress && task.assigned_to === input.agent_id) {
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify(
            {
              success: true,
              message: `Task ${input.task_id} already claimed by ${input.agent_id}`,
              task: {
                id: task.id,
                title: task.title,
                status: task.status,
                assigned_to: task.assigned_to,
              },
            },
            null,
            2
          ),
        },
      ],
    };
  }

  // Check if task is blocked by dependencies
  const isBlocked = await storage.isBlockedByDependencies(input.task_id);
  if (isBlocked) {
    throw new McpError(
      ErrorCode.BLOCKED,
      `Task ${input.task_id} is blocked by incomplete dependencies`
    );
  }

  // Check if task is already claimed by someone else
  if (task.status === TaskStatus.InProgress && task.assigned_to !== input.agent_id) {
    throw new McpError(
      ErrorCode.ALREADY_CLAIMED,
      `Task ${input.task_id} is already claimed by ${task.assigned_to}`
    );
  }

  // Check if task can transition to in_progress
  if (!storage.canTransitionTo(task.status, TaskStatus.InProgress)) {
    throw new McpError(
      ErrorCode.INVALID_STATE,
      `Task ${input.task_id} cannot transition from ${task.status} to in_progress`
    );
  }

  // Claim the task
  const updatedTask = await storage.updateTask(input.task_id, {
    status: TaskStatus.InProgress,
    assigned_to: input.agent_id,
  });

  return {
    content: [
      {
        type: "text",
        text: JSON.stringify(
          {
            success: true,
            message: `Task ${input.task_id} claimed by ${input.agent_id}`,
            task: {
              id: updatedTask.id,
              title: updatedTask.title,
              status: updatedTask.status,
              assigned_to: updatedTask.assigned_to,
              description: updatedTask.description,
              acceptance_criteria: updatedTask.acceptance_criteria,
              tests: updatedTask.tests,
              depends_on: updatedTask.depends_on,
            },
            next_steps: [
              "Review the task description and acceptance criteria",
              "Check the file scope to understand what files to modify",
              "Use update_progress to mark acceptance criteria as completed",
              "Use complete_task when all criteria are met and work is done",
            ],
          },
          null,
          2
        ),
      },
    ],
  };
}
