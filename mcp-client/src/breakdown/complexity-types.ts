/**
 * Types for AI-powered complexity analysis
 */

/**
 * Effort level estimation
 */
export enum EffortLevel {
  Low = "low",
  Medium = "medium",
  High = "high",
}

/**
 * Complexity analysis result
 */
export interface ComplexityAnalysis {
  /** Complexity score from 1 (trivial) to 10 (architectural change) */
  score: number;

  /** Recommended number of subtasks to generate */
  recommendedTaskCount: number;

  /** Detailed explanation of complexity assessment */
  reasoning: string;

  /** High-level suggestions for breaking down the feature */
  suggestedBreakdown: string[];

  /** Potential blocking issues or dependencies */
  potentialBlockers: string[];

  /** Overall effort estimation */
  estimatedEffort: EffortLevel;
}

/**
 * Request for complexity analysis
 */
export interface ComplexityRequest {
  /** Feature description to analyze */
  description: string;

  /** Optional project context */
  projectContext?: string;
}

/**
 * Raw complexity analysis from AI (before validation)
 */
export interface RawComplexityFromAI {
  score: number;
  recommendedTaskCount?: number;
  recommended_task_count?: number; // snake_case variant
  reasoning: string;
  suggestedBreakdown?: string[];
  suggested_breakdown?: string[]; // snake_case variant
  potentialBlockers?: string[];
  potential_blockers?: string[]; // snake_case variant
  estimatedEffort?: string;
  estimated_effort?: string; // snake_case variant
}
