/**
 * Process-based AI assistant auto-detection
 */

import { exec } from "child_process";
import { promisify } from "util";
import {
  AIAssistant,
  TransportType,
  MCPEndpoint,
  DiscoveryMethod,
  ProcessInfo,
} from "../types/index.js";

const execAsync = promisify(exec);

/**
 * Known process names for AI assistants
 */
const PROCESS_PATTERNS: Record<AIAssistant, string[]> = {
  [AIAssistant.ClaudeCode]: ["claude", "claude-code"],
  [AIAssistant.Cursor]: ["cursor", "cursor-server"],
  [AIAssistant.Windsurf]: ["windsurf"],
  [AIAssistant.Unknown]: [],
};

/**
 * Detects AI assistant from process name
 */
function detectAssistantFromProcess(processName: string): AIAssistant {
  const nameLower = processName.toLowerCase();

  for (const [assistant, patterns] of Object.entries(PROCESS_PATTERNS)) {
    for (const pattern of patterns) {
      if (nameLower.includes(pattern)) {
        return assistant as AIAssistant;
      }
    }
  }

  return AIAssistant.Unknown;
}

/**
 * Scans running processes to find AI assistants (macOS-specific)
 */
async function scanProcessesMacOS(): Promise<ProcessInfo[]> {
  try {
    // Use ps to list all processes
    const { stdout } = await execAsync("ps -A -o pid,comm");
    const lines = stdout.split("\n").slice(1); // Skip header

    const processes: ProcessInfo[] = [];

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;

      const parts = trimmed.split(/\s+/);
      if (parts.length < 2) continue;

      const pid = parseInt(parts[0], 10);
      const name = parts.slice(1).join(" ");

      const assistant = detectAssistantFromProcess(name);
      if (assistant !== AIAssistant.Unknown) {
        processes.push({
          pid,
          name,
          assistant,
        });
      }
    }

    return processes;
  } catch (error) {
    // Error scanning processes
    return [];
  }
}

/**
 * Gets MCP socket/endpoint for a detected Claude Code process
 *
 * Claude Code typically creates a socket in a known location.
 * This is a best-effort attempt - actual implementation depends on
 * how Claude Code exposes its MCP server.
 */
function getClaudeCodeEndpoint(): MCPEndpoint | null {
  // For P0, we'll assume Claude Code exposes via stdio
  // In production, we'd need to:
  // 1. Check for socket files in ~/.claude/
  // 2. Check for HTTP endpoints in config files
  // 3. Query process environment variables

  // Placeholder: return null for now, will be filled in when we have
  // actual Claude Code MCP server details
  return null;
}

/**
 * Gets MCP endpoint for a detected Cursor process
 */
function getCursorEndpoint(): MCPEndpoint | null {
  // Similar to Claude Code, Cursor's MCP endpoint detection
  // would be implemented here
  return null;
}

/**
 * Gets MCP endpoint for a detected Windsurf process
 */
function getWindsurfEndpoint(): MCPEndpoint | null {
  // Windsurf MCP endpoint detection
  return null;
}

/**
 * Auto-detects running AI assistants and returns MCP endpoints
 */
export async function autoDetectEndpoints(): Promise<MCPEndpoint[]> {
  const endpoints: MCPEndpoint[] = [];

  // Scan for running processes
  const processes = await scanProcessesMacOS();

  // Try to get endpoints for each detected assistant
  const assistantsSeen = new Set<AIAssistant>();

  for (const proc of processes) {
    if (!proc.assistant || assistantsSeen.has(proc.assistant)) {
      continue;
    }

    assistantsSeen.add(proc.assistant);

    let endpoint: MCPEndpoint | null = null;

    switch (proc.assistant) {
      case AIAssistant.ClaudeCode:
        endpoint = getClaudeCodeEndpoint();
        break;
      case AIAssistant.Cursor:
        endpoint = getCursorEndpoint();
        break;
      case AIAssistant.Windsurf:
        endpoint = getWindsurfEndpoint();
        break;
    }

    if (endpoint) {
      endpoints.push({
        ...endpoint,
        discoveryMethod: DiscoveryMethod.AutoDetect,
      });
    }
  }

  return endpoints;
}

/**
 * Checks if a specific AI assistant is currently running
 */
export async function isAssistantRunning(
  assistant: AIAssistant
): Promise<boolean> {
  const processes = await scanProcessesMacOS();
  return processes.some((proc) => proc.assistant === assistant);
}

/**
 * Gets all running AI assistant processes
 */
export async function getRunningAssistants(): Promise<ProcessInfo[]> {
  return await scanProcessesMacOS();
}
