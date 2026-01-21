/**
 * list_tasks MCP Tool
 *
 * Lists and filters tasks from Tandemonium's tasks.yml
 */
import { TaskStorage } from "../storage/tasks.js";
import { ListTasksInputSchema, ListTasksInput } from "../schemas/index.js";
import { Task } from "../types/index.js";

export async function listTasks(
  storage: TaskStorage,
  args: unknown
): Promise<{ content: Array<{ type: string; text: string }> }> {
  // Validate input
  const input = ListTasksInputSchema.parse(args) as ListTasksInput;

  // Read all tasks
  const data = await storage.readTasks();
  let filteredTasks: Task[] = data.tasks;

  // Apply filters
  if (input.status) {
    filteredTasks = filteredTasks.filter((task) => task.status === input.status);
  }

  if (input.assigned_to) {
    filteredTasks = filteredTasks.filter(
      (task) => task.assigned_to === input.assigned_to
    );
  }

  if (input.has_dependencies !== undefined) {
    if (input.has_dependencies) {
      // Only tasks with dependencies
      filteredTasks = filteredTasks.filter((task) => task.depends_on.length > 0);
    } else {
      // Only tasks without dependencies
      filteredTasks = filteredTasks.filter((task) => task.depends_on.length === 0);
    }
  }

  // Return filtered tasks
  return {
    content: [
      {
        type: "text",
        text: JSON.stringify(
          {
            tasks: filteredTasks,
            count: filteredTasks.length,
            filters: input,
          },
          null,
          2
        ),
      },
    ],
  };
}
