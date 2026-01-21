import { describe, it, expect, beforeEach } from "vitest";
import { completeTask } from "../complete-task.js";
import { MockTaskStorage, createMockTask } from "../../__tests__/helpers.js";
import { TaskStatus, ErrorCode } from "../../types/index.js";

describe("complete-task", () => {
  let storage: MockTaskStorage;

  beforeEach(() => {
    storage = new MockTaskStorage();
  });

  it("should complete task in review status with all criteria done", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Review,
          acceptance_criteria: [
            { text: "Criterion 1", completed: true },
            { text: "Criterion 2", completed: true },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await completeTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      base_branch: "main",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.Done);
  });

  it("should fail when task is not in review status", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.InProgress, // Not in review
          acceptance_criteria: [
            { text: "Criterion 1", completed: true },
            { text: "Criterion 2", completed: true },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      completeTask(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        base_branch: "main",
      })
    ).rejects.toThrow("review");
  });

  it("should fail when acceptance criteria are incomplete", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Review,
          acceptance_criteria: [
            { text: "Criterion 1", completed: true },
            { text: "Criterion 2", completed: false }, // Incomplete
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      completeTask(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        base_branch: "main",
      })
    ).rejects.toThrow("incomplete acceptance criteria");
  });

  it("should succeed when task has no acceptance criteria", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Review,
          acceptance_criteria: [], // No criteria
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await completeTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      base_branch: "main",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.Done);
  });

  it("should fail with NOT_FOUND for non-existent task", async () => {
    await expect(
      completeTask(storage, {
        task_id: "tsk_99Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9",
        base_branch: "main",
      })
    ).rejects.toThrow("Task not found");
  });

  it("should provide next steps guidance in response", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Review,
          branch: "feature/tsk-01J6QX3-test",
          acceptance_criteria: [{ text: "Done", completed: true }],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await completeTask(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      base_branch: "main",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.next_steps).toBeDefined();
    expect(response.next_steps.length).toBeGreaterThan(0);
    expect(response.note).toContain("Tauri backend");
  });
});
