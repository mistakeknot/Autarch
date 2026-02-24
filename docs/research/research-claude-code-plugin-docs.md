# Research: Claude Code Local Plugin Registration (Official Docs)

**Date:** 2026-02-05
**Source URLs:**
- https://code.claude.com/docs/en/plugins (redirected from docs.anthropic.com)
- https://code.claude.com/docs/en/discover-plugins
- https://code.claude.com/docs/en/plugins-reference
- https://code.claude.com/docs/en/settings
- https://code.claude.com/docs/en/plugin-marketplaces

## Executive Summary

The official Claude Code docs describe **two approaches** for loading local plugins:

1. **`--plugin-dir` flag** — session-only, for development/testing
2. **Marketplace-based installation** — permanent, via `/plugin marketplace add` + `/plugin install`

The docs do **NOT** document a `localPlugins` settings key anywhere. However, it exists and works in practice (see "Undocumented: localPlugins" section below). The documented permanent installation path requires wrapping your plugin in a local marketplace.

---

## 1. How to Register a Local Plugin So It Loads on Startup

### Documented Approach A: `--plugin-dir` (Session-Only)

From the **Plugins** page:

> **Test your plugins locally**
>
> Use the `--plugin-dir` flag to test plugins during development. This loads your plugin directly without requiring installation.
>
> ```bash
> claude --plugin-dir ./my-plugin
> ```
>
> As you make changes to your plugin, restart Claude Code to pick up the updates. Test your plugin components:
> * Try your commands with `/command-name`
> * Check that agents appear in `/agents`
> * Verify hooks work as expected
>
> You can load multiple plugins at once by specifying the flag multiple times:
>
> ```bash
> claude --plugin-dir ./plugin-one --plugin-dir ./plugin-two
> ```

**Limitation:** This is session-only. The plugin does not persist across restarts without the flag.

### Documented Approach B: Local Marketplace (Permanent)

From the **Plugin Marketplaces** page, the walkthrough for creating a local marketplace:

> **Walkthrough: create a local marketplace**
>
> This example creates a marketplace with one plugin...
>
> **Step 1: Create the directory structure**
> ```bash
> mkdir -p my-marketplace/.claude-plugin
> mkdir -p my-marketplace/plugins/review-plugin/.claude-plugin
> mkdir -p my-marketplace/plugins/review-plugin/skills/review
> ```
>
> **Step 3: Create the plugin manifest**
> ```json
> // my-marketplace/plugins/review-plugin/.claude-plugin/plugin.json
> {
>   "name": "review-plugin",
>   "description": "Adds a /review skill for quick code reviews",
>   "version": "1.0.0"
> }
> ```
>
> **Step 4: Create the marketplace file**
> ```json
> // my-marketplace/.claude-plugin/marketplace.json
> {
>   "name": "my-plugins",
>   "owner": {
>     "name": "Your Name"
>   },
>   "plugins": [
>     {
>       "name": "review-plugin",
>       "source": "./plugins/review-plugin",
>       "description": "Adds a /review skill for quick code reviews"
>     }
>   ]
> }
> ```
>
> **Step 5: Add and install**
> ```shell
> /plugin marketplace add ./my-marketplace
> /plugin install review-plugin@my-plugins
> ```

This is the **officially documented way** to permanently install a local plugin.

### Documented Approach C: `extraKnownMarketplaces` with File/Directory Source

From the **Settings** page, the `extraKnownMarketplaces` setting supports local paths:

> **File paths:**
> ```json
> { "source": "file", "path": "/usr/local/share/claude/acme-marketplace.json" }
> ```
>
> **Directory paths:**
> ```json
> { "source": "directory", "path": "/usr/local/share/claude/acme-plugins" }
> ```
> Fields: `path` (required: absolute path to directory containing `.claude-plugin/marketplace.json`)

This means you can add a local marketplace to `settings.json` so it's automatically available:

```json
{
  "extraKnownMarketplaces": {
    "my-local-tools": {
      "source": {
        "source": "directory",
        "path": "/absolute/path/to/my-marketplace"
      }
    }
  }
}
```

---

## 2. The Exact settings.json Keys

### `enabledPlugins`

From the **Settings** page:

