// Package domain is the CORE stratum of sign-in: value objects and nothing
// else. No I/O, no clock, no randomness; every other auth package speaks in
// these words. The strata gate (auth/strata_test.go) holds it to the standard
// library's pure corner.
package domain

import (
	"errors"
	"strings"
	"time"
)

// Issuer is an OIDC issuer URL, the realm's identity.
type Issuer string

// ClientID is the public OIDC client the CLI signs in as.
type ClientID string

// Realm is WHERE a sign-in happens: one issuer, one client.
type Realm struct {
	Issuer   Issuer
	ClientID ClientID
}

// Scope is one OAuth scope token.
type Scope string

const (
	ScopeOpenID  Scope = "openid"
	ScopeOffline Scope = "offline_access"
)

// Scopes is an ordered scope request.
type Scopes []Scope

func (s Scopes) String() string {
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = string(v)
	}
	return strings.Join(parts, " ")
}

// Endpoints is the part of an issuer's discovery document sign-in uses.
type Endpoints struct {
	Authorization string
	Token         string
	Device        string
	Revocation    string
}

// Tokens is what a sign-in yields. ExpiresAt is a local deadline, already
// shortened by a safety margin, so a token is renewed before the issuer would
// call it expired.
type Tokens struct {
	Access    string    `json:"access_token"`
	Refresh   string    `json:"refresh_token,omitempty"`
	ID        string    `json:"id_token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ValidAt reports whether the access token may still be presented at now.
func (t Tokens) ValidAt(now time.Time) bool { return t.Access != "" && now.Before(t.ExpiresAt) }

// PKCE is one proof-key pair (RFC 7636). The verifier stays in this process;
// only the challenge travels to the browser.
type PKCE struct {
	Verifier  string
	Challenge string
}

// DeviceGrant is what the user is shown in the device flow (RFC 8628).
type DeviceGrant struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

// Method names a way of signing in. An OPEN set: each is a registered flow
// (pipeline.Register), so a new one is a registration, not an edit.
type Method string

const (
	MethodBrowser Method = "browser" // authorization code + PKCE on a loopback redirect
	MethodDevice  Method = "device"  // device authorization grant
)

// Environment is what decides the default Method: facts about this machine,
// gathered at the boundary, judged by policy.
type Environment struct {
	OverSSH         bool
	HasDisplay      bool
	BrowserOverride bool // $BROWSER is set
	DesktopOS       bool // macOS or Windows: a browser is assumed
	HasOpener       bool // xdg-open or equivalent exists
}

// Plan is a subscription as the store resolves it at the moment of asking.
type Plan struct {
	Name     string
	Status   string
	Entitles []string
}

// Account is who the store says is signed in.
type Account struct {
	Email string
	Plan  *Plan // nil: no subscription
}

// ArtifactToken is a store-minted hv_live_ token. Secret is shown once by the
// store; String never prints it.
type ArtifactToken struct {
	ID     string
	Prefix string
	Secret string
}

func (a ArtifactToken) String() string { return a.Prefix + "…" }

// MachineLabel names the artifact token a machine mints, so the dashboard
// shows which machine holds which token.
type MachineLabel string

// Session is what a completed sign-in leaves behind, minus the secrets (those
// live in the credential store).
type Session struct {
	Realm           Realm     `json:"realm"`
	Email           string    `json:"email,omitempty"`
	Plan            string    `json:"plan,omitempty"`
	SignedIn        time.Time `json:"signed_in"`
	ArtifactTokenID string    `json:"artifact_token_id,omitempty"`
	CredentialStore string    `json:"credential_store"`
}

// OAuthError is RFC 6749 section 5.2. Its Code drives decisions, so it is a
// value, never a string match on a message.
type OAuthError struct {
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}

// ErrNotSignedIn is the absence of a session: the answer is `hive login`.
var ErrNotSignedIn = errors.New("not signed in: run hive login")

// ErrNotFound is a credential that was never stored, or was removed.
var ErrNotFound = errors.New("credential not found")

// Failure classifies why a sign-in did not complete, for the boundary to word.
type Failure int

const (
	FailureOther         Failure = iota
	FailureUnknownClient         // the realm has no hive-cli client
	FailureCancelled
	FailureTimedOut
	FailureDeclined
	FailureExpired
)
