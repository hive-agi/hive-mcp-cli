package doctor

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// CheckMCP verifies MCP server registration with Claude CLI
func CheckMCP() []CheckResult {
	return []CheckResult{
		checkMCPRegistration(),
		checkMCPServerListed(),
	}
}

func checkMCPRegistration() CheckResult {
	result := CheckResult{
		Name:    "MCP Server Registration",
		FixHint: "Register with: claude mcp add emacs -- bb -x hive-mcp.core/main",
		CanFix:  true,
		Fix:     registerMCPServer,
	}

	// Check if claude mcp list shows emacs
	cmd := exec.Command("claude", "mcp", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = StatusError
		result.Message = "failed to query MCP servers"
		result.Details = err.Error()
		return result
	}

	output := string(out)
	if strings.Contains(output, "emacs") {
		result.Status = StatusOK
		result.Message = "emacs server registered"
	} else {
		result.Status = StatusError
		result.Message = "emacs server not registered"
		result.Details = "Run 'claude mcp add emacs -- bb -x hive-mcp.core/main' to register"
	}

	return result
}

func checkMCPServerListed() CheckResult {
	result := CheckResult{
		Name:    "MCP Server Config",
		FixHint: "Check ~/.config/claude-code/settings.json for MCP configuration",
	}

	// Try to parse claude mcp list output for detailed info
	cmd := exec.Command("claude", "mcp", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		// JSON output might not be available
		result.Status = StatusWarning
		result.Message = "could not verify server config"
		return result
	}

	// Try to parse JSON
	var servers interface{}
	if err := json.Unmarshal(out, &servers); err != nil {
		result.Status = StatusWarning
		result.Message = "could not parse server list"
		return result
	}

	// If we got here, JSON parsing worked
	result.Status = StatusOK
	result.Message = "server config accessible"
	return result
}

func registerMCPServer() error {
	hiveMCPDir := getEnv("HIVE_MCP_DIR", "")
	if hiveMCPDir == "" {
		hiveMCPDir = getEnv("HOME", "") + "/hive-mcp"
	}

	cmd := exec.Command("claude", "mcp", "add", "emacs",
		"--",
		"bb", "-x", "hive-mcp.core/main")
	cmd.Dir = hiveMCPDir
	return cmd.Run()
}

// CheckIntegration performs end-to-end integration tests
func CheckIntegration() []CheckResult {
	return []CheckResult{
		checkEmacsMCPConnection(),
		checkMCPToolExecution(),
	}
}

func checkEmacsMCPConnection() CheckResult {
	result := CheckResult{
		Name:    "Emacs MCP Connection",
		FixHint: "Ensure Emacs daemon is running and hive-mcp.el is loaded",
	}

	// Try to execute emacs_status via MCP
	// This requires the MCP server to be running and connected to Emacs
	cmd := exec.Command("claude", "mcp", "run", "emacs", "emacs_status")

	// Set a timeout
	done := make(chan error)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			result.Status = StatusWarning
			result.Message = "connection failed"
			result.Details = "MCP server may not be running"
			return result
		}
		result.Status = StatusOK
		result.Message = "connected"
	case <-time.After(5 * time.Second):
		result.Status = StatusWarning
		result.Message = "connection timeout"
		result.Details = "MCP server did not respond within 5 seconds"
	}

	return result
}

func checkMCPToolExecution() CheckResult {
	result := CheckResult{
		Name:    "MCP Tool Execution",
		FixHint: "Check MCP server logs for errors",
	}

	// Try a simple MCP tool call
	cmd := exec.Command("claude", "mcp", "run", "emacs", "mcp_capabilities")
	out, err := cmd.CombinedOutput()

	if err != nil {
		result.Status = StatusWarning
		result.Message = "tool execution failed"
		result.Details = string(out)
		return result
	}

	// Check if we got a valid response
	output := string(out)
	if strings.Contains(output, "capabilities") || strings.Contains(output, "hive-mcp") || len(output) > 10 {
		result.Status = StatusOK
		result.Message = "tools executing correctly"
	} else {
		result.Status = StatusWarning
		result.Message = "unexpected tool response"
		result.Details = "Response may be empty or malformed"
	}

	return result
}
