package setup

import "path/filepath"

// MCPServerName is the name hive is registered under in every MCP client.
// Registration itself is bb-mcp's job (see BBMCPStep); this CLI never writes
// a client config.
const MCPServerName = "hive"

// LauncherPath returns the FOSS launcher inside a hive-mcp checkout: the
// hive-mcp JVM (services, starter pack, nREPL on 7910) that bb-mcp talks to.
// `hive start` runs it.
func LauncherPath(hiveMCPDir string) string {
	return filepath.Join(hiveMCPDir, "bin", "hive-mcp-foss")
}
