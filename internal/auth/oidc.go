package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultIssuer is the realm every hive surface signs in against.
// HIVE_AUTH_ISSUER points the CLI at another one (a dev Keycloak).
const DefaultIssuer = "https://auth.hive-mcp.com/realms/hive"

// DefaultClientID is the PUBLIC OIDC client the CLI signs in as. Public: a
// binary on a customer's machine cannot keep a client secret, so the flows
// below prove possession with PKCE and device codes instead.
const DefaultClientID = "hive-cli"

func Issuer() string {
	if v := strings.TrimSpace(os.Getenv("HIVE_AUTH_ISSUER")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultIssuer
}

func ClientID() string {
	if v := strings.TrimSpace(os.Getenv("HIVE_AUTH_CLIENT_ID")); v != "" {
		return v
	}
	return DefaultClientID
}

// Endpoints is the part of the discovery document the CLI uses.
type Endpoints struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Device        string `json:"device_authorization_endpoint"`
	Revocation    string `json:"revocation_endpoint"`
	UserInfo      string `json:"userinfo_endpoint"`
}

// Token is what a sign-in yields. ExpiresAt is computed locally from
// expires_in, with a margin, so a token is refreshed before the server
// would call it expired.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.ExpiresAt)
}

// Client talks OIDC to one issuer as one client id.
type Client struct {
	Issuer   string
	ClientID string
	Scopes   []string
	HTTP     *http.Client
	ep       *Endpoints
}

func NewClient() *Client {
	return &Client{
		Issuer:   Issuer(),
		ClientID: ClientID(),
		// offline_access: a refresh token that survives the browser session,
		// so a CLI signed in once stays signed in, as gh does. profile and
		// email are the client's DEFAULT scopes and arrive without being
		// asked for; naming them would couple the CLI to how a realm spells
		// its scopes, and an unknown one fails the whole request.
		Scopes: []string{"openid", "offline_access"},
		HTTP:   &http.Client{Timeout: 30 * time.Second},
	}
}

// WithoutOffline drops offline_access, for an account whose role does not grant it.
func WithoutOffline(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if s != "offline_access" {
			out = append(out, s)
		}
	}
	return out
}

func (c *Client) Discover(ctx context.Context) (*Endpoints, error) {
	if c.ep != nil {
		return c.ep, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.Issuer+"/.well-known/openid-configuration", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reaching %s: %w", c.Issuer, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery at %s answered %s", c.Issuer, resp.Status)
	}
	var ep Endpoints
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, fmt.Errorf("discovery document: %w", err)
	}
	c.ep = &ep
	return c.ep, nil
}

// oauthError is RFC 6749 section 5.2. Its code drives the device-flow loop,
// so it is decoded, not stringified.
type oauthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *oauthError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var oe oauthError
		if json.Unmarshal(body, &oe) == nil && oe.Code != "" {
			return &oe
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

func (r tokenResponse) token() *Token {
	exp := time.Duration(r.ExpiresIn) * time.Second
	if exp > time.Minute {
		exp -= 30 * time.Second
	}
	return &Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, IDToken: r.IDToken, ExpiresAt: time.Now().Add(exp)}
}

// ---- device authorization grant (RFC 8628) ------------------------------
//
// The flow for a machine with no browser: the CLI shows a short code and a
// URL, the user opens it on ANY device, types the code, signs in (with
// whatever second factor the realm demands), and the CLI, polling, is let go.

// DeviceCode is what the user is shown.
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	// PKCE on the device grant too: Keycloak enforces a client's PKCE setting
	// at the device endpoint ("Missing parameter: code_challenge_method"), and
	// the verifier then has to come with every poll.
	verifier string
}

