package system

import (
	"reflect"
	"testing"
)

func TestOpenersHonourBrowserListThenFallBack(t *testing.T) {
	got := openers("chromium:firefox", "linux")
	want := [][]string{{"chromium"}, {"firefox"}, {"xdg-open"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := openers("", "darwin"); !reflect.DeepEqual(got, [][]string{{"open"}}) {
		t.Fatalf("darwin: %v", got)
	}
}

// BROWSER=chromium on a machine with only Firefox made hive login say "could
// not open a browser" (2026-09-24). A missing $BROWSER must fall through.
func TestMissingBrowserFallsThroughToThePlatformOpener(t *testing.T) {
	t.Setenv("BROWSER", "definitely-not-installed-browser")
	t.Setenv("PATH", t.TempDir()) // nothing installed at all
	if c := firstOpener(); c != nil {
		t.Fatalf("nothing is installed, got %v", c)
	}
}
