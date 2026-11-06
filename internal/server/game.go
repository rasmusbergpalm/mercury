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
	speed   float64 // days per second while running
	carry   float64 // fractional ticks owed by the clock

	// Turn-based games advance turnDays at once when every player has ended
	// the turn (real-time games have turnDays 0).
	turnDays int
	ready    map[int]bool // nations whose players have ended the turn
}

// Speeds the clock allows, in days per second.
var Speeds = []float64{1, 2, 3, 7, 15, 30}

// TurnLengths are the turn lengths (in days) a turn-based game may have; 0
// means real time.
var TurnLengths = []int{0, 7, 30, 90, 365}

// Errors from game commands.
var (
	ErrTaken      = errors.New("that nation already has a player")
	ErrNotPlayer  = errors.New("you don't control that nation")
	ErrNotInGame  = errors.New("only this game's players can do that")
	ErrBadSpeed   = errors.New("unsupported speed")
	ErrNoSuchSlot = errors.New("no such save")
	ErrTurnBased  = errors.New("this game is turn-based: end your turn instead")
	ErrRealTime   = errors.New("this game runs in real time")
)

func newGame(name string, w *engine.World, turnDays int) *Game {
	for _, n := range w.Nations {
		n.AI = true // the computer runs every nation until a player joins
	}
	return &Game{ID: randomHex(4), Name: name, Created: time.Now(), w: w,
		players: map[string]int{}, speed: 3, turnDays: turnDays, ready: map[int]bool{}}
}

// endTurn marks the player's nation ready. When every player is ready the
// game advances a turn. It reports whether it did.
func (g *Game) endTurn(token string) (bool, error) {
	if g.turnDays == 0 {
		return false, ErrRealTime
	}
	n := g.nationOf(token)
	if n < 0 {
		return false, ErrNotInGame
	}
	g.ready[n] = true
	for _, p := range g.players {
		if !g.ready[p] {
			return false, nil
		}
	}
	for i := 0; i < g.turnDays; i++ {
		g.w.Step()
	}
	g.ready = map[int]bool{}
	return true, nil
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
	delete(g.ready, nation)
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
	if !g.running || g.turnDays > 0 {
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
	Date    string          `json:"date"`
	Turn    int             `json:"turn_days"` // 0: real time
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
	s := GameSummary{ID: g.ID, Name: g.Name, Tick: g.w.Tick, Running: g.running, Speed: g.speed,
		Date: g.w.Date().Format("2 January 2006"), Turn: g.turnDays}
	for _, n := range g.w.Nations {
		c := "human"
		if n.AI {
			c = "ai"
		}
		s.Nations = append(s.Nations, NationSummary{n.ID, n.Name, n.Color, c, len(n.Regions), n.Eliminated()})
	}
	return s
}
