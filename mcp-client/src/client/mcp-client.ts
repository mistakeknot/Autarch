/**
 * MCP Client for connecting to AI assistants
 */

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import {
  MCPEndpoint,
  TransportType,
  ConnectionStatus,
  ClientState,
  ConnectionResult,
} from "../types/index.js";

export class MCPClient {
  private client: Client | null = null;
  private transport: StdioClientTransport | null = null;
  private state: ClientState = {
    status: ConnectionStatus.Disconnected,
  };

  /**
   * Gets the current connection state
   */
  public getState(): ClientState {
    return { ...this.state };
  }

  /**
   * Connects to an MCP endpoint
   * @param endpoint MCP endpoint configuration
   * @param timeout Connection timeout in milliseconds
   * @returns Connection result with validation info
   */
  public async connect(
    endpoint: MCPEndpoint,
    timeout = 5000
  ): Promise<ConnectionResult> {
    // Check if already connected
    if (this.state.status === ConnectionStatus.Connected) {
      return {
        success: false,
        error: "Already connected. Disconnect first.",
      };
    }

    this.state.status = ConnectionStatus.Connecting;
    this.state.endpoint = endpoint;

    try {
      // Only stdio transport is supported in P0
      if (endpoint.transport !== TransportType.Stdio) {
        throw new Error(
          `Unsupported transport type: ${endpoint.transport}. Only stdio is supported in P0.`
        );
      }

      // Create stdio transport
      this.transport = new StdioClientTransport({
        command: endpoint.endpoint,
        args: endpoint.args || [],
        env: endpoint.env,
      });

      // Create client
      this.client = new Client(
        {
          name: "tandemonium-mcp-client",
          version: "0.1.0",
        },
        {
          capabilities: {},
        }
      );

      // Connect with timeout
      const connectPromise = this.client.connect(this.transport);
      const timeoutPromise = new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error("Connection timeout")), timeout)
      );

      await Promise.race([connectPromise, timeoutPromise]);

      // Update state
      this.state.status = ConnectionStatus.Connected;
      this.state.connectedAt = new Date();
      this.state.lastActivity = new Date();
      this.state.error = undefined;

      // Get server info for validation
      const serverInfo = await this.client.getServerVersion();

      return {
        success: true,
        endpoint,
        validationInfo: serverInfo
          ? {
              serverName: serverInfo.name,
              serverVersion: serverInfo.version,
            }
          : undefined,
      };
    } catch (error) {
      // Connection failed
      this.state.status = ConnectionStatus.Error;
      this.state.error = error instanceof Error ? error.message : String(error);

      // Cleanup
      await this.cleanup();

      return {
        success: false,
        error: this.state.error,
      };
    }
  }

  /**
   * Disconnects from the current MCP endpoint
   */
  public async disconnect(): Promise<void> {
    await this.cleanup();
    this.state.status = ConnectionStatus.Disconnected;
    this.state.endpoint = undefined;
    this.state.connectedAt = undefined;
    this.state.lastActivity = undefined;
    this.state.error = undefined;
  }

  /**
   * Checks if client is currently connected
   */
  public isConnected(): boolean {
    return this.state.status === ConnectionStatus.Connected;
  }

  /**
   * Lists available tools from the connected AI assistant
   */
  public async listTools(): Promise<any[]> {
    if (!this.client || !this.isConnected()) {
      throw new Error("Not connected to any AI assistant");
    }

    this.state.lastActivity = new Date();
    const response = await this.client.listTools();
    return response.tools || [];
  }

  /**
   * Calls a tool on the connected AI assistant
   * @param toolName Name of the tool to call
   * @param args Tool arguments
   */
  public async callTool(toolName: string, args: any = {}): Promise<any> {
    if (!this.client || !this.isConnected()) {
      throw new Error("Not connected to any AI assistant");
    }

    this.state.lastActivity = new Date();
    const response = await this.client.callTool({
      name: toolName,
      arguments: args,
    });

    return response;
  }

  /**
   * Lists available prompts from the connected AI assistant
   */
  public async listPrompts(): Promise<any[]> {
    if (!this.client || !this.isConnected()) {
      throw new Error("Not connected to any AI assistant");
    }

    this.state.lastActivity = new Date();
    const response = await this.client.listPrompts();
    return response.prompts || [];
  }

  /**
   * Gets a prompt from the connected AI assistant
   * @param promptName Name of the prompt
   * @param args Prompt arguments
   */
  public async getPrompt(promptName: string, args: any = {}): Promise<any> {
    if (!this.client || !this.isConnected()) {
      throw new Error("Not connected to any AI assistant");
    }

    this.state.lastActivity = new Date();
    const response = await this.client.getPrompt({
      name: promptName,
      arguments: args,
    });

    return response;
  }

  /**
   * Cleanup internal resources
   */
  private async cleanup(): Promise<void> {
    if (this.client) {
      try {
        await this.client.close();
      } catch {
        // Ignore cleanup errors
      }
      this.client = null;
    }

    if (this.transport) {
      try {
        await this.transport.close();
      } catch {
        // Ignore cleanup errors
      }
      this.transport = null;
    }
  }
}