> **`enabledPlugins`**
>
> **Purpose**: Controls which plugins are enabled.
>
> **Format**: `"plugin-name@marketplace-name": true/false`
>
> **Scopes**:
> - **User settings** (`~/.claude/settings.json`): Personal plugin preferences
> - **Project settings** (`.claude/settings.json`): Project-specific plugins shared with team
> - **Local settings** (`.claude/settings.local.json`): Per-machine overrides (not committed)
>
> **Example**:
> ```json
> {
>   "enabledPlugins": {
>     "code-formatter@team-tools": true,
>     "deployment-tools@team-tools": true,
>     "experimental-features@personal": false
>   }
> }
> ```

### `extraKnownMarketplaces`

From the **Settings** page:

> **`extraKnownMarketplaces`**
>
> **Purpose**: Defines additional marketplaces that should be made available for the repository.
>
> **Behavior**:
> 1. Team members are prompted to install the marketplace when they trust the folder
> 2. Team members are then prompted to install plugins from that marketplace
> 3. Users can skip unwanted marketplaces or plugins (stored in user settings)
> 4. Installation respects trust boundaries and requires explicit consent
>
> **Example**:
> ```json
> {
>   "extraKnownMarketplaces": {
>     "acme-tools": {
>       "source": {
>         "source": "github",
>         "repo": "acme-corp/claude-plugins"
>       }
>     },
>     "security-plugins": {
>       "source": {
>         "source": "git",
>         "url": "https://git.example.com/security/plugins.git"
>       }
>     }
>   }
> }
> ```

### Plugin Installation Scopes (from Plugins Reference)

> **Plugin installation scopes**
>
> When you install a plugin, you choose a **scope** that determines where the plugin is available and who else can use it:
>
> | Scope     | Settings file                 | Use case                                                 |
> | :-------- | :---------------------------- | :------------------------------------------------------- |
> | `user`    | `~/.claude/settings.json`     | Personal plugins available across all projects (default) |
> | `project` | `.claude/settings.json`       | Team plugins shared via version control                  |
> | `local`   | `.claude/settings.local.json` | Project-specific plugins, gitignored                     |
> | `managed` | `managed-settings.json`       | Managed plugins (read-only, update only)                 |

---

## 3. Is `localPlugins` a Real Setting?

### Official Documentation: NOT Documented

The `localPlugins` key does **not appear anywhere** in the official Claude Code documentation across all pages checked:
- https://code.claude.com/docs/en/plugins
- https://code.claude.com/docs/en/discover-plugins
- https://code.claude.com/docs/en/plugins-reference
- https://code.claude.com/docs/en/settings
- https://code.claude.com/docs/en/plugin-marketplaces

### Empirical Reality: It Works

The `localPlugins` key is used in the current Autarch project and demonstrably functions:

**In `~/.claude/settings.json` (user-level):**
```json
{
  "localPlugins": [
    "/root/projects/tool-time",
    "/root/projects/Autarch/gurgeh-plugin"
  ]
}
```

**In `/root/projects/Autarch/.claude/settings.local.json` (project-level):**
```json
{
  "localPlugins": [
    "/root/projects/Autarch/gurgeh-plugin"
  ]
}
```

Both are arrays of absolute paths to plugin directories. When `localPlugins` is set, you also need `enabledPlugins` to have the plugin name (without marketplace suffix) set to `true`:

```json
{
  "enabledPlugins": {
    "gurgeh-plugin": true
  }
}
```

### Key Insight from Project Memory

From `MEMORY.md`:

> **Local plugins need BOTH `localPlugins` AND `enabledPlugins`** — `localPlugins` tells Claude Code where the plugin directory is, but `enabledPlugins` controls whether it actually loads. Without the `enabledPlugins` entry, the plugin is invisible (no skills, no commands, no agents). For local-only plugins (not on a marketplace), use just the plugin name as the key: `"gurgeh-plugin": true`

---

## 4. The `claude plugin install` Command

From the **Plugins Reference** CLI commands section:

> ### plugin install
>
> Install a plugin from available marketplaces.
>
> ```bash
> claude plugin install <plugin> [options]
> ```
>
> **Arguments:**
> * `<plugin>`: Plugin name or `plugin-name@marketplace-name` for a specific marketplace
>
> **Options:**
>
> | Option                | Description                                       | Default |
> | :-------------------- | :------------------------------------------------ | :------ |
> | `-s, --scope <scope>` | Installation scope: `user`, `project`, or `local` | `user`  |
> | `-h, --help`          | Display help for command                          |         |
>
> Scope determines which settings file the installed plugin is added to. For example, --scope project writes to `enabledPlugins` in .claude/settings.json, making the plugin available to everyone who clones the project repository.
>
> **Examples:**
>
> ```bash
> # Install to user scope (default)
> claude plugin install formatter@my-marketplace
>
> # Install to project scope (shared with team)
> claude plugin install formatter@my-marketplace --scope project
>
> # Install to local scope (gitignored)
> claude plugin install formatter@my-marketplace --scope local
> ```

