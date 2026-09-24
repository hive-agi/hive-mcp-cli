package hive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/BuddhiLW/bonzai"
	"github.com/fatih/color"
	"github.com/hive-agi/hive-mcp-cli/internal/auth"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// loginCmd signs the CLI in to the hive account, as `gh auth login` does:
// a browser round-trip where there is a browser, a one-time code where there
// is not, and never a password in the terminal.
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
HIVE_CREDENTIAL_STORE=file|keyring (force a backend).`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		device, withToken := false, false
		for _, a := range args {
			switch a {
			case "--device", "-d", "--no-browser":
				device = true
			case "--with-token":
				withToken = true
			case "--web", "-w":
				// the default where a browser exists; accepted for gh muscle memory
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
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		return browserLogin(ctx, device || !auth.CanOpenBrowser())
	},
}

func browserLogin(ctx context.Context, device bool) error {
	c := auth.NewClient()
	signIn := func() (*auth.Token, error) {
		if device {
			return deviceSignIn(ctx, c)
		}
		return loopbackSignIn(ctx, c)
	}
	tok, err := signIn()
	if err != nil && strings.Contains(err.Error(), "Offline tokens not allowed") {
		// The account's role lacks offline_access. Sign in again without it:
		// the session then lasts as long as the realm's SSO session instead of
		// until logout, and the browser's own session makes the retry instant.
		fmt.Println(color.YellowString("--"), "this account cannot hold a long-lived session; signing in with a session-length one")
		c.Scopes = auth.WithoutOffline(c.Scopes)
		tok, err = signIn()
	}
	if err != nil {
		return signInError(err)
	}

	client := store.New()
	client.Token = tok.AccessToken
	me, err := client.Me(ctx)
	if err != nil {
		return fmt.Errorf("signed in, but the store did not accept the session: %w", err)
	}
	sess := &auth.Session{Issuer: c.Issuer, ClientID: c.ClientID, Email: me.Email, SignedIn: time.Now()}
	if me.Subscription != nil {
		sess.Plan = me.Subscription.Plan + " (" + me.Subscription.Status + ")"
	}
	fmt.Println(color.GreenString("✓"), "Signed in as", color.CyanString(me.Email))

	st, err := auth.OpenStore()
	if err != nil {
		return err
	}
	// A second login replaces the first; its machine token is revoked once the
	// new one exists, so re-running login never leaves orphans behind.
	previous, _ := auth.LoadSession()
	previousToken, _ := auth.ArtifactToken(st)

	artifact := ""
	if me.Subscription == nil {
		fmt.Println(color.YellowString("--"), "No subscription on this account, so no artifact token was made.")
		fmt.Println("   The FOSS build needs none. To add paid addons:", store.BaseURL()+"/pricing, then hive login again.")
	} else {
		host, _ := os.Hostname()
		minted, err := client.MintToken(ctx, fmt.Sprintf("hive-cli %s %s", host, time.Now().Format("2006-01-02")))
		if err != nil {
			return fmt.Errorf("signed in, but minting this machine's artifact token failed: %w", err)
		}
		artifact, sess.ArtifactTokenID = minted.Secret, minted.ID
		backup, err := store.WriteSettings(store.SettingsPath(), store.RepoID, artifact)
		if err != nil {
			_ = client.RevokeToken(ctx, minted.ID)
			return fmt.Errorf("could not write %s (the new token was revoked): %w", store.SettingsPath(), err)
		}
		fmt.Println(color.GreenString("✓"), "Artifact token", minted.Prefix+"…", "written to", store.SettingsPath())
		if backup != "" {
			fmt.Println("  previous settings kept at", backup)
		}
		fmt.Printf("%s Plan: %s, includes %d addon(s)\n", color.GreenString("✓"), sess.Plan, len(me.Subscription.Entitles))
	}

	if err := auth.Save(st, sess, tok, artifact); err != nil {
		return fmt.Errorf("could not store the session: %w", err)
	}
	fmt.Println(color.GreenString("✓"), "Session stored in", st.Kind())

	if previous != nil && previous.ArtifactTokenID != "" && previous.ArtifactTokenID != sess.ArtifactTokenID {
		if err := client.RevokeToken(ctx, previous.ArtifactTokenID); err == nil {
			fmt.Println("  the previous login's artifact token was revoked")
		}
		if previousToken != "" && artifact == "" {
			_, _ = store.RemoveServerIfToken(store.SettingsPath(), store.RepoID, previousToken)
		}
	}
	_ = reportShadow(store.Inspect())
	fmt.Println()
	fmt.Println("Next: hive addon search, then hive addon add <id> for anything on your plan.")
	return nil
}

func loopbackSignIn(ctx context.Context, c *auth.Client) (*auth.Token, error) {
	lb, err := c.StartLoopback(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Println("Opening your browser to sign in to hive...")
	if err := auth.OpenBrowser(lb.URL); err != nil {
		fmt.Println(color.YellowString("--"), "could not open a browser. Open this URL yourself:")
	} else {
		fmt.Println("If it did not open, use this URL:")
	}
	fmt.Println(" ", lb.URL)
	fmt.Println(color.HiBlackString("Waiting for the browser... (Ctrl-C to cancel; hive login --device if this machine has no browser)"))
	return lb.Wait(ctx)
}

func deviceSignIn(ctx context.Context, c *auth.Client) (*auth.Token, error) {
	dc, err := c.StartDevice(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	fmt.Println("  First copy your one-time code:", color.New(color.Bold).Sprint(dc.UserCode))
	fmt.Println("  Then open:                    ", dc.VerificationURI)
	if dc.VerificationURIComplete != "" {
		fmt.Println("  (or, with the code filled in:", dc.VerificationURIComplete+")")
	}
	fmt.Println()
	if auth.CanOpenBrowser() {
		target := dc.VerificationURIComplete
		if target == "" {
			target = dc.VerificationURI
		}
		_ = auth.OpenBrowser(target)
	}
	fmt.Println(color.HiBlackString("Waiting for you to approve the code... (Ctrl-C to cancel)"))
	return c.PollDevice(ctx, dc)
}

// signInError turns the realm's refusal into what the user should do. The one
// that matters during rollout: the realm does not know the CLI's client yet.
func signInError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unauthorized_client"), strings.Contains(msg, "invalid_client"):
		return fmt.Errorf("the sign-in server does not recognise this CLI (%s).\n"+
			"  Paste an artifact token from %s/dashboard instead: hive store login", msg, store.BaseURL())
	case errors.Is(err, context.Canceled):
		return errors.New("sign-in cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("sign-in timed out; run hive login again")
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sess, err := auth.LoadSession()
		if errors.Is(err, auth.ErrNotFound) {
			fmt.Println("Not signed in.")
			return nil
		}
		if err != nil {
			return err
		}
		st, err := auth.OpenStore()
		if err != nil {
			return err
		}
		artifact, _ := auth.ArtifactToken(st)
		if sess.ArtifactTokenID != "" {
			client := store.New() // resolves the session's access token, refreshing it
			if err := client.RevokeToken(ctx, sess.ArtifactTokenID); err != nil {
				fmt.Println(color.YellowString("--"), "could not revoke the artifact token:", err)
				fmt.Println("   revoke it at", store.BaseURL()+"/dashboard")
			} else {
				fmt.Println(color.GreenString("✓"), "Artifact token revoked")
			}
		}
		if artifact != "" {
			if removed, _ := store.RemoveServerIfToken(store.SettingsPath(), store.RepoID, artifact); removed {
				fmt.Println(color.GreenString("✓"), "Removed it from", store.SettingsPath())
			}
		}
		if _, err := auth.Clear(ctx); err != nil {
			return err
		}
		fmt.Println(color.GreenString("✓"), "Signed out", sess.Email)
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
		sess, err := auth.LoadSession()
		if errors.Is(err, auth.ErrNotFound) {
			fmt.Println("Not signed in. Run: hive login")
			return reportWiring()
		}
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, tokErr := auth.AccessToken(ctx)
		mark := color.GreenString("✓")
		state := "active"
		if tokErr != nil {
			mark, state = color.RedString("✗"), tokErr.Error()
		}
		fmt.Printf("%s Signed in as %s (%s)\n", mark, color.CyanString(sess.Email), state)
		if sess.Plan != "" {
			fmt.Println("  plan:     ", sess.Plan)
		}
		fmt.Println("  since:    ", sess.SignedIn.Format("2006-01-02 15:04"))
		fmt.Println("  stored in:", sess.Store)
		fmt.Println("  issuer:   ", sess.Issuer)
		fmt.Println()
		return reportWiring()
	},
}

var authTokenCmd = &bonzai.Cmd{
	Name:  "token",
	Short: "print a current store access token",
	Do: func(x *bonzai.Cmd, args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		t, err := auth.AccessToken(ctx)
		if errors.Is(err, auth.ErrNotFound) {
			return errors.New("not signed in: run hive login")
		}
		if err != nil {
			return err
		}
		fmt.Println(t)
		return nil
	},
}
