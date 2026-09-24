package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

// Every port is a fake below. That the use cases run on them unchanged is the
// LSP claim made concrete: the pipeline cannot tell a fake from Keycloak.

type fakeIdP struct {
	pending      int
	offlineDeny  bool // refuse the first sign-in that asks for offline_access
	sawScopes    []domain.Scopes
	refreshCalls int
	revoked      string
	exchangeErr  error
}

func (f *fakeIdP) Realm() domain.Realm {
	return domain.Realm{Issuer: "https://idp.test", ClientID: "hive-cli"}
}
func (f *fakeIdP) Endpoints(context.Context) (domain.Endpoints, error) {
	return domain.Endpoints{Authorization: "https://idp.test/auth"}, nil
}
func (f *fakeIdP) StartDevice(_ context.Context, s domain.Scopes, p domain.PKCE) (domain.DeviceGrant, error) {
	f.sawScopes = append(f.sawScopes, s)
	if p.Challenge == "" {
		return domain.DeviceGrant{}, errors.New("device request without PKCE")
	}
	return domain.DeviceGrant{DeviceCode: "dc", UserCode: "ABCD-EFGH", ExpiresIn: time.Minute, Interval: time.Second}, nil
}
func (f *fakeIdP) PollDevice(context.Context, domain.DeviceGrant, domain.PKCE) (domain.Tokens, error) {
	if f.pending > 0 {
		f.pending--
		return domain.Tokens{}, &domain.OAuthError{Code: "authorization_pending"}
	}
	return f.tokens("device"), nil
}
func (f *fakeIdP) ExchangeCode(_ context.Context, code, _ string, p domain.PKCE) (domain.Tokens, error) {
	if f.exchangeErr != nil {
		return domain.Tokens{}, f.exchangeErr
	}
	if code != "the-code" || p.Verifier == "" {
		return domain.Tokens{}, errors.New("bad exchange")
	}
	return f.tokens("browser"), nil
}
func (f *fakeIdP) tokens(kind string) domain.Tokens {
	return domain.Tokens{Access: "access-" + kind, Refresh: "refresh-" + kind, ExpiresAt: fixedNow.Add(time.Hour)}
}
func (f *fakeIdP) Refresh(_ context.Context, r string) (domain.Tokens, error) {
	f.refreshCalls++
	return domain.Tokens{Access: "access-refreshed", ExpiresAt: fixedNow.Add(time.Hour)}, nil
}
func (f *fakeIdP) Revoke(_ context.Context, r string) error { f.revoked = r; return nil }

type fakeCallback struct{ state chan string }

func (c *fakeCallback) Listen(context.Context) (string, error) {
	return "http://127.0.0.1:1/callback", nil
}
func (c *fakeCallback) Await(context.Context, string) (string, error) { return "the-code", nil }
func (c *fakeCallback) Close() error                                  { return nil }

type fakeBrowser struct{ opened []string }

func (b *fakeBrowser) Open(u string) error { b.opened = append(b.opened, u); return nil }

type memCreds map[string]string

func (m memCreds) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", domain.ErrNotFound
	}
	return v, nil
}
func (m memCreds) Set(k, v string) error { m[k] = v; return nil }
func (m memCreds) Delete(k string) error { delete(m, k); return nil }
func (m memCreds) Kind() string          { return "memory" }

type memSessions struct{ s *domain.Session }

func (m *memSessions) Load() (*domain.Session, error) {
	if m.s == nil {
		return nil, domain.ErrNotSignedIn
	}
	c := *m.s
	return &c, nil
}
func (m *memSessions) Save(s domain.Session) error { m.s = &s; return nil }
func (m *memSessions) Remove() error               { m.s = nil; return nil }

type fakeAccounts struct {
	plan    *domain.Plan
	minted  int
	live    map[string]bool
	mintErr error
}

