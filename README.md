# hive-mcp-cli

Automated setup CLI for [hive-mcp](https://github.com/hive-agi/hive-mcp), the Clojure MCP
host that gives Claude Code persistent memory, a knowledge graph, kanban and swarm
coordination. It installs the **batteries-included FOSS stack**: the hive-mcp core plus the
starter pack of open-source addons, wired into Claude Code through `bin/hive-mcp-foss`.

## Installation

```bash
# 1. Install the CLI and its MCP server
go install github.com/hive-agi/hive-mcp-cli/cmd/hive@latest
go install github.com/hive-agi/hive-mcp-cli/cmd/hive-setup-mcp@latest

# 2. Either drive it yourself...
hive detect
hive setup

# ...or let Claude drive it
claude mcp add hive-setup --scope user -- hive-setup-mcp
claude   # ask: "help me set up hive-mcp"
```

Verify:

```bash
hive doctor
claude mcp list | grep hive   # hive: ~/hive-mcp/bin/hive-mcp-foss
```

## Usage

```bash
hive detect            # check prerequisites and running services
hive setup             # full setup, idempotent
hive setup --emacs     # also sync Doom and start the Emacs daemon
hive doctor            # health checks
hive doctor --fix      # attempt automatic fixes
```

## What `hive setup` does

1. **Clone** hive-mcp to `~/hive-mcp` (`HIVE_MCP_DIR` overrides)
2. **Shell**: exports `HIVE_MCP_DIR` in your shell rc file
3. **Prerequisites**: Java 21, Clojure CLI, Docker, Git (platform package manager)
4. **Dependencies**: resolves the classpath the launcher boots, core plus `starter.deps.edn`
5. **Chroma**: starts the vector store with docker compose and waits for its heartbeat
6. **Ollama**: pulls the embedding model when Ollama is installed
7. **Register**: `claude mcp add hive -- ~/hive-mcp/bin/hive-mcp-foss`

With `--emacs`, Doom sync and the Emacs daemon run before registration. Without it the
Emacs vessel stays dormant until a daemon appears, and lings run in tmux.

Every step is idempotent: a step whose `Check` already passes is skipped, and a failing
step rolls back what it did.

## The starter pack

`starter.deps.edn` in the hive-mcp repo is the FOSS addon set merged over the core at boot:

| Role | Addon | Needs on the host | Status |
|---|---|---|---|
| Vessel, where lings run | hive-emacs | an Emacs daemon | shipped |
| Knowledge graph store | hive-datahike | nothing, embedded | shipped |
| Vessel, headless | hive-tmux | tmux, Python 3 with libtmux | pending |
| Harness bridge | hive-claude | Claude Code | pending |
| Code intelligence | lsp-mcp, clj-kondo-mcp, scc-mcp, basic-tools-mcp | clojure-lsp, clj-kondo, scc | pending |

Pending rows are commented in `starter.deps.edn` until their releases carry the addon
manifest the host discovers them by. A host tool that is missing degrades that one addon
with a logged reason; the boot still completes. `HIVE_STARTER=0 bin/hive-mcp-foss` boots
the bare core.

## Requirements

| Tool | Minimum | Purpose |
|------|---------|---------|
| Go | 1.21+ | install this CLI |
| Java | 21 | Clojure runtime (CI and the container image run 21) |
| Clojure CLI | 1.12+ | run the host from source |
| Docker | 20.0+ | Chroma and the clojure-lsp sidecar |
| Git | 2.0+ | clone the repo and git-sourced addons |
| Claude Code | latest | MCP client |

### Optional

- **tmux** (with Python 3 + libtmux) for the headless vessel
- **Emacs 28.1+** (Doom recommended) for the Emacs vessel and IDE integration
- **Ollama** for local embeddings (semantic search over memory)
- **clojure-lsp, clj-kondo, scc** for the code-intelligence addons

## Commands

### `hive detect`

Scans your system and reports platform and package manager, shell configuration files,
installed tools and versions, running services (Emacs daemon, Chroma, Ollama) and
environment variables.

### `hive setup`

Runs the installation sequence above. Idempotent: safe to run multiple times.

### `hive doctor`

Health checks for your installation: version verification, service health (Chroma, Ollama),
environment validation, MCP registration (the `hive` server pointing at `bin/hive-mcp-foss`),
launcher present and executable, nREPL reachable on 7910. Use `--fix` to attempt repairs.

## MCP Server

`hive-setup-mcp` exposes the CLI commands as MCP tools so an assistant can run the setup.

```bash
claude mcp add hive-setup -- hive-setup-mcp
```

| Tool | Description |
|------|-------------|
| `hive_detect` | Detect installed components, prerequisites, and environment |
| `hive_setup` | Install and configure hive-mcp; `emacs` parameter adds the Emacs steps |
| `hive_doctor` | Run health checks with optional `fix` parameter |

The server uses [Bonzai](https://github.com/rwxrob/bonzai) with MCP extensions to
generate tool schemas from command metadata. Only commands tagged with
`Mcp: &bonzai.McpMeta{...}` are exposed.

## Environment Variables

After setup, your shell exports:

```bash
HIVE_MCP_DIR=~/hive-mcp
OPENROUTER_API_KEY=<your-key>  # optional, for cloud LLM delegation
```

## License

MIT
