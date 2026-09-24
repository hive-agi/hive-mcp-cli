package hive

import (
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/BuddhiLW/bonzai"
)

// Bonzai validates Short at RUN time and aborts the command with a
// developer-error, so a too-long Short ships as a command that cannot run.
// Found by the auth e2e test, where `hive logout` refused to start.
func TestEveryShortIsValidForBonzai(t *testing.T) {
	var walk func(c *bonzai.Cmd, path string)
	walk = func(c *bonzai.Cmd, path string) {
		if s := c.Short; s != "" {
			if n := utf8.RuneCountInString(s); n >= 50 {
				t.Errorf("%s: Short is %d runes, bonzai wants < 50: %q", path, n, s)
			}
			if r, _ := utf8.DecodeRuneInString(s); !unicode.IsLower(r) {
				t.Errorf("%s: Short must start lowercase: %q", path, s)
			}
		}
		for _, sub := range c.Cmds {
			walk(sub, path+" "+sub.Name)
		}
	}
	walk(Cmd, "hive")
}
