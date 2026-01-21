/**
 * Config file-based AI assistant discovery
 */

import * as fs from "fs/promises";
import * as path from "path";
import * as os from "os";
import {
  MCPEndpoint,
  DiscoveryMethod,
  ConfigFileDiscovery,
} from "../types/index.js";

/**
 * Default config file path: ~/.tandemonium/mcp-config.json
 */
export function getDefaultConfigPath(): string {
  return path.join(os.homedir(), ".tandemonium", "mcp-config.json");
}

/**
 * Discovers MCP endpoint from config file
 * @param configPath Optional custom config path (defaults to ~/.tandemonium/mcp-config.json)
 */
export async function discoverFromConfigFile(
  configPath?: string
): Promise<MCPEndpoint | null> {
  const filePath = configPath || getDefaultConfigPath();

  try {
    // Check if file exists
    await fs.access(filePath);

    // Read and parse config file
    const fileContent = await fs.readFile(filePath, "utf-8");
    const config: ConfigFileDiscovery = JSON.parse(fileContent);

    // If no endpoints configured, return null
    if (!config.endpoints || config.endpoints.length === 0) {
      return null;
    }

    // Try to find default endpoint
    if (config.defaultEndpoint) {
      const defaultEp = config.endpoints.find(
        (ep) => ep.endpoint === config.defaultEndpoint
      );
      if (defaultEp) {
        return {
          ...defaultEp,
          discoveryMethod: DiscoveryMethod.Config,
        };
      }
    }

    // Return first endpoint if no default specified
    return {
      ...config.endpoints[0],
      discoveryMethod: DiscoveryMethod.Config,
    };
  } catch (error) {
    // File doesn't exist or is invalid - not an error, just no config
    return null;
  }
}

/**
 * Saves an MCP endpoint to the config file
 * @param endpoint Endpoint to save
 * @param configPath Optional custom config path
 * @param setAsDefault Whether to set this as the default endpoint
 */
export async function saveEndpointToConfig(
  endpoint: MCPEndpoint,
  configPath?: string,
  setAsDefault = true
): Promise<void> {
  const filePath = configPath || getDefaultConfigPath();
  const dirPath = path.dirname(filePath);

  // Ensure directory exists
  await fs.mkdir(dirPath, { recursive: true });

  let config: ConfigFileDiscovery = {
    endpoints: [],
    enableAutoDetect: true,
  };

  // Read existing config if it exists
  try {
    const fileContent = await fs.readFile(filePath, "utf-8");
    config = JSON.parse(fileContent);
  } catch {
    // File doesn't exist yet, use default config
  }

  // Ensure endpoints array exists
  if (!config.endpoints) {
    config.endpoints = [];
  }

  // Check if endpoint already exists (by endpoint URL/command)
  const existingIndex = config.endpoints.findIndex(
    (ep) => ep.endpoint === endpoint.endpoint
  );

  if (existingIndex >= 0) {
    // Update existing endpoint
    config.endpoints[existingIndex] = endpoint;
  } else {
    // Add new endpoint
    config.endpoints.push(endpoint);
  }

  // Set as default if requested
  if (setAsDefault) {
    config.defaultEndpoint = endpoint.endpoint;
  }

  // Write config file
  await fs.writeFile(filePath, JSON.stringify(config, null, 2), "utf-8");
}

/**
 * Loads all saved endpoints from config file
 * @param configPath Optional custom config path
 */
export async function loadAllEndpoints(
  configPath?: string
): Promise<MCPEndpoint[]> {
  const filePath = configPath || getDefaultConfigPath();

  try {
    const fileContent = await fs.readFile(filePath, "utf-8");
    const config: ConfigFileDiscovery = JSON.parse(fileContent);
    return config.endpoints || [];
  } catch {
    return [];
  }
}
