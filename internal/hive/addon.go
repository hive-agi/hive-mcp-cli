package hive

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/addon"
	"github.com/hive-agi/hive-mcp-cli/internal/setup"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// addonCmd groups the end-user addon commands.
//
// The split between them is deliberate and is the same split the system has:
// search, show and coord ask the STORE what exists and what was bought; status
// asks the HOST what would mount; new writes files the user then owns. No
// command in this tree decides entitlement for itself.
var addonCmd = &bonzai.Cmd{
	Name:  "addon",
	Alias: "a|addons",
	Short: "search, inspect, scaffold and check hive addons",

	Mcp: &bonzai.McpMeta{
		Desc: "Browse the hive addon store, print the deps.edn coordinate for an addon, scaffold a self-owned IAddon (optionally one that extends an addon you bought), and report which addons on a project's classpath would mount.",
	},

	Long: `Addon commands for someone using hive, not building it.

  hive addon search [query]     search the store; --shelf narrows to a tag
  hive addon show <id>          one addon in full, and whether you own it
  hive addon coord <id>         the deps.edn entry to add
  hive addon add <id>           load it into your hive-mcp host (~/hive-mcp/local.deps.edn)
  hive addon skill [<id>]       render Claude Code skills from the catalog
  hive addon skill --install    write them to ~/.claude/skills
  hive addon new <id>           scaffold your own addon in this project
  hive addon new <id> --extends <other>
                                scaffold one that WRAPS an addon you bought
  hive addon status             what this project would mount, and why not

Environment:
  HIVE_STORE_URL     the store to talk to (default https://store.hive-mcp.com)
  HIVE_STORE_TOKEN   overrides the hive login session (CI); search works without either

Registering your own addon needs no account and no upload. The mounter scans
the classpath for META-INF/hive-addons/*.edn and mounts what it finds, whoever
wrote it, so "new" writes a manifest and a namespace and you are registered.

An addon you bought is extended by COMPOSING it: declare it in
:addon/dependencies and the mounter injects its live instance into your
constructor under :mount/dependencies. You never edit the vendor jar, and it
stays sealed.`,

	Cmds: []*bonzai.Cmd{addonSearchCmd, addonShowCmd, addonCoordCmd, addonAddCmd, addonSkillCmd, addonNewCmd, addonStatusCmd},

	Do: func(x *bonzai.Cmd, args ...string) error { return showHelp(x) },
}

func fetchCatalog() (*store.Catalog, *store.Client, error) {
	c := storeClient()
	cat, err := c.Catalog(context.Background())
	if err != nil {
		return nil, nil, err
	}
	return cat, c, nil
}

// entitlement asks the store what this customer owns. It is advisory: it says
// what was BOUGHT, never what will mount, which only the host can answer.
func entitlement(c *store.Client) *store.Subscription {
	if c.Token == "" {
		return nil
	}
	me, err := c.Me(context.Background())
	if err != nil {
		return nil
	}
	return me.Subscription
}

func flagValue(args []string, name string) (string, []string) {
	rest := make([]string, 0, len(args))
	value := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--"+name && i+1 < len(args) {
			value = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--"+name+"=") {
			value = strings.TrimPrefix(args[i], "--"+name+"=")
			continue
		}
		rest = append(rest, args[i])
	}
	return value, rest
}

var addonSearchCmd = &bonzai.Cmd{
	Name:  "search",
	Alias: "s|find|ls",
	Short: "search the addon store",

	Mcp: &bonzai.McpMeta{
		Desc: "Search the hive addon store by name, blurb, tag or capability. With no query, lists everything on the shelf.",
		Params: []bonzai.McpParam{
			{Name: "query", Desc: "Free text matched against id, name, blurb, tags and capabilities", Type: "string"},
			{Name: "shelf", Desc: "Narrow to one shelf: code, knowledge, agents, verification, editor, infrastructure", Type: "string"},
		},
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		shelf, rest := flagValue(args, "shelf")
		query := strings.Join(rest, " ")

		cat, client, err := fetchCatalog()
		if err != nil {
			return err
		}
		sub := entitlement(client)
		hits := store.Search(cat.Addons, query, shelf)
		if len(hits) == 0 {
			fmt.Printf("Nothing on the shelf matches %q.\n", query)
			if len(cat.Shelves) > 0 {
				fmt.Println("Shelves:", strings.Join(cat.Shelves, ", "))
			}
			return nil
		}
		fmt.Printf("%-20s %-10s %-9s %s\n", "ID", "STATUS", "OWNED", "NAME")
		for _, a := range hits {
			owned := "-"
			if sub == nil {
				owned = "?"
			} else if store.Entitled(sub, a.ID) {
				owned = color.GreenString("yes")
			}
			fmt.Printf("%-20s %-10s %-9s %s\n", a.ID, a.Status, owned, a.Name)
		}
		fmt.Println()
		fmt.Printf("%d of %d addon(s). `hive addon show <id>` for detail.\n", len(hits), len(cat.Addons))
		if sub == nil {
			fmt.Println("OWNED is unknown until you sign in: hive login")
		}
		return nil
	},
}