**Note:** The `claude plugin install` command requires the plugin to exist in a marketplace. There is no documented way to `claude plugin install` from a bare local directory — you must first wrap it in a marketplace (even a local one).

For local directories, the flow is:

```shell
# Step 1: Add local directory as marketplace
/plugin marketplace add ./my-plugin-directory

# Step 2: Install from that marketplace
/plugin install plugin-name@marketplace-name
```

Or use `--plugin-dir` for development without marketplace setup.

---

## 5. Differences Between Marketplace Plugins and Local-Only Plugins

### From the Docs: Marketplace vs Standalone

From the **Plugins** page:

> | Approach                                                    | Skill names          | Best for                                                                                        |
> | :---------------------------------------------------------- | :------------------- | :---------------------------------------------------------------------------------------------- |
> | **Standalone** (`.claude/` directory)                       | `/hello`             | Personal workflows, project-specific customizations, quick experiments                          |
> | **Plugins** (directories with `.claude-plugin/plugin.json`) | `/plugin-name:hello` | Sharing with teammates, distributing to community, versioned releases, reusable across projects |
>
> **Use standalone configuration when**:
> * You're customizing Claude Code for a single project
> * The configuration is personal and doesn't need to be shared
> * You're experimenting with skills or hooks before packaging them
> * You want short skill names like `/hello` or `/review`
>
> **Use plugins when**:
> * You want to share functionality with your team or community
> * You need the same skills/agents across multiple projects
> * You want version control and easy updates for your extensions
> * You're distributing through a marketplace
> * You're okay with namespaced skills like `/my-plugin:hello` (namespacing prevents conflicts between plugins)

### Key Differences

| Aspect | Standalone (`.claude/`) | Plugin (marketplace) | Plugin (`localPlugins`) |
|--------|------------------------|---------------------|------------------------|
| **Skill namespace** | `/hello` | `/plugin-name:hello` | `/plugin-name:hello` |
| **Persistence** | Always loaded for that project | Installed permanently via marketplace | Loaded on startup (undocumented) |
| **Sharing** | Must manually copy files | Install from marketplace | N/A (local paths only) |
| **Updates** | Manual | Auto-update supported | Manual (re-read on restart) |
| **Caching** | No cache (in-place) | Copied to `~/.claude/plugins/cache` | Appears to load in-place |
| **Configuration** | None needed | `enabledPlugins` in settings | `localPlugins` + `enabledPlugins` |

### Plugin Caching Behavior (Marketplace Plugins)

From the **Plugins Reference**:

> **How plugin caching works**
>
> Plugins are specified in one of two ways:
> * Through `claude --plugin-dir`, for the duration of a session.
> * Through a marketplace, installed to the local plugin cache.
>
> When you install a plugin, Claude Code locates its marketplace and the plugin's `source` field within that marketplace.
>
> The source can be one of five types:
> * Relative path: copied recursively to the plugin cache
> * npm - copied to the plugin cache from npm
> * pip - copied to the plugin cache from pip
> * url - any https:// URL ending in .git
> * github - any owner/repo shorthand
>
> **Path traversal limitations**
>
> Plugins cannot reference files outside their copied directory structure. Paths that traverse outside the plugin root (such as `../shared-utils`) will not work after installation because those external files are not copied to the cache.

---

## 6. Complete Plugin Directory Structure

From the **Plugins Reference**:

> ### Standard plugin layout
>
> ```
> enterprise-plugin/
> ├── .claude-plugin/           # Metadata directory (optional)
> │   └── plugin.json             # plugin manifest
> ├── commands/                 # Default command location
> │   ├── status.md
> │   └── logs.md
> ├── agents/                   # Default agent location
> │   ├── security-reviewer.md
> │   ├── performance-tester.md
> │   └── compliance-checker.md
> ├── skills/                   # Agent Skills
> │   ├── code-reviewer/
> │   │   └── SKILL.md
> │   └── pdf-processor/
> │       ├── SKILL.md
> │       └── scripts/
> ├── hooks/                    # Hook configurations
> │   ├── hooks.json           # Main hook config
> │   └── security-hooks.json  # Additional hooks
> ├── .mcp.json                # MCP server definitions
> ├── .lsp.json                # LSP server configurations
> ├── scripts/                 # Hook and utility scripts
> │   ├── security-scan.sh
> │   ├── format-code.py
> │   └── deploy.js
> ├── LICENSE                  # License file
> └── CHANGELOG.md             # Version history
> ```
>
> **Warning:** The `.claude-plugin/` directory contains the `plugin.json` file. All other directories (commands/, agents/, skills/, hooks/) must be at the plugin root, not inside `.claude-plugin/`.

