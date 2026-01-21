#!/usr/bin/env node
/**
 * Tandemonium MCP Server Entry Point
 *
 * This server provides Model Context Protocol tools for AI agents
 * to interact with Tandemonium task management system.
 */
import { TandemoniumServer } from "./server.js";

async function main() {
  // Get project root from args or use current directory
  const projectRoot = process.argv[2] || process.cwd();

  const server = new TandemoniumServer(projectRoot);

  try {
    await server.start();
  } catch (error) {
    console.error("Failed to start MCP server:", error);
    process.exit(1);
  }
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