var addonShowCmd = &bonzai.Cmd{
	Name:  "show",
	Alias: "info",
	Short: "show one addon in full",

	Mcp: &bonzai.McpMeta{
		Desc: "Show one addon: what it is, its coordinate and version, the capabilities it provides and requires, what it depends on, and whether the current token owns it.",
		Params: []bonzai.McpParam{
			{Name: "id", Desc: "Addon id, or an unambiguous shorthand", Type: "string"},
		},
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		if len(args) < 1 {
			return fmt.Errorf("which addon? try `hive addon search`")
		}
		cat, client, err := fetchCatalog()
		if err != nil {
			return err
		}
		a, ok := store.Find(cat.Addons, args[0])
		if !ok {
			return fmt.Errorf("no addon matches %q unambiguously; try `hive addon search %s`", args[0], args[0])
		}
		sub := entitlement(client)

		fmt.Printf("%s  %s\n", color.CyanString(a.Name), a.ID)
		fmt.Println()
		fmt.Println(" ", a.Blurb)
		fmt.Println()
		line := func(k, v string) {
			if v != "" {
				fmt.Printf("  %-14s %s\n", k, v)
			}
		}
		line("coordinate", a.Coordinate)
		line("version", a.Version)
		line("status", a.Status)
		line("kind", a.Kind)
		line("shelves", strings.Join(a.Tags, ", "))
		line("provides", strings.Join(a.Capabilities, ", "))
		line("needs", strings.Join(a.RequiresCapabilities, ", "))
		line("depends on", strings.Join(a.Requires, ", "))
		line("needed by", strings.Join(a.RequiredBy, ", "))
		line("docs", a.Docs)
		if len(a.Highlights) > 0 {
			fmt.Println()
			for _, h := range a.Highlights {
				fmt.Println("  *", h)
			}
		}
		fmt.Println()
		switch {
		case sub == nil:
			fmt.Println("  Ownership unknown until you sign in: hive login")
		case store.Entitled(sub, a.ID):
			fmt.Println(" ", color.GreenString("You own this."), "`hive addon coord "+a.ID+"` for the deps.edn entry.")
		default:
			fmt.Printf("  Not on your %s plan (%s).\n", sub.Plan, sub.Status)
		}
		return nil
	},
}

var addonCoordCmd = &bonzai.Cmd{
	Name:  "coord",
	Alias: "dep|coordinate",
	Short: "print the deps.edn entry for an addon",

	Mcp: &bonzai.McpMeta{
		Desc: "Print the deps.edn coordinate for an addon, and the private Maven registry a paid addon is served from. Prints rather than edits: deps.edn carries hand-maintained aliases a rewrite would eventually lose.",
		Params: []bonzai.McpParam{
			{Name: "id", Desc: "Addon id, or an unambiguous shorthand", Type: "string"},
		},
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		if len(args) < 1 {
			return fmt.Errorf("which addon? try `hive addon search`")
		}
		cat, _, err := fetchCatalog()
		if err != nil {
			return err
		}
		a, ok := store.Find(cat.Addons, args[0])
		if !ok {
			return fmt.Errorf("no addon matches %q unambiguously", args[0])
		}
		fmt.Print(addon.Coordinate(a.Coordinate, a.Version, store.BaseURL()))
		return nil
	},
}

