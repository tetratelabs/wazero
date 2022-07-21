package moremath

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var (
	canonicalF32  = math.Float32frombits(F32CanonicalNaNBits)
	arithmeticF32 = math.Float32frombits(F32ArithmeticNaNBits)
	canonicalF64  = math.Float64frombits(F64CanonicalNaNBits)
	arithmeticF64 = math.Float64frombits(F64ArithmeticNaNBits)
)

func f32EqualBit(t *testing.T, f1, f2 float32) {
	require.Equal(t, math.Float32bits(f1), math.Float32bits(f2))
}

func f64EqualBit(t *testing.T, f1, f2 float64) {
	require.Equal(t, math.Float64bits(f1), math.Float64bits(f2))
}

func TestWasmCompatMin32(t *testing.T) {
	require.Equal(t, WasmCompatMin32(-1.1, 123), float32(-1.1))
	require.Equal(t, WasmCompatMin32(-1.1, float32(math.Inf(1))), float32(-1.1))
	require.Equal(t, WasmCompatMin32(float32(math.Inf(-1)), 123), float32(math.Inf(-1)))

	f32EqualBit(t, canonicalF32, WasmCompatMin32(canonicalF32, canonicalF32))
	f32EqualBit(t, canonicalF32, WasmCompatMin32(canonicalF32, arithmeticF32))
	f32EqualBit(t, canonicalF32, WasmCompatMin32(canonicalF32, 1.0))
	f32EqualBit(t, arithmeticF32, WasmCompatMin32(1.0, arithmeticF32))
	f32EqualBit(t, arithmeticF32, WasmCompatMin32(arithmeticF32, arithmeticF32))
}

func TestWasmCompatMin64(t *testing.T) {
	require.Equal(t, WasmCompatMin64(-1.1, 123), -1.1)
	require.Equal(t, WasmCompatMin64(-1.1, math.Inf(1)), -1.1)
	require.Equal(t, WasmCompatMin64(math.Inf(-1), 123), math.Inf(-1))

	f64EqualBit(t, canonicalF64, WasmCompatMin64(canonicalF64, canonicalF64))
	f64EqualBit(t, canonicalF64, WasmCompatMin64(canonicalF64, arithmeticF64))
	f64EqualBit(t, canonicalF64, WasmCompatMin64(canonicalF64, 1.0))
	f64EqualBit(t, arithmeticF64, WasmCompatMin64(1.0, arithmeticF64))
	f64EqualBit(t, arithmeticF64, WasmCompatMin64(arithmeticF64, arithmeticF64))
}

func TestWasmCompatMax32(t *testing.T) {
	require.Equal(t, WasmCompatMax32(-1.1, 123), float32(123))
	require.Equal(t, WasmCompatMax32(-1.1, float32(math.Inf(1))), float32(math.Inf(1)))
	require.Equal(t, WasmCompatMax32(float32(math.Inf(-1)), 123), float32(123))

	f32EqualBit(t, canonicalF32, WasmCompatMax32(canonicalF32, canonicalF32))
	f32EqualBit(t, canonicalF32, WasmCompatMax32(canonicalF32, arithmeticF32))
	f32EqualBit(t, canonicalF32, WasmCompatMax32(canonicalF32, 1.0))
	f32EqualBit(t, arithmeticF32, WasmCompatMax32(1.0, arithmeticF32))
	f32EqualBit(t, arithmeticF32, WasmCompatMax32(arithmeticF32, arithmeticF32))
}

func TestWasmCompatMax64(t *testing.T) {
	require.Equal(t, WasmCompatMax64(-1.1, 123.1), 123.1)
	require.Equal(t, WasmCompatMax64(-1.1, math.Inf(1)), math.Inf(1))
	require.Equal(t, WasmCompatMax64(math.Inf(-1), 123.1), 123.1)

	f64EqualBit(t, canonicalF64, WasmCompatMax64(canonicalF64, canonicalF64))
	f64EqualBit(t, canonicalF64, WasmCompatMax64(canonicalF64, arithmeticF64))
	f64EqualBit(t, canonicalF64, WasmCompatMax64(canonicalF64, 1.0))
	f64EqualBit(t, arithmeticF64, WasmCompatMax64(1.0, arithmeticF64))
	f64EqualBit(t, arithmeticF64, WasmCompatMax64(arithmeticF64, arithmeticF64))
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

func TestUniOp_NaNPropagation(t *testing.T) {
	tests := []struct {
		name string
		f32  func(f float32) float32
		f64  func(f float64) float64
	}{
		{name: "trunc.f32", f32: WasmCompatTruncF32},
		{name: "trunc.f64", f64: WasmCompatTruncF64},
		{name: "nearest.f32", f32: WasmCompatNearestF32},
		{name: "nearest.f64", f64: WasmCompatNearestF64},
		{name: "ceil.f32", f32: WasmCompatCeilF32},
		{name: "ceil.f64", f64: WasmCompatCeilF64},
		{name: "floor.f32", f32: WasmCompatFloorF32},
		{name: "floor.f64", f64: WasmCompatFloorF64},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.f32 != nil {
				f32EqualBit(t, canonicalF32, tc.f32(canonicalF32))
				f32EqualBit(t, arithmeticF32, tc.f32(arithmeticF32))
			} else {
				f64EqualBit(t, canonicalF64, tc.f64(canonicalF64))
				f64EqualBit(t, arithmeticF64, tc.f64(arithmeticF64))
			}
		})
	}
}

