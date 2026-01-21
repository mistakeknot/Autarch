/**
 * Test helper utilities for MCP server tests
 */

import { Task, TaskStatus, ProgressMode, TasksData } from "../types/index.js";
import { TaskStorage } from "../storage/tasks.js";

/**
 * Create a mock task with sensible defaults
 */
export function createMockTask(overrides?: Partial<Task>): Task {
  return {
    id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",  // Valid 26-character ULID
    slug: "test-task",
    title: "Test Task",
    description: "A test task for unit tests",
    status: TaskStatus.Todo,
    assigned_to: null,
    branch: null,
    worktree: null,
    base_sha: null,
    pr_url: null,
    scope: {
      files_glob: [],
      files_resolved: [],
      locked_paths: [],
      shared_with: [],
    },
    acceptance_criteria: [
      {
        text: "Criterion 1",
        completed: false,
      },
      {
        text: "Criterion 2",
        completed: false,
      },
    ],
    progress: {
      mode: ProgressMode.Automatic,
      value: 0,
    },
    tests: [],
    depends_on: [],
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

/**
 * Create mock tasks data
 */
export function createMockTasksData(tasks?: Task[]): TasksData {
  return {
    version: 1,
    updated_at: "2025-01-01T00:00:00Z",
    tasks: tasks || [
      createMockTask({ id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC", title: "Task 1" }),
      createMockTask({ id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF", title: "Task 2", status: TaskStatus.InProgress }),
      createMockTask({ id: "tsk_01J6QX3N2ZAYFGHJ1234567GHJ", title: "Task 3", status: TaskStatus.Done }),
    ],
  };
}

/**
 * Mock TaskStorage implementation for testing
 */
export class MockTaskStorage extends TaskStorage {
  private mockData: TasksData;

  constructor(tasksData?: TasksData) {
    super(); // Call parent constructor with no args
    this.mockData = tasksData || createMockTasksData();
  }

  async readTasks(): Promise<TasksData> {
    return { ...this.mockData };
  }

  async writeTasks(data: TasksData): Promise<void> {
    this.mockData = { ...data };
  }

  async getTask(taskId: string): Promise<Task> {
    const task = this.mockData.tasks.find((t) => t.id === taskId);
    if (!task) {
      throw new Error(`Task not found: ${taskId}`);
    }
    return { ...task };
  }

  async updateTask(taskId: string, updates: Partial<Task>): Promise<Task> {
    const taskIndex = this.mockData.tasks.findIndex((t) => t.id === taskId);
    if (taskIndex === -1) {
      throw new Error(`Task not found: ${taskId}`);
    }

    this.mockData.tasks[taskIndex] = {
      ...this.mockData.tasks[taskIndex],
      ...updates,
      updated_at: new Date().toISOString(),
    };

    return { ...this.mockData.tasks[taskIndex] };
  }

  async isBlockedByDependencies(taskId: string): Promise<boolean> {
    const task = await this.getTask(taskId);
    if (task.depends_on.length === 0) {
      return false;
    }

    // Check if any dependency is not done
    for (const depId of task.depends_on) {
      try {
        const depTask = await this.getTask(depId);
        if (depTask.status !== TaskStatus.Done) {
          return true;
        }
      } catch {
        // Dependency doesn't exist - treat as blocking
        return true;
      }
    }

    return false;
  }

  // Test helper: Set mock data directly
  setMockData(data: TasksData): void {
    this.mockData = data;
  }

  // Test helper: Get current mock data
  getMockData(): TasksData {
    return { ...this.mockData };
  }
}
