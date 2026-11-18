package game

import "fmt"

// Event is something that happened, for the players' news feed.
type Event struct {
	Tick   int    `json:"tick"`
	Nation int    `json:"nation"` // concerned nation, or -1 for everyone
	Kind   string `json:"kind"`   // tech, built, port, war, peace, battle, siege, taken, lost, army, eliminated
	Region int    `json:"region"` // -1 if none
	Text   string `json:"text"`
}

// maxEvents is how many recent events a world keeps.
const maxEvents = 500

func (w *World) emit(nation int, kind string, region int, format string, args ...any) {
	w.Events = append(w.Events, Event{w.Tick, nation, kind, region, fmt.Sprintf(format, args...)})
	if len(w.Events) > maxEvents {
		w.Events = append([]Event(nil), w.Events[len(w.Events)-maxEvents:]...)
	}
}

// EventsSince lists events from tick on that concern a nation (or everyone);
// nation < 0 lists all.
func (w *World) EventsSince(tick, nation int) []Event {
	out := []Event{}
	for _, e := range w.Events {
		if e.Tick >= tick && (nation < 0 || e.Nation < 0 || e.Nation == nation) {
			out = append(out, e)
		}
	}
	return out
}
