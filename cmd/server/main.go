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
	// the classic map ships settled, so its games open in a second or two
	// instead of settling it first (engine.SettleYears)
	if err := preloadSettled(); err != nil {
		log.Printf("the shipped settled map doesn't fit this build (%v): games will settle it", err)
		go engine.BuildGame(settledSpec())
	}
	s := server.New(web.Static, *data)
	if s.NumGames() == 0 { // nothing to resume: open the default game, shipped ready to play
		w, err := shippedThreeCrowns()
		if err == nil {
			_, err = s.AddGame("The Three Crowns", w, 0)
		}
		if err != nil {
			log.Printf("the shipped game doesn't fit this build (%v): building it", err)
			go func() {
				if _, err := s.CreateGame("The Three Crowns", classicCost, 0); err != nil {
					log.Fatal(err)
				}
			}()
		}
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
