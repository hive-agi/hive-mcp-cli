package nrepl

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := map[string]any{"op": "eval", "code": "(+ 1 2)", "n": 3, "l": []any{"a", "b"}}
	var buf bytes.Buffer
	if err := Encode(&buf, in); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "d4:code7:(+ 1 2)1:ll1:a1:be1:ni3e2:op4:evale" {
		t.Fatalf("encoded %q", got)
	}
	out, err := Decode(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["code"] != "(+ 1 2)" || m["n"] != int64(3) || len(m["l"].([]any)) != 2 {
		t.Fatalf("decoded %#v", m)
	}
}

func TestDecodeMultibyteString(t *testing.T) {
	s := "olá, ☂"
	var buf bytes.Buffer
	_ = Encode(&buf, s)
	got, err := Decode(bufio.NewReader(&buf))
	if err != nil || got != s {
		t.Fatalf("got %q, %v", got, err)
	}
}

// fakeServer answers one eval with the given messages, each stamped with
// the request id, the last one carrying status done.
func fakeServer(t *testing.T, msgs []map[string]any) string {
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
		req, err := Decode(bufio.NewReader(c))
		if err != nil {
			return
		}
		id := req.(map[string]any)["id"]
		_ = Encode(c, map[string]any{"id": "someone-else", "value": "ignored"})
		for i, m := range msgs {
			m["id"] = id
			if i == len(msgs)-1 {
				m["status"] = []any{"done"}
			}
			_ = Encode(c, m)
		}
	}()
	return ln.Addr().String()
}

func TestEvalCollectsOutValueAndIgnoresOtherIds(t *testing.T) {
	addr := fakeServer(t, []map[string]any{{"out": "{\"a\":"}, {"out": "1}"}, {"value": "false"}})
	res, err := Eval(addr, "(x)", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Out != "{\"a\":1}" || res.Value != "false" {
		t.Fatalf("got %#v", res)
	}
}

func TestEvalReportsAnUnreachableHost(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	_, err := Eval(addr, "(x)", time.Second)
	if err == nil || !strings.Contains(err.Error(), "hive start") {
		t.Fatalf("want a hint to start hive-mcp, got %v", err)
	}
}

func TestPortPrecedence(t *testing.T) {
	t.Setenv("HIVE_MCP_NREPL_PORT", "")
	t.Setenv("BB_MCP_NREPL_PORT", "")
	if Port() != DefaultPort {
		t.Fatal("default")
	}
	t.Setenv("BB_MCP_NREPL_PORT", "7999")
	if Port() != 7999 {
		t.Fatal("bb-mcp port")
	}
	t.Setenv("HIVE_MCP_NREPL_PORT", "7111")
	if Port() != 7111 {
		t.Fatal(fmt.Sprint("hive port wins, got ", Port()))
	}
}
