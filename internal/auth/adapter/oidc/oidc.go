// Package oidc is a BOUNDARY adapter: port.IdentityProvider over an OIDC
// issuer's HTTP endpoints (Keycloak in production). One request per method;
// the loops and waits are the pipeline's.
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/policy"
)

// DefaultIssuer is the realm every hive surface signs in against.
const DefaultIssuer domain.Issuer = "https://auth.hive-mcp.com/realms/hive"

// DefaultClientID is the PUBLIC client the CLI signs in as: a binary on a
// customer's machine cannot keep a secret, so PKCE and device codes prove
// possession instead (k8s-agi terraform/keycloak-hive-realm/clients-cli.tf).
const DefaultClientID domain.ClientID = "hive-cli"

// RealmFromEnv is the default realm, overridable for a dev Keycloak.
func RealmFromEnv() domain.Realm {
	r := domain.Realm{Issuer: DefaultIssuer, ClientID: DefaultClientID}
	if v := strings.TrimSpace(os.Getenv("HIVE_AUTH_ISSUER")); v != "" {
		r.Issuer = domain.Issuer(strings.TrimRight(v, "/"))
	}
	if v := strings.TrimSpace(os.Getenv("HIVE_AUTH_CLIENT_ID")); v != "" {
		r.ClientID = domain.ClientID(v)
	}
	return r
}

// Provider implements port.IdentityProvider.
type Provider struct {
	realm domain.Realm
	http  *http.Client
	now   func() time.Time

	once sync.Once
	ep   discovery
	err  error
}

func New(realm domain.Realm) *Provider {
	return &Provider{realm: realm, http: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
}

func (p *Provider) Realm() domain.Realm { return p.realm }

type discovery struct {
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Device        string `json:"device_authorization_endpoint"`
	Revocation    string `json:"revocation_endpoint"`
}

func (p *Provider) Endpoints(ctx context.Context) (domain.Endpoints, error) {
	p.once.Do(func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, string(p.realm.Issuer)+"/.well-known/openid-configuration", nil)
		resp, err := p.http.Do(req)
		if err != nil {
			p.err = fmt.Errorf("reaching %s: %w", p.realm.Issuer, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			p.err = fmt.Errorf("discovery at %s answered %s", p.realm.Issuer, resp.Status)
			return
		}
		p.err = json.NewDecoder(resp.Body).Decode(&p.ep)
	})
	return domain.Endpoints{Authorization: p.ep.Authorization, Token: p.ep.Token, Device: p.ep.Device, Revocation: p.ep.Revocation}, p.err
}

type wireError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (p *Provider) post(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var we wireError
		if json.Unmarshal(body, &we) == nil && we.Code != "" {
			return &domain.OAuthError{Code: we.Code, Description: we.Description}
		}
		return fmt.Errorf("%s answered %s", endpoint, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (p *Provider) grant(ctx context.Context, form url.Values) (domain.Tokens, error) {
	ep, err := p.Endpoints(ctx)
	if err != nil {
		return domain.Tokens{}, err
	}
	form.Set("client_id", string(p.realm.ClientID))
	var tr tokenResponse
	if err := p.post(ctx, ep.Token, form, &tr); err != nil {
		return domain.Tokens{}, err
	}
	return domain.Tokens{
		Access: tr.AccessToken, Refresh: tr.RefreshToken, ID: tr.IDToken,
		ExpiresAt: policy.ExpiresAt(p.now(), time.Duration(tr.ExpiresIn)*time.Second),
	}, nil
}

func (p *Provider) StartDevice(ctx context.Context, scopes domain.Scopes, pkce domain.PKCE) (domain.DeviceGrant, error) {
	ep, err := p.Endpoints(ctx)
	if err != nil {
		return domain.DeviceGrant{}, err
	}
	if ep.Device == "" {
		return domain.DeviceGrant{}, errors.New("this issuer does not offer the device flow")
	}
	var dc struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	err = p.post(ctx, ep.Device, url.Values{
		"client_id":             {string(p.realm.ClientID)},
		"scope":                 {scopes.String()},
		"code_challenge":        {pkce.Challenge},
		"code_challenge_method": {"S256"},
	}, &dc)
	if err != nil {
		return domain.DeviceGrant{}, err
	}
	g := domain.DeviceGrant{
		DeviceCode: dc.DeviceCode, UserCode: dc.UserCode,
		VerificationURI: dc.VerificationURI, VerificationURIComplete: dc.VerificationURIComplete,
		ExpiresIn: time.Duration(dc.ExpiresIn) * time.Second, Interval: time.Duration(dc.Interval) * time.Second,
	}
	if g.Interval <= 0 {
		g.Interval = 5 * time.Second // RFC 8628 3.2 default
	}
	if g.ExpiresIn <= 0 {
		g.ExpiresIn = 10 * time.Minute
	}
	return g, nil
}

func (p *Provider) PollDevice(ctx context.Context, g domain.DeviceGrant, pkce domain.PKCE) (domain.Tokens, error) {
	return p.grant(ctx, url.Values{
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code":   {g.DeviceCode},
		"code_verifier": {pkce.Verifier},
	})
}

func (p *Provider) ExchangeCode(ctx context.Context, code, redirect string, pkce domain.PKCE) (domain.Tokens, error) {
	return p.grant(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"code_verifier": {pkce.Verifier},
	})
}

func (p *Provider) Refresh(ctx context.Context, refresh string) (domain.Tokens, error) {
	return p.grant(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

// Revoke ends a refresh token's session (RFC 7009).
func (p *Provider) Revoke(ctx context.Context, refresh string) error {
	ep, err := p.Endpoints(ctx)
	if err != nil || ep.Revocation == "" {
		return err
	}
	return p.post(ctx, ep.Revocation, url.Values{
		"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {string(p.realm.ClientID)},
	}, nil)
}
