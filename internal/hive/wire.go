package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/adapter/credstore"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/adapter/loopback"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/adapter/oidc"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/adapter/storeapi"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/adapter/system"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/pipeline"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// authDeps is the COMPOSITION ROOT for sign-in: the one place that knows which
// adapter fills which port. Everything below it speaks only in ports.
func authDeps() (pipeline.Deps, error) {
	dir := system.ConfigDir()
	creds, err := credstore.Open(dir)
	if err != nil {
		return pipeline.Deps{}, err
	}
	return pipeline.Deps{
		IdP:         oidc.New(oidc.RealmFromEnv()),
		Callback:    loopback.New(),
		Browser:     system.Browser{},
		Credentials: creds,
		Sessions:    system.SessionFile{Dir: dir},
		Accounts:    storeapi.Accounts{},
		Maven:       storeapi.Maven{},
		Prompter:    consolePrompter{},
		Clock:       system.Clock{},
		Entropy:     system.Entropy{},
		Machine:     system.Machine{},
	}, nil
}

// storeClient is a store client carrying the best credential available:
// HIVE_STORE_TOKEN when set (CI), else the `hive login` session.
func storeClient() *store.Client {
	c := store.New()
	if c.Token != "" {
		return c
	}
	d, err := authDeps()
	if err != nil {
		return c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if t, err := pipeline.AccessToken(ctx, d); err == nil {
		c.Token = t
	}
	return c
}

// consolePrompter implements port.Prompter on the terminal.
type consolePrompter struct{}

func (consolePrompter) OpeningBrowser(url string, opened bool) {
	fmt.Println("Opening your browser to sign in to hive...")
	if opened {
		fmt.Println("If it did not open, use this URL:")
	} else {
		fmt.Println(color.YellowString("--"), "could not open a browser. Open this URL yourself:")
	}
	fmt.Println(" ", url)
	fmt.Println(color.HiBlackString("Waiting for the browser... (Ctrl-C to cancel; hive login --device if this machine has no browser)"))
}

func (consolePrompter) ShowDeviceCode(g domain.DeviceGrant) {
	fmt.Println()
	fmt.Println("  First copy your one-time code:", color.New(color.Bold).Sprint(g.UserCode))
	fmt.Println("  Then open:                    ", g.VerificationURI)
	if g.VerificationURIComplete != "" {
		fmt.Println("  (or, with the code filled in:", g.VerificationURIComplete+")")
	}
	fmt.Println()
	fmt.Println(color.HiBlackString("Waiting for you to approve the code... (Ctrl-C to cancel)"))
}

func (consolePrompter) Note(msg string) { fmt.Println(color.YellowString("--"), msg) }
