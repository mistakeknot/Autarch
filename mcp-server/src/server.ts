/**
 * Tandemonium MCP Server
 *
 * Implements the Model Context Protocol server with stdio transport.
 * Provides tools for AI agents to interact with Tandemonium task management.
 */
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { TaskStorage } from "./storage/tasks.js";
import { ErrorCode, McpError } from "./types/index.js";
import { listTasks } from "./tools/list-tasks.js";
import { claimTask } from "./tools/claim-task.js";
import { updateProgress } from "./tools/update-progress.js";
import { completeTask } from "./tools/complete-task.js";

export class TandemoniumServer {
  private server: Server;
  private storage: TaskStorage;

  constructor(projectRoot?: string) {
    this.storage = new TaskStorage(projectRoot);

    this.server = new Server(
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

    this.setupHandlers();
    this.setupErrorHandling();
  }

  /**
   * Set up MCP request handlers
   */
  private setupHandlers(): void {
    // List available tools
    this.server.setRequestHandler(ListToolsRequestSchema, async () => ({
      tools: [
        {
          name: "list_tasks",
          description:
            "List and filter tasks from Tandemonium. Supports filtering by status, assignee, and dependencies.",
          inputSchema: {
            type: "object",
            properties: {
              status: {
                type: "string",
                enum: ["todo", "inprogress", "review", "done", "blocked"],
                description: "Filter tasks by status",
              },
              assigned_to: {
                type: "string",
                description: "Filter tasks by assignee",
              },
              has_dependencies: {
                type: "boolean",
                description: "Filter tasks that have dependencies",
              },
            },
          },
        },
        {
          name: "claim_task",
          description:
            "Claim a task (transition to in-progress) and create git worktree. This is an idempotent operation.",
          inputSchema: {
            type: "object",
            properties: {
              task_id: {
                type: "string",
                description: "Task ID (e.g., tsk_01J6QX3N2Z8)",
                pattern: "^tsk_[0-9A-HJKMNP-TV-Z]{26}$",
              },
              agent_id: {
                type: "string",
                description: "Agent identifier claiming the task",
              },
            },
            required: ["task_id", "agent_id"],
          },
        },
        {
          name: "update_progress",
          description:
            "Update task status, progress value, or mark acceptance criteria as completed.",
          inputSchema: {
            type: "object",
            properties: {
              task_id: {
                type: "string",
                description: "Task ID (e.g., tsk_01J6QX3N2Z8)",
                pattern: "^tsk_[0-9A-HJKMNP-TV-Z]{26}$",
              },
              status: {
                type: "string",
                enum: ["todo", "inprogress", "review", "done", "blocked"],
                description: "New task status",
              },
              progress_value: {
                type: "number",
                minimum: 0,
                maximum: 100,
                description: "Progress percentage (0-100)",
              },
              completed_criteria: {
                type: "array",
                items: { type: "number" },
                description: "Indices of completed acceptance criteria",
              },
            },
            required: ["task_id"],
          },
        },
        {
          name: "complete_task",
          description:
            "Complete a task by creating a pull request and cleaning up the worktree. Runs preflight checks first.",
          inputSchema: {
            type: "object",
            properties: {
              task_id: {
                type: "string",
                description: "Task ID (e.g., tsk_01J6QX3N2Z8)",
                pattern: "^tsk_[0-9A-HJKMNP-TV-Z]{26}$",
              },
              base_branch: {
                type: "string",
                description: "Base branch for pull request",
                default: "main",
              },
            },
            required: ["task_id"],
          },
        },
      ],
    }));

    // Handle tool calls
    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      try {
        switch (request.params.name) {
          case "list_tasks":
            return await this.handleListTasks(request.params.arguments);
          case "claim_task":
            return await this.handleClaimTask(request.params.arguments);
          case "update_progress":
            return await this.handleUpdateProgress(request.params.arguments);
          case "complete_task":
            return await this.handleCompleteTask(request.params.arguments);
          default:
            throw new McpError(
              ErrorCode.INTERNAL,
              `Unknown tool: ${request.params.name}`
            );
        }
      } catch (error) {
        if (error instanceof McpError) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({
                  error: {
                    code: error.code,
                    message: error.message,
                  },
                }),
              },
            ],
            isError: true,
          };
        }

        // Unexpected error
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                error: {
                  code: ErrorCode.INTERNAL,
                  message:
                    error instanceof Error ? error.message : String(error),
                },
              }),
            },
          ],
          isError: true,
        };
      }
    });
  }

  /**
   * Set up error handling
   */
  private setupErrorHandling(): void {
    this.server.onerror = (error) => {
      console.error("[MCP Error]", error);
    };

    process.on("SIGINT", async () => {
      await this.close();
      process.exit(0);
    });

    process.on("SIGTERM", async () => {
      await this.close();
      process.exit(0);
    });
  }

  /**
   * Handle list_tasks tool
   */
  private async handleListTasks(args: unknown): Promise<{ content: Array<{ type: string; text: string }> }> {
    return await listTasks(this.storage, args);
  }

  /**
   * Handle claim_task tool
   */
  private async handleClaimTask(args: unknown): Promise<{ content: Array<{ type: string; text: string }> }> {
    return await claimTask(this.storage, args);
  }

  /**
   * Handle update_progress tool
   */
  private async handleUpdateProgress(args: unknown): Promise<{ content: Array<{ type: string; text: string }> }> {
    return await updateProgress(this.storage, args);
  }

  /**
   * Handle complete_task tool
   */
  private async handleCompleteTask(args: unknown): Promise<{ content: Array<{ type: string; text: string }> }> {
    return await completeTask(this.storage, args);
  }

  /**
   * Start the server with stdio transport
   */
  async start(): Promise<void> {
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    console.error("Tandemonium MCP Server running on stdio");
  }

  /**
   * Close the server
   */
  async close(): Promise<void> {
    await this.server.close();
    console.error("Tandemonium MCP Server closed");
  }
}
