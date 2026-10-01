package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/hive-agi/hive-mcp-cli/internal/setup"
)

func TestBbmcpCheckWithoutAnAnchorFailsAndOffersSetup(t *testing.T) {
	r := bbmcpCheckResult(setup.BBMCPObserved{Anchor: "/h/.local/share/hive-mcp/bb-mcp"}, "", nil, nil)
	if r.Status != StatusError || !r.CanFix || !strings.Contains(r.FixHint, "bb-mcp setup") {
		t.Errorf("got %+v", r)
	}
}

func TestBbmcpCheckFailureCarriesItsOutputAndTheFix(t *testing.T) {
	o := setup.BBMCPObserved{Anchor: "/h/a", AnchorCheckout: "/src/bb-mcp"}
	r := bbmcpCheckResult(o, "anchor ok\ncodex STALE /other-user/bb-mcp\n", errors.New("exit 1"), nil)
	if r.Status != StatusError || !strings.Contains(r.Details, "STALE") {
		t.Errorf("got %+v", r)
	}
	if r.FixHint != "/h/a/bb-mcp setup" {
		t.Errorf("fix hint %q must run setup through the anchor", r.FixHint)
	}
}

func TestBbmcpCheckPassNamesTheAnchor(t *testing.T) {
	o := setup.BBMCPObserved{Anchor: "/h/a", AnchorCheckout: "/src/bb-mcp"}
	r := bbmcpCheckResult(o, "", nil, nil)
	if r.Status != StatusOK || !strings.Contains(r.Message, "/h/a/start-bb-mcp.sh") {
		t.Errorf("got %+v", r)
	}
}

func TestLegacyCheck(t *testing.T) {
	if r := legacyCheckResult(nil); r.Status != StatusOK {
		t.Errorf("none: %+v", r)
	}
	r := legacyCheckResult([]setup.LegacyEntry{{Scope: "local", Project: "/p", Command: "/x/hive-mcp-foss"}})
	if r.Status != StatusWarning || !strings.Contains(r.Details, "--scope local") {
		t.Errorf("got %+v", r)
	}
}
