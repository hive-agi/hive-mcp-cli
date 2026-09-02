package setup

import (
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

func (s *MCPStep) Check() (bool, error) {
	cmd := exec.Command("claude", "mcp", "list")
	output, err := cmd.Output()
	if err != nil {
		// Claude CLI might not be installed or configured
		return false, nil
	}
	return strings.Contains(string(output), "hive-mcp-foss"), nil
}

func (s *MCPStep) Run() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found - please install from https://github.com/anthropics/claude-code")
	}

	launcher := LauncherPath(s.hiveMCPDir())
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Errorf("launcher not found at %s (is the hive-mcp checkout complete?)", launcher)
	}

	// claude mcp add hive -- <hive-mcp>/bin/hive-mcp-foss
	cmd := exec.Command("claude", "mcp", "add", MCPServerName, "--", launcher)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to register MCP server: %w", err)
	}

	return nil
}

func (s *MCPStep) Rollback() error {
	cmd := exec.Command("claude", "mcp", "remove", MCPServerName)
	return cmd.Run()
}
