// Package clock implements ports.Clock. Production uses System; tests use
// Fixed for deterministic state-machine behaviour.
package clock

import "time"

// System returns time.Now().UTC().
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed returns a constant time. Useful for table-driven tests.
type Fixed struct{ T time.Time }

func (f Fixed) Now() time.Time { return f.T }

// Advance is a mutable test clock that allows controlled passage of time.
type Advance struct {
	now time.Time
}

func NewAdvance(start time.Time) *Advance       { return &Advance{now: start} }
func (a *Advance) Now() time.Time                { return a.now }
func (a *Advance) Set(t time.Time)               { a.now = t }
func (a *Advance) Add(d time.Duration)           { a.now = a.now.Add(d) }
