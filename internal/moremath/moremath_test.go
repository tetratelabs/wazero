package moremath

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWasmCompatMin(t *testing.T) {
	require.Equal(t, WasmCompatMin(-1.1, 123), -1.1)
	require.Equal(t, WasmCompatMin(-1.1, math.Inf(1)), -1.1)
	require.Equal(t, WasmCompatMin(math.Inf(-1), 123), math.Inf(-1))

	// NaN cannot be compared with themselves, so we have to use IsNaN
	require.True(t, math.IsNaN(WasmCompatMin(math.NaN(), 1.0)))
	require.True(t, math.IsNaN(WasmCompatMin(1.0, math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMin(math.Inf(-1), math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMin(math.Inf(1), math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMin(math.NaN(), math.NaN())))
}

func TestWasmCompatMax(t *testing.T) {
	require.Equal(t, WasmCompatMax(-1.1, 123.1), 123.1)
	require.Equal(t, WasmCompatMax(-1.1, math.Inf(1)), math.Inf(1))
	require.Equal(t, WasmCompatMax(math.Inf(-1), 123.1), 123.1)

	// NaN cannot be compared with themselves, so we have to use IsNaN
	require.True(t, math.IsNaN(WasmCompatMax(math.NaN(), 1.0)))
	require.True(t, math.IsNaN(WasmCompatMax(1.0, math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMax(math.Inf(-1), math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMax(math.Inf(1), math.NaN())))
	require.True(t, math.IsNaN(WasmCompatMax(math.NaN(), math.NaN())))
}

func TestWasmCompatNearestF32(t *testing.T) {
	require.Equal(t, WasmCompatNearestF32(-1.5), float32(-2.0))

	// This is the diff from math.Round.
	require.Equal(t, WasmCompatNearestF32(-4.5), float32(-4.0))
	require.Equal(t, float32(math.Round(-4.5)), float32(-5.0))
}

func TestWasmCompatNearestF64(t *testing.T) {
	require.Equal(t, WasmCompatNearestF64(-1.5), -2.0)

	// This is the diff from math.Round.
	require.Equal(t, WasmCompatNearestF64(-4.5), -4.0)
	require.Equal(t, math.Round(-4.5), -5.0)
}
