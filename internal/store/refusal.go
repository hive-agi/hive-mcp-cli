package store

// Why the gateway refused a token, in a sentence a customer can act on.
//
// aether and tools.deps keep only the status of a refused request, so a revoked
// token, a lapsed subscription and a typo all surface as the same "401" or
// "402". The gateway names the reason in the x-hive-store-reason header and in
// the error="..." parameter of WWW-Authenticate; this file reads either and
// turns it into the sentence the Maven client will never show.

import (
	"fmt"
	"net/http"
	"strings"
)

// ReasonHeader is the response header the gateway names a refusal in.
const ReasonHeader = "X-Hive-Store-Reason"

// RefusalReason reads the gateway's reason for refusing a request: the
// x-hive-store-reason header when present, else the error="..." parameter of
// WWW-Authenticate. Empty when the response names none (an older gateway).
func RefusalReason(h http.Header) string {
	if r := strings.TrimSpace(h.Get(ReasonHeader)); r != "" {
		return r
	}
	return authParam(h.Get("WWW-Authenticate"), "error")
}

// authParam returns the value of the quoted parameter name in a
// WWW-Authenticate challenge, or "".
func authParam(challenge, name string) string {
	key := name + `="`
	for i := 0; i+len(key) <= len(challenge); i++ {
		if !strings.HasPrefix(challenge[i:], key) {
			continue
		}
		if i > 0 && challenge[i-1] != ' ' && challenge[i-1] != ',' {
			continue
		}
		rest := challenge[i+len(key):]
		if end := strings.IndexByte(rest, '"'); end >= 0 {
			return rest[:end]
		}
		return ""
	}
	return ""
}

// ExplainRefusal is the actionable sentence for a gateway refusal. reason is
// the wire name (deny/token-revoked, ...); status is the HTTP status, used when
// the reason is empty or unknown to this CLI.
func ExplainRefusal(reason string, status int) string {
	dash := BaseURL() + "/dashboard"
	switch strings.TrimPrefix(reason, "deny/") {
	case "token-absent":
		return "no token reached the gateway: check that the deps.edn :mvn/repos key and the settings.xml <id> are both " + RepoID
	case "token-malformed":
		return "the configured password is not a hive artifact token (they start hv_live_); run hive store login"
	case "token-unknown":
		return "the gateway does not know this token: it was mistyped, or minted on another store; mint one at " + dash
	case "token-revoked":
		return "this token was revoked; mint a new one at " + dash + " and run hive store login"
	case "token-expired":
		return "this token has expired; mint a new one at " + dash + " and run hive store login"
	case "token-scope":
		return "this token is scoped to other addons; mint one that covers this addon at " + dash
	case "subscription-missing":
		return "this account has no subscription; subscribe at " + BaseURL() + "/pricing"
	case "subscription-inactive":
		return "your subscription is not active (lapsed or unpaid); renew it at " + dash
	case "artifact-not-entitled":
		return "your plan does not include this addon; compare plans at " + BaseURL() + "/pricing"
	case "plan-unknown", "malformed-entitles":
		return "the store cannot read your plan (" + reason + "); this is on the store's side, contact support"
	}
	switch status {
	case http.StatusUnauthorized:
		return "the gateway rejected this token; it may be revoked, expired or mistyped. Mint a new one at " + dash
	case http.StatusPaymentRequired:
		return "the gateway wants payment: the subscription is missing or lapsed; renew it at " + dash
	case http.StatusForbidden:
		return "the token is valid but does not cover this artifact"
	}
	if reason != "" {
		return fmt.Sprintf("the gateway refused the request (%s, HTTP %d)", reason, status)
	}
	return fmt.Sprintf("the gateway answered HTTP %d", status)
}
