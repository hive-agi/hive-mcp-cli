package hive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BuddhiLW/bonzai"
	"github.com/hive-agi/hive-mcp-cli/internal/nrepl"
)

// cloakTimeout bounds one cloak verb; reveal and unlock may wait on the operator.
const cloakTimeout = 5 * time.Minute

// cloakForm is the Clojure the running hive-mcp evaluates for `hive cloak`:
// the hive.cloak addon's own handler, so the CLI and the MCP tool answer alike.
// It prints the tool's text and evaluates to true when the tool answered an error.
func cloakForm(command string) string {
	quoted, _ := json.Marshal(command)
	return fmt.Sprintf(`(let [h (try (requiring-resolve 'hive-cloak.root-tool/handler) (catch Throwable _addon-not-loaded nil))]
  (if h
    (let [res (h {:command %s})]
      (print (:text res))
      (flush)
      (boolean (:isError res)))
    (do (print "{\"error\":\"the hive.cloak addon is not loaded in hive-mcp; add it to the host's classpath and restart (hive start)\"}")
        (flush)
        true)))`, quoted)
}

// splitCloakArgs separates the CLI's own flags from the cloak command.
func splitCloakArgs(args []string) (command string, raw bool) {
	var words []string
	for _, a := range args {
		if a == "--json" {
			raw = true
			continue
		}
		words = append(words, a)
	}
	return strings.Join(words, " "), raw
}

// prettyJSON indents `text` when it is JSON, else returns it unchanged.
func prettyJSON(text string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
		return text
	}
	return buf.String()
}

var cloakCmd = &bonzai.Cmd{
	Name:  "cloak",
	Alias: "seal",
	Short: "sealing: enclaves, keyring, reveal, sealed edges",

	Mcp: &bonzai.McpMeta{
		Desc: "Run a cloak verb on the running hive-mcp: status, unlock, lock, hide, reveal, on, off, enclave <verb>, edges hide|list|restore, project [status], mode, keys <verb>, handoff <verb>. Params go inline as key=value.",
		Params: []bonzai.McpParam{
			{Name: "json", Desc: "Print the raw JSON answer", Type: "boolean"},
		},
	},

	Usage: "hive cloak <verb> [key=value ...] [--json]",

	Long: `Run a verb of the cloak tool (the hive.cloak addon) on the running hive-mcp.
The same verbs and answers as the MCP tool, so what an agent learns works here.

  hive cloak                      every verb and the usual workflow
  hive cloak status               keyring, enclave, layers, and what to run next
  hive cloak unlock               open the work keyring (pinentry may ask)
  hive cloak reveal id=X reason="why"
  hive cloak project id=P         dry run of making project P private
  hive cloak keys                 the work key's lifecycle verbs

Joining an enclave (cloak on) is per agent session, so it is done through the
MCP tool, not from this shell. The nREPL port is HIVE_MCP_NREPL_PORT, then
BB_MCP_NREPL_PORT, then 7910.`,

	Do: func(x *bonzai.Cmd, args ...string) error {
		command, raw := splitCloakArgs(args)
		addr := fmt.Sprintf("127.0.0.1:%d", nrepl.Port())
		res, err := nrepl.Eval(addr, cloakForm(command), cloakTimeout)
		if err != nil {
			return err
		}
		if res.Ex != "" {
			return fmt.Errorf("hive-mcp raised %s: %s", res.Ex, strings.TrimSpace(res.Err))
		}
		out := res.Out
		if !raw {
			out = prettyJSON(out)
		}
		if res.Value == "true" {
			fmt.Fprintln(os.Stderr, out)
			return errors.New("cloak answered an error")
		}
		fmt.Println(out)
		return nil
	},
}
