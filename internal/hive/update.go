package hive

import (
	"fmt"

	"github.com/BuddhiLW/bonzai"
	"github.com/hive-agi/hive-mcp-cli/internal/setup"
)

// updateCmd fast-forwards the bb-mcp checkout the anchor resolves to (and,
// with --all, the hive-mcp checkout) and lets bb-mcp verify itself.
var updateCmd = &bonzai.Cmd{
	Name:  "update",
	Alias: "up-to-date|upgrade",
	Short: "fast-forward bb-mcp to its latest release",

	Mcp: &bonzai.McpMeta{
		Desc: "Update bb-mcp: resolve the checkout published at ~/.local/share/hive-mcp/bb-mcp, refuse when it has local changes or is not on a tracked branch, git fetch then fast-forward only, then run '<checkout>/bb-mcp setup --check'. --all also updates the hive-mcp checkout at $HIVE_MCP_DIR (default ~/hive-mcp). --dry-run prints the plan.",
		Params: []bonzai.McpParam{
			{Name: "dry-run", Desc: "Print the plan without changing anything", Type: "boolean"},
			{Name: "all", Desc: "Also update the hive-mcp checkout hive setup cloned", Type: "boolean"},
		},
	},

	Long: `Update brings your bb-mcp checkout up to date. MCP clients launch bb-mcp
through the anchor ~/.local/share/hive-mcp/bb-mcp, so updating means
fast-forwarding the checkout the anchor points at:

  1. resolve the checkout through the anchor (else ${BB_MCP_DIR:-~/bb-mcp})
  2. refuse when it has local changes, a detached HEAD, or no tracked branch,
     and say how to proceed; nothing is stashed, rebased or switched for you
  3. git fetch the tracked remote, then git merge --ff-only @{u}
  4. run '<checkout>/bb-mcp setup --check' and show its result

  hive update              # bb-mcp only
  hive update --all        # also the hive-mcp checkout ($HIVE_MCP_DIR, ~/hive-mcp)
  hive update --dry-run    # print the plan, change nothing

Restart your MCP clients afterwards so they launch the new bb-mcp; after
--all, restart the backend too (hive start).`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		var opts setup.UpdateOptions
		for _, a := range args {
			switch a {
			case "--dry-run", "-n":
				opts.DryRun = true
			case "--all", "-a":
				opts.All = true
			default:
				return fmt.Errorf("unknown argument %q (try: hive help update)", a)
			}
		}
		fmt.Println("hive update")
		if err := setup.RunUpdate(setup.DefaultUpdateSys(), opts); err != nil {
			fmt.Println()
			fmt.Println("Update incomplete; see the lines above for how to proceed.")
			return err
		}
		if !opts.DryRun {
			fmt.Println()
			fmt.Println("Up to date. Restart your MCP clients to pick up the new bb-mcp.")
		}
		return nil
	},
}
