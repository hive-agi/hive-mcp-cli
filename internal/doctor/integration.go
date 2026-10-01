package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hive-agi/hive-mcp-cli/internal/setup"
)

func hiveMCPDir() string {
	dir := getEnv("HIVE_MCP_DIR", "")
	if dir == "" {
		dir = filepath.Join(getEnv("HOME", ""), "hive-mcp")
	}
	return dir
}

func launcherPath() string {
	return setup.LauncherPath(hiveMCPDir())
}

// CheckMCP verifies the MCP client registration. bb-mcp owns registration,
// so the verdict is `<ANCHOR>/bb-mcp setup --check`, and the fix is
// `bb-mcp setup` (through the setup step, which clones bb-mcp first when the
// anchor does not resolve).
func CheckMCP() []CheckResult {
	sys := setup.DefaultBBMCPSys()
	o := setup.ObserveBBMCP(sys)

	var out string
	var runErr error
	if o.AnchorCheckout != "" {
		argv := setup.BBMCPSetupArgv(o.Anchor, "--check")
		b, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		out, runErr = string(b), err
	}
	return []CheckResult{
		bbmcpCheckResult(o, out, runErr, fixWithBBMCPSetup),
		legacyCheckResult(o.Legacy),
	}
}

func fixWithBBMCPSetup() error {
	return (&setup.BBMCPStep{}).Run()
}

// bbmcpCheckResult turns the observed anchor and the `--check` outcome into a
// verdict. Pure, apart from carrying fix.
func bbmcpCheckResult(o setup.BBMCPObserved, out string, runErr error, fix func() error) CheckResult {
	r := CheckResult{
		Name:   "MCP registration (bb-mcp setup --check)",
		CanFix: true,
		Fix:    fix,
	}
	switch {
	case o.AnchorCheckout == "":
		r.Status = StatusError
		r.Message = "bb-mcp is not published at " + o.Anchor
		r.FixHint = "hive setup, or <your bb-mcp checkout>/bb-mcp setup"
	case runErr != nil:
		r.Status = StatusError
		r.Message = "bb-mcp setup --check failed"
		r.Details = lastLines(out, 12)
		r.FixHint = filepath.Join(o.Anchor, "bb-mcp") + " setup"
	default:
		r.Status = StatusOK
		r.Message = "hive -> " + filepath.Join(o.Anchor, "start-bb-mcp.sh") + " (bb-mcp at " + o.AnchorCheckout + ")"
	}
	return r
}

// legacyCheckResult flags registrations of the old direct launcher. The user
// entry is replaced by `bb-mcp setup`; a local one shadows it in its project.
func legacyCheckResult(entries []setup.LegacyEntry) CheckResult {
	r := CheckResult{Name: "Legacy hive-mcp-foss registrations"}
	if len(entries) == 0 {
		r.Status = StatusOK
		r.Message = "none"
		return r
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, setup.LegacyRemedy(e))
	}
	r.Status = StatusWarning
	r.Message = fmt.Sprintf("%d entry(ies) still launch bin/hive-mcp-foss directly", len(entries))
	r.Details = strings.Join(lines, "\n    ")
	r.FixHint = "bb-mcp setup replaces the user entry; run the commands above for project-local ones"
	return r
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n    ")
}

// CheckIntegration checks the backend bb-mcp talks to: the hive-mcp JVM.
func CheckIntegration() []CheckResult {
	return []CheckResult{
		checkLauncher(),
		checkNreplReachable(),
	}
}

func checkLauncher() CheckResult {
	result := CheckResult{
		Name:    "hive-mcp launcher (backend)",
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

// checkNreplReachable probes the backend's nREPL port, which every bb-mcp
// tool call goes through.
func checkNreplReachable() CheckResult {
	addr := setup.BackendNreplAddr(os.Getenv)
	result := CheckResult{
		Name:    "hive-mcp nREPL",
		FixHint: "hive start (runs bin/hive-mcp-foss in the background)",
	}
	if !setup.NreplListening(addr) {
		result.Status = StatusWarning
		result.Message = "not listening on " + addr
		result.Details = "bb-mcp is a client; its tools fail until the backend runs"
		return result
	}
	result.Status = StatusOK
	result.Message = "listening on " + addr
	return result
}
