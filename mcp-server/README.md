# Tandemonium MCP Server

Model Context Protocol (MCP) server for Tandemonium task management system.

## Overview

This MCP server enables AI agents to interact with Tandemonium via structured tools over stdio transport. It provides 4 core tools for task lifecycle management:

1. **list_tasks** - Query and filter tasks
2. **claim_task** - Transition task to in-progress + create worktree
3. **update_progress** - Update task status and progress
4. **complete_task** - Create PR + cleanup worktree

## Architecture

```
mcp-server/
├── src/
│   ├── index.ts           # Entry point, server initialization
│   ├── server.ts          # MCP server class
│   ├── tools/             # Tool implementations
│   │   ├── list-tasks.ts
│   │   ├── claim-task.ts
│   │   ├── update-progress.ts
│   │   └── complete-task.ts
│   ├── schemas/           # Zod validation schemas
│   │   └── index.ts
│   ├── storage/           # YAML file I/O
│   │   └── tasks.ts
│   └── types/             # TypeScript type definitions
│       └── index.ts
├── package.json
├── tsconfig.json
└── README.md
```

## Development

```bash
# Install dependencies
pnpm install

# Build
pnpm build

# Development mode (watch)
pnpm dev

# Type checking
pnpm typecheck

# Start server (after build)
pnpm start
```

## Error Codes

The server uses structured error codes for consistent error handling:

- `BLOCKED` - Task is blocked by dependencies
- `NOT_FOUND` - Task does not exist
- `ALREADY_CLAIMED` - Task is already in progress
- `INVALID_STATE` - Invalid state transition attempted
- `INTERNAL` - Internal server error

## Transport

- **P0**: stdio transport only
- **P1**: HTTP transport support (future)

## Integration with Tauri App

The Tauri backend launches this MCP server as a subprocess using `tokio::process::Command`. Communication happens over stdio using the MCP protocol.

See `/app/src-tauri/src/mcp.rs` for integration details.
