/**
 * Main AI assistant discovery orchestrator
 * Implements hierarchical discovery: env → config → manual → auto-detect
 */

import {
  MCPEndpoint,
  DiscoveryOptions,
  DiscoveryResult,
  DiscoveryMethod,
} from "../types/index.js";
import { discoverFromEnvironment } from "./environment.js";
import { discoverFromConfigFile } from "./config-file.js";
import { autoDetectEndpoints } from "./process-detect.js";

/**
 * Default discovery options
 */
const DEFAULT_OPTIONS: DiscoveryOptions = {
  useEnvironment: true,
  useConfig: true,
  useManual: true,
  useAutoDetect: true,
  connectionTimeout: 5000,
};

/**
 * Discovers AI assistant MCP endpoint using hierarchical discovery
 *
 * Discovery order:
 * 1. Environment variables (MCP_ENDPOINT, MCP_TRANSPORT, MCP_ASSISTANT)
 * 2. Config file (~/.tandemonium/mcp-config.json)
 * 3. Manual endpoint specification
 * 4. Auto-detect running processes
 *
 * @param options Discovery options
 * @returns Discovery result with endpoint and attempt details
 */
export async function discoverAIAssistant(
  options: DiscoveryOptions = {}
): Promise<DiscoveryResult> {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const result: DiscoveryResult = {
    success: false,
    attempts: [],
  };

  // 1. Try environment variables
  if (opts.useEnvironment) {
    const envEndpoint = discoverFromEnvironment();
    if (envEndpoint) {
      result.attempts.push({
        method: DiscoveryMethod.Environment,
        success: true,
        endpoint: envEndpoint,
      });
      result.success = true;
      result.endpoint = envEndpoint;
      result.method = DiscoveryMethod.Environment;
      return result;
    } else {
      result.attempts.push({
        method: DiscoveryMethod.Environment,
        success: false,
        error: "No environment variables configured",
      });
    }
  }

  // 2. Try config file
  if (opts.useConfig) {
    try {
      const configEndpoint = await discoverFromConfigFile(opts.configPath);
      if (configEndpoint) {
        result.attempts.push({
          method: DiscoveryMethod.Config,
          success: true,
          endpoint: configEndpoint,
        });
        result.success = true;
        result.endpoint = configEndpoint;
        result.method = DiscoveryMethod.Config;
        return result;
      } else {
        result.attempts.push({
          method: DiscoveryMethod.Config,
          success: false,
          error: "No config file found or no endpoints configured",
        });
      }
    } catch (error) {
      result.attempts.push({
        method: DiscoveryMethod.Config,
        success: false,
        error: `Config file error: ${error}`,
      });
    }
  }

  // 3. Try manual endpoint
  if (opts.useManual && opts.manualEndpoint) {
    result.attempts.push({
      method: DiscoveryMethod.Manual,
      success: true,
      endpoint: opts.manualEndpoint,
    });
    result.success = true;
    result.endpoint = opts.manualEndpoint;
    result.method = DiscoveryMethod.Manual;
    return result;
  } else if (opts.useManual) {
    result.attempts.push({
      method: DiscoveryMethod.Manual,
      success: false,
      error: "No manual endpoint provided",
    });
  }

  // 4. Try auto-detect
  if (opts.useAutoDetect) {
    try {
      const detectedEndpoints = await autoDetectEndpoints();
      if (detectedEndpoints.length > 0) {
        const endpoint = detectedEndpoints[0]; // Use first detected
        result.attempts.push({
          method: DiscoveryMethod.AutoDetect,
          success: true,
          endpoint,
        });
        result.success = true;
        result.endpoint = endpoint;
        result.method = DiscoveryMethod.AutoDetect;
        return result;
      } else {
        result.attempts.push({
          method: DiscoveryMethod.AutoDetect,
          success: false,
          error: "No running AI assistants detected",
        });
      }
    } catch (error) {
      result.attempts.push({
        method: DiscoveryMethod.AutoDetect,
        success: false,
        error: `Auto-detect error: ${error}`,
      });
    }
  }

  // All methods failed
  result.error = "Failed to discover AI assistant endpoint using all methods";
  return result;
}

/**
 * Discovers all available AI assistant endpoints (does not stop at first success)
 *
 * @param options Discovery options
 * @returns Array of discovered endpoints from all methods
 */
export async function discoverAllEndpoints(
  options: DiscoveryOptions = {}
): Promise<MCPEndpoint[]> {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const endpoints: MCPEndpoint[] = [];

  // Environment
  if (opts.useEnvironment) {
    const envEndpoint = discoverFromEnvironment();
    if (envEndpoint) {
      endpoints.push(envEndpoint);
    }
  }

  // Config file
  if (opts.useConfig) {
    try {
      const configEndpoint = await discoverFromConfigFile(opts.configPath);
      if (configEndpoint) {
        endpoints.push(configEndpoint);
      }
    } catch {
      // Ignore config errors
    }
  }

  // Manual
  if (opts.useManual && opts.manualEndpoint) {
    endpoints.push(opts.manualEndpoint);
  }

  // Auto-detect
  if (opts.useAutoDetect) {
    try {
      const detectedEndpoints = await autoDetectEndpoints();
      endpoints.push(...detectedEndpoints);
    } catch {
      // Ignore auto-detect errors
    }
  }

  return endpoints;
}
