import { describe, it, expect, beforeEach } from "vitest";
import { updateProgress } from "../update-progress.js";
import { MockTaskStorage, createMockTask } from "../../__tests__/helpers.js";
import { TaskStatus, ProgressMode } from "../../types/index.js";

describe("update-progress", () => {
  let storage: MockTaskStorage;

  beforeEach(() => {
    storage = new MockTaskStorage();
  });

  it("should update task status with valid transition", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      status: TaskStatus.InProgress,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.InProgress);
    expect(response.changes.status).toContain("todo");
    expect(response.changes.status).toContain("inprogress");
  });

  it("should fail with invalid status transition", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      updateProgress(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        status: TaskStatus.Done, // Invalid: can't go from todo to done directly
      })
    ).rejects.toThrow("cannot transition");
  });

  it("should update progress value in manual mode", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          progress: {
            mode: ProgressMode.Manual,
            value: 0,
          },
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      progress_value: 50,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.progress.value).toBe(50);
    expect(response.changes.progress_value).toContain("0");
    expect(response.changes.progress_value).toContain("50");
  });

  it("should fail to update progress value in automatic mode", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          progress: {
            mode: ProgressMode.Automatic,
            value: 0,
          },
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      updateProgress(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        progress_value: 50,
      })
    ).rejects.toThrow("automatic progress mode");
  });

  it("should mark acceptance criteria as completed", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          acceptance_criteria: [
            { text: "Criterion 1", completed: false },
            { text: "Criterion 2", completed: false },
            { text: "Criterion 3", completed: false },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      completed_criteria: [0, 2],
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.acceptance_criteria_completed).toBe("2/3");
    expect(response.changes.criteria_completed).toContain("2 criteria");
  });

  it("should automatically calculate progress when criteria are completed", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          progress: {
            mode: ProgressMode.Automatic,
            value: 0,
          },
          acceptance_criteria: [
            { text: "Criterion 1", completed: false },
            { text: "Criterion 2", completed: false },
            { text: "Criterion 3", completed: false },
            { text: "Criterion 4", completed: false },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      completed_criteria: [0, 1], // Complete 2 out of 4 = 50%
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.progress.value).toBe(50);
    expect(response.task.progress.mode).toBe(ProgressMode.Automatic);
  });

  it("should fail with invalid criteria index", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          acceptance_criteria: [
            { text: "Criterion 1", completed: false },
            { text: "Criterion 2", completed: false },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    await expect(
      updateProgress(storage, {
        task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
        completed_criteria: [0, 5], // Index 5 is out of range
      })
    ).rejects.toThrow("Invalid criteria index");
  });

  it("should update multiple fields at once", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
          acceptance_criteria: [
            { text: "Criterion 1", completed: false },
            { text: "Criterion 2", completed: false },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      status: TaskStatus.InProgress,
      completed_criteria: [0],
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.success).toBe(true);
    expect(response.task.status).toBe(TaskStatus.InProgress);
    expect(response.task.acceptance_criteria_completed).toBe("1/2");
    expect(response.changes.status).toBeDefined();
    expect(response.changes.criteria_completed).toBeDefined();
  });

  it("should fail with NOT_FOUND for non-existent task", async () => {
    await expect(
      updateProgress(storage, {
        task_id: "tsk_99Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9",
        status: TaskStatus.InProgress,
      })
    ).rejects.toThrow("Task not found");
  });

  it("should return completion stats in response", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          acceptance_criteria: [
            { text: "Criterion 1", completed: true },
            { text: "Criterion 2", completed: false },
            { text: "Criterion 3", completed: false },
          ],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await updateProgress(storage, {
      task_id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
      completed_criteria: [1],
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.task.acceptance_criteria_completed).toBe("2/3");
  });
});
