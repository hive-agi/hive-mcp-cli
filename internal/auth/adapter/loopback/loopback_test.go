package loopback

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// One Listener serves consecutive attempts. The offline_access fallback signs
// in twice through the same port, and a leftover state once hung the second.
func TestAListenerServesASecondAttempt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	l := New()
	for i, state := range []string{"first", "second"} {
		redirect, err := l.Listen(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan string, 1)
		go func() { code, _ := l.Await(ctx, state); got <- code }()
		if _, err := http.Get(redirect + "?code=c" + state + "&state=" + state); err != nil {
			t.Fatal(err)
		}
		select {
		case code := <-got:
			if code != "c"+state {
				t.Fatalf("attempt %d: code %q", i, code)
			}
		case <-ctx.Done():
			t.Fatalf("attempt %d hung", i)
		}
		l.Close()
	}
}

func TestForgedStateIsRefusedAndTheRealRedirectLands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	l := New()
	redirect, err := l.Listen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan string, 1)
	go func() {
		code, _ := l.Await(ctx, "the-state")
		got <- code
	}()
	bad, err := http.Get(redirect + "?code=evil&state=forged")
	if err != nil || bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged: %v %v", bad, err)
	}
	good, err := http.Get(redirect + "?code=the-code&state=the-state")
	if err != nil || good.StatusCode != http.StatusOK {
		t.Fatalf("real: %v %v", good, err)
	}
	if code := <-got; code != "the-code" {
		t.Fatalf("code %q", code)
	}
	// A reload of the success page must not block or panic.
	if again, err := http.Get(redirect + "?code=the-code&state=the-state"); err != nil || again.StatusCode != http.StatusOK {
		t.Fatalf("reload: %v %v", again, err)
	}
}
