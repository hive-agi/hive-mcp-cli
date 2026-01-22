# hive-mcp-cli

Automated setup CLI for [hive-mcp](https://github.com/hive-agi/hive-mcp) - a Clojure-based MCP server that supercharges Claude Code with persistent memory, project management, and swarm coordination.

## Installation

```bash
go install github.com/hive-agi/hive-mcp-cli/cmd/hive@latest
```

## Usage

```bash
# Check system prerequisites
hive detect

# Run full setup (interactive)
hive setup

# Diagnose issues
hive doctor

# Attempt automatic fixes
hive doctor --fix
```

## What it Does

The `hive setup` command automates the complete hive-mcp installation:

1. **Clone** - Clones hive-mcp repository to `~/hive-mcp`
2. **Shell** - Configures environment variables in your shell rc file
3. **Prerequisites** - Installs platform-specific dependencies
4. **Dependencies** - Downloads Clojure dependencies via `clojure -P`
5. **Doom Sync** - Syncs Emacs packages (if using Doom Emacs)
6. **Chroma** - Sets up Docker volume and starts ChromaDB for vector storage
7. **Ollama** - Configures Ollama with embedding model
8. **Emacs Daemon** - Starts Emacs in daemon mode
9. **MCP Registration** - Registers hive-mcp server with Claude CLI

## What hive-mcp Provides

Once installed, hive-mcp adds 100+ MCP tools to Claude Code:

- **Persistent Memory** - Project-scoped notes, decisions, and conventions stored in ChromaDB
- **Kanban Board** - Task management with todo/doing/review/done states
- **Git Integration** - Magit-powered git operations
- **CIDER REPL** - Clojure evaluation via Emacs CIDER
- **Swarm Coordination** - Spawn and coordinate multiple Claude agents
- **Knowledge Graph** - Semantic relationships between memories
- **Code Analysis** - clj-kondo linting and scc metrics

## Requirements

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.21+ | To install this CLI |
| Emacs | 28.1+ | IDE integration and MCP server host |
| Java | 17+ | Clojure runtime |
| Clojure CLI | 1.11.0+ | Run hive-mcp server |
| Babashka | 1.3.0+ | Fast Clojure scripting |
| Docker | 20.0+ | Run ChromaDB |
| Git | 2.0+ | Clone repositories |
| Claude CLI | 0.1.0+ | MCP server registration |

### Optional

- **Ollama** - Local LLM for agent delegation
- **Doom Emacs** - Enhanced Emacs configuration (recommended)

## Commands

### `hive detect`

Scans your system and reports:
- Platform and package manager
- Shell configuration files
- Installed tools and versions
- Running services (Emacs daemon, Chroma, Ollama)
- Environment variables

### `hive setup`

Runs the full installation sequence. Idempotent - safe to run multiple times. Skips steps that are already complete.

### `hive doctor`

Health checks for your installation:
- Version verification
- Service health (Chroma, Ollama endpoints)
- Environment validation
- MCP registration status
- Integration tests

Use `--fix` to attempt automatic repairs.

## Environment Variables

After setup, these are configured in your shell:

```bash
HIVE_MCP_DIR=~/hive-mcp
BB_MCP_DIR=~/hive-mcp
OPENROUTER_API_KEY=<your-key>  # Optional, for cloud LLM delegation
```

## License

MIT
