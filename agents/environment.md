# Environment Variables

Bigend reads no environment variables for its port or scan roots: use `--port` (default 8099) and `--scan-root`.

| Variable | Tool | Purpose |
|----------|------|---------|
| `INTERMUTE_URL` | All | Intermute server URL |
| `INTERMUTE_API_KEY` | All | Intermute authentication |
| `INTERMUTE_PROJECT` | All | Project scope |
| `GITHUB_TOKEN` | Pollard | GitHub API (optional) |
| `USDA_API_KEY` | Pollard | USDA hunter (required) |
| `COURTLISTENER_API_KEY` | Pollard | Legal hunter (required) |
