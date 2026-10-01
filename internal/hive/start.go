package hive

import (
	"os"

	"github.com/BuddhiLW/bonzai"
	"github.com/hive-agi/hive-mcp-cli/internal/setup"
)

// startCmd boots the hive-mcp backend. MCP clients launch bb-mcp, which is a
// client of this JVM's nREPL and never starts it, so this is how it runs.
var startCmd = &bonzai.Cmd{
	Name:  "start",
	Alias: "up",
	Short: "start the hive-mcp backend (bb-mcp talks to it)",

	Mcp: &bonzai.McpMeta{
		Desc: "Start the hive-mcp backend JVM (bin/hive-mcp-foss in $HIVE_MCP_DIR, nREPL on 7910) that bb-mcp, the registered MCP server, talks to. Detached by default, logging to ~/.local/state/hive-mcp/backend.log; does nothing when the nREPL already answers.",
	},

	Long: `Start boots the hive-mcp backend: bin/hive-mcp-foss in $HIVE_MCP_DIR
(default ~/hive-mcp). It starts the services, merges the starter pack and
local.deps.edn, and serves nREPL on 7910 (HIVE_NREPL_PORT overrides).

Your MCP clients launch bb-mcp, registered by 'bb-mcp setup'; bb-mcp is a
client of this nREPL and never starts it.

  hive start                 # detached; log in ~/.local/state/hive-mcp/backend.log
  hive start --foreground    # attached to this terminal (Ctrl-C stops it)

Nothing happens when the nREPL already answers.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		foreground := false
		for _, a := range args {
			if a == "--foreground" || a == "-f" {
				foreground = true
			}
		}
		dir := os.Getenv("HIVE_MCP_DIR")
		if dir == "" {
			dir = setup.DefaultHiveMCPDir()
		}
		return setup.StartBackend(dir, foreground)
	},
}
