/**
 * AI-powered complexity analysis for feature requests
 */

import { z } from "zod";
import { MCPClient } from "../client/mcp-client.js";
import { createComplexityPrompt } from "./complexity-prompt.js";
import { extractJSON } from "./parser.js";
import {
  ComplexityAnalysis,
  ComplexityRequest,
  EffortLevel,
  RawComplexityFromAI,
} from "./complexity-types.js";

/**
 * Zod schema for validating raw complexity analysis from AI
 */
const RawComplexitySchema = z.object({
  score: z.number().int().min(1).max(10),
  recommendedTaskCount: z.number().int().positive().optional(),
  recommended_task_count: z.number().int().positive().optional(),
  reasoning: z.string().min(10),
  suggestedBreakdown: z.array(z.string()).optional(),
  suggested_breakdown: z.array(z.string()).optional(),
  potentialBlockers: z.array(z.string()).optional(),
  potential_blockers: z.array(z.string()).optional(),
  estimatedEffort: z.string().optional(),
  estimated_effort: z.string().optional(),
});

/**
 * Parse and validate AI complexity analysis response
 */
function parseComplexityResponse(rawResponse: string): ComplexityAnalysis {
  try {
    // Parse JSON
    const parsed: unknown = JSON.parse(rawResponse);

    // Validate schema
    const validationResult = RawComplexitySchema.safeParse(parsed);
    if (!validationResult.success) {
      throw new Error(
        `AI response does not match expected schema: ${validationResult.error.message}`
      );
    }

    const raw: RawComplexityFromAI = validationResult.data;

    // Normalize field names (handle both camelCase and snake_case)
    const recommendedTaskCount =
      raw.recommendedTaskCount ?? raw.recommended_task_count ?? raw.score;
    const suggestedBreakdown =
      raw.suggestedBreakdown ?? raw.suggested_breakdown ?? [];
    const potentialBlockers =
      raw.potentialBlockers ?? raw.potential_blockers ?? [];
    const effortString =
      raw.estimatedEffort ?? raw.estimated_effort ?? "medium";

    // Validate and normalize effort level
    let estimatedEffort: EffortLevel;
    const effortLower = effortString.toLowerCase();
    if (effortLower === "low") {
      estimatedEffort = EffortLevel.Low;
    } else if (effortLower === "high") {
      estimatedEffort = EffortLevel.High;
    } else {
      estimatedEffort = EffortLevel.Medium;
    }

    // Construct validated complexity analysis
    const analysis: ComplexityAnalysis = {
      score: raw.score,
      recommendedTaskCount,
      reasoning: raw.reasoning,
      suggestedBreakdown,
      potentialBlockers,
      estimatedEffort,
    };

    return analysis;
  } catch (error) {
    throw new Error(
      `Failed to parse complexity analysis: ${error instanceof Error ? error.message : String(error)}`
    );
  }
}

/**
 * Send complexity analysis prompt to AI via MCP
 */
async function sendComplexityPromptToAI(
  client: MCPClient,
  prompt: string
): Promise<string> {
  // Try to use completion tools
  try {
    const tools = await client.listTools();

    // Look for completion/generation tools
    const completionTool = tools.find(
      (tool: any) =>
        tool.name === "complete" ||
        tool.name === "generate" ||
        tool.name === "chat" ||
        tool.name === "ai_complete"
    );

    if (completionTool) {
      const result = await client.callTool(completionTool.name, {
        prompt,
        max_tokens: 2000,
      });

      // Extract text from result
      if (result && result.content) {
        if (Array.isArray(result.content)) {
          const textBlock = result.content.find(
            (block: any) => block.type === "text"
          );
          return textBlock?.text || JSON.stringify(result.content);
        }
        return String(result.content);
      }

      return JSON.stringify(result);
    }

    // Fallback: Try using prompts API
    const prompts = await client.listPrompts();
    if (prompts.length > 0) {
      const result = await client.getPrompt("complexity", { prompt });
      if (result && result.messages) {
        const textContent = result.messages
          .map((msg: any) => msg.content)
          .join("\n");
        return textContent;
      }
    }

    throw new Error(
      "No completion tools or prompts available in connected AI assistant"
    );
  } catch (error) {
    throw new Error(
      `Failed to communicate with AI assistant: ${error instanceof Error ? error.message : String(error)}`
    );
  }
}

/**
 * Analyze the complexity of a feature request
 *
 * @param client Connected MCP client
 * @param description Feature description to analyze
 * @param projectContext Optional project context
 * @returns Complexity analysis with score, recommendations, and blockers
 *
 * @example
 * ```typescript
 * const analysis = await analyzeComplexity(
 *   client,
 *   "Build real-time collaborative document editor",
 *   "React + TypeScript + WebSocket project"
 * );
 *
 * console.log(`Complexity: ${analysis.score}/10`);
 * console.log(`Recommended tasks: ${analysis.recommendedTaskCount}`);
 * console.log(`Effort: ${analysis.estimatedEffort}`);
 * ```
 */
export async function analyzeComplexity(
  client: MCPClient,
  description: string,
  projectContext?: string
): Promise<ComplexityAnalysis> {
  // Ensure client is connected
  if (!client.isConnected()) {
    throw new Error("MCP client is not connected. Please connect first.");
  }

  // Create request
  const request: ComplexityRequest = {
    description,
    projectContext,
  };

  // Generate prompt
  const prompt = createComplexityPrompt(request);

  // Send to AI
  const response = await sendComplexityPromptToAI(client, prompt);

  // Extract JSON from response (handles markdown code blocks)
  const jsonResponse = extractJSON(response);

  // Parse and validate
  const analysis = parseComplexityResponse(jsonResponse);

  return analysis;
}
