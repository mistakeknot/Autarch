/**
 * AI Assistant types and discovery interfaces
 */

/**
 * Supported AI assistants that can be discovered
 */
export enum AIAssistant {
  ClaudeCode = "claude-code",
  Cursor = "cursor",
  Windsurf = "windsurf",
  Unknown = "unknown",
}

/**
 * Transport protocols supported for MCP communication
 */
export enum TransportType {
  Stdio = "stdio",
  HTTP = "http",
}

/**
 * Discovery method used to find the AI assistant
 */
export enum DiscoveryMethod {
  Environment = "environment",
  Config = "config",
  Manual = "manual",
  AutoDetect = "auto-detect",
}

/**
 * Configuration for connecting to an MCP endpoint
 */
export interface MCPEndpoint {
  /** Type of AI assistant */
  assistant: AIAssistant;
  /** Transport protocol */
  transport: TransportType;
  /** Endpoint URL for HTTP transport, or command for stdio */
  endpoint: string;
  /** Optional additional arguments for stdio transport */
  args?: string[];
  /** Optional environment variables */
  env?: Record<string, string>;
  /** How this endpoint was discovered */
  discoveryMethod: DiscoveryMethod;
}

/**
 * Result of a connection attempt
 */
export interface ConnectionResult {
  /** Whether connection was successful */
  success: boolean;
  /** The connected endpoint if successful */
  endpoint?: MCPEndpoint;
  /** Error message if failed */
  error?: string;
  /** Validation details */
  validationInfo?: {
    serverName?: string;
    serverVersion?: string;
    capabilities?: string[];
  };
}

/**
 * Discovery configuration from environment variables
 */
export interface EnvironmentConfig {
  /** MCP endpoint URL from env var */
  MCP_ENDPOINT?: string;
  /** Transport type from env var */
  MCP_TRANSPORT?: string;
  /** Assistant type from env var */
  MCP_ASSISTANT?: string;
}

/**
 * Discovery configuration from config file
 */
export interface ConfigFileDiscovery {
  /** Saved MCP endpoints */
  endpoints?: MCPEndpoint[];
  /** Default endpoint to use */
  defaultEndpoint?: string;
  /** Whether to auto-detect if no saved endpoints work */
  enableAutoDetect?: boolean;
}

/**
 * Process information for auto-detection
 */
export interface ProcessInfo {
  /** Process ID */
  pid: number;
  /** Process name */
  name: string;
  /** Command line arguments */
  commandLine?: string;
  /** Detected AI assistant type */
  assistant?: AIAssistant;
}

/**
 * Options for AI assistant discovery
 */
export interface DiscoveryOptions {
  /** Allow environment variable discovery */
  useEnvironment?: boolean;
  /** Allow config file discovery */
  useConfig?: boolean;
  /** Config file path (defaults to ~/.tandemonium/mcp-config.json) */
  configPath?: string;
  /** Allow manual endpoint specification */
  useManual?: boolean;
  /** Manual endpoint if provided */
  manualEndpoint?: MCPEndpoint;
  /** Allow auto-detection of running processes */
  useAutoDetect?: boolean;
  /** Timeout for connection validation (ms) */
  connectionTimeout?: number;
}

/**
 * Result of the discovery process
 */
export interface DiscoveryResult {
  /** Whether discovery was successful */
  success: boolean;
  /** Discovered endpoint if successful */
  endpoint?: MCPEndpoint;
  /** Discovery method that succeeded */
  method?: DiscoveryMethod;
  /** List of all attempted methods with results */
  attempts: Array<{
    method: DiscoveryMethod;
    success: boolean;
    error?: string;
    endpoint?: MCPEndpoint;
  }>;
  /** Error message if all methods failed */
  error?: string;
}

/**
 * MCP Client connection status
 */
export enum ConnectionStatus {
  Disconnected = "disconnected",
  Connecting = "connecting",
  Connected = "connected",
  Error = "error",
}

/**
 * MCP Client state
 */
export interface ClientState {
  status: ConnectionStatus;
  endpoint?: MCPEndpoint;
  error?: string;
  connectedAt?: Date;
  lastActivity?: Date;
}
