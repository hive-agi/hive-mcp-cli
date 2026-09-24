package policy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

// Pure functions, so every test here is a table: no fakes, no clock, no I/O.

func TestChooseMethod(t *testing.T) {
	cases := []struct {
		name   string
		env    domain.Environment
		forced domain.Method
		want   domain.Method
	}{
		{"linux desktop", domain.Environment{HasDisplay: true, HasOpener: true}, "", domain.MethodBrowser},
		{"ssh beats a display", domain.Environment{OverSSH: true, HasDisplay: true, HasOpener: true}, "", domain.MethodDevice},
		{"headless linux", domain.Environment{}, "", domain.MethodDevice},
		{"display but no opener", domain.Environment{HasDisplay: true}, "", domain.MethodDevice},
		{"macOS", domain.Environment{DesktopOS: true}, "", domain.MethodBrowser},
		{"$BROWSER", domain.Environment{BrowserOverride: true}, "", domain.MethodBrowser},
		{"forced wins", domain.Environment{HasDisplay: true, HasOpener: true}, domain.MethodDevice, domain.MethodDevice},
	}
	for _, c := range cases {
		if got := ChooseMethod(c.env, c.forced); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestChallengeMatchesAnIndependentImplementation(t *testing.T) {
	// Computed with: printf %s VERIFIER | openssl dgst -sha256 -binary | base64url
	const verifier = "dBjftJeZ4CVP-mJ0kBSNZfnHS-1Djn-zqg-uZ0Pq-VoFpM"
	if got := Challenge(verifier); got != "lqUONu2tAVhJQtri51zvNy-cVwi2Oh3Ni75W4I606YQ" {
		t.Fatalf("challenge %s", got)
	}
}

func TestJudgePoll(t *testing.T) {
	oe := func(code string) error { return &domain.OAuthError{Code: code} }
	cases := []struct {
		err     error
		verdict PollVerdict
		failure domain.Failure
	}{
		{nil, PollDone, domain.FailureOther},
		{oe("authorization_pending"), PollContinue, domain.FailureOther},
		{oe("slow_down"), PollSlowDown, domain.FailureOther},
		{oe("access_denied"), PollStop, domain.FailureDeclined},
		{oe("expired_token"), PollStop, domain.FailureExpired},
		{fmt.Errorf("wrapped: %w", oe("authorization_pending")), PollContinue, domain.FailureOther},
		{errors.New("network down"), PollStop, domain.FailureOther},
	}
	for _, c := range cases {
		v, f := JudgePoll(c.err)
		if v != c.verdict || f != c.failure {
			t.Errorf("%v: got (%v,%v) want (%v,%v)", c.err, v, f, c.verdict, c.failure)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[error]domain.Failure{
		&domain.OAuthError{Code: "invalid_client"}:      domain.FailureUnknownClient,
		&domain.OAuthError{Code: "unauthorized_client"}: domain.FailureUnknownClient,
		&domain.OAuthError{Code: "access_denied"}:       domain.FailureDeclined,
		context.Canceled:         domain.FailureCancelled,
		context.DeadlineExceeded: domain.FailureTimedOut,
		errors.New("x"):          domain.FailureOther,
	}
	for err, want := range cases {
		if got := Classify(err); got != want {
			t.Errorf("%v: got %v want %v", err, got, want)
		}
	}
}

func TestOfflineFallback(t *testing.T) {
	kc := &domain.OAuthError{Code: "not_allowed", Description: "Offline tokens not allowed for the user or client"}
	if !NeedsOfflineFallback(fmt.Errorf("exchange: %w", kc)) {
		t.Error("Keycloak's refusal must trigger the fallback")
	}
	if NeedsOfflineFallback(&domain.OAuthError{Code: "not_allowed", Description: "something else"}) {
		t.Error("an unrelated not_allowed must not")
	}
	got := WithoutOffline(DefaultScopes())
	if got.String() != "openid" {
		t.Errorf("fallback scopes %q", got)
	}
}

func TestExpiresAtKeepsAMargin(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	if got := ExpiresAt(now, 5*time.Minute); got != now.Add(4*time.Minute+30*time.Second) {
		t.Errorf("got %v", got)
	}
	if got := ExpiresAt(now, 30*time.Second); got != now.Add(30*time.Second) {
		t.Errorf("short lifetimes are not shortened further: %v", got)
	}
}

func TestMintAndSupersede(t *testing.T) {
	if ShouldMint(domain.Account{}) {
		t.Error("no plan, no token")
	}
	if !ShouldMint(domain.Account{Plan: &domain.Plan{Name: "pro"}}) {
		t.Error("a plan gets a token")
	}
	prev := &domain.Session{ArtifactTokenID: "a"}
	if !Supersedes(prev, domain.Session{ArtifactTokenID: "b"}) || Supersedes(prev, domain.Session{ArtifactTokenID: "a"}) || Supersedes(nil, domain.Session{}) {
		t.Error("supersede rules")
	}
	if Label("box", time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)) != "hive-cli box 2026-09-24" {
		t.Error("label")
	}
}
