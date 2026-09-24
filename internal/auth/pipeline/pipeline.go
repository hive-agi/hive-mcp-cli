// Package pipeline is the PIPELINE stratum: the sign-in use cases, composed
// from policy decisions and port calls. It performs no I/O of its own; every
// effect goes through a port, so a test hands it fakes (LSP) and the boundary
// hands it adapters.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/policy"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/port"
)

// Deps is every port the use cases reach through.
type Deps struct {
	IdP         port.IdentityProvider
	Callback    port.CallbackListener
	Browser     port.Browser
	Credentials port.CredentialStore
	Sessions    port.SessionRepo
	Accounts    port.Accounts
	Maven       port.MavenSettings
	Prompter    port.Prompter
	Clock       port.Clock
	Entropy     port.Entropy
	Machine     port.Machine
}

// Keys in the credential store. Only these two secrets are kept.
const (
	keyTokens   = "oidc-token"
	keyArtifact = "artifact-token"
)

// LoginResult is what a completed login tells the boundary to report.
type LoginResult struct {
	Session          domain.Session
	Account          domain.Account
	Artifact         *domain.ArtifactToken // nil: no plan, nothing minted
	SettingsBackup   string
	SessionOnly      bool // the account could not hold an offline session
	RevokedPrevious  bool
	CredentialsStore string
}

// Login signs in with method (empty: policy chooses), then mints this
// machine's artifact token when the account has a plan, and stores it all.
func Login(ctx context.Context, d Deps, method domain.Method) (LoginResult, error) {
	var res LoginResult
	flow, err := flowFor(policy.ChooseMethod(d.Machine.Environment(), method))
	if err != nil {
		return res, err
	}
	scopes := policy.DefaultScopes()
	tok, err := flow.SignIn(ctx, d, scopes)
	if policy.NeedsOfflineFallback(err) {
		d.Prompter.Note("this account cannot hold a long-lived session; signing in with a session-length one")
		res.SessionOnly = true
		tok, err = flow.SignIn(ctx, d, policy.WithoutOffline(scopes))
	}
	if err != nil {
		return res, err
	}

	acct, err := d.Accounts.Me(ctx, tok.Access)
	if err != nil {
		return res, err
	}
	res.Account = acct
	previous, _ := d.Sessions.Load()
	previousSecret, _ := d.Credentials.Get(keyArtifact)

	sess := domain.Session{
		Realm:    d.IdP.Realm(),
		Email:    acct.Email,
		Plan:     policy.PlanSummary(acct.Plan),
		SignedIn: d.Clock.Now(),
	}
	secret := ""
	if policy.ShouldMint(acct) {
		art, err := d.Accounts.Mint(ctx, tok.Access, policy.Label(d.Machine.Hostname(), d.Clock.Now()))
		if err != nil {
			return res, err
		}
		backup, err := d.Maven.Write(art.Secret)
		if err != nil {
			// Never leave a live token behind that nothing references.
			_ = d.Accounts.RevokeArtifact(ctx, tok.Access, art.ID)
			return res, err
		}
		res.Artifact, res.SettingsBackup = &art, backup
		sess.ArtifactTokenID, secret = art.ID, art.Secret
	}

	if err := saveSecrets(d.Credentials, tok, secret); err != nil {
		return res, err
	}
	sess.CredentialStore = d.Credentials.Kind()
	if err := d.Sessions.Save(sess); err != nil {
		return res, err
	}
	res.Session, res.CredentialsStore = sess, sess.CredentialStore

	if policy.Supersedes(previous, sess) {
		res.RevokedPrevious = d.Accounts.RevokeArtifact(ctx, tok.Access, previous.ArtifactTokenID) == nil
		if secret == "" && previousSecret != "" {
			_, _ = d.Maven.RemoveIfHolds(previousSecret)
		}
	}
	return res, nil
}

func saveSecrets(st port.CredentialStore, tok domain.Tokens, artifact string) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if err := st.Set(keyTokens, string(b)); err != nil {
		return err
	}
	if artifact != "" {
		return st.Set(keyArtifact, artifact)
	}
	return st.Delete(keyArtifact)
}

func loadTokens(st port.CredentialStore) (domain.Tokens, error) {
	var tok domain.Tokens
	raw, err := st.Get(keyTokens)
	if err != nil {
		return tok, err
	}
	return tok, json.Unmarshal([]byte(raw), &tok)
}

// AccessToken is a currently valid access token for the signed-in account,
// renewed from the stored refresh token when it has expired.
func AccessToken(ctx context.Context, d Deps) (string, error) {
	if _, err := d.Sessions.Load(); err != nil {
		return "", err
	}
	tok, err := loadTokens(d.Credentials)
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrNotSignedIn
	}
	if err != nil {
		return "", err
	}
	if tok.ValidAt(d.Clock.Now()) {
		return tok.Access, nil
	}
	if tok.Refresh == "" {
		return "", errors.New("session expired: run hive login")
	}
	fresh, err := d.IdP.Refresh(ctx, tok.Refresh)
	if err != nil {
		return "", errors.New("session expired or revoked (" + err.Error() + "): run hive login")
	}
	if fresh.Refresh == "" {
		fresh.Refresh = tok.Refresh // an issuer that does not rotate keeps the old one valid
	}
	b, _ := json.Marshal(fresh)
	_ = d.Credentials.Set(keyTokens, string(b))
	return fresh.Access, nil
}

// LogoutResult is what logout managed; each part is best effort, and the
// local state is cleared regardless.
type LogoutResult struct {
	Session         *domain.Session
	ArtifactRevoked bool
	RevokeError     error
	SettingsCleaned bool
}

// Logout revokes this machine's artifact token, removes it from the Maven
// settings when it is still the one there, ends the session at the issuer,
// and deletes every stored secret.
func Logout(ctx context.Context, d Deps) (LogoutResult, error) {
	var res LogoutResult
	sess, err := d.Sessions.Load()
	if err != nil {
		return res, err
	}
	res.Session = sess
	secret, _ := d.Credentials.Get(keyArtifact)
	if sess.ArtifactTokenID != "" {
		access, err := AccessToken(ctx, d)
		if err == nil {
			err = d.Accounts.RevokeArtifact(ctx, access, sess.ArtifactTokenID)
		}
		res.ArtifactRevoked, res.RevokeError = err == nil, err
	}
	if secret != "" {
		res.SettingsCleaned, _ = d.Maven.RemoveIfHolds(secret)
	}
	if tok, err := loadTokens(d.Credentials); err == nil && tok.Refresh != "" {
		_ = d.IdP.Revoke(ctx, tok.Refresh)
	}
	_ = d.Credentials.Delete(keyTokens)
	_ = d.Credentials.Delete(keyArtifact)
	return res, d.Sessions.Remove()
}

// Status is the signed-in session and whether it is still usable.
type Status struct {
	Session *domain.Session
	Usable  error // nil: a valid access token is available
}

func CurrentStatus(ctx context.Context, d Deps) (Status, error) {
	sess, err := d.Sessions.Load()
	if err != nil {
		return Status{}, err
	}
	_, usable := AccessToken(ctx, d)
	return Status{Session: sess, Usable: usable}, nil
}
