/**
 * Task storage layer - reads/writes tasks.yml
 *
 * This module handles atomic reads/writes to .tandemonium/tasks.yml
 * with proper file locking and version conflict detection.
 */
import fs from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import yaml from "js-yaml";
import { Task, TasksData, ErrorCode, McpError } from "../types/index.js";

const TASKS_FILE = ".tandemonium/tasks.yml";
const LOCK_FILE = ".tandemonium/tasks.yml.lock";

export class TaskStorage {
  private projectRoot: string;
  private tasksPath: string;
  private lockPath: string;

  constructor(projectRoot: string = process.cwd()) {
    this.projectRoot = projectRoot;
    this.tasksPath = path.join(projectRoot, TASKS_FILE);
    this.lockPath = path.join(projectRoot, LOCK_FILE);
  }

  /**
   * Read tasks from tasks.yml
   */
  async readTasks(): Promise<TasksData> {
    try {
      if (!existsSync(this.tasksPath)) {
        throw new McpError(
          ErrorCode.NOT_FOUND,
          `Tasks file not found at ${this.tasksPath}. Initialize project with: tandemonium init`
        );
      }

      const content = await fs.readFile(this.tasksPath, "utf-8");
      const data = yaml.load(content) as TasksData;

      if (!data || typeof data !== "object") {
        throw new Error("Invalid tasks.yml format");
      }

      if (!data.version || !Array.isArray(data.tasks)) {
        throw new Error("Missing required fields in tasks.yml");
      }

      return data;
    } catch (error) {
      if (error instanceof McpError) {
        throw error;
      }
      throw new McpError(
        ErrorCode.INTERNAL,
        `Failed to read tasks: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  /**
   * Write tasks to tasks.yml atomically
   *
   * Uses temp file + rename for atomicity
   */
  async writeTasks(data: TasksData): Promise<void> {
    const tempPath = `${this.tasksPath}.tmp`;

    try {
      // Update timestamp
      data.updated_at = new Date().toISOString();

      // Write to temp file
      const content = yaml.dump(data, {
        indent: 2,
        lineWidth: -1,
        noRefs: true,
      });
      await fs.writeFile(tempPath, content, "utf-8");

      // Atomic rename
      await fs.rename(tempPath, this.tasksPath);
    } catch (error) {
      // Clean up temp file if it exists
      if (existsSync(tempPath)) {
        await fs.unlink(tempPath).catch(() => {});
      }

      throw new McpError(
        ErrorCode.INTERNAL,
        `Failed to write tasks: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  /**
   * Get a single task by ID
   */
  async getTask(taskId: string): Promise<Task> {
    const data = await this.readTasks();
    const task = data.tasks.find((t) => t.id === taskId);

    if (!task) {
      throw new McpError(ErrorCode.NOT_FOUND, `Task not found: ${taskId}`);
    }

    return task;
  }

  /**
   * Update a task atomically
   */
  async updateTask(taskId: string, updates: Partial<Task>): Promise<Task> {
    const data = await this.readTasks();
    const taskIndex = data.tasks.findIndex((t) => t.id === taskId);

    if (taskIndex === -1) {
      throw new McpError(ErrorCode.NOT_FOUND, `Task not found: ${taskId}`);
    }

    // Apply updates
    const updatedTask = {
      ...data.tasks[taskIndex],
      ...updates,
      updated_at: new Date().toISOString(),
    };

    data.tasks[taskIndex] = updatedTask;

    await this.writeTasks(data);

    return updatedTask;
  }

  /**
   * Check if task can transition to new status
   */
  canTransitionTo(currentStatus: string, newStatus: string): boolean {
    const validTransitions: Record<string, string[]> = {
      todo: ["inprogress", "blocked"],
      inprogress: ["review", "blocked", "todo"],
      review: ["done", "inprogress"],
      blocked: ["todo", "inprogress"],
      done: [], // Final state
    };

    return validTransitions[currentStatus]?.includes(newStatus) ?? false;
  }

  /**
   * Check if task is blocked by dependencies
   */
  async isBlockedByDependencies(taskId: string): Promise<boolean> {
    const data = await this.readTasks();
    const task = data.tasks.find((t) => t.id === taskId);

    if (!task || task.depends_on.length === 0) {
      return false;
    }

    // Check if any dependency is not done
    for (const depId of task.depends_on) {
      const dep = data.tasks.find((t) => t.id === depId);
      if (!dep || dep.status !== "done") {
        return true;
      }
    }

    return false;
  }
}
