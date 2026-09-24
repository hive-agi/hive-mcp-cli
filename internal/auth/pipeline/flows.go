package pipeline

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/policy"
)

// Flow is one way of turning a user's approval into tokens. The set is OPEN:
// a new method (CIBA, a hardware key, ...) is a type implementing Flow plus a
// Register call, and nothing else in auth changes (OCP).
type Flow interface {
	Method() domain.Method
	SignIn(ctx context.Context, d Deps, scopes domain.Scopes) (domain.Tokens, error)
}

var (
	flowsMu sync.RWMutex
	flows   = map[domain.Method]Flow{}
)

// Register adds a flow, replacing any registered under the same method.
func Register(f Flow) {
	flowsMu.Lock()
	defer flowsMu.Unlock()
	flows[f.Method()] = f
}

func flowFor(m domain.Method) (Flow, error) {
	flowsMu.RLock()
	defer flowsMu.RUnlock()
	if f, ok := flows[m]; ok {
		return f, nil
	}
	known := make([]string, 0, len(flows))
	for k := range flows {
		known = append(known, string(k))
	}
	sort.Strings(known)
	return nil, fmt.Errorf("no sign-in method %q (have %v)", m, known)
}

func init() {
	Register(browserFlow{})
	Register(deviceFlow{})
}

// browserFlow: authorization code + PKCE on a loopback redirect (RFC 8252).
// PKCE binds the code to this process; state binds the redirect to this attempt.
type browserFlow struct{}

func (browserFlow) Method() domain.Method { return domain.MethodBrowser }

func (browserFlow) SignIn(ctx context.Context, d Deps, scopes domain.Scopes) (domain.Tokens, error) {
	ep, err := d.IdP.Endpoints(ctx)
	if err != nil {
		return domain.Tokens{}, err
	}
	redirect, err := d.Callback.Listen(ctx)
	if err != nil {
		return domain.Tokens{}, err
	}
	defer d.Callback.Close()
	pkce := policy.NewPKCE(d.Entropy.Token(32))
	state := d.Entropy.Token(16)
	u := policy.AuthorizationURL(ep, d.IdP.Realm(), redirect, scopes, state, pkce)
	d.Prompter.OpeningBrowser(u, d.Browser.Open(u) == nil)
	code, err := d.Callback.Await(ctx, state)
	if err != nil {
		return domain.Tokens{}, err
	}
	return d.IdP.ExchangeCode(ctx, code, redirect, pkce)
}

// deviceFlow: the device authorization grant (RFC 8628), PKCE-bound because
// Keycloak enforces a client's PKCE setting at the device endpoint too.
type deviceFlow struct{}

func (deviceFlow) Method() domain.Method { return domain.MethodDevice }

func (deviceFlow) SignIn(ctx context.Context, d Deps, scopes domain.Scopes) (domain.Tokens, error) {
	pkce := policy.NewPKCE(d.Entropy.Token(32))
	grant, err := d.IdP.StartDevice(ctx, scopes, pkce)
	if err != nil {
		return domain.Tokens{}, err
	}
	d.Prompter.ShowDeviceCode(grant)
	if policy.BrowserPlausible(d.Machine.Environment()) {
		target := grant.VerificationURIComplete
		if target == "" {
			target = grant.VerificationURI
		}
		_ = d.Browser.Open(target)
	}
	interval := grant.Interval
	deadline := d.Clock.Now().Add(grant.ExpiresIn)
	for {
		if !d.Clock.Now().Before(deadline) {
			return domain.Tokens{}, &domain.OAuthError{Code: "expired_token", Description: "the code expired before it was approved"}
		}
		if err := d.Clock.Sleep(ctx, interval); err != nil {
			return domain.Tokens{}, err
		}
		tok, err := d.IdP.PollDevice(ctx, grant, pkce)
		switch verdict, _ := policy.JudgePoll(err); verdict {
		case policy.PollDone:
			return tok, nil
		case policy.PollContinue:
		case policy.PollSlowDown:
			interval += policy.SlowDownStep
		default:
			return domain.Tokens{}, err
		}
	}
}
