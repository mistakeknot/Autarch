# TUI Keybindings

See [../docs/tui/SHORTCUTS.md](../docs/tui/SHORTCUTS.md) for full reference.

## Universal Keys

| Key | Action |
|-----|--------|
| `Ctrl+C` | Clear input (once) / Quit (twice) |
| `↓` / `↑` | Move down / up |
| `Enter` | Select / confirm |
| `Esc` | Cancel / go back |
| `Tab` | Cycle pane focus |
| `Ctrl+B` | Toggle sidebar |
| `Ctrl+G` | Agent selector |
| `Ctrl+P` | Command palette |
| `Ctrl+L` | Toggle log pane |
| `Ctrl+Left` / `Ctrl+Right` | Cycle tabs |
| `Ctrl+R` | Refresh (context-dependent; `/refresh` works everywhere) |
| `Ctrl+A` | Accept (context-dependent) |
| `PgUp/PgDn` | Scroll doc panel (works from chat) |

## Slash Commands

Type `/` to open fuzzy command picker. Global commands and aliases:

| Command | Aliases | Action |
|---------|---------|--------|
| `/help` | `/h` | Help overlay |
| `/quit` | `/q`, `/exit` | Quit |
| `/settings` | `/config` | Chat settings |
| `/model` | `/m` | Model selector |
| `/palette` | `/p` | Command palette |
| `/refresh` | `/r` | Refresh current view |
| `/back` | `/b` | Go back / cancel |
| `/new` | `/clear` | New conversation |
| `/bigend` `/gurgeh` `/coldwine` `/pollard` | `/big` `/gur` `/cold` `/pol` | Switch tool tab`) |
| `/signals` | `/sig` | Toggle Signals overlay |
| `/logs` | `/log`, `/l` | Toggle log pane |

View-specific sets (kickoff, sprint, epic review, tasks such as `/group` = `/g`) are defined beside these in
`pkg/tui/command_picker.go`; check `GlobalCommands()` there before adding a new command or alias.
