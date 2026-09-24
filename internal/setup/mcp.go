package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MCPServerName is the name hive-mcp is registered under in Claude Code.
const MCPServerName = "hive"

// LauncherPath returns the FOSS launcher inside a hive-mcp checkout. The
// launcher starts the services, merges starter.deps.edn and boots the host,
// so registering it is the whole MCP wiring.
func LauncherPath(hiveMCPDir string) string {
	return filepath.Join(hiveMCPDir, "bin", "hive-mcp-foss")
}

// MCPStep registers the hive-mcp launcher with Claude Code.
type MCPStep struct {
	HiveMCPDir string
}

func (s *MCPStep) Name() string {
	return "Register hive-mcp with Claude Code"
}

func (s *MCPStep) hiveMCPDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

// Check reads the user-scope registration straight from ~/.claude.json.
// `claude mcp list` would answer too, but it health-checks every server, which
// boots the whole host JVM (over a minute) just to learn that it is registered.
func (s *MCPStep) Check() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, nil
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return false, nil
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return false, nil
	}
	return strings.HasSuffix(cfg.MCPServers[MCPServerName].Command, "hive-mcp-foss"), nil
}

func (s *MCPStep) Run() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found - please install from https://github.com/anthropics/claude-code")
	}

	launcher := LauncherPath(s.hiveMCPDir())
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Errorf("launcher not found at %s (is the hive-mcp checkout complete?)", launcher)
	}

	// User scope: without it `claude mcp add` registers for the directory setup
	// ran in (usually $HOME), and Claude Code opened in any real project does
	// not see the server at all.
	cmd := exec.Command("claude", "mcp", "add", "--scope", "user", MCPServerName, "--", launcher)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to register MCP server: %w", err)
	}

	return nil
}

func (s *MCPStep) Rollback() error {
	cmd := exec.Command("claude", "mcp", "remove", "--scope", "user", MCPServerName)
	return cmd.Run()
}
