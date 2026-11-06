// Command server runs a Mercury world and serves the web viewer and JSON API.
package main

import (
	"flag"
	"log"
	"math"
	"net/http"

	"github.com/rasmusbergpalm/mercury/internal/server"
	"github.com/rasmusbergpalm/mercury/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	cost := flag.Float64("cost", 1, "transport cost multiplier (negative = autarky)")
	flag.Parse()
	c := *cost
	if c < 0 {
		c = math.Inf(1)
	}
	s, err := server.New(c, web.Static)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Mercury on http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, s))
}
