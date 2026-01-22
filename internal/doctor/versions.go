package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// getEnv returns the value of an environment variable or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// versionSpec defines a tool version requirement
type versionSpec struct {
	name       string
	command    string
	versionArg string
	versionRe  string // regex to extract version
	minVersion string
	fixHint    string
}

var versionSpecs = []versionSpec{
	{
		name:       "Emacs",
		command:    "emacs",
		versionArg: "--version",
		versionRe:  `GNU Emacs (\d+\.\d+)`,
		minVersion: "28.1",
		fixHint:    "Install Emacs 28.1+ via package manager or build from source",
	},
	{
		name:       "Java",
		command:    "java",
		versionArg: "-version",
		versionRe:  `version "?(\d+)(?:\.(\d+))?`,
		minVersion: "17",
		fixHint:    "Install OpenJDK 17+: sudo apt install openjdk-17-jdk",
	},
	{
		name:       "Clojure",
		command:    "clojure",
		versionArg: "--version",
		versionRe:  `Clojure CLI version (\d+\.\d+\.\d+)`,
		minVersion: "1.11.0",
		fixHint:    "Install Clojure: curl -L -O https://github.com/clojure/brew-install/releases/latest/download/posix-install.sh && chmod +x posix-install.sh && sudo ./posix-install.sh",
	},
	{
		name:       "Babashka",
		command:    "bb",
		versionArg: "--version",
		versionRe:  `babashka v?(\d+\.\d+\.\d+)`,
		minVersion: "1.3.0",
		fixHint:    "Install Babashka: bash < <(curl -s https://raw.githubusercontent.com/babashka/babashka/master/install)",
	},
	{
		name:       "Docker",
		command:    "docker",
		versionArg: "--version",
		versionRe:  `Docker version (\d+\.\d+\.\d+)`,
		minVersion: "20.0.0",
		fixHint:    "Install Docker: https://docs.docker.com/engine/install/",
	},
	{
		name:       "Git",
		command:    "git",
		versionArg: "--version",
		versionRe:  `git version (\d+\.\d+\.\d+)`,
		minVersion: "2.0.0",
		fixHint:    "Install Git: sudo apt install git",
	},
	{
		name:       "Claude CLI",
		command:    "claude",
		versionArg: "--version",
		versionRe:  `(\d+\.\d+\.\d+)`,
		minVersion: "0.1.0",
		fixHint:    "Install Claude CLI: npm install -g @anthropic-ai/claude-code",
	},
}

// CheckVersions runs all version checks
func CheckVersions() []CheckResult {
	results := make([]CheckResult, 0, len(versionSpecs))
	for _, spec := range versionSpecs {
		results = append(results, checkVersion(spec))
	}
	return results
}

func checkVersion(spec versionSpec) CheckResult {
	result := CheckResult{
		Name:    spec.name,
		FixHint: spec.fixHint,
	}

	// Check if command exists
	path, err := exec.LookPath(spec.command)
	if err != nil {
		result.Status = StatusError
		result.Message = "not installed"
		result.Details = fmt.Sprintf("Requires %s %s+", spec.name, spec.minVersion)
		return result
	}

	// Get version
	var out []byte
	if spec.command == "java" {
		// Java outputs version to stderr
		cmd := exec.Command(path, spec.versionArg)
		out, _ = cmd.CombinedOutput()
	} else {
		out, err = exec.Command(path, spec.versionArg).Output()
		if err != nil {
			// Try combined output (some tools output to stderr)
			out, _ = exec.Command(path, spec.versionArg).CombinedOutput()
		}
	}

	// Extract version using regex
	re := regexp.MustCompile(spec.versionRe)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) < 2 {
		result.Status = StatusWarning
		result.Message = "version unknown"
		result.Details = "Could not parse version output"
		return result
	}

	version := matches[1]
	// For Java, version 17 might show as "17.0.x"
	if spec.command == "java" && len(matches) > 2 && matches[2] != "" {
		version = matches[1] + "." + matches[2]
	}

	// Compare versions
	cmp := compareVersions(version, spec.minVersion)
	if cmp >= 0 {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("v%s (>= %s)", version, spec.minVersion)
	} else {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("v%s (requires %s+)", version, spec.minVersion)
		result.Details = fmt.Sprintf("Installed version %s is below minimum %s", version, spec.minVersion)
	}

	return result
}

// compareVersions compares two version strings
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		aVal := 0
		bVal := 0
		if i < len(aParts) {
			aVal = aParts[i]
		}
		if i < len(bParts) {
			bVal = bParts[i]
		}

		if aVal < bVal {
			return -1
		}
		if aVal > bVal {
			return 1
		}
	}

	return 0
}

func parseVersion(v string) []int {
	// Remove any prefix like "v"
	v = strings.TrimPrefix(v, "v")

	// Split by dots
	parts := strings.Split(v, ".")
	result := make([]int, 0, len(parts))

	for _, p := range parts {
		// Extract leading digits only
		digits := ""
		for _, c := range p {
			if c >= '0' && c <= '9' {
				digits += string(c)
			} else {
				break
			}
		}
		if digits != "" {
			n, _ := strconv.Atoi(digits)
			result = append(result, n)
		}
	}

	return result
}

// CheckEnvVars checks required environment variables
func CheckEnvVars() []CheckResult {
	var results []CheckResult

	envSpecs := []struct {
		name     string
		required bool
		fixHint  string
	}{
		{"HIVE_MCP_DIR", true, "Add to shell config: export HIVE_MCP_DIR=$HOME/hive-mcp"},
		{"BB_MCP_DIR", true, "Add to shell config: export BB_MCP_DIR=$HOME/bb-mcp"},
		{"OPENROUTER_API_KEY", false, "Get API key from https://openrouter.ai and add to shell config"},
	}

	for _, spec := range envSpecs {
		results = append(results, checkEnvVar(spec.name, spec.required, spec.fixHint))
	}

	return results
}

func checkEnvVar(name string, required bool, fixHint string) CheckResult {
	result := CheckResult{
		Name:    name,
		FixHint: fixHint,
	}

	value := getEnv(name, "")
	if value != "" {
		// Mask sensitive values
		displayVal := value
		if strings.Contains(strings.ToLower(name), "key") || strings.Contains(strings.ToLower(name), "secret") {
			if len(displayVal) > 8 {
				displayVal = displayVal[:4] + "****" + displayVal[len(displayVal)-4:]
			} else {
				displayVal = "****"
			}
		}
		result.Status = StatusOK
		result.Message = displayVal
	} else if required {
		result.Status = StatusError
		result.Message = "not set (required)"
	} else {
		result.Status = StatusWarning
		result.Message = "not set (optional)"
	}

	return result
}
