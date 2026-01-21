/**
 * Environment variable-based AI assistant discovery
 */

import {
  AIAssistant,
  TransportType,
  MCPEndpoint,
  DiscoveryMethod,
  EnvironmentConfig,
} from "../types/index.js";

/**
 * Discovers MCP endpoint from environment variables
 * Checks: MCP_ENDPOINT, MCP_TRANSPORT, MCP_ASSISTANT
 */
export function discoverFromEnvironment(): MCPEndpoint | null {
  const config: EnvironmentConfig = {
    MCP_ENDPOINT: process.env.MCP_ENDPOINT,
    MCP_TRANSPORT: process.env.MCP_TRANSPORT,
    MCP_ASSISTANT: process.env.MCP_ASSISTANT,
  };

  // Require at least MCP_ENDPOINT to be set
  if (!config.MCP_ENDPOINT) {
    return null;
  }

  // Parse transport type (default to stdio)
  let transport = TransportType.Stdio;
  if (config.MCP_TRANSPORT) {
    const transportLower = config.MCP_TRANSPORT.toLowerCase();
    if (transportLower === "http" || transportLower === "https") {
      transport = TransportType.HTTP;
    }
  }

  // Parse assistant type (default to unknown)
  let assistant = AIAssistant.Unknown;
  if (config.MCP_ASSISTANT) {
    const assistantLower = config.MCP_ASSISTANT.toLowerCase();
    switch (assistantLower) {
      case "claude-code":
      case "claudecode":
        assistant = AIAssistant.ClaudeCode;
        break;
      case "cursor":
        assistant = AIAssistant.Cursor;
        break;
      case "windsurf":
        assistant = AIAssistant.Windsurf;
        break;
    }
  }

  // For stdio, endpoint might be a command with args
  // Format: "command arg1 arg2" or just "command"
  const endpointParts = config.MCP_ENDPOINT.split(" ");
  const command = endpointParts[0];
  const args = endpointParts.length > 1 ? endpointParts.slice(1) : undefined;

  return {
    assistant,
    transport,
    endpoint: command,
    args,
    discoveryMethod: DiscoveryMethod.Environment,
  };
}

/**
 * Validates environment-based endpoint configuration
 */
export function validateEnvironmentConfig(
  endpoint: MCPEndpoint
): { valid: boolean; error?: string } {
  if (!endpoint.endpoint) {
    return { valid: false, error: "Endpoint is required" };
  }

  if (endpoint.transport === TransportType.HTTP) {
    // HTTP endpoints should be valid URLs
    try {
      new URL(endpoint.endpoint);
    } catch {
      return {
        valid: false,
        error: `Invalid HTTP endpoint URL: ${endpoint.endpoint}`,
      };
    }
  }

  return { valid: true };
}
