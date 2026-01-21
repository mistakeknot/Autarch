# Agent CLI

Agent registry commands mirror MCP Agent Mail registry/health behaviors.

## Commands

Ensure project registry exists:

```bash
tandemonium agent ensure
```

Register or update an agent:

```bash
tandemonium agent register \
  --name BlueLake \
  --program codex-cli \
  --model gpt-5 \
  --task "Auth refactor"
```

Lookup agent profile:

```bash
tandemonium agent whois --name BlueLake
```

Health check:

```bash
tandemonium agent health
```

## Output formats

All commands accept `--json` for machine-readable output.
