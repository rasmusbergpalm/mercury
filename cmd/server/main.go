// Command server hosts Mercury games: the web client and the JSON API.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/rasmusbergpalm/mercury/internal/server"
	"github.com/rasmusbergpalm/mercury/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "saves", "directory for saved games (empty disables saving)")
	flag.Parse()
	s := server.New(web.Static, *data)
	if _, err := s.CreateGame("The Three Crowns", 1); err != nil {
		log.Fatal(err)
	}
	go s.Run(make(chan struct{}))
	log.Printf("Mercury on http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, s))
}
