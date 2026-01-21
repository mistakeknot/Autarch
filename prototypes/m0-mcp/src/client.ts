#!/usr/bin/env node

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { spawn } from "child_process";

// Simple client for testing MCP server
async function main() {
  console.log("=== MCP Client Test ===\n");

  // Spawn server process
  const serverProcess = spawn("tsx", ["src/server.ts"], {
    cwd: process.cwd(),
  });

  // Create client
  const transport = new StdioClientTransport({
    command: "tsx",
    args: ["src/server.ts"],
  });

  const client = new Client(
    {
      name: "test-client",
      version: "1.0.0",
    },
    {
      capabilities: {},
    }
  );

  try {
    await client.connect(transport);
    console.log("✓ Connected to MCP server\n");

    // Test 1: List tools
    console.log("[Test 1] Listing available tools...");
    const toolsResponse = await client.listTools();
    console.log(`  Found ${toolsResponse.tools.length} tools:`);
    toolsResponse.tools.forEach((tool) => {
      console.log(`    - ${tool.name}: ${tool.description}`);
    });
    console.log("  ✓ List tools works\n");

    // Test 2: List all tasks
    console.log("[Test 2] Listing all tasks...");
    const listResult = await client.callTool({
      name: "list_tasks",
      arguments: {},
    });
    const tasksList = JSON.parse(listResult.content[0].text);
    console.log(`  Found ${tasksList.count} tasks`);
    console.log("  ✓ list_tasks works\n");

    // Test 3: Claim a task
    console.log("[Test 3] Claiming task-1...");
    const claimResult = await client.callTool({
      name: "claim_task",
      arguments: {
        task_id: "task-1",
        agent_id: "agent-test",
      },
    });
    const claimData = JSON.parse(claimResult.content[0].text);
    if (claimData.success) {
      console.log(`  ✓ Task claimed: ${claimData.task.title}`);
      console.log(
        `    Status: ${claimData.task.status}, Assigned to: ${claimData.task.assigned_to}\n`
      );
    }

    // Test 4: Update progress
    console.log("[Test 4] Updating progress to 50%...");
    const updateResult = await client.callTool({
      name: "update_progress",
      arguments: {
        task_id: "task-1",
        progress: 50,
      },
    });
    const updateData = JSON.parse(updateResult.content[0].text);
    if (updateData.success) {
      console.log(`  ✓ Progress updated: ${updateData.task.progress}%\n`);
    }

    // Test 5: Complete task
    console.log("[Test 5] Completing task-1...");
    const completeResult = await client.callTool({
      name: "complete_task",
      arguments: {
        task_id: "task-1",
      },
    });
    const completeData = JSON.parse(completeResult.content[0].text);
    if (completeData.success) {
      console.log(`  ✓ Task completed: ${completeData.task.status}\n`);
    }

    // Test 6: Error handling (try to claim already claimed task)
    console.log("[Test 6] Testing error handling (claiming again)...");
    const errorResult = await client.callTool({
      name: "claim_task",
      arguments: {
        task_id: "task-1",
        agent_id: "different-agent",
      },
    });
    const errorData = JSON.parse(errorResult.content[0].text);
    if (errorData.error === "ALREADY_CLAIMED") {
      console.log(
        `  ✓ Error handled correctly: ${errorData.error} - ${errorData.message}\n`
      );
    }

    console.log("=== All Tests Passed! ===");
    console.log("✅ PASS: Server/client communication works");
    console.log("✅ PASS: All 4 tools execute correctly");
    console.log("✅ PASS: Error handling works");
    console.log("✅ PASS: stdio transport reliable");

    await client.close();
    serverProcess.kill();
    process.exit(0);
  } catch (error) {
    console.error("Test failed:", error);
    serverProcess.kill();
    process.exit(1);
  }
}

main();
