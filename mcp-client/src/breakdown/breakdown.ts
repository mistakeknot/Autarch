/**
 * Main API for AI-powered task breakdown
 */

import { MCPClient } from "../client/mcp-client.js";
import { generateBreakdownPrompt } from "./prompts.js";
import { parseBreakdownResponse, extractJSON } from "./parser.js";
import {
  BreakdownRequest,
  BreakdownResponse,
  BreakdownComplexity,
} from "./types.js";

/**
 * Break down a feature description into structured tasks using AI
 * @param client Connected MCP client
 * @param request Breakdown request
 * @returns Parsed and validated tasks
 */
export async function breakdownFeature(
  client: MCPClient,
  request: BreakdownRequest
): Promise<BreakdownResponse> {
  // Ensure client is connected
  if (!client.isConnected()) {
    return {
      tasks: [],
      errors: ["MCP client is not connected. Please connect first."],
    };
  }

  try {
    // Generate prompt based on complexity
    const prompt = generateBreakdownPrompt(request);

    // Send prompt to AI via MCP
    // Note: This uses the connected AI assistant's prompt/completion capabilities
    // The exact method depends on the AI assistant's MCP implementation
    const response = await sendPromptToAI(client, prompt);

    // Extract JSON from response (handles markdown code blocks)
    const jsonResponse = extractJSON(response);

    // Parse and validate response
    const result = parseBreakdownResponse(jsonResponse);

    return result;
  } catch (error) {
    return {
      tasks: [],
      errors: [
        `Failed to break down feature: ${error instanceof Error ? error.message : String(error)}`,
      ],
    };
  }
}

/**
 * Send a prompt to the AI assistant via MCP
 * @param client MCP client
 * @param prompt Prompt text
 * @returns AI response text
 */
async function sendPromptToAI(
  client: MCPClient,
  prompt: string
): Promise<string> {
  // P0 Implementation: Use MCP's sampling capability if available
  // This is a placeholder - actual implementation depends on MCP server capabilities

  // Try to use a "complete" or "generate" tool if available
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
        max_tokens: 4000,
      });

      // Extract text from result
      if (result && result.content) {
        if (Array.isArray(result.content)) {
          // MCP returns array of content blocks
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
      const result = await client.getPrompt("breakdown", { prompt });
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
 * Break down a simple feature (single task)
 * @param client Connected MCP client
 * @param description Feature description
 * @param projectContext Optional project context
 * @returns Parsed task
 */
export async function breakdownSimple(
  client: MCPClient,
  description: string,
  projectContext?: string
): Promise<BreakdownResponse> {
  return breakdownFeature(client, {
    description,
    complexity: BreakdownComplexity.Simple,
    projectContext,
  });
}

/**
 * Break down a complex feature (multiple tasks)
 * @param client Connected MCP client
 * @param description Feature description
 * @param projectContext Optional project context
 * @param dependsOn Optional task IDs this depends on
 * @returns Parsed tasks
 */
export async function breakdownComplex(
  client: MCPClient,
  description: string,
  projectContext?: string,
  dependsOn?: string[]
): Promise<BreakdownResponse> {
  return breakdownFeature(client, {
    description,
    complexity: BreakdownComplexity.Complex,
    projectContext,
    dependsOn,
  });
}
