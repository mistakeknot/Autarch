/**
 * Prompt templates for AI-powered task breakdown
 */

import { BreakdownRequest } from "./types.js";

/**
 * System prompt for task breakdown
 */
const SYSTEM_PROMPT = `You are a task breakdown expert for software development. Your job is to break down feature descriptions into structured, actionable tasks with clear acceptance criteria.

IMPORTANT OUTPUT FORMAT RULES:
- Return ONLY valid JSON, no markdown code blocks, no explanations
- Use lowercase for all enum values (status: "todo", not "Todo")
- Each task must have: title, description, acceptance_criteria (array of strings)
- Optional fields: tests (array), depends_on (array of task indices)
- Be specific and actionable in descriptions and acceptance criteria`;

/**
 * Simple breakdown prompt (single task)
 */
export function createSimpleBreakdownPrompt(
  request: BreakdownRequest
): string {
  const contextSection = request.projectContext
    ? `\n\nProject Context:\n${request.projectContext}`
    : "";

  return `${SYSTEM_PROMPT}

Break down this feature into a SINGLE well-defined task:

Feature: ${request.description}${contextSection}

Return a JSON object with this exact structure:
{
  "tasks": [{
    "title": "Clear, action-oriented title",
    "description": "Detailed description of what needs to be built",
    "acceptance_criteria": [
      "Specific, testable criterion 1",
      "Specific, testable criterion 2",
      "Specific, testable criterion 3"
    ],
    "tests": ["path/to/test/file.test.ts"],
    "depends_on": []
  }]
}

Example output for "Add user login":
{
  "tasks": [{
    "title": "Implement user login with JWT authentication",
    "description": "Create a login system that authenticates users via email/password and returns a JWT token for session management",
    "acceptance_criteria": [
      "User can log in with valid email and password",
      "Invalid credentials show appropriate error message",
      "Successful login returns JWT token",
      "Token is stored securely in httpOnly cookie"
    ],
    "tests": ["src/auth/login.test.ts"],
    "depends_on": []
  }]
}

Return ONLY the JSON, no other text:`;
}

/**
 * Complex breakdown prompt (multiple tasks)
 */
export function createComplexBreakdownPrompt(
  request: BreakdownRequest
): string {
  const contextSection = request.projectContext
    ? `\n\nProject Context:\n${request.projectContext}`
    : "";

  const dependsOnHint = request.dependsOn?.length
    ? `\n\nNote: These tasks depend on existing tasks: ${request.dependsOn.join(", ")}`
    : "";

  return `${SYSTEM_PROMPT}

Break down this complex feature into MULTIPLE well-defined, ordered tasks:

Feature: ${request.description}${contextSection}${dependsOnHint}

Return a JSON object with this exact structure:
{
  "tasks": [
    {
      "title": "First task title",
      "description": "What needs to be built first",
      "acceptance_criteria": ["criterion 1", "criterion 2"],
      "tests": ["path/to/test1.test.ts"],
      "depends_on": []
    },
    {
      "title": "Second task title",
      "description": "What depends on the first task",
      "acceptance_criteria": ["criterion 1", "criterion 2"],
      "tests": ["path/to/test2.test.ts"],
      "depends_on": [0]
    }
  ]
}

Example output for "Build user dashboard with analytics":
{
  "tasks": [
    {
      "title": "Create dashboard layout component",
      "description": "Build the main dashboard container with grid layout for widgets, responsive design, and loading states",
      "acceptance_criteria": [
        "Dashboard renders with responsive grid layout",
        "Loading skeleton appears while data loads",
        "Layout adapts to mobile/tablet/desktop",
        "Dashboard header shows user info"
      ],
      "tests": ["src/dashboard/Dashboard.test.tsx"],
      "depends_on": []
    },
    {
      "title": "Implement analytics data fetching",
      "description": "Create API integration to fetch user analytics data including pageviews, sessions, and conversion metrics",
      "acceptance_criteria": [
        "Fetches analytics data from /api/analytics endpoint",
        "Handles loading and error states",
        "Caches data for 5 minutes",
        "Refetches on user action"
      ],
      "tests": ["src/api/analytics.test.ts"],
      "depends_on": []
    },
    {
      "title": "Add analytics visualization widgets",
      "description": "Create chart components to visualize analytics data including line charts for trends and bar charts for comparisons",
      "acceptance_criteria": [
        "Line chart shows pageview trends over time",
        "Bar chart compares metrics across categories",
        "Charts update when data changes",
        "Charts have proper accessibility labels"
      ],
      "tests": ["src/dashboard/widgets/Charts.test.tsx"],
      "depends_on": [0, 1]
    }
  ]
}

Guidelines:
- Order tasks logically (dependencies first)
- Use depends_on array with task indices (0-based)
- Be specific in acceptance criteria
- Include realistic test file paths
- Aim for 2-5 tasks for most features

Return ONLY the JSON, no other text:`;
}

/**
 * Generate appropriate prompt based on complexity
 */
export function generateBreakdownPrompt(request: BreakdownRequest): string {
  if (request.complexity === "simple") {
    return createSimpleBreakdownPrompt(request);
  } else {
    return createComplexBreakdownPrompt(request);
  }
}