func (c *Client) StartDevice(ctx context.Context) (*DeviceCode, error) {
	ep, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if ep.Device == "" {
		return nil, errors.New("this issuer does not offer the device flow")
	}
	var dc DeviceCode
	verifier := randomURLSafe(32)
	err = c.postForm(ctx, ep.Device, url.Values{
		"client_id":             {c.ClientID},
		"scope":                 {strings.Join(c.Scopes, " ")},
		"code_challenge":        {challengeOf(verifier)},
		"code_challenge_method": {"S256"},
	}, &dc)
	if err != nil {
		return nil, err
	}
	dc.verifier = verifier
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// PollDevice waits for the user to approve dc, honouring the server's
// interval and slow_down, and gives up when the code expires.
func (c *Client) PollDevice(ctx context.Context, dc *DeviceCode) (*Token, error) {
	ep, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	interval := time.Duration(dc.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn <= 0 {
		deadline = time.Now().Add(10 * time.Minute)
	}
	for {
		if time.Now().After(deadline) {
			return nil, errors.New("the code expired before it was approved; run the login again")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		var tr tokenResponse
		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {dc.DeviceCode},
			"client_id":   {c.ClientID},
		}
		if dc.verifier != "" {
			form.Set("code_verifier", dc.verifier)
		}
		err := c.postForm(ctx, ep.Token, form, &tr)
		var oe *oauthError
		switch {
		case err == nil:
			return tr.token(), nil
		case errors.As(err, &oe) && oe.Code == "authorization_pending":
			continue
		case errors.As(err, &oe) && oe.Code == "slow_down":
			interval += 5 * time.Second
		case errors.As(err, &oe) && oe.Code == "access_denied":
			return nil, errors.New("the sign-in was declined in the browser")
		case errors.As(err, &oe) && oe.Code == "expired_token":
			return nil, errors.New("the code expired before it was approved; run the login again")
		default:
			return nil, err
		}
	}
}

// ---- authorization code + PKCE on a loopback redirect (RFC 8252) --------
//
// The flow for a machine with a browser: the CLI listens on 127.0.0.1 on a
// port the OS picks, opens the browser at the sign-in page, and the realm
// redirects back to that port with a one-time code. PKCE binds the code to
// this process, and state binds the redirect to this attempt.

func randomURLSafe(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// challengeOf is the PKCE S256 challenge for verifier (RFC 7636 section 4.2).
func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Loopback is one pending browser sign-in.
type Loopback struct {
	URL      string // open this in the browser
	c        *Client
	ln       net.Listener
	redirect string
	verifier string
	state    string
	result   chan loopbackResult
}

type loopbackResult struct {
	code string
	err  error
}

func (c *Client) StartLoopback(ctx context.Context) (*Loopback, error) {
	ep, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	// 127.0.0.1 literally, not localhost: RFC 8252 section 8.3, and it keeps
	// the listener off every other interface.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	lb := &Loopback{
		c:        c,
		ln:       ln,
		redirect: fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port),
		verifier: randomURLSafe(32),
		state:    randomURLSafe(16),
		result:   make(chan loopbackResult, 1),
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.ClientID},
		"redirect_uri":          {lb.redirect},
		"scope":                 {strings.Join(c.Scopes, " ")},
		"state":                 {lb.state},
		"code_challenge":        {challengeOf(lb.verifier)},
		"code_challenge_method": {"S256"},
	}
	lb.URL = ep.Authorization + "?" + q.Encode()

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", lb.handle)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return lb, nil
}

const pageDone = `<!doctype html><meta charset="utf-8"><title>hive</title>
<body style="font-family:system-ui;max-width:32rem;margin:4rem auto;text-align:center">
<h2>%s</h2><p>%s</p><p style="color:#888">You can close this tab and go back to the terminal.</p></body>`

func (lb *Loopback) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	send := func(res loopbackResult) {
		select {
		case lb.result <- res:
		default: // a second hit (a reload) must not block or overwrite the first
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if q.Get("state") != lb.state {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, pageDone, "Sign-in not accepted", "This response does not belong to the login this terminal started.")
		return
	}
	if e := q.Get("error"); e != "" {
		fmt.Fprintf(w, pageDone, "Sign-in cancelled", html.EscapeString(q.Get("error_description")))
		send(loopbackResult{err: &oauthError{Code: e, Description: q.Get("error_description")}})
		return
	}
	fmt.Fprintf(w, pageDone, "Signed in to hive", "The terminal has what it needs.")
	send(loopbackResult{code: q.Get("code")})
}

// Wait blocks until the browser comes back, then trades the code for tokens.
func (lb *Loopback) Wait(ctx context.Context) (*Token, error) {
	defer lb.ln.Close()
	var res loopbackResult
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-lb.result:
	}
	if res.err != nil {
		return nil, res.err
	}
	ep, err := lb.c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	var tr tokenResponse
	err = lb.c.postForm(ctx, ep.Token, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {res.code},
		"redirect_uri":  {lb.redirect},
		"client_id":     {lb.c.ClientID},
		"code_verifier": {lb.verifier},
	}, &tr)
	if err != nil {
		return nil, err
	}
	return tr.token(), nil
}

// Refresh trades a refresh token for a fresh access token.
func (c *Client) Refresh(ctx context.Context, refresh string) (*Token, error) {
	ep, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	var tr tokenResponse
	if err := c.postForm(ctx, ep.Token, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {c.ClientID},
	}, &tr); err != nil {
		return nil, err
	}
	t := tr.token()
	if t.RefreshToken == "" {
		t.RefreshToken = refresh // a server that does not rotate keeps the old one valid
	}
	return t, nil
}

// Revoke ends a refresh token's session at the issuer (RFC 7009). Best effort:
// a logout must still clear local state when the issuer is unreachable.
func (c *Client) Revoke(ctx context.Context, refresh string) error {
	ep, err := c.Discover(ctx)
	if err != nil {
		return err
	}
	if ep.Revocation == "" {
		return nil
	}
	return c.postForm(ctx, ep.Revocation, url.Values{
		"token":           {refresh},
		"token_type_hint": {"refresh_token"},
		"client_id":       {c.ClientID},
	}, nil)
}
