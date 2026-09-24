package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeIssuer is a minimal Keycloak stand-in: discovery, device, token, revoke.
type fakeIssuer struct {
	*httptest.Server
	mu          sync.Mutex
	pending     int    // device polls to answer authorization_pending before success
	slowDown    bool   // answer slow_down once
	deny        bool   // answer access_denied
	challenge   string // PKCE challenge seen at /auth
	gotVerifier string
	revoked     string
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	f := &fakeIssuer{}
	mux := http.NewServeMux()
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                        f.URL,
			"authorization_endpoint":        f.URL + "/auth",
			"token_endpoint":                f.URL + "/token",
			"device_authorization_endpoint": f.URL + "/device",
			"revocation_endpoint":           f.URL + "/revoke",
		})
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-1", "user_code": "ABCD-EFGH",
			"verification_uri": f.URL + "/device", "expires_in": 30, "interval": 0,
		})
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		f.revoked = r.Form.Get("token")
		f.mu.Unlock()
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		fail := func(code string) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": code})
		}
		ok := func(access string) {
			json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "r-" + access, "expires_in": 300})
		}
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			switch {
			case f.deny:
				fail("access_denied")
			case f.slowDown:
				f.slowDown = false
				fail("slow_down")
			case f.pending > 0:
				f.pending--
				fail("authorization_pending")
			default:
				ok("device")
			}
		case "authorization_code":
			f.gotVerifier = r.Form.Get("code_verifier")
			sum := sha256.Sum256([]byte(f.gotVerifier))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("code") != "the-code" {
				fail("invalid_grant")
				return
			}
			ok("browser")
		case "refresh_token":
			ok("refreshed")
		default:
			fail("unsupported_grant_type")
		}
	})
	return f
}

func testClient(f *fakeIssuer) *Client {
	c := NewClient()
	c.Issuer = f.URL
	return c
}

func TestDeviceFlowWaitsThroughPendingAndSucceeds(t *testing.T) {
	f := newFakeIssuer(t)
	f.pending = 2
	c := testClient(f)
	dc, err := c.StartDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "ABCD-EFGH" {
		t.Fatalf("user code %q", dc.UserCode)
	}
	dc.Interval = 0 // StartDevice floors it at 5s; tests do not wait
	tok, err := c.PollDevice(context.Background(), &DeviceCode{DeviceCode: dc.DeviceCode, ExpiresIn: 30})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "device" || tok.RefreshToken != "r-device" || !tok.Valid() {
		t.Fatalf("token %+v", tok)
	}
}

func TestDeviceFlowReportsADecline(t *testing.T) {
	f := newFakeIssuer(t)
	f.deny = true
	_, err := testClient(f).PollDevice(context.Background(), &DeviceCode{DeviceCode: "d", ExpiresIn: 30})
	if err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("got %v", err)
	}
}

func TestLoopbackPKCERoundTrip(t *testing.T) {
	f := newFakeIssuer(t)
	c := testClient(f)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lb, err := c.StartLoopback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(lb.URL)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") {
		t.Fatalf("auth url %s", lb.URL)
	}
	f.challenge = q.Get("code_challenge")

	// A forged callback (wrong state) is refused and does not end the wait.
	bad, _ := http.Get(q.Get("redirect_uri") + "?code=evil&state=forged")
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged state answered %d", bad.StatusCode)
	}
	// The browser comes back.
	good, err := http.Get(q.Get("redirect_uri") + "?code=the-code&state=" + url.QueryEscape(q.Get("state")))
	if err != nil || good.StatusCode != http.StatusOK {
		t.Fatalf("callback: %v %v", good, err)
	}
	tok, err := lb.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "browser" {
		t.Fatalf("token %+v", tok)
	}
}

func TestRefreshAndRevoke(t *testing.T) {
	f := newFakeIssuer(t)
	c := testClient(f)
	tok, err := c.Refresh(context.Background(), "r-old")
	if err != nil || tok.AccessToken != "refreshed" {
		t.Fatalf("%+v %v", tok, err)
	}
	if err := c.Revoke(context.Background(), "r-old"); err != nil || f.revoked != "r-old" {
		t.Fatalf("revoke: %v, server saw %q", err, f.revoked)
	}
}

func TestFileStoreIsPrivateAndRemovesItselfWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive", "credentials.json")
	s := NewFileStore(path)
	if _, err := s.Get("x"); err != ErrNotFound {
		t.Fatalf("empty store: %v", err)
	}
	if err := s.Set("x", "secret"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v", info.Mode().Perm())
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", dir.Mode().Perm())
	}
	if v, _ := s.Get("x"); v != "secret" {
		t.Errorf("got %q", v)
	}
	s.Delete("x")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty store should leave no file behind")
	}
}
