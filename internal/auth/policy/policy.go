// Package policy is the PROMOTE stratum: every decision sign-in makes, as a
// pure function of domain values. No I/O, no clock (now is an argument), no
// randomness (verifiers are arguments), so each rule is testable with no fake.
package policy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

// DefaultScopes asks for a refresh token that outlives the browser session
// (offline_access), as gh does. profile and email are the client's DEFAULT
// scopes and arrive unasked; naming them would couple the CLI to how a realm
// spells its scopes, and an unknown scope fails the whole request.
func DefaultScopes() domain.Scopes { return domain.Scopes{domain.ScopeOpenID, domain.ScopeOffline} }

// WithoutOffline is the fallback scope set for an account whose role does not
// grant offline_access.
func WithoutOffline(s domain.Scopes) domain.Scopes {
	out := make(domain.Scopes, 0, len(s))
	for _, v := range s {
		if v != domain.ScopeOffline {
			out = append(out, v)
		}
	}
	return out
}

// NeedsOfflineFallback recognises Keycloak refusing an offline token to an
// account, which is worth one retry without it rather than a failed login.
func NeedsOfflineFallback(err error) bool {
	var oe *domain.OAuthError
	return errors.As(err, &oe) && oe.Code == "not_allowed" && strings.Contains(oe.Description, "Offline tokens")
}

// ChooseMethod picks the sign-in flow. An explicit choice wins; otherwise the
// browser flow where a browser on THIS machine is plausible, and the device
// flow over ssh or with no display (a browser there would open on a machine
// the user is not looking at, or nowhere).
func ChooseMethod(env domain.Environment, forced domain.Method) domain.Method {
	if forced != "" {
		return forced
	}
	if BrowserPlausible(env) {
		return domain.MethodBrowser
	}
	return domain.MethodDevice
}

// BrowserPlausible reports whether opening a URL here reaches the user.
func BrowserPlausible(env domain.Environment) bool {
	switch {
	case env.OverSSH:
		return false
	case env.BrowserOverride, env.DesktopOS:
		return true
	default:
		return env.HasDisplay && env.HasOpener
	}
}

// Challenge is the S256 PKCE challenge of verifier (RFC 7636 section 4.2).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewPKCE pairs a verifier with its challenge.
func NewPKCE(verifier string) domain.PKCE {
	return domain.PKCE{Verifier: verifier, Challenge: Challenge(verifier)}
}

// AuthorizationURL is the browser flow's sign-in page for one attempt.
func AuthorizationURL(ep domain.Endpoints, realm domain.Realm, redirect string, scopes domain.Scopes, state string, pkce domain.PKCE) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {string(realm.ClientID)},
		"redirect_uri":          {redirect},
		"scope":                 {scopes.String()},
		"state":                 {state},
		"code_challenge":        {pkce.Challenge},
		"code_challenge_method": {"S256"},
	}
	return ep.Authorization + "?" + q.Encode()
}

// ExpiresAt turns a relative lifetime into a local deadline with a margin.
func ExpiresAt(now time.Time, lifetime time.Duration) time.Time {
	if lifetime > time.Minute {
		lifetime -= 30 * time.Second
	}
	return now.Add(lifetime)
}

// PollVerdict is what one device-flow poll answer means.
type PollVerdict int

const (
	PollDone     PollVerdict = iota // tokens arrived
	PollContinue                    // not approved yet
	PollSlowDown                    // the issuer asks for a longer interval
	PollStop                        // a final answer; the error says which
)

// JudgePoll maps one poll outcome to what the loop does next (RFC 8628 3.5).
func JudgePoll(err error) (PollVerdict, domain.Failure) {
	if err == nil {
		return PollDone, domain.FailureOther
	}
	var oe *domain.OAuthError
	if !errors.As(err, &oe) {
		return PollStop, domain.FailureOther
	}
	switch oe.Code {
	case "authorization_pending":
		return PollContinue, domain.FailureOther
	case "slow_down":
		return PollSlowDown, domain.FailureOther
	case "access_denied":
		return PollStop, domain.FailureDeclined
	case "expired_token":
		return PollStop, domain.FailureExpired
	}
	return PollStop, domain.FailureOther
}

// SlowDownStep is how much a slow_down lengthens the interval (RFC 8628 3.5).
const SlowDownStep = 5 * time.Second

// Classify says why a sign-in failed, so the boundary can say what to do.
func Classify(err error) domain.Failure {
	var oe *domain.OAuthError
	switch {
	case errors.As(err, &oe) && (oe.Code == "invalid_client" || oe.Code == "unauthorized_client"):
		return domain.FailureUnknownClient
	case errors.As(err, &oe) && oe.Code == "access_denied":
		return domain.FailureDeclined
	case errors.As(err, &oe) && oe.Code == "expired_token":
		return domain.FailureExpired
	case errors.Is(err, context.Canceled):
		return domain.FailureCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return domain.FailureTimedOut
	}
	return domain.FailureOther
}

// Label names this machine's artifact token for the dashboard.
func Label(host string, now time.Time) domain.MachineLabel {
	if host == "" {
		host = "unknown-host"
	}
	return domain.MachineLabel(fmt.Sprintf("hive-cli %s %s", host, now.Format("2006-01-02")))
}

// ShouldMint: an artifact token is only worth minting for an account that has
// a plan; without one it would resolve nothing.
func ShouldMint(a domain.Account) bool { return a.Plan != nil }

// PlanSummary is how a plan is recorded in the session.
func PlanSummary(p *domain.Plan) string {
	if p == nil {
		return ""
	}
	return p.Name + " (" + p.Status + ")"
}

// Supersedes reports whether a previous session's machine token should be
// revoked now that the new session exists: a re-login must never leave
// orphaned tokens behind.
func Supersedes(previous *domain.Session, current domain.Session) bool {
	return previous != nil && previous.ArtifactTokenID != "" && previous.ArtifactTokenID != current.ArtifactTokenID
}
