/**
 * Zod schemas for input validation
 */
import { z } from "zod";

export const TaskStatusSchema = z.enum([
  "todo",
  "inprogress",
  "review",
  "done",
  "blocked",
]);

export const ListTasksInputSchema = z.object({
  status: TaskStatusSchema.optional(),
  assigned_to: z.string().optional(),
  has_dependencies: z.boolean().optional(),
});

export const ClaimTaskInputSchema = z.object({
  task_id: z.string().regex(/^tsk_[0-9A-HJKMNP-TV-Z]{26}$/, "Invalid task ID format"),
  agent_id: z.string().min(1, "Agent ID is required"),
});

export const UpdateProgressInputSchema = z.object({
  task_id: z.string().regex(/^tsk_[0-9A-HJKMNP-TV-Z]{26}$/, "Invalid task ID format"),
  status: TaskStatusSchema.optional(),
  progress_value: z.number().min(0).max(100).optional(),
  completed_criteria: z.array(z.number()).optional(),
});

export const CompleteTaskInputSchema = z.object({
  task_id: z.string().regex(/^tsk_[0-9A-HJKMNP-TV-Z]{26}$/, "Invalid task ID format"),
  base_branch: z.string().default("main"),
});

export type ListTasksInput = z.infer<typeof ListTasksInputSchema>;
export type ClaimTaskInput = z.infer<typeof ClaimTaskInputSchema>;
export type UpdateProgressInput = z.infer<typeof UpdateProgressInputSchema>;
export type CompleteTaskInput = z.infer<typeof CompleteTaskInputSchema>;
