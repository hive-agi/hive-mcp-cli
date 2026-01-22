package detect

import (
	"fmt"
	"strings"
)

// Status represents the state of a check
type Status int

const (
	StatusUnknown Status = iota
	StatusOK
	StatusWarning
	StatusError
	StatusMissing
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusWarning:
		return "warning"
	case StatusError:
		return "error"
	case StatusMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// Symbol returns the check mark or X for the status
func (s Status) Symbol() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusWarning:
		return "!"
	case StatusError, StatusMissing:
		return "✗"
	default:
		return "?"
	}
}

// DetectionResult contains all detection results
type DetectionResult struct {
	Platform PlatformInfo
	Shell    ShellInfo
	Prereqs  []PrereqCheck
	Services []ServiceCheck
	EnvVars  []EnvVarCheck
}

// Summary returns a summary of all checks
func (r *DetectionResult) Summary() (ok, warn, fail int) {
	checks := []Status{r.Platform.Status, r.Shell.Status}
	for _, p := range r.Prereqs {
		checks = append(checks, p.Status)
	}
	for _, s := range r.Services {
		checks = append(checks, s.Status)
	}
	for _, e := range r.EnvVars {
		checks = append(checks, e.Status)
	}

	for _, s := range checks {
		switch s {
		case StatusOK:
			ok++
		case StatusWarning:
			warn++
		default:
			fail++
		}
	}
	return
}

// IsReady returns true if all required checks pass
func (r *DetectionResult) IsReady() bool {
	_, _, fail := r.Summary()
	return fail == 0
}

// Run executes all detection checks and returns the results
func Run() (*DetectionResult, error) {
	result := &DetectionResult{}

	// Detect platform
	result.Platform = DetectPlatform()

	// Detect shell
	result.Shell = DetectShell()

	// Check prerequisites
	result.Prereqs = CheckAllPrereqs()

	// Check services
	result.Services = CheckAllServices()

	// Check environment variables
	result.EnvVars = CheckAllEnvVars()

	return result, nil
}

// PrintResult prints the detection result in a formatted way
func PrintResult(r *DetectionResult, colorize func(Status, string) string) {
	if colorize == nil {
		colorize = func(_ Status, s string) string { return s }
	}

	fmt.Println("System Detection Results")
	fmt.Println(strings.Repeat("=", 50))

	// Platform
	fmt.Println("\nPlatform:")
	fmt.Printf("  %s %s (%s)\n",
		colorize(r.Platform.Status, r.Platform.Status.Symbol()),
		r.Platform.OS,
		r.Platform.PackageManager)

	// Shell
	fmt.Println("\nShell:")
	fmt.Printf("  %s %s\n",
		colorize(r.Shell.Status, r.Shell.Status.Symbol()),
		r.Shell.Name)
	if r.Shell.ConfigFile != "" {
		fmt.Printf("    Config: %s\n", r.Shell.ConfigFile)
	}

	// Prerequisites
	fmt.Println("\nPrerequisites:")
	for _, p := range r.Prereqs {
		status := colorize(p.Status, p.Status.Symbol())
		if p.Status == StatusOK || p.Status == StatusWarning {
			fmt.Printf("  %s %s: %s (requires %s)\n",
				status, p.Name, p.Version, p.Required)
		} else {
			fmt.Printf("  %s %s: %s (requires %s)\n",
				status, p.Name, "not found", p.Required)
		}
	}

	// Services
	fmt.Println("\nServices:")
	for _, s := range r.Services {
		status := colorize(s.Status, s.Status.Symbol())
		if s.Status == StatusOK {
			fmt.Printf("  %s %s: running", status, s.Name)
			if s.Endpoint != "" {
				fmt.Printf(" at %s", s.Endpoint)
			}
			fmt.Println()
		} else {
			fmt.Printf("  %s %s: not running\n", status, s.Name)
		}
	}

	// Environment Variables
	fmt.Println("\nEnvironment Variables:")
	for _, e := range r.EnvVars {
		status := colorize(e.Status, e.Status.Symbol())
		if e.Status == StatusOK {
			val := e.Value
			if e.Sensitive && len(val) > 8 {
				val = val[:4] + "****" + val[len(val)-4:]
			}
			fmt.Printf("  %s %s: %s\n", status, e.Name, val)
		} else if e.Required {
			fmt.Printf("  %s %s: not set (required)\n", status, e.Name)
		} else {
			fmt.Printf("  %s %s: not set (optional)\n", status, e.Name)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 50))
	ok, warn, fail := r.Summary()
	fmt.Printf("Summary: %d passed, %d warnings, %d failed\n", ok, warn, fail)

	if r.IsReady() {
		fmt.Println("\n" + colorize(StatusOK, "✓") + " System is ready for hive-mcp setup")
	} else {
		fmt.Println("\n" + colorize(StatusError, "✗") + " Please resolve issues before running setup")
	}
}

