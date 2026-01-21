/**
 * Parser and validator for AI-generated task breakdowns
 */

import { z } from "zod";
import { ulid } from "ulid";
import {
  Task,
  TaskStatus,
  AcceptanceCriteria,
  Progress,
  ProgressMode,
  RawTaskFromAI,
  BreakdownResponse,
} from "./types.js";

/**
 * Zod schema for raw task from AI
 */
const RawTaskSchema = z.object({
  title: z.string().min(1, "Task title cannot be empty"),
  description: z.string().min(1, "Task description cannot be empty"),
  acceptance_criteria: z
    .union([
      z.array(z.string()), // Simple string array
      z.array(
        z.object({
          text: z.string(),
          completed: z.boolean(),
        })
      ), // Already structured
    ])
    .optional()
    .default([]),
  tests: z.array(z.string()).optional().default([]),
  depends_on: z
    .union([
      z.array(z.string()), // Task IDs
      z.array(z.number()), // Indices (will be converted)
    ])
    .optional()
    .default([]),
});

/**
 * Zod schema for AI response
 */
const BreakdownResponseSchema = z.object({
  tasks: z.array(RawTaskSchema),
});

/**
 * Generate URL-safe slug from task title
 * @param title Task title
 * @returns Lowercase hyphenated slug
 */
function generateSlug(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "") // Remove non-alphanumeric except spaces and hyphens
    .trim()
    .replace(/\s+/g, "-") // Replace spaces with hyphens
    .replace(/-+/g, "-") // Replace multiple hyphens with single
    .substring(0, 50); // Limit length
}

/**
 * Normalize acceptance criteria to structured format
 * @param criteria Raw criteria from AI (string[] or AcceptanceCriteria[])
 * @returns Structured acceptance criteria
 */
function normalizeAcceptanceCriteria(
  criteria: string[] | AcceptanceCriteria[]
): AcceptanceCriteria[] {
  if (criteria.length === 0) {
    return [];
  }

  // Check if already structured
  const first = criteria[0];
  if (typeof first === "object" && "text" in first && "completed" in first) {
    return criteria as AcceptanceCriteria[];
  }

  // Convert string array to structured format
  return (criteria as string[]).map((text) => ({
    text,
    completed: false,
  }));
}

/**
 * Create default progress for a task
 * @param acceptanceCriteria Task acceptance criteria
 * @returns Progress object
 */
function createDefaultProgress(
  acceptanceCriteria: AcceptanceCriteria[]
): Progress {
  return {
    mode: ProgressMode.Automatic,
    value: acceptanceCriteria.length > 0 ? 0 : 100, // 100% if no criteria
  };
}

/**
 * Resolve task dependencies from indices to task IDs
 * @param dependsOn Raw depends_on from AI (indices or IDs)
 * @param tasks All parsed tasks
 * @param currentIndex Current task index
 * @returns Array of task IDs
 */
function resolveDependencies(
  dependsOn: string[] | number[],
  tasks: Task[],
  currentIndex: number
): string[] {
  if (dependsOn.length === 0) {
    return [];
  }

  // Check if already task IDs (strings starting with "tsk_")
  const first = dependsOn[0];
  if (typeof first === "string" && first.startsWith("tsk_")) {
    return dependsOn as string[];
  }

  // Convert indices to task IDs
  return (dependsOn as number[])
    .filter((index) => index >= 0 && index < currentIndex)
    .map((index) => tasks[index].id);
}

/**
 * Parse and validate AI breakdown response
 * @param rawResponse Raw JSON string from AI
 * @returns Parsed and validated breakdown response
 */
export function parseBreakdownResponse(
  rawResponse: string
): BreakdownResponse {
  const errors: string[] = [];
  const tasks: Task[] = [];

  try {
    // Parse JSON
    let parsed: unknown;
    try {
      parsed = JSON.parse(rawResponse);
    } catch (e) {
      return {
        tasks: [],
        errors: [
          `Failed to parse AI response as JSON: ${e instanceof Error ? e.message : String(e)}`,
        ],
        rawResponse,
      };
    }

    // Validate schema
    const validationResult = BreakdownResponseSchema.safeParse(parsed);
    if (!validationResult.success) {
      return {
        tasks: [],
        errors: [
          `AI response does not match expected schema: ${validationResult.error.message}`,
        ],
        rawResponse,
      };
    }

    const { tasks: rawTasks } = validationResult.data;

    // Convert raw tasks to structured tasks
    const now = new Date().toISOString();

    for (let i = 0; i < rawTasks.length; i++) {
      const rawTask = rawTasks[i];

      try {
        // Generate ULID and slug
        const id = `tsk_${ulid()}`;
        const slug = generateSlug(rawTask.title);

        // Normalize acceptance criteria
        const acceptanceCriteria = normalizeAcceptanceCriteria(
          rawTask.acceptance_criteria
        );

        // Create progress
        const progress = createDefaultProgress(acceptanceCriteria);

        // Resolve dependencies
        const dependsOn = resolveDependencies(
          rawTask.depends_on,
          tasks,
          i
        );

        // Create task
        const task: Task = {
          id,
          slug,
          title: rawTask.title,
          description: rawTask.description,
          status: TaskStatus.Todo,
          acceptance_criteria: acceptanceCriteria,
          progress,
          tests: rawTask.tests,
          depends_on: dependsOn,
          created_at: now,
          updated_at: now,
        };

        tasks.push(task);
      } catch (e) {
        errors.push(
          `Failed to parse task ${i + 1} ("${rawTask.title}"): ${e instanceof Error ? e.message : String(e)}`
        );
      }
    }

    return {
      tasks,
      errors: errors.length > 0 ? errors : undefined,
      rawResponse,
    };
  } catch (e) {
    return {
      tasks: [],
      errors: [
        `Unexpected error during parsing: ${e instanceof Error ? e.message : String(e)}`,
      ],
      rawResponse,
    };
  }
}

/**
 * Extract clean JSON from AI response that might contain markdown
 * @param response Raw AI response (might have markdown code blocks)
 * @returns Extracted JSON string
 */
export function extractJSON(response: string): string {
  // Remove markdown code blocks if present
  const jsonMatch = response.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
  if (jsonMatch) {
    return jsonMatch[1].trim();
  }

  // Return as-is if no code blocks
  return response.trim();
}