### Plugin Manifest (Minimal Required Fields)

> If you include a manifest, `name` is the only required field.
>
> | Field  | Type   | Description                               | Example              |
> | :----- | :----- | :---------------------------------------- | :------------------- |
> | `name` | string | Unique identifier (kebab-case, no spaces) | `"deployment-tools"` |

### Complete Manifest Schema

```json
{
  "name": "plugin-name",
  "version": "1.2.0",
  "description": "Brief plugin description",
  "author": {
    "name": "Author Name",
    "email": "author@example.com",
    "url": "https://github.com/author"
  },
  "homepage": "https://docs.example.com/plugin",
  "repository": "https://github.com/author/plugin",
  "license": "MIT",
  "keywords": ["keyword1", "keyword2"],
  "commands": ["./custom/commands/special.md"],
  "agents": "./custom/agents/",
  "skills": "./custom/skills/",
  "hooks": "./config/hooks.json",
  "mcpServers": "./mcp-config.json",
  "outputStyles": "./styles/",
  "lspServers": "./.lsp.json"
}
```

---

## 7. Practical Recommendations

### For the Autarch `gurgeh-plugin`

The current setup uses `localPlugins` + `enabledPlugins`, which works but is undocumented. Two alternatives:

**Option A: Keep using `localPlugins` (current, works, undocumented)**
```json
// ~/.claude/settings.json
{
  "localPlugins": ["/root/projects/Autarch/gurgeh-plugin"],
  "enabledPlugins": { "gurgeh-plugin": true }
}
```

**Option B: Create a local marketplace (documented, more setup)**
1. Create `gurgeh-marketplace/.claude-plugin/marketplace.json`
2. Point it at the plugin via relative source path
3. Run `/plugin marketplace add ./gurgeh-marketplace`
4. Run `/plugin install gurgeh-plugin@gurgeh-marketplace`

**Option C: Use `extraKnownMarketplaces` with directory source (documented, auto-prompt)**
```json
// .claude/settings.json or ~/.claude/settings.json
{
  "extraKnownMarketplaces": {
    "autarch-plugins": {
      "source": {
        "source": "directory",
        "path": "/root/projects/Autarch/gurgeh-marketplace"
      }
    }
  }
}
```

### Recommendation

**Stick with `localPlugins` for development** — it works, it's simpler, and it avoids the marketplace/caching overhead. The caching behavior of marketplace-installed plugins can actually be a hindrance during active development since changes require re-installation.

When ready to distribute, create a proper marketplace.

---

## 8. Environment Variable for Plugin Scripts

From the **Plugins Reference**:

> **`${CLAUDE_PLUGIN_ROOT}`**: Contains the absolute path to your plugin directory. Use this in hooks, MCP servers, and scripts to ensure correct paths regardless of installation location.

---

## 9. Debugging Commands

From the **Plugins Reference**:

> Use `claude --debug` (or `/debug` within the TUI) to see plugin loading details.
>
> This shows:
> * Which plugins are being loaded
> * Any errors in plugin manifests
> * Command, agent, and hook registration
> * MCP server initialization

> **Common issues**:
>
> | Issue                               | Cause                           | Solution                                                                          |
> | :---------------------------------- | :------------------------------ | :-------------------------------------------------------------------------------- |
> | Plugin not loading                  | Invalid `plugin.json`           | Validate JSON syntax with `claude plugin validate` or `/plugin validate`          |
> | Commands not appearing              | Wrong directory structure       | Ensure `commands/` at root, not in `.claude-plugin/`                              |
> | Hooks not firing                    | Script not executable           | Run `chmod +x script.sh`                                                          |
> | MCP server fails                    | Missing `${CLAUDE_PLUGIN_ROOT}` | Use variable for all plugin paths                                                 |
> | Path errors                         | Absolute paths used             | All paths must be relative and start with `./`                                    |

> **Plugin skills not appearing**: Clear the cache with `rm -rf ~/.claude/plugins/cache`, restart Claude Code, and reinstall the plugin.
