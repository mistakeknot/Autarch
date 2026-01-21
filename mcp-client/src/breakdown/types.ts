/**
 * TypeScript types for task breakdown
 * MUST match Rust Task structure exactly (see app/src-tauri/tandemonium-core/src/task.rs)
 */

/**
 * Task status enum - matches Rust with lowercase serialization
 */
export enum TaskStatus {
  Todo = "todo",
  InProgress = "inprogress",
  Review = "review",
  Done = "done",
  Blocked = "blocked",
}

/**
 * Progress mode for task completion tracking
 */
export enum ProgressMode {
  Automatic = "automatic", // Based on acceptance criteria
}

/**
 * Acceptance criteria for a task
 */
export interface AcceptanceCriteria {
  text: string;
  completed: boolean;
}

/**
 * Progress tracking for a task
 */
export interface Progress {
  mode: ProgressMode;
  value: number; // 0-100
}

/**
 * Simplified task structure for breakdown
 * (Excludes runtime fields like worktree, branch, pr_url)
 */
export interface Task {
  id: string; // ULID format: tsk_XXXXXXXXXXXXXXXXXXXX
  slug: string; // URL-safe identifier
  title: string;
  description: string;
  status: TaskStatus;
  acceptance_criteria: AcceptanceCriteria[];
  progress: Progress;
  tests: string[]; // Test file paths
  depends_on: string[]; // Task IDs
  created_at: string; // ISO 8601
  updated_at: string; // ISO 8601
}

/**
 * Complexity level for task breakdown
 */
export enum BreakdownComplexity {
  Simple = "simple", // Single task
  Complex = "complex", // Multiple tasks
}

/**
 * Request for task breakdown
 */
export interface BreakdownRequest {
  /** Natural language feature description */
  description: string;

  /** Complexity level (simple = 1 task, complex = multiple tasks) */
  complexity: BreakdownComplexity;

  /** Optional context about the project */
  projectContext?: string;

  /** Optional dependencies on other tasks */
  dependsOn?: string[];
}

/**
 * Response from task breakdown
 */
export interface BreakdownResponse {
  /** Successfully parsed tasks */
  tasks: Task[];

  /** Any errors encountered during parsing */
  errors?: string[];

  /** Raw AI response (for debugging) */
  rawResponse?: string;
}

/**
 * Raw task from AI (before parsing and validation)
 */
export interface RawTaskFromAI {
  title: string;
  description: string;
  acceptance_criteria?: string[] | AcceptanceCriteria[];
  tests?: string[];
  depends_on?: string[];
}
