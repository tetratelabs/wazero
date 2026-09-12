package fuel

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMeter_SetGet(t *testing.T) {
	m := New(100)
	require.Equal(t, uint64(100), m.Remaining())

	m.Set(50)
	require.Equal(t, uint64(50), m.Remaining())

	m.Set(0)
	require.Equal(t, uint64(0), m.Remaining())
}

func TestMeter_Add(t *testing.T) {
	m := New(10)

	m.Add(5)
	require.Equal(t, uint64(15), m.Remaining())

	// Add recovers a negative balance from prior over-consumption.
	require.False(t, m.Consume(100)) // 15 - 100 = -85
	require.Equal(t, uint64(0), m.Remaining())
	m.Add(100) // -85 + 100 = 15
	require.Equal(t, uint64(15), m.Remaining())
}

func TestMeter_ConsumeAlwaysDeducts(t *testing.T) {
	m := New(5)

	require.True(t, m.Consume(3))
	require.Equal(t, uint64(2), m.Remaining())

	require.False(t, m.Consume(10))
	require.Equal(t, uint64(0), m.Remaining())

	m.Set(100)
	require.Equal(t, uint64(100), m.Remaining())
}

func TestMeter_RejectsOverflow(t *testing.T) {
	err := require.CapturePanic(func() {
		New(math.MaxInt64 + 1)
	})
	require.EqualError(t, err, "fuel: units 9223372036854775808 exceed max supported value 9223372036854775807")

	m := New(0)

	err = require.CapturePanic(func() {
		m.Set(math.MaxInt64 + 1)
	})
	require.EqualError(t, err, "fuel: units 9223372036854775808 exceed max supported value 9223372036854775807")

	err = require.CapturePanic(func() {
		m.Consume(math.MaxInt64 + 1)
	})
	require.EqualError(t, err, "fuel: units 9223372036854775808 exceed max supported value 9223372036854775807")
}
