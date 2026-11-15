package main

import (
	"errors"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/rasmusbergpalm/mercury/internal/server"
	"github.com/rasmusbergpalm/mercury/web"
)

// Regression: a spectator can watch a game. The world chart built its lines
// before every nation's history had loaded, and the page threw on opening
// a game ("Cannot read properties of null"), sending the spectator back to
// the lobby. This serves the shipped Three Crowns and has a browser press
// Watch and go through every tab and chart (web/e2e/watch.js); it is
// skipped where Node or Playwright isn't installed.
func TestWatchGame(t *testing.T) {
	if testing.Short() {
		t.Skip("drives a browser")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	w, err := shippedThreeCrowns()
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(web.Static, "")
	if _, err := s.AddGame("The Three Crowns", w, 0); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s)
	defer ts.Close()
	cmd := exec.Command(node, "../../web/e2e/watch.js", ts.URL)
	cmd.Env = append(cmd.Environ(), "NO_PROXY=*", "no_proxy=*")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 3 {
		t.Skip("playwright not installed")
	}
	if err != nil {
		t.Fatalf("watching a game failed: %v\n%s", err, out)
	}
}
