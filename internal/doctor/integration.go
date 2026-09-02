package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	mcpServerName = "hive"
	nreplAddr     = "127.0.0.1:7910"
)

func hiveMCPDir() string {
	dir := getEnv("HIVE_MCP_DIR", "")
	if dir == "" {
		dir = filepath.Join(getEnv("HOME", ""), "hive-mcp")
	}
	return dir
}

func launcherPath() string {
	return filepath.Join(hiveMCPDir(), "bin", "hive-mcp-foss")
}

// CheckMCP verifies MCP server registration with Claude Code
func CheckMCP() []CheckResult {
	return []CheckResult{
		checkMCPRegistration(),
		checkMCPServerListed(),
	}
}

func checkMCPRegistration() CheckResult {
	result := CheckResult{
		Name:    "MCP Server Registration",
		FixHint: fmt.Sprintf("Register with: claude mcp add %s -- %s", mcpServerName, launcherPath()),
		CanFix:  true,
		Fix:     registerMCPServer,
	}

	cmd := exec.Command("claude", "mcp", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = StatusError
		result.Message = "failed to query MCP servers"
		result.Details = err.Error()
		return result
	}

	output := string(out)
	if strings.Contains(output, "hive-mcp-foss") {
		result.Status = StatusOK
		result.Message = mcpServerName + " server registered (bin/hive-mcp-foss)"
	} else {
		result.Status = StatusError
		result.Message = mcpServerName + " server not registered"
		result.Details = fmt.Sprintf("Run 'claude mcp add %s -- %s' to register", mcpServerName, launcherPath())
	}

	return result
}

func checkMCPServerListed() CheckResult {
	result := CheckResult{
		Name:    "MCP Server Config",
		FixHint: "Check ~/.claude.json for the MCP server entry",
	}

	cmd := exec.Command("claude", "mcp", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "could not verify server config"
		return result
	}

	var servers interface{}
	if err := json.Unmarshal(out, &servers); err != nil {
		result.Status = StatusWarning
		result.Message = "could not parse server list"
		return result
	}

	result.Status = StatusOK
	result.Message = "server config accessible"
	return result
}

func registerMCPServer() error {
	launcher := launcherPath()
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Errorf("launcher not found at %s", launcher)
	}
	cmd := exec.Command("claude", "mcp", "add", mcpServerName, "--", launcher)
	return cmd.Run()
}

// CheckIntegration performs end-to-end checks against a running host
func CheckIntegration() []CheckResult {
	return []CheckResult{
		checkLauncher(),
		checkNreplReachable(),
	}
}

func checkLauncher() CheckResult {
	result := CheckResult{
		Name:    "FOSS launcher",
		FixHint: "Re-run 'hive setup' or 'git -C $HIVE_MCP_DIR pull' to restore bin/hive-mcp-foss",
	}

	info, err := os.Stat(launcherPath())
	if err != nil {
		result.Status = StatusError
		result.Message = "bin/hive-mcp-foss missing"
		result.Details = launcherPath()
		return result
	}
	if info.Mode()&0111 == 0 {
		result.Status = StatusError
		result.Message = "bin/hive-mcp-foss is not executable"
		result.Details = "chmod +x " + launcherPath()
		return result
	}

	result.Status = StatusOK
	result.Message = "present and executable"
	return result
}

// checkNreplReachable probes the host's nREPL port, the same readiness
// signal the launcher and the container healthcheck use.
func checkNreplReachable() CheckResult {
	result := CheckResult{
		Name:    "hive-mcp nREPL",
		FixHint: "Start the host with bin/hive-mcp-foss (or open Claude Code, which starts it on demand)",
	}

	conn, err := net.DialTimeout("tcp", nreplAddr, 2*time.Second)
	if err != nil {
		result.Status = StatusWarning
		result.Message = "not listening on " + nreplAddr
		result.Details = "the host is only up while an MCP client holds it open"
		return result
	}
	conn.Close()

	result.Status = StatusOK
	result.Message = "listening on " + nreplAddr
	return result
}
