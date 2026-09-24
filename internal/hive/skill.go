package hive

import (
	"fmt"
	"strings"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/skill"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// addonSkillCmd renders a Claude Code skill for one addon.
//
// The skill is a PROJECTION of the catalog, not a hand-written guide: every
// line it emits comes from a field the store sends. That is what keeps it from
// drifting, and it is why this command lives beside `show` rather than shipping
// a folder of prose in the binary.
//
// Exposed over MCP so an assistant can read a skill for an addon it has never
// seen without anything being installed first.
var addonSkillCmd = &bonzai.Cmd{
	Name:  "skill",
	Alias: "skills",
	Short: "render Claude Code skills for addons",

	Mcp: &bonzai.McpMeta{
		Desc: "Render a Claude Code skill teaching how to use a hive addon: what it is, whether its coordinate resolves today, the capabilities it provides and needs, the addons above and below it, and every extension point it opens with the port and registrar to implement. With no id, renders one skill per addon in the catalog. Use install=true to write them under the user's skills directory.",
		Params: []bonzai.McpParam{
			{Name: "id", Desc: "Addon id, or an unambiguous shorthand. Omit for every addon.", Type: "string"},
			{Name: "install", Desc: "Write the skills to disk instead of printing them", Type: "boolean"},
			{Name: "owned", Desc: "Only addons the signed-in account (hive login) is entitled to", Type: "boolean"},
		},
	},

	Long: `Render a Claude Code skill per addon, from the storefront catalog.

  hive addon skill <id>          print one skill
  hive addon skill               print a skill for every addon
  hive addon skill --install     write them to your skills directory
  hive addon skill --owned --install
                                 write only the ones you are entitled to

Each skill states whether the coordinate resolves TODAY, so a preview addon is
described without pretending it can be fetched. Where an addon opens extension
points, the skill names the port to implement and the registrar to call, and
says plainly when a seam is declared but vacant.

The skills deliberately carry no invented tool invocations. Ask the running
host for its command surface; a listing derived from the live handler table
cannot go stale, and a pasted command that does not run is worse than none.

Environment:
  HIVE_SKILLS_DIR    where --install writes (default ~/.claude/skills)
  HIVE_STORE_URL     the store to talk to (default https://store.hive-mcp.com)
  HIVE_STORE_TOKEN   overrides the hive login session (CI); rendering works without either`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		install, owned, rest := skillFlags(args)

		cat, client, err := fetchCatalog()
		if err != nil {
			return err
		}

		targets, err := skillTargets(cat, client, owned, rest)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return fmt.Errorf("no addons to render; try `hive addon search`")
		}

		skills := skill.RenderAll(targets)
		if !install {
			for i, s := range skills {
				if i > 0 {
					fmt.Println()
				}
				fmt.Print(s.Body)
			}
			return nil
		}

		root := skill.DefaultRoot()
		written, err := skill.InstallAll(root, skills)
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
		fmt.Printf("\n%d skill(s) under %s, %d changed.\n", len(written), root, changed)
		if changed > 0 {
			fmt.Println("Claude Code picks them up on its next start.")
		}
		return nil
	},
}

// skillFlags splits the flags this command understands from its positional
// args. Bonzai leaves flag parsing to the command, and `addon new` does the
// same by hand.
func skillFlags(args []string) (install, owned bool, rest []string) {
	for _, a := range args {
		switch a {
		case "--install", "-i", "install", "install=true":
			install = true
		case "--owned", "-o", "owned", "owned=true":
			owned = true
		case "install=false", "owned=false":
			// an MCP client may send the negative explicitly
		default:
			if !strings.HasPrefix(a, "-") {
				rest = append(rest, a)
			}
		}
	}
	return install, owned, rest
}

// skillTargets picks which addons to render.
//
// `owned` without a token is an ERROR rather than an empty result: silently
// rendering nothing would read as "you own nothing", which is a different
// claim from "I could not ask".
func skillTargets(cat *store.Catalog, client *store.Client, owned bool, ids []string) ([]store.Addon, error) {
	if len(ids) > 0 {
		out := make([]store.Addon, 0, len(ids))
		for _, id := range ids {
			a, ok := store.Find(cat.Addons, id)
			if !ok {
				return nil, fmt.Errorf("no addon matches %q unambiguously; try `hive addon search %s`", id, id)
			}
			out = append(out, a)
		}
		return out, nil
	}
	if !owned {
		return cat.Addons, nil
	}
	sub := entitlement(client)
	if sub == nil {
		return nil, fmt.Errorf("--owned needs you signed in: run hive login, or drop --owned to render the whole catalog")
	}
	out := make([]store.Addon, 0, len(cat.Addons))
	for _, a := range cat.Addons {
		if store.Entitled(sub, a.ID) {
			out = append(out, a)
		}
	}
	return out, nil
}
