package hive

import (
	"fmt"
	"runtime"

	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/detect"
	"github.com/hive-agi/hive-mcp-cli/internal/doctor"
	"github.com/hive-agi/hive-mcp-cli/internal/setup"
	"github.com/BuddhiLW/bonzai"
)

// showHelp displays help information for a command
func showHelp(cmd *bonzai.Cmd) error {
	// Header
	fmt.Printf("%s - %s\n", color.CyanString(cmd.Name), cmd.Short)
	if cmd.Vers != "" {
		fmt.Printf("Version: %s\n", cmd.Vers)
	}
	fmt.Println()

	// Usage
	if cmd.Usage != "" {
		fmt.Printf("Usage: %s\n\n", cmd.Usage)
	}

	// Commands
	if len(cmd.Cmds) > 0 {
		fmt.Println("Commands:")
		for _, c := range cmd.Cmds {
			if c.Name != "" && !c.IsHidden() {
				aliasStr := ""
				if c.Alias != "" {
					aliasStr = fmt.Sprintf(" (%s)", c.Alias)
				}
				fmt.Printf("  %s%s - %s\n", color.GreenString(c.Name), aliasStr, c.Short)
			}
		}
		fmt.Println()
	}

	// Long description
	if cmd.Long != "" {
		fmt.Println(cmd.Long)
	}

	return nil
}

// helpCmd displays help for commands
var helpCmd = &bonzai.Cmd{
	Name:  "help",
	Alias: "h|?|-h|--help",
	Short: "display help information",
	Long: `Display help for hive-mcp-cli commands.

Usage:
  hive help           # Show main help
  hive help detect    # Show help for detect command
  hive help setup     # Show help for setup command`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		caller := x.Caller()

		// If args provided, seek to that subcommand
		if len(args) > 0 && caller != nil {
			target, _, _ := caller.SeekInit(args...)
			if target != nil && target != caller {
				return showHelp(target)
			}
		}

		// Show help for caller (parent command)
		if caller != nil && caller != x {
			return showHelp(caller)
		}

		// Fallback: show help for self
		return showHelp(x)
	},
}

// Cmd is the root command for hive-mcp-cli
var Cmd = &bonzai.Cmd{
	Name:  "hive",
	Alias: "hive-mcp",
	Vers:  "v0.2.2",
	Short: "automated hive-mcp setup CLI",

	// MCP metadata for AI tool discovery
	Mcp: &bonzai.McpMeta{
		Desc: "Automated setup and management CLI for hive-mcp swarm infrastructure. Provides detection, installation, and health checking capabilities.",
	},

	Long: `hive-mcp-cli automates the installation and verification of hive-mcp.

Commands:
  detect  - Detect system prerequisites and installed components
  setup   - Install and configure hive-mcp components
  doctor  - Diagnose and fix common issues
  help    - Display help information

Examples:
  hive detect          # Check system prerequisites
  hive setup           # Run full setup
  hive doctor          # Diagnose issues
  hive help detect     # Show help for detect command`,

	Cmds: []*bonzai.Cmd{helpCmd, detectCmd, setupCmd, doctorCmd},

	// Show help when called without arguments
	Do: func(x *bonzai.Cmd, args ...string) error {
		return showHelp(x)
	},
}

// detectCmd checks system prerequisites
var detectCmd = &bonzai.Cmd{
	Name:  "detect",
	Alias: "d|check",
	Short: "detect system prerequisites and components",

	// MCP metadata for AI tool discovery
	Mcp: &bonzai.McpMeta{
		Desc: "Detect installed hive-mcp components, prerequisites, and environment configuration. Scans for Emacs, Java, Clojure, Babashka, Docker, Git, Claude CLI, and checks environment variables.",
	},

	Long: `Detect scans your system for:
  - Platform (Linux/macOS) and package manager
  - Shell configuration files
  - Required tools: Emacs, Java, Clojure, Babashka, Docker, Git, Claude CLI
  - Running services: Emacs daemon, Chroma, Ollama
  - Environment variables: HIVE_MCP_DIR, BB_MCP_DIR, OPENROUTER_API_KEY`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		fmt.Println("Detecting system configuration...")
		fmt.Println()

		result, err := detect.Run()
		if err != nil {
			return fmt.Errorf("detection failed: %w", err)
		}

		// Create colorizer function
		colorize := func(status detect.Status, s string) string {
			switch status {
			case detect.StatusOK:
				return color.GreenString(s)
			case detect.StatusWarning:
				return color.YellowString(s)
			case detect.StatusError, detect.StatusMissing:
				return color.RedString(s)
			default:
				return s
			}
		}

		detect.PrintResult(result, colorize)
		return nil
	},
}

