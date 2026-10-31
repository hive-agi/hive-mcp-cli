package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefusalReasonPrefersTheHeaderThenWWWAuthenticate(t *testing.T) {
	h := http.Header{}
	h.Set("WWW-Authenticate", `Basic realm="hive-store", error="deny/token-expired", error_description="x"`)
	if got := RefusalReason(h); got != "deny/token-expired" {
		t.Errorf("from WWW-Authenticate: got %q", got)
	}
	h.Set(ReasonHeader, "deny/token-revoked")
	if got := RefusalReason(h); got != "deny/token-revoked" {
		t.Errorf("header wins: got %q", got)
	}
	if got := RefusalReason(http.Header{"Www-Authenticate": {`Basic realm="hive-store"`}}); got != "" {
		t.Errorf("an old gateway names no reason: got %q", got)
	}
}

func TestAuthParamDoesNotMatchInsideAnotherParameter(t *testing.T) {
	if got := authParam(`Basic myerror="no", error="yes"`, "error"); got != "yes" {
		t.Errorf("got %q", got)
	}
}

func TestExplainRefusalNamesTheActionForEveryKnownReason(t *testing.T) {
	t.Setenv("HIVE_STORE_URL", "https://store.example")
	cases := map[string]string{
		"deny/token-absent":          RepoID,
		"deny/token-malformed":       "hv_live_",
		"deny/token-unknown":         "mistyped",
		"deny/token-revoked":         "revoked",
		"deny/token-expired":         "expired",
		"deny/token-scope":           "scoped",
		"deny/subscription-missing":  "/pricing",
		"deny/subscription-inactive": "renew",
		"deny/artifact-not-entitled": "plan does not include",
		"deny/plan-unknown":          "store's side",
		"deny/malformed-entitles":    "store's side",
	}
	for reason, want := range cases {
		if got := ExplainRefusal(reason, 401); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", reason, got, want)
		}
	}
}

func TestExplainRefusalFallsBackToTheStatus(t *testing.T) {
	if got := ExplainRefusal("", 402); !strings.Contains(got, "subscription") {
		t.Errorf("402: %q", got)
	}
	if got := ExplainRefusal("deny/new-thing", 418); !strings.Contains(got, "deny/new-thing") {
		t.Errorf("unknown reason is still named: %q", got)
	}
}

func TestDiagnoseTokenCarriesTheGatewayReasonAndNeverTheToken(t *testing.T) {
	const token = "hv_live_SECRETSECRETSECRET"
	cases := []struct {
		status int
		reason string
		want   ProbeResult
		says   string
	}{
		{http.StatusOK, "", ProbeAccepted, ""},
		{http.StatusUnauthorized, "deny/token-revoked", ProbeRejected, "revoked"},
		{http.StatusPaymentRequired, "deny/subscription-inactive", ProbeNotEntitled, "renew"},
		{http.StatusForbidden, "deny/artifact-not-entitled", ProbeNotEntitled, "plan"},
		{http.StatusBadGateway, "", ProbeInconclusive, "502"},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, p, ok := r.BasicAuth(); !ok || u != token || p != token {
				t.Errorf("token not sent in both positions")
			}
			if c.reason != "" {
				w.Header().Set(ReasonHeader, c.reason)
			}
			w.WriteHeader(c.status)
		}))
		t.Setenv("HIVE_STORE_URL", srv.URL)
		d := DiagnoseToken(context.Background(), token)
		srv.Close()
		if d.Result != c.want || d.Reason != c.reason || !strings.Contains(d.Explanation, c.says) {
			t.Errorf("%d: got %+v", c.status, d)
		}
		if strings.Contains(d.Explanation+d.Status, "SECRET") {
			t.Errorf("%d: diagnosis leaks the token", c.status)
		}
	}
}
