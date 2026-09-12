package fuel

import (
	"fmt"
	"math"
	"sync/atomic"
)

// Meter is a concurrency-safe fuel counter.
type Meter struct {
	remaining atomic.Int64
}

// New returns a Meter with the given initial budget.
func New(initial uint64) *Meter {
	m := &Meter{}
	m.Set(initial)
	return m
}

// Set replaces the budget with units.
func (m *Meter) Set(units uint64) { m.remaining.Store(toInt64(units)) }

// Add adds units to the budget.
func (m *Meter) Add(units uint64) { m.remaining.Add(toInt64(units)) }

// Remaining returns the current balance clamped to >= 0.
func (m *Meter) Remaining() uint64 {
	if r := m.remaining.Load(); r > 0 {
		return uint64(r)
	}
	return 0
}

// Consume deducts units from the budget and reports whether it had enough
// fuel. Units are deducted unconditionally.
func (m *Meter) Consume(units uint64) bool {
	return m.remaining.Add(-toInt64(units)) >= 0
}

func toInt64(units uint64) int64 {
	if units > math.MaxInt64 {
		panic(fmt.Sprintf("fuel: units %d exceed max supported value %d", units, uint64(math.MaxInt64)))
	}
	return int64(units)
}
