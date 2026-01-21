import { describe, it, expect, beforeEach } from "vitest";
import { listTasks } from "../list-tasks.js";
import { MockTaskStorage, createMockTask } from "../../__tests__/helpers.js";
import { TaskStatus } from "../../types/index.js";

describe("list-tasks", () => {
  let storage: MockTaskStorage;

  beforeEach(() => {
    storage = new MockTaskStorage();
  });

  it("should list all tasks when no filters are provided", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          title: "Task 1",
          status: TaskStatus.Todo,
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          title: "Task 2",
          status: TaskStatus.InProgress,
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          title: "Task 3",
          status: TaskStatus.Done,
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {});

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(3);
    expect(response.count).toBe(3);
  });

  it("should filter tasks by status", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.Todo,
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          status: TaskStatus.InProgress,
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          status: TaskStatus.InProgress,
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {
      status: TaskStatus.InProgress,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(2);
    expect(response.count).toBe(2);
    expect(response.tasks.every((t: any) => t.status === TaskStatus.InProgress)).toBe(true);
  });

  it("should filter tasks by assigned_to", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          assigned_to: "agent-1",
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          assigned_to: "agent-2",
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          assigned_to: "agent-1",
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {
      assigned_to: "agent-1",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(2);
    expect(response.count).toBe(2);
    expect(response.tasks.every((t: any) => t.assigned_to === "agent-1")).toBe(true);
  });

  it("should filter tasks with dependencies (has_dependencies: true)", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          depends_on: [],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          depends_on: ["tsk_01J6QX3N2Z8YFGHJ1234567ABC"],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          depends_on: ["tsk_01J6QX3N2Z8YFGHJ1234567ABC", "tsk_01J6QX3N2Z9YFGHJ1234567DEF"],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {
      has_dependencies: true,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(2);
    expect(response.count).toBe(2);
    expect(response.tasks.every((t: any) => t.depends_on.length > 0)).toBe(true);
  });

  it("should filter tasks without dependencies (has_dependencies: false)", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          depends_on: [],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          depends_on: ["tsk_01J6QX3N2Z8YFGHJ1234567ABC"],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          depends_on: [],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {
      has_dependencies: false,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(2);
    expect(response.count).toBe(2);
    expect(response.tasks.every((t: any) => t.depends_on.length === 0)).toBe(true);
  });

  it("should combine multiple filters", async () => {
    const mockTasks = {
      version: 1,
      updated_at: "2025-01-01T00:00:00Z",
      tasks: [
        createMockTask({
          id: "tsk_01J6QX3N2Z8YFGHJ1234567ABC",
          status: TaskStatus.InProgress,
          assigned_to: "agent-1",
          depends_on: [],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2Z9YFGHJ1234567DEF",
          status: TaskStatus.InProgress,
          assigned_to: "agent-1",
          depends_on: ["tsk_01J6QX3N2Z8YFGHJ1234567ABC"],
        }),
        createMockTask({
          id: "tsk_01J6QX3N2ZAYFGHJ1234567GHI",
          status: TaskStatus.Todo,
          assigned_to: "agent-1",
          depends_on: ["tsk_01J6QX3N2Z8YFGHJ1234567ABC"],
        }),
      ],
    };
    storage.setMockData(mockTasks);

    const result = await listTasks(storage, {
      status: TaskStatus.InProgress,
      assigned_to: "agent-1",
      has_dependencies: true,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(1);
    expect(response.count).toBe(1);
    expect(response.tasks[0].id).toBe("tsk_01J6QX3N2Z9YFGHJ1234567DEF");
  });

  it("should return empty list when no tasks match filters", async () => {
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

    const result = await listTasks(storage, {
      status: TaskStatus.Done,
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.tasks).toHaveLength(0);
    expect(response.count).toBe(0);
  });

  it("should include filters in response", async () => {
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

    const result = await listTasks(storage, {
      status: TaskStatus.InProgress,
      assigned_to: "agent-1",
    });

    const response = JSON.parse(result.content[0].text);
    expect(response.filters).toEqual({
      status: TaskStatus.InProgress,
      assigned_to: "agent-1",
    });
  });
});
