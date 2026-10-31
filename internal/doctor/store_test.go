package doctor

import (
	"strings"
	"testing"

	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

func TestStoreCheckWithoutAServerOnlyWarns(t *testing.T) {
	c := storeCheckResult(store.Wiring{SettingsPath: "/x/settings.xml"}, store.Diagnosis{})
	if c.Status != StatusWarning || c.FixHint == "" {
		t.Errorf("got %+v", c)
	}
}

func TestStoreCheckOneFieldTokenFails(t *testing.T) {
	c := storeCheckResult(store.Wiring{ServerFound: true, Token: "hv_live_a"}, store.Diagnosis{})
	if c.Status != StatusError {
		t.Errorf("got %+v", c)
	}
}

func TestStoreCheckRevokedTokenSaysWhyAndRedacts(t *testing.T) {
	w := store.Wiring{ServerFound: true, UserEqualsPass: true, Token: "hv_live_SECRETSECRETSECRET"}
	d := store.Diagnosis{Result: store.ProbeRejected, Status: "401 Unauthorized", Code: 401,
		Reason: "deny/token-revoked", Explanation: store.ExplainRefusal("deny/token-revoked", 401)}
	c := storeCheckResult(w, d)
	if c.Status != StatusError || !strings.Contains(c.Details, "revoked") {
		t.Errorf("got %+v", c)
	}
	if strings.Contains(c.Message+c.Details+c.FixHint, "SECRETSECRET") {
		t.Errorf("check leaks the token: %+v", c)
	}
}

func TestStoreCheckLapsedSubscriptionWarnsWithTheReason(t *testing.T) {
	w := store.Wiring{ServerFound: true, UserEqualsPass: true, Token: "hv_live_SECRETSECRETSECRET"}
	d := store.Diagnosis{Result: store.ProbeNotEntitled, Status: "402 Payment Required",
		Explanation: store.ExplainRefusal("deny/subscription-inactive", 402)}
	if c := storeCheckResult(w, d); c.Status != StatusWarning || !strings.Contains(c.Details, "renew") {
		t.Errorf("got %+v", c)
	}
}