func (a *fakeAccounts) Me(_ context.Context, access string) (domain.Account, error) {
	if access == "" {
		return domain.Account{}, errors.New("no token")
	}
	return domain.Account{Email: "c@example.com", Plan: a.plan}, nil
}
func (a *fakeAccounts) Mint(context.Context, string, domain.MachineLabel) (domain.ArtifactToken, error) {
	if a.mintErr != nil {
		return domain.ArtifactToken{}, a.mintErr
	}
	a.minted++
	id := string(rune('a' + a.minted))
	a.live[id] = true
	return domain.ArtifactToken{ID: id, Prefix: "hv_live_" + id, Secret: "hv_live_secret_" + id}, nil
}
func (a *fakeAccounts) RevokeArtifact(_ context.Context, _, id string) error {
	delete(a.live, id)
	return nil
}

type fakeMaven struct {
	holds    string
	writeErr error
}

func (m *fakeMaven) Write(s string) (string, error) {
	if m.writeErr != nil {
		return "", m.writeErr
	}
	m.holds = s
	return "", nil
}
func (m *fakeMaven) RemoveIfHolds(s string) (bool, error) {
	if m.holds != s {
		return false, nil
	}
	m.holds = ""
	return true, nil
}
func (m *fakeMaven) Path() string { return "/fake/settings.xml" }

type quietPrompter struct{ notes, codes int }

func (p *quietPrompter) OpeningBrowser(string, bool)       {}
func (p *quietPrompter) ShowDeviceCode(domain.DeviceGrant) { p.codes++ }
func (p *quietPrompter) Note(string)                       { p.notes++ }

var fixedNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

// fakeClock advances instantly: the device loop never really sleeps.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	return nil
}

type counterEntropy struct{ n int }

func (e *counterEntropy) Token(int) string { e.n++; return "tok" + string(rune('0'+e.n)) }

type fakeMachine struct{ env domain.Environment }

func (m fakeMachine) Environment() domain.Environment { return m.env }
func (m fakeMachine) Hostname() string                { return "box" }

type world struct {
	Deps
	idp      *fakeIdP
	accounts *fakeAccounts
	maven    *fakeMaven
	creds    memCreds
	sessions *memSessions
	prompt   *quietPrompter
	browser  *fakeBrowser
}

func newWorld(plan *domain.Plan, env domain.Environment) *world {
	w := &world{
		idp: &fakeIdP{}, accounts: &fakeAccounts{plan: plan, live: map[string]bool{}},
		maven: &fakeMaven{}, creds: memCreds{}, sessions: &memSessions{}, prompt: &quietPrompter{}, browser: &fakeBrowser{},
	}
	w.Deps = Deps{
		IdP: w.idp, Callback: &fakeCallback{}, Browser: w.browser, Credentials: w.creds,
		Sessions: w.sessions, Accounts: w.accounts, Maven: w.maven, Prompter: w.prompt,
		Clock: &fakeClock{now: fixedNow}, Entropy: &counterEntropy{}, Machine: fakeMachine{env: env},
	}
	return w
}

var desktop = domain.Environment{HasDisplay: true, HasOpener: true}
var pro = &domain.Plan{Name: "pro", Status: "active", Entitles: []string{"x"}}

func TestBrowserLoginMintsStoresAndReports(t *testing.T) {
	w := newWorld(pro, desktop)
	res, err := Login(context.Background(), w.Deps, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.browser.opened) != 1 || res.Artifact == nil || w.maven.holds != res.Artifact.Secret {
		t.Fatalf("browser=%v res=%+v maven=%q", w.browser.opened, res, w.maven.holds)
	}
	if res.Session.Email != "c@example.com" || res.Session.Plan != "pro (active)" || w.sessions.s == nil {
		t.Errorf("session %+v", res.Session)
	}
	if _, err := w.creds.Get(keyArtifact); err != nil {
		t.Error("artifact secret not in the credential store")
	}
}

func TestDeviceLoginWaitsThroughPending(t *testing.T) {
	w := newWorld(pro, domain.Environment{OverSSH: true})
	w.idp.pending = 3
	res, err := Login(context.Background(), w.Deps, "")
	if err != nil {
		t.Fatal(err)
	}
	if w.prompt.codes != 1 || len(w.browser.opened) != 0 {
		t.Errorf("over ssh the code is shown and no browser opened: codes=%d opened=%v", w.prompt.codes, w.browser.opened)
	}
	if res.Account.Email == "" {
		t.Error("no account")
	}
}

