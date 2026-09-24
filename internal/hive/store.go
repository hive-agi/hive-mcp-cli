package hive

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
	"golang.org/x/term"
)

// storeCmd holds the commands that wire THIS machine to the store. Browsing the
// catalog lives under `hive addon`; this is about credentials.
var storeCmd = &bonzai.Cmd{
	Name:  "store",
	Short: "connect this machine to the hive store",

	Long: `Store commands.

  hive store login            ask for your artifact token (hv_live_...), check it
                              against the gateway, write ~/.m2/settings.xml
  hive store login --check    report how this machine is wired, change nothing

The artifact token comes from https://store.hive-mcp.com/dashboard and is shown
once. It is NOT the store session token (HIVE_STORE_TOKEN).

Pass the token without it landing in your shell history:
  hive store login                         prompts, input hidden
  printf %s "$TOKEN" | hive store login    reads stdin
  HIVE_ARTIFACT_TOKEN=... hive store login`,

	Cmds: []*bonzai.Cmd{storeLoginCmd},
	Do:   func(x *bonzai.Cmd, args ...string) error { return showHelp(x) },
}

var storeLoginCmd = &bonzai.Cmd{
	Name:  "login",
	Short: "write your artifact token into ~/.m2/settings.xml",

	Mcp: &bonzai.McpMeta{
		Desc: "Report whether this machine's ~/.m2/settings.xml carries a working hive-store artifact token (check=true), or write one. Writing needs the token in HIVE_ARTIFACT_TOKEN; the tool never takes a token as a parameter, so it never lands in a transcript.",
		Params: []bonzai.McpParam{
			{Name: "check", Desc: "Only report the wiring; change nothing", Type: "boolean"},
		},
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		for _, a := range args {
			if a == "--check" || a == "-c" {
				return reportWiring()
			}
		}
		token, err := readToken(args)
		if err != nil {
			return err
		}
		return login(token)
	},
}

func readToken(args []string) (string, error) {
	if v, _ := flagValue(args, "token"); v != "" {
		fmt.Fprintln(os.Stderr, color.YellowString("note:"), "--token leaves the token in your shell history; prefer the prompt or stdin")
		return strings.TrimSpace(v), nil
	}
	if v := strings.TrimSpace(os.Getenv("HIVE_ARTIFACT_TOKEN")); v != "" {
		return v, nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("Artifact token (hv_live_..., input hidden): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no token: pass it on stdin, in HIVE_ARTIFACT_TOKEN, or run in a terminal to be prompted")
	}
	return strings.TrimSpace(line), nil
}

func login(token string) error {
	if token == "" {
		return errors.New("empty token")
	}
	if !strings.HasPrefix(token, "hv_live_") {
		fmt.Println(color.YellowString("--"), "this does not look like an artifact token (they start hv_live_).")
		fmt.Println("   A store SESSION token goes in HIVE_STORE_TOKEN instead; it cannot fetch artifacts.")
	}

	fmt.Printf("Checking the token against %s ...\n", store.MavenURL())
	result, status := store.ProbeToken(context.Background(), token)
	switch result {
	case store.ProbeRejected:
		return fmt.Errorf("the gateway rejected this token (%s). Nothing was written.\n"+
			"Mint a new one at %s/dashboard; a revoked token stops working immediately", status, store.BaseURL())
	case store.ProbeAccepted:
		fmt.Println(color.GreenString("  ok"), "token accepted")
	case store.ProbeNotEntitled:
		fmt.Println(color.YellowString("  --"), "token accepted, but the probe artifact is outside your plan ("+status+"); writing it anyway")
	default:
		fmt.Println(color.YellowString("  --"), "could not reach the gateway ("+status+"); writing the token unverified")
	}

	path := store.SettingsPath()
	backup, err := store.WriteSettings(path, store.RepoID, token)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Println(color.GreenString("  ok"), "server", store.RepoID, "in", path, "(mode 0600)")
	if backup != "" {
		fmt.Println("     previous file kept at", backup)
	}
	return reportShadow(store.Inspect())
}

func reportShadow(w store.Wiring) error {
	if w.Shadow != "" {
		fmt.Println(color.YellowString("  --"), w.Shadow, "also defines", store.RepoID+";",
			"a user-level definition wins over the project's, so check its URL is", store.MavenURL())
	}
	return nil
}

func reportWiring() error {
	w := store.Inspect()
	mark := func(ok bool) string {
		if ok {
			return color.GreenString("✓")
		}
		return color.RedString("✗")
	}
	fmt.Println("Store wiring")
	fmt.Printf("  %s %s exists\n", mark(w.SettingsExists), w.SettingsPath)
	fmt.Printf("  %s server <id>%s</id> present\n", mark(w.ServerFound), store.RepoID)
	if w.ServerFound {
		fmt.Printf("  %s token in both <username> and <password>\n", mark(w.UserEqualsPass))
	}
	if w.Token != "" {
		result, status := store.ProbeToken(context.Background(), w.Token)
		fmt.Printf("  %s gateway accepts %s (%s)\n", mark(result == store.ProbeAccepted || result == store.ProbeNotEntitled), store.Redact(w.Token), status)
	}
	_ = reportShadow(w)
	if !w.ServerFound {
		fmt.Println("\nRun: hive store login")
	}
	return nil
}
