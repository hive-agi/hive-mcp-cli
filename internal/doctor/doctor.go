// Package doctor provides health checks and diagnostics for hive-mcp installations
package doctor

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// Status represents the outcome of a health check
type Status int

const (
	StatusOK Status = iota
	StatusWarning
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusWarning:
		return "warning"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// Symbol returns a visual indicator for the status
func (s Status) Symbol() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusWarning:
		return "!"
	case StatusError:
		return "✗"
	default:
		return "?"
	}
}

// CheckResult represents the outcome of a single health check
type CheckResult struct {
	Name    string
	Status  Status
	Message string
	Details string       // Additional context
	CanFix  bool         // Whether auto-fix is available
	Fix     func() error // Optional auto-fix function
	FixHint string       // Manual fix instructions
}

// Category groups related checks
type Category struct {
	Name   string
	Checks []CheckResult
}

// DoctorResult contains all diagnostic results
type DoctorResult struct {
	Categories []Category
}

// Summary returns counts of ok, warning, and error checks
func (r *DoctorResult) Summary() (ok, warn, fail int) {
	for _, cat := range r.Categories {
		for _, check := range cat.Checks {
			switch check.Status {
			case StatusOK:
				ok++
			case StatusWarning:
				warn++
			case StatusError:
				fail++
			}
		}
	}
	return
}

// IsHealthy returns true if there are no errors
func (r *DoctorResult) IsHealthy() bool {
	_, _, fail := r.Summary()
	return fail == 0
}

// FixableChecks returns checks that have auto-fix available
func (r *DoctorResult) FixableChecks() []CheckResult {
	var fixable []CheckResult
	for _, cat := range r.Categories {
		for _, check := range cat.Checks {
			if check.CanFix && check.Status != StatusOK {
				fixable = append(fixable, check)
			}
		}
	}
	return fixable
}

// RunAll executes all health checks and returns the results
func RunAll() (*DoctorResult, error) {
	result := &DoctorResult{}

	// Version checks
	result.Categories = append(result.Categories, Category{
		Name:   "Version Requirements",
		Checks: CheckVersions(),
	})

	// Environment variables
	result.Categories = append(result.Categories, Category{
		Name:   "Environment Variables",
		Checks: CheckEnvVars(),
	})

	// Service health
	result.Categories = append(result.Categories, Category{
		Name:   "Service Health",
		Checks: CheckServices(),
	})

	// MCP registration
	result.Categories = append(result.Categories, Category{
		Name:   "MCP Configuration",
		Checks: CheckMCP(),
	})

	// Integration test
	result.Categories = append(result.Categories, Category{
		Name:   "Integration Tests",
		Checks: CheckIntegration(),
	})

	// Optional: Observability stack
	result.Categories = append(result.Categories, Category{
		Name:   "Observability (Optional)",
		Checks: CheckObservability(),
	})

	return result, nil
}

// PrintResult outputs the diagnostic results with color formatting
func PrintResult(r *DoctorResult) {
	fmt.Println()
	fmt.Println("hive-mcp Health Check")
	fmt.Println(strings.Repeat("=", 50))

	for _, cat := range r.Categories {
		fmt.Printf("\n%s:\n", cat.Name)

		for _, check := range cat.Checks {
			printCheck(check)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 50))
	ok, warn, fail := r.Summary()

	summaryParts := []string{}
	if ok > 0 {
		summaryParts = append(summaryParts, color.GreenString("%d passed", ok))
	}
	if warn > 0 {
		summaryParts = append(summaryParts, color.YellowString("%d warnings", warn))
	}
	if fail > 0 {
		summaryParts = append(summaryParts, color.RedString("%d failed", fail))
	}

	fmt.Printf("Summary: %s\n", strings.Join(summaryParts, ", "))

	// Final status
	if r.IsHealthy() {
		fmt.Printf("\n%s hive-mcp is healthy\n", color.GreenString("✓"))
	} else {
		fmt.Printf("\n%s Some issues need attention\n", color.RedString("✗"))

		// List fixable issues
		fixable := r.FixableChecks()
		if len(fixable) > 0 {
			fmt.Printf("\nRun 'hive doctor --fix' to attempt automatic fixes for %d issue(s)\n", len(fixable))
		}
	}
}

func printCheck(check CheckResult) {
	var symbol string
	switch check.Status {
	case StatusOK:
		symbol = color.GreenString("%s", check.Status.Symbol())
	case StatusWarning:
		symbol = color.YellowString("%s", check.Status.Symbol())
	case StatusError:
		symbol = color.RedString("%s", check.Status.Symbol())
	default:
		symbol = check.Status.Symbol()
	}

	fmt.Printf("  %s %s", symbol, check.Name)
	fmt.Printf("  %s %s", symbol, check.Name)

	if check.Message != "" {
		fmt.Printf(": %s", check.Message)
	}
	fmt.Println()

	if check.Details != "" && check.Status != StatusOK {
		fmt.Printf("    %s\n", color.HiBlackString(check.Details))
	}

	if check.FixHint != "" && check.Status != StatusOK {
		fmt.Printf("    %s %s\n", color.CyanString("Fix:"), check.FixHint)
	}
}

// RunFixes attempts to fix all fixable issues
func RunFixes(r *DoctorResult) (fixed, failed int) {
	fixable := r.FixableChecks()

	for _, check := range fixable {
		fmt.Printf("Fixing %s... ", check.Name)
		if err := check.Fix(); err != nil {
			color.Red("failed: %v\n", err)
			failed++
		} else {
			color.Green("done\n")
			fixed++
		}
	}

	return fixed, failed
}
