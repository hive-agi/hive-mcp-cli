// Package loopback is a BOUNDARY adapter: port.CallbackListener as an HTTP
// listener on 127.0.0.1 (RFC 8252). The OS picks the port; the realm accepts
// any port on a 127.0.0.1 redirect URI (RFC 8252 section 7.3), verified
// against Keycloak 26.6.1 by test/auth/e2e.sh.
package loopback

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

type result struct {
	code string
	err  error
}

// Listener implements port.CallbackListener.
type Listener struct {
	ln     net.Listener
	srv    *http.Server
	state  chan string
	result chan result
}

func New() *Listener {
	return &Listener{state: make(chan string, 1), result: make(chan result, 1)}
}

const page = `<!doctype html><meta charset="utf-8"><title>hive</title>
<body style="font-family:system-ui;max-width:32rem;margin:4rem auto;text-align:center">
<h2>%s</h2><p>%s</p><p style="color:#888">You can close this tab and go back to the terminal.</p></body>`

func (l *Listener) Listen(ctx context.Context) (string, error) {
	// 127.0.0.1 literally, not localhost: RFC 8252 section 8.3, and it keeps
	// the listener off every other interface.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	// Each Listen is a new attempt: a retry (the offline_access fallback) reuses
	// this Listener, and the last attempt's state must not linger in the
	// channel, where it would block the next Await forever.
	l.ln = ln
	l.state, l.result = make(chan string, 1), make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", l.handle)
	l.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = l.srv.Serve(ln) }()
	return fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port), nil
}

func (l *Listener) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The state arrives with Await. A browser faster than that simply waits
	// here for it; taking it and putting it back keeps it for a reload.
	var want string
	select {
	case want = <-l.state:
		l.state <- want
	case <-r.Context().Done():
		return
	}
	send := func(res result) {
		select {
		case l.result <- res:
		default: // a second hit must neither block nor overwrite the first
		}
	}
	if q.Get("state") != want {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, page, "Sign-in not accepted", "This response does not belong to the login this terminal started.")
		return
	}
	if e := q.Get("error"); e != "" {
		fmt.Fprintf(w, page, "Sign-in cancelled", html.EscapeString(q.Get("error_description")))
		send(result{err: &domain.OAuthError{Code: e, Description: q.Get("error_description")}})
		return
	}
	fmt.Fprintf(w, page, "Signed in to hive", "The terminal has what it needs.")
	send(result{code: q.Get("code")})
}

func (l *Listener) Await(ctx context.Context, state string) (string, error) {
	if l.ln == nil {
		return "", errors.New("loopback: Await before Listen")
	}
	l.state <- state
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-l.result:
		return r.code, r.err
	}
}

func (l *Listener) Close() error {
	if l.srv == nil {
		return nil
	}
	return l.srv.Close()
}
