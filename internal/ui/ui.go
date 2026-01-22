// Package ui provides terminal UI helpers for hive-mcp-cli
package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

// Colors for consistent output styling
var (
	Success = color.New(color.FgGreen, color.Bold)
	Warning = color.New(color.FgYellow, color.Bold)
	Error   = color.New(color.FgRed, color.Bold)
	Info    = color.New(color.FgCyan)
	Dim     = color.New(color.FgHiBlack)
)

// Symbols for status indicators
const (
	SymbolCheck   = "\u2713" // Check mark
	SymbolCross   = "\u2717" // Cross mark
	SymbolWarning = "\u26A0" // Warning triangle
	SymbolInfo    = "\u2139" // Info symbol
	SymbolArrow   = "\u2192" // Right arrow
)

// PrintSuccess prints a success message with green checkmark
func PrintSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	Success.Printf("%s %s\n", SymbolCheck, msg)
}

// PrintError prints an error message with red cross
func PrintError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	Error.Printf("%s %s\n", SymbolCross, msg)
}

// PrintWarning prints a warning message with yellow triangle
func PrintWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	Warning.Printf("%s %s\n", SymbolWarning, msg)
}

// PrintInfo prints an info message with cyan color
func PrintInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	Info.Printf("%s %s\n", SymbolInfo, msg)
}

// PrintStep prints a step indicator for multi-step operations
func PrintStep(current, total int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	Dim.Printf("[%d/%d] ", current, total)
	fmt.Println(msg)
}

// PrintHeader prints a section header
func PrintHeader(title string) {
	fmt.Println()
	Info.Println(title)
	Info.Println(repeat("-", len(title)))
}

// Spinner wraps briandowns/spinner for progress indication
type Spinner struct {
	s *spinner.Spinner
}

// NewSpinner creates a new spinner with a message
func NewSpinner(message string) *Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + message
	s.Writer = os.Stderr
	return &Spinner{s: s}
}

// Start begins the spinner animation
func (sp *Spinner) Start() {
	sp.s.Start()
}

// Stop ends the spinner animation
func (sp *Spinner) Stop() {
	sp.s.Stop()
}

// Success stops the spinner and shows success message
func (sp *Spinner) Success(message string) {
	sp.s.Stop()
	PrintSuccess(message)
}

// Fail stops the spinner and shows error message
func (sp *Spinner) Fail(message string) {
	sp.s.Stop()
	PrintError(message)
}

// UpdateMessage changes the spinner message
func (sp *Spinner) UpdateMessage(message string) {
	sp.s.Suffix = " " + message
}

// repeat returns a string of n copies of s
func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// ConfirmPrompt asks user for yes/no confirmation
func ConfirmPrompt(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	var response string
	fmt.Scanln(&response)
	return response == "y" || response == "Y" || response == "yes" || response == "Yes"
}
