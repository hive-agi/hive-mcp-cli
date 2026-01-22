package detect

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PrereqCheck contains the result of a prerequisite check
type PrereqCheck struct {
	Status   Status
	Name     string
	Command  string // command used to check
	Version  string // detected version
	Required string // minimum required version
}

// prereqSpec defines a prerequisite to check
type prereqSpec struct {
	name       string
	command    string
	versionArg string
	versionRe  string // regex to extract version
	minVersion string
}

var prereqs = []prereqSpec{
	{
		name:       "Emacs",
		command:    "emacs",
		versionArg: "--version",
		versionRe:  `GNU Emacs (\d+\.\d+)`,
		minVersion: "28.1",
	},
	{
		name:       "Java",
		command:    "java",
		versionArg: "-version",
		versionRe:  `version "?(\d+)(?:\.(\d+))?`,
		minVersion: "17",
	},
	{
		name:       "Clojure",
		command:    "clojure",
		versionArg: "--version",
		versionRe:  `Clojure CLI version (\d+\.\d+\.\d+)`,
		minVersion: "1.11.0",
	},
	{
		name:       "Babashka",
		command:    "bb",
		versionArg: "--version",
		versionRe:  `babashka v?(\d+\.\d+\.\d+)`,
		minVersion: "1.3.0",
	},
	{
		name:       "Docker",
		command:    "docker",
		versionArg: "--version",
		versionRe:  `Docker version (\d+\.\d+\.\d+)`,
		minVersion: "20.0.0",
	},
	{
		name:       "Git",
		command:    "git",
		versionArg: "--version",
		versionRe:  `git version (\d+\.\d+\.\d+)`,
		minVersion: "2.0.0",
	},
	{
		name:       "Claude CLI",
		command:    "claude",
		versionArg: "--version",
		versionRe:  `(\d+\.\d+\.\d+)`,
		minVersion: "0.1.0",
	},
}

// CheckAllPrereqs checks all prerequisites
func CheckAllPrereqs() []PrereqCheck {
	results := make([]PrereqCheck, 0, len(prereqs))
	for _, spec := range prereqs {
		results = append(results, checkPrereq(spec))
	}
	return results
}

func checkPrereq(spec prereqSpec) PrereqCheck {
	check := PrereqCheck{
		Name:     spec.name,
		Command:  spec.command,
		Required: spec.minVersion,
	}

	// Check if command exists
	path, err := exec.LookPath(spec.command)
	if err != nil {
		check.Status = StatusMissing
		return check
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
		check.Status = StatusWarning
		check.Version = "unknown"
		return check
	}

	check.Version = matches[1]
	// For Java, version 17 might show as "17.0.x"
	if spec.command == "java" && len(matches) > 2 && matches[2] != "" {
		check.Version = matches[1] + "." + matches[2]
	}

	// Compare versions
	if compareVersions(check.Version, spec.minVersion) >= 0 {
		check.Status = StatusOK
	} else {
		check.Status = StatusWarning
	}

	return check
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
