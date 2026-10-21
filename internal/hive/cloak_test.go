package hive

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/hive-agi/hive-mcp-cli/internal/nrepl"
)

func TestSplitCloakArgs(t *testing.T) {
	cmd, raw := splitCloakArgs([]string{"reveal", "id=x", "reason=why", "--json"})
	if cmd != "reveal id=x reason=why" || !raw {
		t.Fatalf("got %q %v", cmd, raw)
	}
	if cmd, raw := splitCloakArgs(nil); cmd != "" || raw {
		t.Fatalf("empty args: %q %v", cmd, raw)
	}
}

func TestCloakFormQuotesTheCommand(t *testing.T) {
	form := cloakForm(`reveal id=x reason="a \"quoted\" why"`)
	if !strings.Contains(form, `{:command "reveal id=x reason=\"a \\\"quoted\\\" why\""}`) {
		t.Fatalf("command not quoted as a Clojure string: %s", form)
	}
	if !strings.Contains(form, "hive-cloak.root-tool/handler") {
		t.Fatal("form does not reach the cloak handler")
	}
}

func TestPrettyJSON(t *testing.T) {
	if got := prettyJSON(`{"a":1}`); got != "{\n  \"a\": 1\n}" {
		t.Fatalf("got %q", got)
	}
	if got := prettyJSON("not json"); got != "not json" {
		t.Fatalf("got %q", got)
	}
}

// serve answers one eval with `out` printed and `value` returned.
func serve(t *testing.T, out, value string) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer ln.Close()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		req, err := nrepl.Decode(bufio.NewReader(c))
		if err != nil {
			return
		}
		id := req.(map[string]any)["id"]
		_ = nrepl.Encode(c, map[string]any{"id": id, "out": out})
		_ = nrepl.Encode(c, map[string]any{"id": id, "value": value, "status": []any{"done"}})
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestCloakCmdRunsAVerbOnTheHost(t *testing.T) {
	t.Setenv("HIVE_MCP_NREPL_PORT", strconv.Itoa(serve(t, `{"keyring":{"unlocked?":true}}`, "false")))
	if err := cloakCmd.Do(cloakCmd, "status"); err != nil {
		t.Fatalf("status: %v", err)
	}
}

func TestCloakCmdFailsWhenTheToolAnswersAnError(t *testing.T) {
	t.Setenv("HIVE_MCP_NREPL_PORT", strconv.Itoa(serve(t, `{"error":"not a cloak verb"}`, "true")))
	if err := cloakCmd.Do(cloakCmd, "frobnicate"); err == nil {
		t.Fatal("an error answer must fail the command")
	}
}

func TestCloakIsARootCommand(t *testing.T) {
	for _, c := range Cmd.Cmds {
		if c == cloakCmd {
			return
		}
	}
	t.Fatal("hive cloak is not registered on the root command")
}
