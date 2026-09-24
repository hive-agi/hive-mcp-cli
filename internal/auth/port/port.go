// Package port is the PROTOCOL stratum: the seams sign-in reaches the world
// through. Each is small (ISP) and says WHAT, never HOW; adapters under
// auth/adapter implement them, and any implementation substitutes for any
// other (LSP), which is how the pipeline is tested with no network at all.
package port

import (
	"context"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

// IdentityProvider is the OIDC issuer: one request per method, no loops and
// no waiting, so every retry and wait policy lives in the pipeline.
type IdentityProvider interface {
	Realm() domain.Realm
	Endpoints(ctx context.Context) (domain.Endpoints, error)
	StartDevice(ctx context.Context, scopes domain.Scopes, pkce domain.PKCE) (domain.DeviceGrant, error)
	PollDevice(ctx context.Context, grant domain.DeviceGrant, pkce domain.PKCE) (domain.Tokens, error)
	ExchangeCode(ctx context.Context, code, redirect string, pkce domain.PKCE) (domain.Tokens, error)
	Refresh(ctx context.Context, refresh string) (domain.Tokens, error)
	Revoke(ctx context.Context, refresh string) error
}

// CallbackListener receives the browser flow's redirect on this machine.
type CallbackListener interface {
	// Listen binds and returns the redirect URI to register with the attempt.
	Listen(ctx context.Context) (redirect string, err error)
	// Await blocks for the redirect carrying state, and returns its code.
	Await(ctx context.Context, state string) (code string, err error)
	Close() error
}

// Browser opens a URL for the user.
type Browser interface {
	Open(url string) error
}

// CredentialStore keeps secrets by key.
type CredentialStore interface {
	Get(key string) (string, error) // domain.ErrNotFound when absent
	Set(key, value string) error
	Delete(key string) error
	Kind() string // said to the user, e.g. "system keyring"
}

// SessionRepo keeps the non-secret half of a session.
type SessionRepo interface {
	Load() (*domain.Session, error) // domain.ErrNotSignedIn when absent
	Save(domain.Session) error
	Remove() error
}

// Accounts is the store API, authenticated with an access token.
type Accounts interface {
	Me(ctx context.Context, access string) (domain.Account, error)
	Mint(ctx context.Context, access string, label domain.MachineLabel) (domain.ArtifactToken, error)
	RevokeArtifact(ctx context.Context, access, id string) error
}

// MavenSettings is where the artifact token must live for builds to resolve.
type MavenSettings interface {
	Write(secret string) (backup string, err error)
	RemoveIfHolds(secret string) (removed bool, err error)
	Path() string
}

// Prompter is how the pipeline speaks to the user mid-flow. It only reports;
// it never decides.
type Prompter interface {
	OpeningBrowser(url string, opened bool)
	ShowDeviceCode(grant domain.DeviceGrant)
	Note(msg string)
}

// Clock is time as an effect: now, and waiting.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

// Entropy makes unguessable URL-safe strings (PKCE verifiers, OAuth state).
type Entropy interface {
	Token(bytes int) string
}

// Machine is facts about where the CLI runs.
type Machine interface {
	Environment() domain.Environment
	Hostname() string
}