func Test_returnF32UniOp(t *testing.T) {
	for _, tc := range []struct {
		original, result, exp uint32
	}{
		{result: math.Float32bits(1.1), exp: math.Float32bits(1.1)},
		{original: 1.0, result: math.Float32bits(float32(math.NaN())), exp: F32CanonicalNaNBits},
		{original: F32ArithmeticNaNBits, result: math.Float32bits(float32(math.NaN())), exp: F32ArithmeticNaNBits},
		// Even if the MSB of the payload is unset (signaling NaN), the result must it set, therefore an arithmetic NaN.
		{original: F32ArithmeticNaNBits ^ (1 << 22), result: math.Float32bits(float32(math.NaN())), exp: F32ArithmeticNaNBits},
	} {
		actual := returnF32UniOp(math.Float32frombits(tc.original), math.Float32frombits(tc.result))
		require.Equal(t, tc.exp, math.Float32bits(actual))
	}
}

func Test_returnF64UniOp(t *testing.T) {
	for _, tc := range []struct {
		original, result, exp uint64
	}{
		{result: math.Float64bits(1.1), exp: math.Float64bits(1.1)},
		{original: 1.0, result: math.Float64bits(math.NaN()), exp: F64CanonicalNaNBits},
		{original: F64ArithmeticNaNBits, result: math.Float64bits(math.NaN()), exp: F64ArithmeticNaNBits},
		// Even if the MSB of the payload is unset (signaling NaN), the result must it set, therefore an arithmetic NaN.
		{original: F64ArithmeticNaNBits ^ (1 << 51), result: math.Float64bits(math.NaN()), exp: F64ArithmeticNaNBits},
	} {
		actual := returnF64UniOp(math.Float64frombits(tc.original), math.Float64frombits(tc.result))
		require.Equal(t, tc.exp, math.Float64bits(actual))
	}
}

func Test_returnF32NaNBinOp(t *testing.T) {
	for _, tc := range []struct {
		x, y, exp uint32
	}{
		{x: F32CanonicalNaNBits, y: F32CanonicalNaNBits, exp: F32CanonicalNaNBits},
		{x: F32CanonicalNaNBits, y: 0, exp: F32CanonicalNaNBits},
		{x: 0, y: F32CanonicalNaNBits, exp: F32CanonicalNaNBits},
		{x: F32ArithmeticNaNBits, y: F32ArithmeticNaNBits, exp: F32ArithmeticNaNBits},
		{x: F32ArithmeticNaNBits, y: 0, exp: F32ArithmeticNaNBits},
		{x: 0, y: F32ArithmeticNaNBits, exp: F32ArithmeticNaNBits},
		// Even if the MSB of the payload is unset (signaling NaN), the result must it set, therefore an arithmetic NaN.
		{x: 0, y: F32ArithmeticNaNBits ^ (1 << 22), exp: F32ArithmeticNaNBits},
		{x: F32ArithmeticNaNBits ^ (1 << 22), y: 0, exp: F32ArithmeticNaNBits},
	} {
		actual := returnF32NaNBinOp(math.Float32frombits(tc.x), math.Float32frombits(tc.y))
		require.Equal(t, tc.exp, math.Float32bits(actual))
	}
}

func Test_returnF64NaNBinOp(t *testing.T) {
	for _, tc := range []struct {
		x, y, exp uint64
	}{
		{x: F64CanonicalNaNBits, y: F64CanonicalNaNBits, exp: F64CanonicalNaNBits},
		{x: F64CanonicalNaNBits, y: 0, exp: F64CanonicalNaNBits},
		{x: 0, y: F64CanonicalNaNBits, exp: F64CanonicalNaNBits},
		{x: F64ArithmeticNaNBits, y: F64ArithmeticNaNBits, exp: F64ArithmeticNaNBits},
		{x: F64ArithmeticNaNBits, y: 0, exp: F64ArithmeticNaNBits},
		{x: 0, y: F64ArithmeticNaNBits, exp: F64ArithmeticNaNBits},
		// Even if the MSB of the payload is unset (signaling NaN), the result must it set, therefore an arithmetic NaN.
		{x: 0, y: F64ArithmeticNaNBits ^ (1 << 51), exp: F64ArithmeticNaNBits},
		{x: F64ArithmeticNaNBits ^ (1 << 51), y: 0, exp: F64ArithmeticNaNBits},
	} {
		actual := returnF64NaNBinOp(math.Float64frombits(tc.x), math.Float64frombits(tc.y))
		require.Equal(t, tc.exp, math.Float64bits(actual))
	}
}