func TestOfflineRefusalRetriesWithoutOffline(t *testing.T) {
	w := newWorld(pro, desktop)
	w.idp.exchangeErr = &domain.OAuthError{Code: "not_allowed", Description: "Offline tokens not allowed for the user or client"}
	calls := 0
	Register(retryProbe{w: w, calls: &calls})
	defer Register(browserFlow{})
	res, err := Login(context.Background(), w.Deps, "probe")
	if err != nil || !res.SessionOnly || calls != 2 || w.prompt.notes != 1 {
		t.Fatalf("err=%v sessionOnly=%v calls=%d notes=%d", err, res.SessionOnly, calls, w.prompt.notes)
	}
}

// retryProbe is a flow registered by the test: proof that a new method is a
// registration, and a probe of what the pipeline does with the first refusal.
type retryProbe struct {
	w     *world
	calls *int
}

func (retryProbe) Method() domain.Method { return "probe" }
func (p retryProbe) SignIn(ctx context.Context, d Deps, s domain.Scopes) (domain.Tokens, error) {
	*p.calls++
	if *p.calls == 1 {
		return domain.Tokens{}, p.w.idp.exchangeErr
	}
	if s.String() != "openid" {
		return domain.Tokens{}, errors.New("retry still asked for offline_access")
	}
	return p.w.idp.tokens("probe"), nil
}

func TestNoPlanMintsNothing(t *testing.T) {
	w := newWorld(nil, desktop)
	res, err := Login(context.Background(), w.Deps, "")
	if err != nil || res.Artifact != nil || w.accounts.minted != 0 || w.maven.holds != "" {
		t.Fatalf("err=%v res=%+v minted=%d", err, res, w.accounts.minted)
	}
}

func TestFailedSettingsWriteRevokesTheNewToken(t *testing.T) {
	w := newWorld(pro, desktop)
	w.maven.writeErr = errors.New("disk full")
	if _, err := Login(context.Background(), w.Deps, ""); err == nil {
		t.Fatal("want the write error")
	}
	if len(w.accounts.live) != 0 {
		t.Errorf("a token nothing references was left live: %v", w.accounts.live)
	}
}

func TestReloginRevokesThePreviousMachineToken(t *testing.T) {
	w := newWorld(pro, desktop)
	Login(context.Background(), w.Deps, "")
	res, err := Login(context.Background(), w.Deps, "")
	if err != nil || !res.RevokedPrevious || len(w.accounts.live) != 1 {
		t.Fatalf("err=%v revoked=%v live=%v", err, res.RevokedPrevious, w.accounts.live)
	}
}

func TestAccessTokenRefreshesWhenExpired(t *testing.T) {
	w := newWorld(pro, desktop)
	Login(context.Background(), w.Deps, "")
	w.Clock.(*fakeClock).now = fixedNow.Add(2 * time.Hour)
	tok, err := AccessToken(context.Background(), w.Deps)
	if err != nil || tok != "access-refreshed" || w.idp.refreshCalls != 1 {
		t.Fatalf("tok=%q err=%v refreshes=%d", tok, err, w.idp.refreshCalls)
	}
	stored, _ := loadTokens(w.creds)
	if stored.Refresh == "" {
		t.Error("a non-rotating issuer's refresh token must be kept")
	}
}

func TestLogoutUndoesEverything(t *testing.T) {
	w := newWorld(pro, desktop)
	Login(context.Background(), w.Deps, "")
	res, err := Logout(context.Background(), w.Deps)
	if err != nil || !res.ArtifactRevoked || !res.SettingsCleaned {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if len(w.accounts.live) != 0 || len(w.creds) != 0 || w.sessions.s != nil || w.idp.revoked == "" {
		t.Errorf("left behind: live=%v creds=%v session=%v revoked=%q", w.accounts.live, w.creds, w.sessions.s, w.idp.revoked)
	}
	if _, err := Logout(context.Background(), w.Deps); !errors.Is(err, domain.ErrNotSignedIn) {
		t.Errorf("second logout: %v", err)
	}
}

func TestUnknownMethodNamesTheRegisteredOnes(t *testing.T) {
	w := newWorld(pro, desktop)
	_, err := Login(context.Background(), w.Deps, "carrier-pigeon")
	if err == nil {
		t.Fatal("want an error")
	}
}
