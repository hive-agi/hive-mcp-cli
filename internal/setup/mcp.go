package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// MCPStep registers the hive-mcp server with Claude CLI
type MCPStep struct {
	HiveMCPDir string
}

func (s *MCPStep) Name() string {
	return "Register MCP server with Claude CLI"
}

func (s *MCPStep) hiveMCPDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

func (s *MCPStep) Check() (bool, error) {
	// Check if 'emacs' MCP server is already registered
	cmd := exec.Command("claude", "mcp", "list")
	output, err := cmd.Output()
	if err != nil {
		// Claude CLI might not be installed or configured
		return false, nil
	}

	return strings.Contains(string(output), "emacs"), nil
}

func (s *MCPStep) Run() error {
	// First verify Claude CLI is available
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found - please install from https://github.com/anthropics/claude-code")
	}

	hiveMCP := s.hiveMCPDir()

	// Register the MCP server
	// claude mcp add emacs -- bb --prn -cp <hive-mcp>/bb.edn -m bb.hive-mcp.server/-main
	cmd := exec.Command("claude", "mcp", "add", "emacs", "--",
		"bb", "--prn",
		"-cp", hiveMCP+"/bb.edn",
		"-m", "bb.hive-mcp.server/-main")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to register MCP server: %w", err)
	}

	return nil
}

func (s *MCPStep) Rollback() error {
	// Remove the MCP registration
	cmd := exec.Command("claude", "mcp", "remove", "emacs")
	return cmd.Run()
}
