# Architecture

## Layer Model (L1/L2/L3)

Autarch follows Demarch's 3-layer architecture:

```
L3: Autarch (apps)     — TUI views, user interaction, dashboard
     ↓ calls
L2: Clavain (os)       — Policy layer, sprint lifecycle, gates, phase rules
     ↓ calls
L1: Intercore (core)   — Kernel, state machine, runs, coordination locks
```

**Intent routing**: Policy-governing mutations (sprint create, advance, gate enforcement) go through L2 (`pkg/clavain` → `clavain-cli`). Reads and metadata writes can call L1 directly (`pkg/intercore` → `ic`).

**Graceful degradation**: When `clavain-cli` is not on `PATH`, `clavain.New()` returns `ErrUnavailable`. All callsites check `if cclient != nil` and fall back to direct `ic` calls.

## Project Structure

```
Autarch/
├── cmd/                        # Entry points (see `ls cmd/`): autarch, autarch-mcp, bigend, coldwine, gurgeh, mycroft, pollard, signals, and helper tools
├── internal/                   # Tool-specific code (see `ls internal/`: bigend/, coldwine/, gurgeh/, mycroft/, pollard/, tui/, and more)
├── pkg/                        # Shared packages (see `ls pkg/` — key ones below)
├── docs/                       # Per-tool docs at docs/{bigend,gurgeh,coldwine,pollard}/AGENTS.md
└── dev                         # Build/run script
```

**Key shared packages (pkg/):**
- `autarch` — unified client (HTTP→Intermute), `clavain` — L2 CLI wrapper, `intercore` — L1 CLI wrapper
- `tui` — shared TUI (styles, Shell/Split layouts, ChatPanel, CommandPicker, Composer)
- `db` — SQLite helpers (WAL, `MaxOpenConns(1)`, 5s busy timeout), `contract` — cross-tool entity types
- Full listing: `ls pkg/`

## Key Architectural Facts

- **Single TUI implementation**: `UnifiedApp` is the only app shell. All views render inside it.
- **Onboarding lives in GurgehView**: `GurgehOnboardingView` + `gurgeh_helpers.go` in `views/`. `GurgehView` delegates to onboarding or spec browser.
- **4 dashboard tabs**: Bigend(0), Gurgeh(1), Coldwine(2), Pollard(3). Signals is an overlay (`/sig`), not a tab.
- **Overlay render order**: palette → chat settings → signals → help. Each intercepts keys when visible.
- **Log pane**: Always created. Ctrl+L toggles. Auto-shows during scan, auto-hides after 3s.
- **Slash commands**: the command and alias list lives in [tui-keybindings.md](tui-keybindings.md); the code is `GlobalCommands()` in `pkg/tui/command_picker.go`.
