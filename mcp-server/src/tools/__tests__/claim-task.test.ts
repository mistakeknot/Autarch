import { describe, it, expect, beforeEach } from "vitest";
import { claimTask } from "../claim-task.js";
import { MockTaskStorage, createMockTask } from "../../__tests__/helpers.js";
import { TaskStatus, ErrorCode } from "../../types/index.js";

describe("claim-task", () => {
  let storage: MockTaskStorage;

  beforeEach(() => {
    // Reset storage with fresh mock data before each test
    storage = new MockTaskStorage();
  });

  it("should claim an available task", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
          assigned_to: null,
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await claimTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      agent_id: "agent-1",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.InProgress);
    expect(response.task.assigned_to).toBe("agent-1");
  });

  it("should be idempotent - claiming already-claimed task by same agent returns success", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.InProgress,
          assigned_to: "agent-1",
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await claimTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      agent_id: "agent-1",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.message).toContain("already claimed");
    expect(response.task.assigned_to).toBe("agent-1");
  });

  it("should fail when task has uncompleted dependencies", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
          depends_on: ["tsk_01J6QX3N2Z7YFGHJ1234567GHJ"], // Dependency not done
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z7YFGHJ1234567GHJ",
          status: TaskStatus.InProgress, // Not done yet
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      claimTask(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        agent_id: "agent-1",
      })
    ).rejects.toThrow("blocked by incomplete dependencies");
  });

  it("should succeed when all dependencies are complete", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
          depends_on: ["tsk_01J6QX3N2Z7YFGHJ1234567GHJ"],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z7YFGHJ1234567GHJ",
          status: TaskStatus.Done, // Completed
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await claimTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      agent_id: "agent-1",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.InProgress);
  });

  it("should fail with NOT_FOUND error for non-existent task", async () => {
    await expect(
      claimTask(storage, {
        task_id: "tsk_99Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9",
        agent_id: "agent-1",
      })
    ).rejects.toThrow("Task not found");
  });

  it("should fail when task is already claimed by different agent", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.InProgress,
          assigned_to: "agent-1",
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      claimTask(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        agent_id: "agent-2", // Different agent
      })
    ).rejects.toThrow("already claimed by agent-1");
  });

  it("should reject invalid task_id format", async () => {
    await expect(
      claimTask(storage, {
        task_id: "invalid-id",
        agent_id: "agent-1",
      })
    ).rejects.toThrow("Invalid task ID format");
  });
});
