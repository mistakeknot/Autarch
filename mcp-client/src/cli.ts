#!/usr/bin/env node
/**
 * CLI wrapper for MCP client - enables Tauri backend to call MCP functionality
 */

import { MCPClient } from "./client/mcp-client.js";
import { discoverAIAssistant } from "./discovery/discoverer.js";
import { breakdownFeature } from "./breakdown/breakdown.js";
import { analyzeComplexity } from "./breakdown/complexity.js";
import { BreakdownComplexity } from "./breakdown/types.js";

interface CLIArgs {
  command: "breakdown" | "complexity";
  description: string;
  complexity?: "simple" | "complex";
  projectContext?: string;
}

async function main() {
  try {
    // Parse command line arguments
    const args: CLIArgs = JSON.parse(process.argv[2] || "{}");

    if (!args.command || !args.description) {
      throw new Error("Missing required arguments: command and description");
    }

    // Discover AI assistant
    const discovery = await discoverAIAssistant();

    if (!discovery.success || !discovery.endpoint) {
      throw new Error("No AI assistant discovered. Please ensure Claude Code, Cursor, or Windsurf is running.");
    }

    // Connect to AI assistant
    const client = new MCPClient();
    const connection = await client.connect(discovery.endpoint);

    if (!connection.success) {
      throw new Error(`Failed to connect to AI assistant: ${connection.error}`);
    }

    // Execute command
    let result: any;

    if (args.command === "breakdown") {
      const complexityLevel = args.complexity === "simple"
        ? BreakdownComplexity.Simple
        : BreakdownComplexity.Complex;

      result = await breakdownFeature(client, {
        description: args.description,
        complexity: complexityLevel,
        projectContext: args.projectContext,
      });
    } else if (args.command === "complexity") {
      result = await analyzeComplexity(
        client,
        args.description,
        args.projectContext
      );
    } else {
      throw new Error(`Unknown command: ${args.command}`);
    }

    // Disconnect
    await client.disconnect();

    // Output result as JSON
    console.log(JSON.stringify({
      success: true,
      data: result,
    }));

  } catch (error) {
    // Output error as JSON
    console.error(JSON.stringify({
      success: false,
      error: error instanceof Error ? error.message : String(error),
    }));
    process.exit(1);
  }
}

main();
