// Package nrepl is a minimal nREPL client: enough to evaluate one form in the
// running hive-mcp JVM and collect its value, output and error.
package nrepl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultPort is the nREPL port hive-mcp listens on unless told otherwise.
const DefaultPort = 7910

// Port returns the hive-mcp nREPL port: HIVE_MCP_NREPL_PORT, then
// BB_MCP_NREPL_PORT, then DefaultPort.
func Port() int {
	for _, k := range []string{"HIVE_MCP_NREPL_PORT", "BB_MCP_NREPL_PORT"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			if p, err := strconv.Atoi(v); err == nil && p > 0 {
				return p
			}
		}
	}
	return DefaultPort
}

// Result is what one eval answered.
type Result struct {
	Value string // the last :value
	Out   string // everything printed to *out*
	Err   string // everything printed to *err*
	Ex    string // the exception class, when the eval threw
}

// Encode writes v (string, int, []any, map[string]any) as bencode.
func Encode(w io.Writer, v any) error {
	switch t := v.(type) {
	case string:
		_, err := fmt.Fprintf(w, "%d:%s", len(t), t)
		return err
	case int:
		_, err := fmt.Fprintf(w, "i%de", t)
		return err
	case []any:
		if _, err := io.WriteString(w, "l"); err != nil {
			return err
		}
		for _, x := range t {
			if err := Encode(w, x); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	case map[string]any:
		if _, err := io.WriteString(w, "d"); err != nil {
			return err
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := Encode(w, k); err != nil {
				return err
			}
			if err := Encode(w, t[k]); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	default:
		return fmt.Errorf("bencode: unsupported type %T", v)
	}
}

// Decode reads one bencoded value: string, int64, []any or map[string]any.
func Decode(r *bufio.Reader) (any, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch {
	case b == 'i':
		s, err := r.ReadString('e')
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(strings.TrimSuffix(s, "e"), 10, 64)
	case b == 'l':
		var out []any
		for {
			c, err := r.Peek(1)
			if err != nil {
				return nil, err
			}
			if c[0] == 'e' {
				_, _ = r.ReadByte()
				return out, nil
			}
			x, err := Decode(r)
			if err != nil {
				return nil, err
			}
			out = append(out, x)
		}
	case b == 'd':
		out := map[string]any{}
		for {
			c, err := r.Peek(1)
			if err != nil {
				return nil, err
			}
			if c[0] == 'e' {
				_, _ = r.ReadByte()
				return out, nil
			}
			k, err := Decode(r)
			if err != nil {
				return nil, err
			}
			ks, ok := k.(string)
			if !ok {
				return nil, errors.New("bencode: dict key is not a string")
			}
			v, err := Decode(r)
			if err != nil {
				return nil, err
			}
			out[ks] = v
		}
	case b >= '0' && b <= '9':
		rest, err := r.ReadString(':')
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(string(b) + strings.TrimSuffix(rest, ":"))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf), nil
	default:
		return nil, fmt.Errorf("bencode: unexpected byte %q", b)
	}
}

func hasStatus(msg map[string]any, want string) bool {
	st, _ := msg["status"].([]any)
	for _, s := range st {
		if s == want {
			return true
		}
	}
	return false
}

// Eval evaluates code on the nREPL at addr and waits for it to finish.
func Eval(addr, code string, timeout time.Duration) (Result, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return Result{}, fmt.Errorf("cannot reach hive-mcp on %s (is it running? `hive start`): %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	id := fmt.Sprintf("hive-cli-%d", time.Now().UnixNano())
	if err := Encode(conn, map[string]any{"op": "eval", "code": code, "id": id}); err != nil {
		return Result{}, err
	}

	var res Result
	r := bufio.NewReader(conn)
	for {
		v, err := Decode(r)
		if err != nil {
			return res, fmt.Errorf("reading the nREPL answer: %w", err)
		}
		msg, ok := v.(map[string]any)
		if !ok || msg["id"] != id {
			continue
		}
		if s, ok := msg["value"].(string); ok {
			res.Value = s
		}
		if s, ok := msg["out"].(string); ok {
			res.Out += s
		}
		if s, ok := msg["err"].(string); ok {
			res.Err += s
		}
		if s, ok := msg["ex"].(string); ok {
			res.Ex = s
		}
		if hasStatus(msg, "done") {
			return res, nil
		}
	}
}
