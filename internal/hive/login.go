package hive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/pipeline"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/policy"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// The auth commands are the BOUNDARY: parse flags, wire adapters (wire.go),
// run a pipeline use case, render its result. No decision is made here.

var loginCmd = &bonzai.Cmd{
	Name:  "login",
	Short: "sign in to your hive account (opens the browser)",

	Long: `Sign in to your hive account.

  hive login               open the browser, sign in, come back here
  hive login --device      print a one-time code to approve on any device
                           (the default over ssh or with no display)
  hive login --with-token  read an existing hv_live_ artifact token on stdin
                           instead (CI; see hive store login)

What it does, once you have approved the sign-in:

  1. keeps the session (a refresh token) in your system keyring, or in
     ~/.config/hive/credentials.json (mode 0600) where there is none
  2. mints an artifact token for this machine, named after it, and writes it
     to ~/.m2/settings.xml, which is what makes paid addons resolve
  3. prints who you are signed in as and what your plan includes

Undo all of it with: hive logout

Environment: HIVE_AUTH_ISSUER, HIVE_AUTH_CLIENT_ID (a dev realm),
HIVE_CREDENTIAL_STORE=keyring|file (force a backend).`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		var method domain.Method
		withToken := false
		for _, a := range args {
			switch a {
			case "--device", "-d", "--no-browser":
				method = domain.MethodDevice
			case "--web", "-w":
				method = domain.MethodBrowser
			case "--with-token":
				withToken = true
			default:
				return fmt.Errorf("unknown option %q; see hive help login", a)
			}
		}
		if withToken {
			token, err := readToken(nil)
			if err != nil {
				return err
			}
			return login(token)
		}
		d, err := authDeps()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		res, err := pipeline.Login(ctx, d, method)
		if err != nil {
			return signInError(err)
		}
		renderLogin(res, d.Maven.Path())
		return nil
	},
}

func renderLogin(r pipeline.LoginResult, settings string) {
	ok := color.GreenString("✓")
	fmt.Println(ok, "Signed in as", color.CyanString(r.Account.Email))
	if r.Artifact == nil {
		fmt.Println(color.YellowString("--"), "No subscription on this account, so no artifact token was made.")
		fmt.Println("   The FOSS build needs none. To add paid addons:", store.BaseURL()+"/pricing, then hive login again.")
	} else {
		fmt.Println(ok, "Artifact token", r.Artifact.String(), "written to", settings)
		if r.SettingsBackup != "" {
			fmt.Println("  previous settings kept at", r.SettingsBackup)
		}
		fmt.Printf("%s Plan: %s, includes %d addon(s)\n", ok, r.Session.Plan, len(r.Account.Plan.Entitles))
	}
	fmt.Println(ok, "Session stored in", r.CredentialsStore)
	if r.RevokedPrevious {
		fmt.Println("  the previous login's artifact token was revoked")
	}
	_ = reportShadow(store.Inspect())
	fmt.Println()
	fmt.Println("Next: hive addon search, then hive addon add <id> for anything on your plan.")
}

// signInError words a failure as what to do next; the classification is policy's.
func signInError(err error) error {
	switch policy.Classify(err) {
	case domain.FailureUnknownClient:
		return fmt.Errorf("the sign-in server does not recognise this CLI (%v).\n"+
			"  Paste an artifact token from %s/dashboard instead: hive store login", err, store.BaseURL())
	case domain.FailureCancelled:
		return errors.New("sign-in cancelled")
	case domain.FailureTimedOut:
		return errors.New("sign-in timed out; run hive login again")
	case domain.FailureDeclined:
		return errors.New("the sign-in was declined in the browser")
	case domain.FailureExpired:
		return errors.New("the code expired before it was approved; run hive login again")
	}
	return fmt.Errorf("sign-in failed: %w", err)
}

var logoutCmd = &bonzai.Cmd{
	Name:  "logout",
	Short: "sign out and revoke this machine's tokens",

	Long: `Sign out of the hive account on this machine.

Revokes the artifact token hive login minted for this machine (other machines'
tokens are untouched), removes it from ~/.m2/settings.xml when it is still the
one there, ends the session at the sign-in server, and deletes the stored
credentials.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		d, err := authDeps()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := pipeline.Logout(ctx, d)
		if errors.Is(err, domain.ErrNotSignedIn) {
			fmt.Println("Not signed in.")
			return nil
		}
		ok := color.GreenString("✓")
		if res.Session != nil && res.Session.ArtifactTokenID != "" {
			if res.ArtifactRevoked {
				fmt.Println(ok, "Artifact token revoked")
			} else {
				fmt.Println(color.YellowString("--"), "could not revoke the artifact token:", res.RevokeError)
				fmt.Println("   revoke it at", store.BaseURL()+"/dashboard")
			}
		}
		if res.SettingsCleaned {
			fmt.Println(ok, "Removed it from", d.Maven.Path())
		}
		if err != nil {
			return err
		}
		fmt.Println(ok, "Signed out", res.Session.Email)
		return nil
	},
}

var authCmd = &bonzai.Cmd{
	Name:  "auth",
	Short: "who you are signed in as, and the session token",
	Long: `  hive auth status   who is signed in, where the session is kept, what is wired
  hive auth token    print a current access token for the store API (scripts)`,
	Cmds: []*bonzai.Cmd{authStatusCmd, authTokenCmd},
	Do:   func(x *bonzai.Cmd, args ...string) error { return showHelp(x) },
}

var authStatusCmd = &bonzai.Cmd{
	Name:  "status",
	Short: "who is signed in on this machine",

	Mcp: &bonzai.McpMeta{
		Desc: "Report whether this machine is signed in to the hive account: the email, plan, where the session is stored, whether it is still valid, and whether ~/.m2/settings.xml carries a working artifact token. Prints no secret.",
	},

	Do: func(x *bonzai.Cmd, args ...string) error {
		d, err := authDeps()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		st, err := pipeline.CurrentStatus(ctx, d)
		if errors.Is(err, domain.ErrNotSignedIn) {
			fmt.Println("Not signed in. Run: hive login")
			return reportWiring()
		}
		if err != nil {
			return err
		}
		mark, state := color.GreenString("✓"), "active"
		if st.Usable != nil {
			mark, state = color.RedString("✗"), st.Usable.Error()
		}
		s := st.Session
		fmt.Printf("%s Signed in as %s (%s)\n", mark, color.CyanString(s.Email), state)
		if s.Plan != "" {
			fmt.Println("  plan:     ", s.Plan)
		}
		fmt.Println("  since:    ", s.SignedIn.Format("2006-01-02 15:04"))
		fmt.Println("  stored in:", s.CredentialStore)
		fmt.Println("  issuer:   ", s.Realm.Issuer)
		fmt.Println()
		return reportWiring()
	},
}

var authTokenCmd = &bonzai.Cmd{
	Name:  "token",
	Short: "print a current store access token",
	Do: func(x *bonzai.Cmd, args ...string) error {
		d, err := authDeps()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		t, err := pipeline.AccessToken(ctx, d)
		if err != nil {
			return err
		}
		fmt.Println(t)
		return nil
	},
}