// setupCmd installs and configures components
var setupCmd = &bonzai.Cmd{
	Name:  "setup",
	Alias: "s|install",
	Short: "install and configure hive-mcp components",

	// MCP metadata for AI tool discovery
	Mcp: &bonzai.McpMeta{
		Desc: "Install and configure hive-mcp components including cloning repos, installing prerequisites, downloading Clojure deps, setting up Emacs, and registering MCP server with Claude CLI.",
	},

	Long: `Setup performs the following steps:
  1. Clone repositories (hive-mcp, bb-mcp)
  2. Configure shell environment
  3. Install prerequisites (platform-specific)
  4. Download Clojure dependencies
  5. Sync Emacs packages
  6. Setup Docker volumes and Chroma
  7. Configure Ollama with embedding model
  8. Start Emacs daemon
  9. Register MCP server with Claude CLI`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		fmt.Println("🐝 hive-mcp setup")
		fmt.Println()

		// Determine platform
		platform := runtime.GOOS

		// Build step list
		steps := []setup.Step{
			&setup.CloneStep{},
			&setup.ShellStep{},
			&setup.PrerequisitesStep{Platform: platform},
			&setup.CloneDepsStep{},
			&setup.DoomSyncStep{},
			&setup.ChromaStep{},
			&setup.OllamaStep{},
			&setup.EmacsDaemonStep{},
			&setup.MCPStep{},
		}

		// Create runner with progress output
		runner := setup.NewRunner(steps)

		// Run all steps
		if err := runner.RunAll(); err != nil {
			fmt.Println()
			fmt.Printf("Setup failed: %v\n", err)
			fmt.Println("Run 'hive doctor' to diagnose issues.")
			return err
		}

		// Summary
		fmt.Println()
		fmt.Println("Setup complete!")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Restart your shell or run: source ~/.bashrc")
		fmt.Println("  2. Verify with: hive doctor")
		fmt.Println("  3. Start using: claude")
		fmt.Println()

		return nil
	},
}

// doctorCmd diagnoses and fixes issues
var doctorCmd = &bonzai.Cmd{
	Name:  "doctor",
	Alias: "dr|diagnose",
	Short: "diagnose and fix common issues",

	// MCP metadata for AI tool discovery
	Mcp: &bonzai.McpMeta{
		Desc: "Run health checks on hive-mcp installation including version verification, service health, environment validation, and MCP registration status. Use --fix flag to attempt automatic fixes.",
		Params: []bonzai.McpParam{
			{Name: "fix", Desc: "Attempt automatic fixes for fixable issues", Type: "boolean"},
		},
	},

	Long: `Doctor performs health checks:
  - Version verification (minimum requirements)
  - Service health (Chroma, Ollama endpoints)
  - Environment variable validation
  - MCP registration status
  - Integration test (Emacs MCP connection)
  - Optional observability stack check

Use --fix to attempt automatic fixes for fixable issues.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		fmt.Println("Running hive-mcp health checks...")

		// Check for --fix flag
		fix := false
		for _, arg := range args {
			if arg == "--fix" || arg == "-f" {
				fix = true
				break
			}
		}

		// Run all health checks
		result, err := doctor.RunAll()
		if err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}

		// Print results
		doctor.PrintResult(result)

		// Attempt fixes if requested
		if fix {
			fixable := result.FixableChecks()
			if len(fixable) > 0 {
				fmt.Println()
				fmt.Println("Attempting automatic fixes...")
				fixed, failed := doctor.RunFixes(result)
				fmt.Printf("\nFixed %d issue(s), %d failed\n", fixed, failed)

				// Re-run checks to show updated status
				if fixed > 0 {
					fmt.Println("\nRe-running health checks...")
					result, _ = doctor.RunAll()
					doctor.PrintResult(result)
				}
			} else {
				fmt.Println("\nNo fixable issues found.")
			}
		}

		return nil
	},
}
