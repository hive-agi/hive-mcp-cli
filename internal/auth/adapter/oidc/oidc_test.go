package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/policy"
)

func issuer(t *testing.T, token func(w http.ResponseWriter, r *http.Request)) *Provider {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": srv.URL + "/auth", "token_endpoint": srv.URL + "/token",
			"device_authorization_endpoint": srv.URL + "/device", "revocation_endpoint": srv.URL + "/revoke",
		})
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code_challenge_method") != "S256" || r.Form.Get("code_challenge") == "" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request", "error_description": "Missing parameter: code_challenge_method"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"device_code": "d", "user_code": "ABCD-EFGH", "verification_uri": srv.URL + "/device"})
	})
	if token != nil {
		mux.HandleFunc("/token", token)
	}
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {})
	p := New(domain.Realm{Issuer: domain.Issuer(srv.URL), ClientID: "hive-cli"})
	p.now = func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) }
	return p
}

func TestDeviceStartSendsPKCEAndDefaultsTheInterval(t *testing.T) {
	p := issuer(t, nil)
	g, err := p.StartDevice(context.Background(), policy.DefaultScopes(), policy.NewPKCE("v"))
	if err != nil {
		t.Fatal(err)
	}
	if g.Interval != 5*time.Second || g.ExpiresIn != 10*time.Minute || g.UserCode != "ABCD-EFGH" {
		t.Errorf("grant %+v", g)
	}
}

func TestOAuthErrorsBecomeDomainValues(t *testing.T) {
	p := issuer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	})
	_, err := p.PollDevice(context.Background(), domain.DeviceGrant{DeviceCode: "d"}, policy.NewPKCE("v"))
	var oe *domain.OAuthError
	if !errors.As(err, &oe) || oe.Code != "authorization_pending" {
		t.Fatalf("got %v", err)
	}
}

func TestExchangeSendsTheVerifierAndStampsExpiry(t *testing.T) {
	p := issuer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code_verifier") != "v" || r.Form.Get("client_id") != "hive-cli" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "refresh_token": "r", "expires_in": 300})
	})
	tok, err := p.ExchangeCode(context.Background(), "c", "http://127.0.0.1:1/callback", policy.NewPKCE("v"))
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "a" || !tok.ExpiresAt.Equal(p.now().Add(4*time.Minute+30*time.Second)) {
		t.Errorf("%+v", tok)
	}
}
