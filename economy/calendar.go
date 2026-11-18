package economy

import "time"

// The calendar: one tick is one day, and games start on 1 January 1820.
// Years have 365 days (no leap days), so a year is always TicksPerYear ticks.
const (
	TicksPerYear = 365.0
	StartYear    = 1820
)

// Date is the calendar date of a tick.
func Date(tick int) time.Time {
	year, day := StartYear+tick/365, tick%365
	leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
	if leap && day >= 59 { // skip 29 February
		day++
	}
	return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day)
}

// Date is the world's current date.
func (w *World) Date() time.Time { return Date(w.Tick) }

// Year is the world's current year, with a fraction (1820.5 is early July 1820).
func (w *World) Year() float64 { return StartYear + float64(w.Tick)/TicksPerYear }
