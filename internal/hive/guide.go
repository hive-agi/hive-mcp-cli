package hive

import (
	"fmt"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/skill"
)

// guideCmd installs the setup skills this binary carries.
//
// Exposed over MCP so an assistant that has just been pointed at a bare machine
// can put the procedure in front of itself before doing anything.
var guideCmd = &bonzai.Cmd{
	Name:  "guide",
	Alias: "guides",
	Short: "install the setup skills Claude Code reads",

	Mcp: &bonzai.McpMeta{
		Desc: "Install or print the setup skills compiled into this binary: how to set up the hive-mcp harness (FOSS or licensed) and how to use the store. Embedded, so this works with no network, no token and no hive-mcp installed. Use install=true to write them under the user's skills directory.",
		Params: []bonzai.McpParam{
			{Name: "name", Desc: "One guide name. Omit for all of them.", Type: "string"},
			{Name: "install", Desc: "Write the guides to disk instead of printing them", Type: "boolean"},
		},
	},

	Long: `Install the setup skills that teach Claude Code how to set this up.

  hive guide                     list what is embedded
  hive guide --install           write them to your skills directory
  hive guide hive-mcp-setup      print one

These are prose, not catalog projections: nothing the store publishes describes
how to install a JVM host. They are compiled into this binary so a first run on
a bare machine leaves the procedure behind before anything else exists.

After installing, start Claude Code and say what you want:

  "help me set up the hive-mcp harness locally, I have a key"
  "help me set up a FOSS build of the hive-mcp harness"

Environment:
  HIVE_SKILLS_DIR    where --install writes (default ~/.claude/skills)`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		install, _, rest := skillFlags(args)

		guides, err := skill.Guides()
		if err != nil {
			return err
		}
		if len(rest) > 0 {
			guides = guides[:0]
			for _, name := range rest {
				g, err := skill.Guide(name)
				if err != nil {
					return err
				}
				guides = append(guides, g)
			}
		}

		if !install {
			if len(rest) > 0 {
				for i, g := range guides {
					if i > 0 {
						fmt.Println()
					}
					fmt.Print(g.Body)
				}
				return nil
			}
			for _, g := range guides {
				fmt.Printf("  %-20s %s\n", color.New(color.Bold).Sprint(g.Name), skill.GuideSummary(g))
			}
			fmt.Printf("\n%d guide(s). `hive guide --install` writes them to %s.\n",
				len(guides), skill.DefaultRoot())
			return nil
		}

		root := skill.DefaultRoot()
		written, err := skill.InstallAll(root, guides)
		if err != nil {
			return err
		}
		changed := 0
		for _, w := range written {
			mark := "unchanged"
			if w.Changed {
				mark = color.GreenString("written")
				changed++
			}
			fmt.Printf("  %-28s %s\n", w.Name, mark)
		}
		fmt.Printf("\n%d guide(s) under %s, %d changed.\n", len(written), root, changed)
		if changed > 0 {
			fmt.Println("Claude Code picks them up on its next start.")
		}
		return nil
	},
}
