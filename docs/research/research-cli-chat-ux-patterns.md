# CLI Chat UX Patterns Research

**Date:** 2026-02-07
**Purpose:** Comparative analysis of chat/input interfaces in Claude Code, Codex CLI, and OpenCode

## Executive Summary

This research examines three prominent CLI-based AI coding assistants to understand their chat interface design patterns, input composition mechanisms, and conversation display strategies. Key findings:

1. **Input patterns diverge**: Claude Code uses Shift+Enter multi-line with bash-style editing, Codex CLI emphasizes real-time steering during execution, OpenCode focuses on @ file mentions with fuzzy search
2. **Approval workflows are central**: Both Claude Code and Codex CLI provide granular approval modes (read-only, auto, full access) for security and control
3. **Markdown rendering is universal**: All three tools render markdown with syntax-highlighted code blocks, though implementation quality varies
4. **Tool visualization differs**: Codex CLI shows the most sophisticated tool-use rendering with dedicated cell types per tool category

## 1. Claude Code CLI (Anthropic)

### Input/Composer Area

**Multi-line Input:**
- Shift+Enter moves cursor to new line; Enter alone submits
- Requires `/terminal-setup` for automatic configuration in VS Code, iTerm2, WezTerm
- Works without config in iTerm2, WezTerm, Ghostty, Kitty
- Fallback: `\` + Enter for line breaks without configuration

**Text Editing Shortcuts (bash-style):**
- `Ctrl+A` / `Ctrl+E`: Jump to start/end of line
- `Option+F` / `Option+B`: Navigate word forward/back
- `Ctrl+W`: Delete previous word
- `Esc Esc` (double-tap): Clear input
- `Tab`: Auto-complete
- `Up/Down`: Navigate command history

**Note:** Option/Alt shortcuts require configuring Option as Meta in terminal settings on macOS

### Conversation Display

**Streaming & Rendering:**
- Real-time streaming responses from Claude
- Full markdown rendering with syntax-highlighted code blocks
- Tool-call visualization integrated into stream
- Syntax highlighting only available in native build (not VS Code extension)
- `Ctrl+T`: Toggle syntax highlighting via theme picker

**Visual Features:**
- Markdown formatting in terminal
- Code blocks with language-specific syntax highlighting
- Tool use shown inline during streaming

### Message Submission & Agent Status

**Loading Indicators:**
- Streaming responses show incremental text as Claude generates
- Tool calls visualized as they execute
- Background task support: commands run async with task ID returned
- Can respond to new prompts while commands execute in background

**Approval Workflow:**
- Multiple permission modes (read-only, auto, full access)
- Control via `/permissions` command
- Shift+Tab: Switch permission modes

### Slash Commands & Special Input

**Command System:**
- 30+ slash commands available
- Invoke skills with `/skill-name`
- Commands stored in `~/.claude/commands` (global) or `.claude/commands` (project)
- Examples: `/clear`, `/compact`, `/model`, `/plan`, `/help`
- `/keybindings`: Customize keyboard shortcuts via JSON

**Help & Discovery:**
- `/help`: Show available commands
- `?`: Display shortcuts for current environment

### Multi-turn Conversation

**Context Management:**
- Full conversation history maintained
- `/clear`: Clear context
- `/compact`: Compress conversation
- Session continuity across turns

**Model Switching:**
- `Cmd+P`: Model picker
- `/model`: Switch models mid-conversation

### Screenshot/Image Support

**Current State (as of 2026):**
- Web chat interface supports Ctrl+V / Cmd+V for screenshots
- CLI has no native image/screenshot paste support
- Workaround on macOS: `Shift+Cmd+Ctrl+4` to copy screenshot, then `Ctrl+V` (not Cmd+V)
- Feature request: Direct clipboard image support in terminal input area (#12644, #2204, #608)

### Additional Features

- `Ctrl+G`: External editor
- `Cmd+T`: Extended thinking mode
- `Ctrl+C`: Cancel current operation
- `Ctrl+D`: Exit Claude Code
- Background process support with async execution
- Session resume hints in recent 2026 updates
- Improved Japanese IME input support

## 2. Codex CLI (OpenAI)

### Input/Composer Area

**Multi-line & File References:**
- Type `@` in composer to open fuzzy file search over workspace root
- `Tab` or `Enter` drops highlighted path into message
- `Enter` during execution injects new instructions into current turn (real-time steering)
- Screenshot support: Drag screenshots directly into terminal composer

**Input Features:**
- Prompts, code snippets, screenshots accepted
- Visual inputs interpreted (error screenshots, UI sketches)
- Multimodal input handling in terminal

### Conversation Display

**Full-Screen TUI Architecture:**
- Built with Ratatui (Rust-based terminal UI framework)
- TypeScript CLI uses marked-terminal for markdown
- Full-screen terminal UI shows repository context, edits, command execution

**Markdown Rendering:**
- tui-markdown for Rust TUI rendering
- pulldown-cmark for markdown parsing
- Syntax highlighting via syntect and ansi-to-tui
- MarkdownStreamCollector processes incremental markdown input
- Known issues: bullet/numbered lists formatting, hyperlink styling
- marked-terminal uses yellow for inline code (more visible than tui-markdown's white)

**Cell Types for Message Display:**
- `UserHistoryCell`: User messages
- `AgentMessageCell`: Streamed assistant responses with markdown
- `ExecCell`: Command execution with live output streaming and status
- `McpToolCallCell`: MCP tool invocations with request/response display
- `WebSearchCell`: Web search results with clickable links
- `ReasoningSummaryCell`: Reasoning block summaries

**Streaming Architecture:**
- Uses OpenAI Node.js library v4+ with `/responses` endpoint
- Streaming responses supported
- `ChatWidget` maintains `active_cell` that mutates in place during streaming
- Better formatted tool calls and diffs in recent upgrades

### Message Submission & Agent Status

**Approval & Execution:**
- Watch Codex explain its plan before making changes
- Approve or reject steps inline
- Real-time progress updates during execution
- Better collaboration in GPT-5.3-Codex with frequent status updates

**Approval Modes:**
- **Read-only**: Explicit approvals required, Codex can browse but not modify
- **Auto**: Full workspace access, approvals required outside workspace
- **Full Access**: Read files anywhere, run commands with network access, no approvals
- `/permissions` command for approval set management
- `/approvals` maintained for compatibility

### Slash Commands & Special Input

**Command System:**
- `!` prefix to run shell commands
- Command output added to conversation as tool result
- Exec subcommand for non-interactive workflows (pipes plan/results to stdout)

**Workflow Commands:**
- Resume subcommand: Reopen earlier thread, preserve transcript/plan/approvals
- `--cd`: Override working directory
- `--add-dir`: Add extra roots before resuming
- Real-time steering: New instructions during execution

### Multi-turn Conversation

**Transcript Management:**
- Transcripts stored locally in `~/.codex/sessions/YYYY/MM/DD/` as JSONL
- Resume capability preserves context across sessions
- Prior context available when supplying new instructions
- History rendering in TUI with approval overlays

**Conversation Features:**
- TerminalMessageHistory component
- TerminalChatResponseItem for different message types
- Each cell implements `display_lines(width)` for viewport
- `transcript_lines(width)` for Ctrl+T overlay

### Tool Use Visualization

**Real-time Feedback:**
- AgentLoop uses openai.responses.create for streaming + tool use
- Live output streaming for command execution
- MCP tool calls shown with request/response
- Web search results rendered with links
- Dedicated cell types for each tool category

**Visual Indicators:**
- Tool calls formatted and highlighted
- Diffs better formatted in recent TUI upgrades
- Exec output streams in real-time with status indicators

## 3. OpenCode (Open Source)

### Input/Composer Area

**File References:**
- `@` triggers fuzzy file search in current working directory
- File content automatically added to conversation
- Example: "How is auth handled in @packages/functions/src/api/index.ts?"
- Flexible matching without exact paths
- **Known Issue:** Fuzzy file search has bugs on Windows (#1374)

**Shell Commands:**
- `!` prefix runs shell command
- Command output added to conversation as tool result

### Conversation Display

**Terminal Interface:**
- Terminal-native interface keeps developers in their environment
- Reads files, makes changes, runs commands
- Receives feedback from LSP servers
- Iterates within terminal context

**Markdown Support:**
- Full markdown support in messages
- Syntax highlighting for code blocks
- Language-specific highlighting
- **Known Issues (#4946, #3845):**
  - Syntax and highlighting errors reported
  - Code block syntax not working properly in some cases
  - Raw markdown syntax with backticks visible instead of formatted code
  - Markdown tables display as plain text instead of formatted tables

### Message Submission & Agent Status

**Execution Model:**
- Terminal-based workflow
- File modifications visible in terminal
- Command execution with output capture
- LSP integration for code intelligence

**Image Support:**
- In-memory images can't be pasted after TUI upgrade (#4077)
- Screenshot/image handling has regression issues

### Slash Commands & Special Input

**Command System:**
- `@` for file mentions (fuzzy search)
- `!` for shell command execution
- Feature request (#8223): Add @ mention support in web UI prompt window

### Multi-turn Conversation

**Context Management:**
- Terminal-native conversation flow
- File references maintain context
- Shell command results integrated

**Third-party Tools:**
- OpenCode Snapshots: Browse/search/recover files from sessions
- View diffs, explore snapshots, download project states
- TUI and web interface for session indexing

### Model & Provider Flexibility

**Open Source Advantage:**
- Fully open-source (Go-based CLI)
- Choose any AI provider: Claude, OpenAI, Google Gemini
- Support for locally hosted open-source models
- Not locked to single commercial provider

### Community Ecosystem

**Active Development:**
- Strong community adoption
- Modern alternative to proprietary tools
- Actively developed and maintained

## Comparative Analysis

### Input Mechanisms

| Tool | Multi-line | File Reference | Shell Exec | Image Support |
|------|-----------|----------------|------------|---------------|
| Claude Code | Shift+Enter (with config) | Not native | Via tools | Workaround only |
| Codex CLI | Yes | @ fuzzy search | ! prefix | Native (drag) |
| OpenCode | Yes | @ fuzzy search | ! prefix | Broken (#4077) |

### Conversation Display

| Tool | Markdown | Syntax Highlight | Streaming | Tool Viz |
|------|----------|------------------|-----------|----------|
| Claude Code | Full | Yes (native) | Real-time | Inline |
| Codex CLI | tui-markdown | syntect | Real-time | Cell types |
| OpenCode | Full | Yes (buggy) | Yes | Basic |

### Approval Workflows

| Tool | Modes Available | Granularity | Real-time Control |
|------|-----------------|-------------|-------------------|
| Claude Code | Read-only, Auto, Full | High (permissions cmd) | Shift+Tab switching |
| Codex CLI | Read-only, Auto, Full | High (inline approve/reject) | Real-time steering |
| OpenCode | Not documented | Unknown | Unknown |

### Unique Differentiators

**Claude Code:**
- Extensive plugin marketplace (36+ curated plugins as of Dec 2025)
- `/keybindings` customization via JSON
- Shift+Tab permission mode switching
- Extended thinking mode (Cmd+T)
- Cowork desktop integration (Jan 2026)
- MCP Apps UI integration in chat window

**Codex CLI:**
- Most sophisticated tool visualization (dedicated cell types)
- Real-time steering during execution (Enter to inject instructions)
- Native screenshot drag-and-drop
- Resume capability with full context preservation
- JSONL transcript storage for analysis
- Ratatui-based TUI (performance-focused)
- GPT-5.3-Codex with 25% faster performance

**OpenCode:**
- Only fully open-source option
- Provider flexibility (any LLM)
- Local model support
- Go-based (cross-platform)
- Community-driven development
- Third-party ecosystem (snapshots, monitoring)

## Architecture Patterns

### Message Loop Architectures

**Claude Code:**
- Streaming from Claude API
- Tool-call visualization during stream
- Background task support with async execution
- Can handle new prompts while commands run

**Codex CLI:**
- AgentLoop with openai.responses.create
- Streaming + tool use via /responses endpoint
- ChatWidget with active_cell mutation
- MarkdownStreamCollector for incremental rendering

**OpenCode:**
- Terminal-native event loop
- LSP server integration
- File change monitoring
- Command execution with output capture

### Rendering Technologies

**Claude Code:**
- Native terminal rendering
- Syntax highlighting via built-in renderer
- Markdown formatting in terminal
- Theme picker with Ctrl+T

**Codex CLI (Rust TUI):**
- Ratatui framework
- tui-markdown for markdown rendering
- pulldown-cmark for parsing
- syntect + ansi-to-tui for syntax highlighting

**Codex CLI (TypeScript):**
- marked-terminal for markdown
- Terminal cell abstraction
- Viewport rendering with display_lines(width)

**OpenCode:**
- Go-based terminal rendering
- Markdown + syntax highlighting (implementation unclear)
- LSP integration for code intelligence

## UX Best Practices Observed

### Input Design

1. **Multi-line support is essential** - All three tools recognize this need
2. **Visual feedback for linebreaks** - Shift+Enter preferred over escape sequences
3. **Fuzzy file search integration** - @ mentions reduce typing, improve accuracy
4. **Shell command shortcuts** - ! prefix for quick command execution
5. **Auto-configuration helpers** - /terminal-setup reduces setup friction

### Conversation Display

1. **Markdown is table stakes** - Users expect formatted text, code blocks
2. **Syntax highlighting critical** - Code must be readable in terminal
3. **Streaming builds trust** - Seeing incremental output reduces perceived latency
4. **Tool use transparency** - Users want to see what the AI is doing
5. **Cell-based rendering** - Different message types need different visual treatment

### Approval & Control

1. **Multiple modes required** - One-size-fits-all doesn't work
2. **Inline approval preferred** - Context-switching breaks flow
3. **Real-time steering valuable** - Users want to correct mid-execution
4. **Visual plan preview** - Show what will happen before it happens
5. **Easy mode switching** - Shift+Tab, /permissions make control accessible

### Special Input Handling

1. **Slash commands for discoverability** - /help, ? for contextual assistance
2. **Keyboard shortcut customization** - Power users want personalization
3. **Command history navigation** - Up/Down arrows expected behavior
4. **Auto-complete reduces errors** - Tab completion for paths, commands
5. **Image support increasingly expected** - Screenshots valuable for UI work

## Implementation Considerations for Autarch

Based on this research, recommendations for Autarch's TUI chat interface:

### High Priority

1. **Multi-line input with Shift+Enter** - Match Claude Code pattern
2. **Markdown rendering with syntax highlighting** - Table stakes feature
3. **@ file mentions with fuzzy search** - Proven pattern in Codex/OpenCode
4. **Streaming response display** - Real-time feedback builds trust
5. **Tool use visualization** - Show what Autarch is doing (file reads, writes, commands)

### Medium Priority

6. **Approval workflow modes** - Start simple (approve all, approve none), expand later
7. **Slash command system** - /help, /model, /clear as baseline
8. **Keyboard shortcut customization** - JSON config file approach works well
9. **Command history navigation** - Up/Down for previous prompts
10. **Background task support** - Allow long-running operations

### Lower Priority (Future)

11. **Screenshot/image paste** - Valuable but complex, defer initially
12. **Real-time steering** - Advanced feature, defer until core UX solid
13. **Resume/transcript management** - Add when multi-session support needed
14. **Cell-based rendering** - Start simpler, optimize later if needed
15. **External editor integration** - Ctrl+G pattern useful for long prompts

### Anti-patterns to Avoid

1. **Don't skip multi-line support** - Single-line input frustrates users
2. **Don't hide tool execution** - Transparency builds trust
3. **Don't force single approval mode** - Users need control granularity
4. **Don't neglect markdown rendering** - Plain text responses feel primitive
5. **Don't block on images** - Nice to have but not MVP blocker

## Technical Implementation Notes

### Markdown Rendering in Bubble Tea

**Libraries to Consider:**
- `glamour`: Bubble Tea ecosystem markdown renderer (likely best choice)
- `goldmark`: Go markdown parser (lower-level)
- `chroma`: Syntax highlighting for Go

**Rendering Strategy:**
- Pre-render markdown to styled text before lipgloss layout
- Use `Width()` constraints to prevent overflow
- Test height calculations carefully (lipgloss Height() is a floor, not ceiling)

### Input Composer in Bubble Tea

**Implementation Approaches:**
- `textarea` component from bubbles library (multi-line support built-in)
- Custom input handling for @ mentions (intercept @ key, show file picker)
- Fuzzy search: `github.com/sahilm/fuzzy` or `github.com/junegunn/fzf` integration
- History: Maintain prompt history slice, navigate with Up/Down

### Streaming Response Display

**Architecture:**
- WebSocket or SSE from Autarch backend to TUI
- Incremental text append to conversation view
- Scroll-to-bottom on new content
- Stop streaming on user interrupt (Ctrl+C)

### Tool Use Visualization

**Display Patterns:**
- Inline: Show tool calls within message stream (Claude Code style)
- Dedicated cells: Separate visual treatment per tool type (Codex style)
- Status indicators: Icons/colors for running/complete/failed
- Collapsible detail: Hide verbose output, expand on demand

## Research Gaps & Future Investigation

### Questions Remaining

1. **OpenCode markdown issues** - Are these recent regressions or design flaws?
2. **Codex CLI color scheme** - How does it handle light vs dark terminals?
3. **Claude Code plugin API** - How do plugins extend the chat interface?
4. **Performance benchmarks** - Which rendering approach is fastest?
5. **Accessibility** - Screen reader support in any of these tools?

### Follow-up Research

- Hands-on testing of all three tools with identical prompts
- Performance profiling of markdown rendering in terminal
- User studies of approval workflow preferences
- Analysis of plugin marketplace chat extensions
- Comparison with IDE-based tools (Cursor, Continue, etc.)

## References & Sources

### Claude Code

- [CLI reference - Claude Code Docs](https://code.claude.com/docs/en/cli-reference)
- [Claude Code by Anthropic](https://claude.com/product/claude-code)
- [GitHub - anthropics/claude-code](https://github.com/anthropics/claude-code)
- [Shipyard | Claude Code CLI Cheatsheet](https://shipyard.build/blog/claude-code-cheat-sheet/)
- [Anthropic's new Cowork tool](https://techcrunch.com/2026/01/12/anthropics-new-cowork-tool-offers-claude-code-without-the-code/)
- [Cooking with Claude Code: The Complete Guide](https://www.siddharthbharath.com/claude-code-the-complete-guide/)
- [A developer's Claude Code CLI reference](https://www.eesel.ai/blog/claude-code-cli-reference)
- [Interactive mode - Claude Code Docs](https://code.claude.com/docs/en/interactive-mode)
- [Claude Code Developer Cheatsheet](https://awesomeclaude.ai/code-cheatsheet)
- [Claude Code Terminal Setup - Shift+Enter Controls](https://claudelog.com/faqs/claude-code-terminal-setup/)
- [Claude Code CLI keyboard shortcuts](https://defkey.com/claude-code-cli-shortcuts)
- [GitHub - Njengah/claude-code-cheat-sheet](https://github.com/Njengah/claude-code-cheat-sheet)
- [Slash commands - Claude Code Docs](https://code.claude.com/docs/en/slash-commands)
- [How to Use Claude Code: A Guide](https://www.producttalk.org/how-to-use-claude-code-features/)
- [How I use Claude Code (+ my best tips)](https://www.builder.io/blog/claude-code)
- [Claude Code Commands: The Ultimate Reference](https://www.gradually.ai/en/claude-code-commands/)
- [Screenshot Support Issue #12644](https://github.com/anthropics/claude-code/issues/12644)
- [Markdown renderer support Issue #13600](https://github.com/anthropics/claude-code/issues/13600)

### Codex CLI

- [Codex | AI Coding Partner from OpenAI](https://openai.com/codex/)
- [Codex CLI](https://developers.openai.com/codex/cli/)
- [Quickstart - Codex](https://developers.openai.com/codex/quickstart/)
- [Codex CLI features](https://developers.openai.com/codex/cli/features/)
- [Codex changelog](https://developers.openai.com/codex/changelog/)
- [Introducing the Codex app](https://openai.com/index/introducing-the-codex-app/)
- [GitHub - openai/codex](https://github.com/openai/codex)
- [Command line options](https://developers.openai.com/codex/cli/reference/)
- [OpenAI Codex CLI, how does it work?](https://www.philschmid.de/openai-codex-cli)
- [Understanding OpenAI Codex CLI Commands](https://machinelearningmastery.com/understanding-openai-codex-cli-commands/)
- [Codex Monitor - Orchestrate Codex agents](https://www.codexmonitor.app/)
- [User Interfaces | openai/codex](https://deepwiki.com/openai/codex/4-developer-guide)
- [GitHub - lulu-sk/CodexFlow](https://github.com/lulu-sk/CodexFlow)
- [Codex CLI approval modes explained](https://vladimirsiedykh.com/blog/codex-cli-approval-modes-2025)
- [Codex CLI approval_policy Implementation](https://smartscope.blog/en/generative-ai/chatgpt/codex-cli-approval-policy-implementation/)
- [Improve Markdown rendering Issue #1246](https://github.com/openai/codex/issues/1246)
- [Zread - Terminal UI Implementation](https://zread.ai/openai/codex/23-terminal-ui-tui-implementation)
- [joshka/tui-markdown](https://deepwiki.com/joshka/tui-markdown)
- [tui_markdown - Rust](https://docs.rs/tui-markdown)

### OpenCode

- [TUI | OpenCode](https://opencode.ai/docs/tui/)
- [GitHub - opencode-ai/opencode](https://github.com/opencode-ai/opencode)
- [OpenCode | The open source AI coding agent](https://opencode.ai/)
- [OpenCode AI: The Complete Guide](https://brlikhon.engineer/blog/opencode-ai-the-complete-guide-to-the-open-source-terminal-coding-agent-revolutionizing-development-in-2026)
- [TUI - Interactive Terminal Interface](https://open-code.ai/en/docs/tui)
- [Intro | OpenCode](https://opencode.ai/docs/)
- [GitHub - anomalyco/opencode](https://github.com/anomalyco/opencode/)
- [Master OpenCode in 5 Minutes](https://help.apiyi.com/en/opencode-ai-coding-agent-beginner-guide-2026-en.html)
- [Building a TUI to index and search sessions](https://stanislas.blog/2026/01/tui-index-search-coding-agent-sessions/)
- [GitHub - phishy/opencode-snapshots](https://github.com/phishy/opencode-snapshots)
- [Fuzzy file search Windows Issue #1374](https://github.com/anomalyco/opencode/issues/1374)
- [Image Pasting Issue #4077](https://github.com/sst/opencode/issues/4077)
- [@ mention support Issue #8223](https://github.com/anomalyco/opencode/issues/8223)
- [OpenCode Product Specification](https://gist.github.com/roninjin10/b597dad618ded7779ae31479611b5312)
- [Syntax highlights error Issue #4946](https://github.com/anomalyco/opencode/issues/4946)
- [Markdown tables Issue #3845](https://github.com/anomalyco/opencode/issues/3845)

---

**Research completed:** 2026-02-07
**Tools analyzed:** Claude Code CLI, Codex CLI, OpenCode
**Total sources:** 80+ web resources reviewed
