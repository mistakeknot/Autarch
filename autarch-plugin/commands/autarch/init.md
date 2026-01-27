---
name: autarch:init
description: Initialize Autarch in the current project
argument-hint: "[--tool gurgeh|coldwine|pollard|all]"
---

# Initialize Autarch

Initialize Autarch tools in the current project directory.

## What This Command Does

1. Creates `.gurgeh/` directory for PRD specifications
2. Creates `.coldwine/` (or `.tandemonium/`) directory for task orchestration
3. Creates `.pollard/` directory for research insights
4. Sets up Intermute messaging between tools

## Usage

```bash
# Initialize all tools
/autarch:init

# Initialize specific tool only
/autarch:init --tool gurgeh
/autarch:init --tool coldwine
/autarch:init --tool pollard
```

## Steps

1. Check if Autarch is already initialized
2. Run initialization for requested tools:
   - `gurgeh init` - PRD management
   - `coldwine init` or `tandemonium init` - Task orchestration
   - `pollard init` - Research intelligence
3. Verify initialization succeeded
4. Suggest next steps based on what was initialized

## After Initialization

- **For PRD work**: Run `/autarch:prd` to create a PRD
- **For research**: Run `/autarch:research` to start research
- **For tasks**: Run `/autarch:tasks` to view task board
