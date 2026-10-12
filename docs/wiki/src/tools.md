# The tools

The repository builds <!-- figure:cmd-count=11 -->11 commands from `cmd/`:

<!-- figure:cmd-names=archviz,autarch,autarch-mcp,autarch-registry,bigend,coldwine,gurgeh,mycroft,pollard,signals,testui -->
`archviz`, `autarch`, `autarch-mcp`, `autarch-registry`, `bigend`, `coldwine`, `gurgeh`, `mycroft`, `pollard`, `signals`, `testui`.

| Tool | Role |
|------|------|
| Bigend | Multi-project mission control, read-only (web and TUI) |
| Gurgeh | PRD generation and validation (TUI) |
| Coldwine | Task orchestration and sprint runs |
| Pollard | General-purpose research intelligence |
| Mycroft | Fleet orchestrator with escalating autonomy |

The unified `autarch` command hosts the TUI, `serve`, the Home asks (`needs-mk`, `decide`) and the maintenance commands.

Shared Go packages live under `internal/` (<!-- figure:internal-count=22 -->22 packages) and reusable ones under `pkg/` (<!-- figure:pkg-count=26 -->26 packages).
