package moremath

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
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

	// Prevent constant folding by using two variables. -float32(0) is not actually negative.
	// https://github.com/golang/go/issues/2196
	zero := float32(0)
	negZero := -zero

	// Sign bit preserved for +/- zero
	require.False(t, math.Signbit(float64(zero)))
	require.False(t, math.Signbit(float64(WasmCompatNearestF32(zero))))
	require.True(t, math.Signbit(float64(negZero)))
	require.True(t, math.Signbit(float64(WasmCompatNearestF32(negZero))))
}

func TestWasmCompatNearestF64(t *testing.T) {
	require.Equal(t, WasmCompatNearestF64(-1.5), -2.0)

	// This is the diff from math.Round.
	require.Equal(t, WasmCompatNearestF64(-4.5), -4.0)
	require.Equal(t, math.Round(-4.5), -5.0)

	// Prevent constant folding by using two variables. -float64(0) is not actually negative.
	// https://github.com/golang/go/issues/2196
	zero := float64(0)
	negZero := -zero

	// Sign bit preserved for +/- zero
	require.False(t, math.Signbit(zero))
	require.False(t, math.Signbit(WasmCompatNearestF64(zero)))
	require.True(t, math.Signbit(negZero))
	require.True(t, math.Signbit(WasmCompatNearestF64(negZero)))
}
