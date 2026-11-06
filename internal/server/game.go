package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/rasmusbergpalm/mercury/engine"
)

// Game is one running world with its players and clock.
type Game struct {
	ID      string
	Name    string
	Created time.Time

	mu      sync.Mutex
	w       *engine.World
	players map[string]int // token -> nation
	running bool
	speed   float64 // ticks per second while running
	carry   float64 // fractional ticks owed by the clock
}

// Speeds the clock allows, in ticks per second.
var Speeds = []float64{1, 2, 5, 10, 20, 50}

// Errors from game commands.
var (
	ErrTaken      = errors.New("that nation already has a player")
	ErrNotPlayer  = errors.New("you don't control that nation")
	ErrNotInGame  = errors.New("only this game's players can do that")
	ErrBadSpeed   = errors.New("unsupported speed")
	ErrNoSuchSlot = errors.New("no such save")
)

func newGame(name string, w *engine.World) *Game {
	for _, n := range w.Nations {
		n.AI = true // the computer runs every nation until a player joins
	}
	return &Game{ID: randomHex(4), Name: name, Created: time.Now(), w: w,
		players: map[string]int{}, speed: 5}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// join hands a computer-run nation to a new player and returns their token.
func (g *Game) join(nation int) (string, error) {
	n := g.w.Nations[nation]
	if !n.AI {
		return "", ErrTaken
	}
	if n.Eliminated() {
		return "", engine.ErrEliminated
	}
	token := randomHex(16)
	g.players[token] = nation
	n.AI = false
	return token, nil
}

// leave hands the player's nation back to the computer.
func (g *Game) leave(token string) error {
	nation, ok := g.players[token]
	if !ok {
		return ErrNotInGame
	}
	delete(g.players, token)
	g.w.Nations[nation].AI = true
	return nil
}

// nationOf is the nation a token controls, or -1.
func (g *Game) nationOf(token string) int {
	if n, ok := g.players[token]; ok {
		return n
	}
	return -1
}

// mayControlClock: anyone in a game without players, else only its players.
func (g *Game) mayControlClock(token string) bool {
	return len(g.players) == 0 || g.nationOf(token) >= 0
}

// advance runs the ticks a running clock owes for elapsed seconds.
func (g *Game) advance(seconds float64) {
	if !g.running {
		return
	}
	g.carry += g.speed * seconds
	for ; g.carry >= 1; g.carry-- {
		g.w.Step()
	}
}

// GameSummary is a game in the lobby.
type GameSummary struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Tick    int             `json:"tick"`
	Running bool            `json:"running"`
	Speed   float64         `json:"speed"`
	Nations []NationSummary `json:"nations"`
}

type NationSummary struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Controller string `json:"controller"`
	Tiles      int    `json:"tiles"`
	Eliminated bool   `json:"eliminated"`
}

func (g *Game) summary() GameSummary {
	s := GameSummary{ID: g.ID, Name: g.Name, Tick: g.w.Tick, Running: g.running, Speed: g.speed}
	for _, n := range g.w.Nations {
		c := "human"
		if n.AI {
			c = "ai"
		}
		s.Nations = append(s.Nations, NationSummary{n.ID, n.Name, n.Color, c, len(n.Regions), n.Eliminated()})
	}
	return s
}
