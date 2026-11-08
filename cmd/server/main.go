// Command server hosts Mercury games: the web client and the JSON API.
package main

import (
	"flag"
	"github.com/rasmusbergpalm/mercury/engine"
	"log"
	"net/http"
	"os"

	"github.com/rasmusbergpalm/mercury/internal/server"
	"github.com/rasmusbergpalm/mercury/web"
)

func main() {
	// hosts like Render say which port to listen on in $PORT, and where a
	// persistent disk is mounted in $MERCURY_DATA
	addr := flag.String("addr", envOr("PORT", ":8080", ":"), "listen address")
	data := flag.String("data", envOr("MERCURY_DATA", "saves", ""), "directory for saved games (empty disables saving)")
	flag.Parse()
	s := server.New(web.Static, *data)
	if s.NumGames() == 0 { // nothing to resume
		if _, err := s.CreateGame("The Three Crowns", 1, 0); err != nil {
			log.Fatal(err)
		}
	} else {
		// settle the map now (engine.SettleYears), so the first new game
		// doesn't wait for it
		go engine.BuildGame(engine.DefaultGame)
	}
	go s.Run(make(chan struct{}))
	log.Printf("Mercury on http://localhost%s (saves in %q)", *addr, *data)
	log.Fatal(http.ListenAndServe(*addr, s))
}

// envOr is prefix+$name if set, else fallback.
func envOr(name, fallback, prefix string) string {
	if v := os.Getenv(name); v != "" {
		return prefix + v
	}
	return fallback
}
