/**
 * Type definitions matching tandemonium-core Rust types
 *
 * IMPORTANT: These must match the Rust serialization exactly:
 * - TaskStatus enums are lowercase (e.g., "todo", "inprogress")
 * - Struct fields are snake_case (e.g., "acceptance_criteria")
 */

export enum TaskStatus {
  Todo = "todo",
  InProgress = "inprogress",
  Review = "review",
  Done = "done",
  Blocked = "blocked",
}

export enum ProgressMode {
  Automatic = "automatic",
  Manual = "manual",
}

export interface AcceptanceCriteria {
  text: string;
  completed: boolean;
}

export interface Progress {
  mode: ProgressMode;
  value: number;
}

export interface ResolvedFile {
  path: string;
  resolved_at: string;
  git_sha: string;
}

export interface FileScope {
  files_glob: string[];
  files_resolved: ResolvedFile[];
  locked_paths: string[];
  shared_with: string[];
}

export interface Task {
  id: string;
  slug: string;
  title: string;
  description: string;
  status: TaskStatus;
  assigned_to: string | null;
  branch: string | null;
  worktree: string | null;
  base_sha: string | null;
  pr_url: string | null;
  scope: FileScope | null;
  acceptance_criteria: AcceptanceCriteria[];
  progress: Progress;
  tests: string[];
  depends_on: string[];
  created_at: string;
  updated_at: string;
}

export interface TasksData {
  version: number;
  updated_at: string;
  tasks: Task[];
}

/**
 * MCP Tool Error Codes
 */
export enum ErrorCode {
  BLOCKED = "BLOCKED",
  NOT_FOUND = "NOT_FOUND",
  ALREADY_CLAIMED = "ALREADY_CLAIMED",
  INVALID_STATE = "INVALID_STATE",
  INTERNAL = "INTERNAL",
}

export class McpError extends Error {
  constructor(
    public code: ErrorCode,
    message: string
  ) {
    super(message);
    this.name = "McpError";
  }
}