var addonAddCmd = &bonzai.Cmd{
	Name:  "add",
	Alias: "install|load",
	Short: "load an addon into your hive-mcp host",

	Mcp: &bonzai.McpMeta{
		Desc: "Put an addon's coordinate, and the hive-store repo, into the host overlay ~/hive-mcp/local.deps.edn, which bin/hive-mcp-foss merges at boot. Creates the file when absent; never rewrites an existing one (prints the lines to paste instead). Restart the host afterwards.",
		Params: []bonzai.McpParam{
			{Name: "id", Desc: "Addon id, or an unambiguous shorthand", Type: "string"},
		},
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		if len(args) < 1 {
			return fmt.Errorf("which addon? try `hive addon search`")
		}
		cat, _, err := fetchCatalog()
		if err != nil {
			return err
		}
		a, ok := store.Find(cat.Addons, args[0])
		if !ok {
			return fmt.Errorf("no addon matches %q unambiguously; try `hive addon search %s`", args[0], args[0])
		}
		if a.Status != "" && a.Status != "available" {
			return fmt.Errorf("%s is %s, not available: its coordinate does not resolve yet", a.ID, a.Status)
		}
		path := addon.OverlayPath(setup.DefaultHiveMCPDir())
		res, snippet, err := addon.AddToOverlay(path, a.Coordinate, a.Version, store.BaseURL())
		if err != nil {
			return err
		}
		switch res {
		case addon.OverlayCreated:
			fmt.Println(color.GreenString("  ok"), "wrote", path)
		case addon.OverlayPresent:
			fmt.Println(color.GreenString("  ok"), a.Coordinate, "is already in", path)
		default:
			fmt.Println(path, "exists and is yours, so it was left alone. Merge these into its map:")
			fmt.Println()
			fmt.Print(snippet)
			fmt.Println()
		}
		if w := store.Inspect(); !w.ServerFound {
			fmt.Println(color.YellowString("  --"), "no hive-store token in ~/.m2/settings.xml yet; a paid addon will not resolve. Run: hive store login")
		}
		fmt.Println("Restart the host to load it: quit and reopen Claude Code (or /mcp, reconnect hive).")
		return nil
	},
}

var addonNewCmd = &bonzai.Cmd{
	Name:  "new",
	Alias: "scaffold|init",
	Short: "scaffold an addon you own",

	Mcp: &bonzai.McpMeta{
		Desc: "Scaffold a self-owned IAddon in the current project: an implementation namespace and the META-INF/hive-addons manifest that registers it. With --extends, the scaffold WRAPS an addon you bought, receiving its live instance through :mount/dependencies rather than editing it.",
		Params: []bonzai.McpParam{
			{Name: "id", Desc: "Addon id, which doubles as the namespace, e.g. acme.rank", Type: "string"},
			{Name: "extends", Desc: "Addon id this one decorates", Type: "string"},
			{Name: "capability", Desc: "Namespaced capability to declare, repeatable", Type: "string"},
		},
	},

	Long: `Writes two files and never overwrites one:

  src/<ns>.clj                              your IAddon and its constructor
  resources/META-INF/hive-addons/<id>.edn   the manifest that registers it

That manifest IS the registration. The mounter scans the classpath for it, so
nothing is uploaded and no account is involved.

With --extends the constructor reads the addon it wraps out of
:mount/dependencies, which the mounter fills with the LIVE instance after
mounting it first. When that addon is refused, by a licence or a failure, yours
still mounts and sees nil.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		extends, rest := flagValue(args, "extends")
		capability, rest := flagValue(rest, "capability")
		if len(rest) < 1 {
			return fmt.Errorf("what is it called? e.g. `hive addon new acme.rank`")
		}
		id := rest[0]
		if !strings.Contains(id, ".") {
			return fmt.Errorf("an addon id needs a namespace segment, e.g. acme.%s", id)
		}
		spec := addon.Spec{ID: id, Extends: extends}
		if capability != "" {
			spec.Capabilities = strings.Split(capability, ",")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		written, err := addon.Write(cwd, spec)
		if err != nil {
			return err
		}
		fmt.Println(color.GreenString("Wrote:"))
		for _, w := range written {
			fmt.Println(" ", w)
		}
		fmt.Println()
		if extends != "" {
			fmt.Printf("%s wraps %s. Put %s on this project's classpath, then:\n", id, extends, extends)
		} else {
			fmt.Printf("%s is registered by its manifest. Check it with:\n", id)
		}
		fmt.Println("  hive addon status")
		return nil
	},
}

var addonStatusCmd = &bonzai.Cmd{
	Name:  "status",
	Alias: "st|check",
	Short: "what this project would mount, and why not",

	Mcp: &bonzai.McpMeta{
		Desc: "Report which addons are on this project's classpath and whether each would mount, by running hive-addon's own dry-run in a JVM. Constructs nothing and initializes nothing. The verdict comes from the mounter under the host's licence gate, never from a decision made in the CLI.",
	},

	Long: `Runs the mounter's dry-run against this project's classpath.

Nothing is constructed and nothing is initialized: this reports what mounting
WOULD do. The verdict comes from hive-addon under whatever licence gate the
host installs, and is not recomputed here. A host with no gate installed refuses
every proprietary addon, which is the closed default and what the table will
say.

Needs the Clojure CLI and a project directory.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if !addon.IsProject(cwd) {
			return fmt.Errorf("no deps.edn, bb.edn or project.clj here: run this in a project")
		}
		if !addon.Available() {
			return fmt.Errorf("the Clojure CLI is not on PATH; `hive setup` installs it")
		}
		version, _ := flagValue(args, "hive-addon-version")
		return addon.Run(cwd, version)
	},
}
