#!/usr/bin/env node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

// Mock task database
interface Task {
  id: string;
  title: string;
  status: "todo" | "in_progress" | "done" | "blocked";
  assigned_to: string | null;
  progress: number;
}

const tasks: Map<string, Task> = new Map([
  [
    "task-1",
    {
      id: "task-1",
      title: "Implement user authentication",
      status: "todo",
      assigned_to: null,
      progress: 0,
    },
  ],
  [
    "task-2",
    {
      id: "task-2",
      title: "Create database schema",
      status: "todo",
      assigned_to: null,
      progress: 0,
    },
  ],
  [
    "task-3",
    {
      id: "task-3",
      title: "Build REST API endpoints",
      status: "todo",
      assigned_to: null,
      progress: 0,
    },
  ],
]);

// Create server instance
const server = new Server(
  {
    name: "tandemonium-mcp-server",
    version: "0.1.0",
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

// Register tool handlers
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: [
      {
        name: "list_tasks",
        description: "List all tasks with their current status",
        inputSchema: {
          type: "object",
          properties: {
            status: {
              type: "string",
              description:
                "Filter by status (optional): todo, in_progress, done, blocked",
              enum: ["todo", "in_progress", "done", "blocked"],
            },
          },
        },
      },
      {
        name: "claim_task",
        description: "Claim a task and assign it to an agent",
        inputSchema: {
          type: "object",
          properties: {
            task_id: {
              type: "string",
              description: "ID of the task to claim",
            },
            agent_id: {
              type: "string",
              description: "ID of the agent claiming the task",
            },
          },
          required: ["task_id", "agent_id"],
        },
      },
      {
        name: "update_progress",
        description: "Update task progress and optionally change status",
        inputSchema: {
          type: "object",
          properties: {
            task_id: {
              type: "string",
              description: "ID of the task to update",
            },
            progress: {
              type: "number",
              description: "Progress percentage (0-100)",
              minimum: 0,
              maximum: 100,
            },
            status: {
              type: "string",
              description: "New status (optional)",
              enum: ["todo", "in_progress", "done", "blocked"],
            },
          },
          required: ["task_id", "progress"],
        },
      },
      {
        name: "complete_task",
        description: "Mark a task as complete",
        inputSchema: {
          type: "object",
          properties: {
            task_id: {
              type: "string",
              description: "ID of the task to complete",
            },
          },
          required: ["task_id"],
        },
      },
    ],
  };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  try {
    switch (name) {
      case "list_tasks": {
        const statusFilter = args?.status as string | undefined;
        const taskList = Array.from(tasks.values()).filter((task) =>
          statusFilter ? task.status === statusFilter : true
        );

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(
                {
                  tasks: taskList,
                  count: taskList.length,
                },
                null,
                2
              ),
            },
          ],
        };
      }

      case "claim_task": {
        const taskId = args?.task_id as string;
        const agentId = args?.agent_id as string;

        if (!taskId || !agentId) {
          throw new Error("Missing required parameters: task_id and agent_id");
        }

        const task = tasks.get(taskId);
        if (!task) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({
                  error: "NOT_FOUND",
                  message: `Task ${taskId} not found`,
                }),
              },
            ],
            isError: true,
          };
        }

        if (task.assigned_to && task.assigned_to !== agentId) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({
                  error: "ALREADY_CLAIMED",
                  message: `Task ${taskId} is already claimed by ${task.assigned_to}`,
                  claimed_by: task.assigned_to,
                }),
              },
            ],
            isError: true,
          };
        }

        task.assigned_to = agentId;
        task.status = "in_progress";

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                success: true,
                task: task,
              }),
            },
          ],
        };
      }

      case "update_progress": {
        const taskId = args?.task_id as string;
        const progress = args?.progress as number;
        const status = args?.status as Task["status"] | undefined;

        if (!taskId || progress === undefined) {
          throw new Error("Missing required parameters: task_id and progress");
        }

        const task = tasks.get(taskId);
        if (!task) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({
                  error: "NOT_FOUND",
                  message: `Task ${taskId} not found`,
                }),
              },
            ],
            isError: true,
          };
        }

        task.progress = progress;
        if (status) {
          task.status = status;
        }

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                success: true,
                task: task,
              }),
            },
          ],
        };
      }

      case "complete_task": {
        const taskId = args?.task_id as string;

        if (!taskId) {
          throw new Error("Missing required parameter: task_id");
        }

        const task = tasks.get(taskId);
        if (!task) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({
                  error: "NOT_FOUND",
                  message: `Task ${taskId} not found`,
                }),
              },
            ],
            isError: true,
          };
        }

        task.status = "done";
        task.progress = 100;

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                success: true,
                task: task,
              }),
            },
          ],
        };
      }

      default:
        throw new Error(`Unknown tool: ${name}`);
    }
  } catch (error) {
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify({
            error: "INTERNAL",
            message: error instanceof Error ? error.message : String(error),
          }),
        },
      ],
      isError: true,
    };
  }
});

// Start server
async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Tandemonium MCP Server running on stdio");
}

main().catch((error) => {
  console.error("Server error:", error);
  process.exit(1);
});
