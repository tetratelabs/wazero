package spectest

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_f32Equal(t *testing.T) {
	tests := []struct {
		f1, f2 float32
		exp    bool
	}{
		{f1: 1.1, f2: 1.1, exp: true},
		{f1: float32(math.NaN()), f2: float32(math.NaN()), exp: true},
		{f1: float32(math.Inf(1)), f2: float32(math.Inf(1)), exp: true},
		{f1: float32(math.Inf(-1)), f2: float32(math.Inf(-1)), exp: true},
		{f1: 1.1, f2: -1.1, exp: false},
		{f1: float32(math.NaN()), f2: -1.1, exp: false},
		{f1: -1.1, f2: float32(math.NaN()), exp: false},
		{f1: float32(math.NaN()), f2: float32(math.Inf(1)), exp: false},
		{f1: float32(math.Inf(1)), f2: float32(math.NaN()), exp: false},
		{f1: float32(math.NaN()), f2: float32(math.Inf(-1)), exp: false},
		{f1: float32(math.Inf(-1)), f2: float32(math.NaN()), exp: false},
		{
			f1:  math.Float32frombits(moremath.F32CanonicalNaNBits),
			f2:  math.Float32frombits(moremath.F32CanonicalNaNBits),
			exp: true,
		},
		{
			f1:  math.Float32frombits(moremath.F32CanonicalNaNBits),
			f2:  math.Float32frombits(moremath.F32ArithmeticNaNBits),
			exp: false,
		},
		{
			f1:  math.Float32frombits(moremath.F32ArithmeticNaNBits),
			f2:  math.Float32frombits(moremath.F32ArithmeticNaNBits),
			exp: true,
		},
		{
			f1: math.Float32frombits(moremath.F32ArithmeticNaNBits),
			f2: math.Float32frombits(moremath.F32ArithmeticNaNBits | 1<<2),
			// The Wasm spec doesn't differentiate different arithmetic nans.
			exp: true,
		},
		{
			f1: math.Float32frombits(moremath.F32CanonicalNaNBits),
			f2: math.Float32frombits(moremath.F32CanonicalNaNBits | 1<<2),
			// Canonical NaN is unique.
			exp: false,
		},
	}

	for i, tc := range tests {
		require.Equal(t, tc.exp, f32Equal(tc.f1, tc.f2), i)
	}
}

func Test_f64Equal(t *testing.T) {
	tests := []struct {
		f1, f2 float64
		exp    bool
	}{
		{f1: 1.1, f2: 1.1, exp: true},
		{f1: math.NaN(), f2: math.NaN(), exp: true},
		{f1: math.Inf(1), f2: math.Inf(1), exp: true},
		{f1: math.Inf(-1), f2: math.Inf(-1), exp: true},
		{f1: 1.1, f2: -1.1, exp: false},
		{f1: math.NaN(), f2: -1.1, exp: false},
		{f1: -1.1, f2: math.NaN(), exp: false},
		{f1: math.NaN(), f2: math.Inf(1), exp: false},
		{f1: math.Inf(1), f2: math.NaN(), exp: false},
		{f1: math.NaN(), f2: math.Inf(-1), exp: false},
		{f1: math.Inf(-1), f2: math.NaN(), exp: false},
		{
			f1:  math.Float64frombits(moremath.F64CanonicalNaNBits),
			f2:  math.Float64frombits(moremath.F64CanonicalNaNBits),
			exp: true,
		},
		{
			f1:  math.Float64frombits(moremath.F64CanonicalNaNBits),
			f2:  math.Float64frombits(moremath.F64ArithmeticNaNBits),
			exp: false,
		},
		{
			f1:  math.Float64frombits(moremath.F64ArithmeticNaNBits),
			f2:  math.Float64frombits(moremath.F64ArithmeticNaNBits),
			exp: true,
		},
		{
			f1: math.Float64frombits(moremath.F64ArithmeticNaNBits),
			f2: math.Float64frombits(moremath.F64ArithmeticNaNBits | 1<<2),
			// The Wasm spec doesn't differentiate different arithmetic nans.
			exp: true,
		},
		{
			f1: math.Float64frombits(moremath.F64CanonicalNaNBits),
			f2: math.Float64frombits(moremath.F64CanonicalNaNBits | 1<<2),
			// Canonical NaN is unique.
			exp: false,
		},
	}

	for i, tc := range tests {
		require.Equal(t, tc.exp, f64Equal(tc.f1, tc.f2), i)
	}
}

func Test_valuesEq(t *testing.T) {
	i32, i64, f32, f64, v128 := wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeV128
	tests := []struct {
		name         string
		exps, actual []uint64
		valueTypes   []wasm.ValueType
		laneTypes    map[int]laneType
		expMatched   bool
		expValuesMsg string
	}{
		{
			name:       "matched/i32",
			exps:       []uint64{0},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{i32},
			expMatched: true,
		},
		{
			name:       "unmatched/i32",
			exps:       []uint64{1},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{i32},
			expMatched: false,
			expValuesMsg: `	have [0]
	want [1]`,
		},
		{
			name:       "unmatched/i32",
			exps:       []uint64{math.MaxUint32},
			actual:     []uint64{1123},
			valueTypes: []wasm.ValueType{i32},
			expMatched: false,
			expValuesMsg: `	have [1123]
	want [4294967295]`,
		},
		{
			name:       "matched/i64",
			exps:       []uint64{0},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{i64},
			expMatched: true,
		},
		{
			name:       "unmatched/i64",
			exps:       []uint64{1},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{i64},
			expMatched: false,
			expValuesMsg: `	have [0]
	want [1]`,
		},
		{
			name:       "unmatched/i64",
			exps:       []uint64{math.MaxUint64},
			actual:     []uint64{1123},
			valueTypes: []wasm.ValueType{i64},
			expMatched: false,
			expValuesMsg: `	have [1123]
	want [18446744073709551615]`,
		},
		{
			name:       "matched/f32",
			exps:       []uint64{0},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{f32},
			expMatched: true,
		},
		{
			name:       "unmatched/f32",
			exps:       []uint64{uint64(math.Float32bits(-13123.1))},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{f32},
			expMatched: false,
			expValuesMsg: `	have [0.000000]
	want [-13123.099609]`,
		},
		{
			name:       "matched/f64",
			exps:       []uint64{0},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{f64},
			expMatched: true,
		},
		{
			name:       "unmatched/f64",
			exps:       []uint64{math.Float64bits(1.0)},
			actual:     []uint64{0},
			valueTypes: []wasm.ValueType{f64},
			expMatched: false,
			expValuesMsg: `	have [0.000000]
	want [1.000000]`,
		},
		{
			name:       "unmatched/f64",
			actual:     []uint64{math.Float64bits(-1231231.0)},
			exps:       []uint64{0},
			valueTypes: []wasm.ValueType{f64},
			expMatched: false,
			expValuesMsg: `	have [-1231231.000000]
	want [0.000000]`,
		},
		{
			name:       "matched/i8x16",
			exps:       []uint64{math.MaxUint64, 123},
			actual:     []uint64{math.MaxUint64, 123},
			laneTypes:  map[int]laneType{0: laneTypeI8},
			valueTypes: []wasm.ValueType{v128},
			expMatched: true,
		},
		{
			name:       "unmatched/i8x16",
			exps:       []uint64{0, 0xff<<56 | 0xaa},
			actual:     []uint64{math.MaxUint64, 0xff<<48 | 0xcc},
			laneTypes:  map[int]laneType{0: laneTypeI8},
			valueTypes: []wasm.ValueType{v128},
			expMatched: false,
			expValuesMsg: `	have [i8x16(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xcc, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0x0)]
	want [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xaa, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff)]`,
		},
		{
			name:       "matched/i16x8",
			exps:       []uint64{math.MaxUint64, 123},
			actual:     []uint64{math.MaxUint64, 123},
			laneTypes:  map[int]laneType{0: laneTypeI16},
			valueTypes: []wasm.ValueType{v128},
			expMatched: true,
		},
		{
			name:       "unmatched/i16x8",
			exps:       []uint64{0xffff << 32, 0},
			actual:     []uint64{0xaabb << 16, ^uint64(0)},
			laneTypes:  map[int]laneType{0: laneTypeI16},
			valueTypes: []wasm.ValueType{v128},
			expMatched: false,
			expValuesMsg: `	have [i16x8(0x0, 0xaabb, 0x0, 0x0, 0xffff, 0xffff, 0xffff, 0xffff)]
	want [i16x8(0x0, 0x0, 0xffff, 0x0, 0x0, 0x0, 0x0, 0x0)]`,
		},
		{
			name:       "matched/i32x4",
			exps:       []uint64{math.MaxUint64, 123},
			actual:     []uint64{math.MaxUint64, 123},
			laneTypes:  map[int]laneType{0: laneTypeI32},
			valueTypes: []wasm.ValueType{v128},
			expMatched: true,
		},
		{
			name:       "matched/i32x4",
			exps:       []uint64{0xffff_ffff<<32 | 0xa, 123},
			actual:     []uint64{0x1a1a_1a1a<<32 | 0xa, 123},
			laneTypes:  map[int]laneType{0: laneTypeI32},
			valueTypes: []wasm.ValueType{v128},
			expMatched: false,
			expValuesMsg: `	have [i32x4(0xa, 0x1a1a1a1a, 0x7b, 0x0)]
	want [i32x4(0xa, 0xffffffff, 0x7b, 0x0)]`,
		},
		{
			name:       "matched/i64x2",
			exps:       []uint64{math.MaxUint64, 123},
			actual:     []uint64{math.MaxUint64, 123},
			laneTypes:  map[int]laneType{0: laneTypeI64},
			valueTypes: []wasm.ValueType{v128},
			expMatched: true,
		},
		{
			name:       "unmatched/i64x2",
			exps:       []uint64{math.MaxUint64, 123},
			actual:     []uint64{math.MaxUint64, 0},
			laneTypes:  map[int]laneType{0: laneTypeI64},
			valueTypes: []wasm.ValueType{v128},
			expMatched: false,
			expValuesMsg: `	have [i64x2(0xffffffffffffffff, 0x0)]
	want [i64x2(0xffffffffffffffff, 0x7b)]`,
		},
		{
			name: "matched/f32x4",
			exps: []uint64{
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
			},
			actual: []uint64{
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
			},
			valueTypes: []wasm.ValueType{v128},
			laneTypes:  map[int]laneType{0: laneTypeF32},
			expMatched: true,
		},
		{
			name: "unmatched/f32x4",
			exps: []uint64{
				(uint64(math.Float32bits(float32(1.213))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
			},
			actual: []uint64{
				(uint64(math.Float32bits(float32(math.NaN()))) << 32) | uint64(math.Float32bits(float32(math.Inf(1)))),
				(uint64(math.Float32bits(float32(math.Inf(-1)))) << 32) | uint64(math.Float32bits(float32(math.NaN()))),
			},
			valueTypes: []wasm.ValueType{v128},
			laneTypes:  map[int]laneType{0: laneTypeF32},
			expMatched: false,
			expValuesMsg: `	have [f32x4(+Inf, NaN, NaN, -Inf)]
	want [f32x4(NaN, 1.213000, NaN, NaN)]`,
		},
		{
			name:       "matched/f64x2",
			exps:       []uint64{math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128},
			laneTypes:  map[int]laneType{0: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "unmatched/f64x2",
			exps:       []uint64{math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{math.Float64bits(-1.0), math.Float64bits(math.Inf(1))},
			valueTypes: []wasm.ValueType{v128},
			laneTypes:  map[int]laneType{0: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [f64x2(-1.000000, +Inf)]
	want [f64x2(1.000000, NaN)]`,
		},
		{
			name:       "unmatched/f64x2",
			exps:       []uint64{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
			actual:     []uint64{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128},
			laneTypes:  map[int]laneType{0: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [f64x2(-Inf, NaN)]
	want [f64x2(+Inf, NaN)]`,
		},
		{
			name:       "matched/[i32,f64x2]",
			exps:       []uint64{1, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{1, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{i32, v128},
			laneTypes:  map[int]laneType{1: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "unmatched/[i32,f64x2]",
			exps:       []uint64{123, math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
			actual:     []uint64{123, math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{i32, v128},
			laneTypes:  map[int]laneType{1: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [123, f64x2(-Inf, NaN)]
	want [123, f64x2(+Inf, NaN)]`,
		},
		{
			name:       "matched/[i32,f64x2]",
			exps:       []uint64{math.Float64bits(1.0), math.Float64bits(math.NaN()), 1},
			actual:     []uint64{math.Float64bits(1.0), math.Float64bits(math.NaN()), 1},
			valueTypes: []wasm.ValueType{v128, i32},
			laneTypes:  map[int]laneType{0: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "unmatched/[f64x2,i32]",
			exps:       []uint64{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN()), 123},
			actual:     []uint64{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN()), 123},
			valueTypes: []wasm.ValueType{v128, i32},
			laneTypes:  map[int]laneType{0: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [f64x2(-Inf, NaN), 123]
	want [f64x2(+Inf, NaN), 123]`,
		},
		{
			name:       "matched/[f32,i32,f64x2]",
			exps:       []uint64{uint64(math.Float32bits(float32(math.NaN()))), math.Float64bits(1.0), math.Float64bits(math.NaN()), 1},
			actual:     []uint64{uint64(math.Float32bits(float32(math.NaN()))), math.Float64bits(1.0), math.Float64bits(math.NaN()), 1},
			valueTypes: []wasm.ValueType{f32, v128, i32},
			laneTypes:  map[int]laneType{1: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "unmatched/[f32,f64x2,i32]",
			exps:       []uint64{uint64(math.Float32bits(1.0)), math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN()), 123},
			actual:     []uint64{uint64(math.Float32bits(1.0)), math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN()), 123},
			valueTypes: []wasm.ValueType{f32, v128, i32},
			laneTypes:  map[int]laneType{1: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [1.000000, f64x2(-Inf, NaN), 123]
	want [1.000000, f64x2(+Inf, NaN), 123]`,
		},
		{
			name:       "matched/[i8x16,f64x2]",
			exps:       []uint64{0, 0, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 1: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "unmatched/[i8x16,f64x2]",
			exps:       []uint64{0, 0xff << 56, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0xaa << 56, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 1: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xaa), f64x2(1.000000, NaN)]
	want [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff), f64x2(1.000000, NaN)]`,
		},
		{
			name:       "unmatched/[i8x16,f64x2]",
			exps:       []uint64{0, 0xff << 56, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0xff << 56, math.Float64bits(1.0), math.Float64bits(math.Inf(1))},
			valueTypes: []wasm.ValueType{v128, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 1: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff), f64x2(1.000000, +Inf)]
	want [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff), f64x2(1.000000, NaN)]`,
		},
		{
			name:       "matched/[i8x16,i32,f64x2]",
			exps:       []uint64{0, 0, math.MaxUint32, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0, math.MaxUint32, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128, i32, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 2: laneTypeF64},
			expMatched: true,
		},
		{
			name:       "matched/[i8x16,i32,f64x2]",
			exps:       []uint64{0, 0, math.MaxUint32, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0, math.MaxUint32 - 1, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128, i32, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 2: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0), 4294967294, f64x2(1.000000, NaN)]
	want [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0), 4294967295, f64x2(1.000000, NaN)]`,
		},
		{
			name:       "matched/[i8x16,i32,f64x2]",
			exps:       []uint64{0, 0, math.MaxUint32, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			actual:     []uint64{0, 0xff << 16, math.MaxUint32, math.Float64bits(1.0), math.Float64bits(math.NaN())},
			valueTypes: []wasm.ValueType{v128, i32, v128},
			laneTypes:  map[int]laneType{0: laneTypeI8, 2: laneTypeF64},
			expMatched: false,
			expValuesMsg: `	have [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0), 4294967295, f64x2(1.000000, NaN)]
	want [i8x16(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0), 4294967295, f64x2(1.000000, NaN)]`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actualMatched, actualValuesMsg := valuesEq(tc.actual, tc.exps, tc.valueTypes, tc.laneTypes)
			require.Equal(t, tc.expMatched, actualMatched)
			require.Equal(t, tc.expValuesMsg, actualValuesMsg)
		})
	}
}

func TestCommandActionVal_toUint64s(t *testing.T) {
	tests := []struct {
		name                string
		rawCommandActionVal string
		exp                 []uint64
	}{
		{
			name:                "i32",
			rawCommandActionVal: `{"type": "i32", "value": "0"}`,
			exp:                 []uint64{0},
		},
		{
			name:                "i32",
			rawCommandActionVal: `{"type": "i32", "value": "4294967295"}`,
			exp:                 []uint64{4294967295},
		},
		{
			name:                "i64",
			rawCommandActionVal: `{"type": "i64", "value": "0"}`,
			exp:                 []uint64{0},
		},
		{
			name:                "i64",
			rawCommandActionVal: `{"type": "i64", "value": "7034535277573963776"}`,
			exp:                 []uint64{7034535277573963776},
		},
		{
			name:                "f32",
			rawCommandActionVal: `{"type": "f32", "value": "0"}`,
			exp:                 []uint64{0},
		},
		{
			name:                "f32",
			rawCommandActionVal: `{"type": "f32", "value": "2147483648"}`,
			exp:                 []uint64{2147483648},
		},
		{
			name:                "f64",
			rawCommandActionVal: `{"type": "f64", "value": "0"}`,
			exp:                 []uint64{0},
		},
		{
			name:                "f64",
			rawCommandActionVal: `{"type": "f64", "value": "4616189618054758400"}`,
			exp:                 []uint64{4616189618054758400},
		},
		{
			name:                "f32x4",
			rawCommandActionVal: `{"type": "v128", "lane_type": "f32", "value": ["645922816", "645922816", "645922816", "645922816"]}`,
			exp:                 []uint64{645922816<<32 | 645922816, 645922816<<32 | 645922816},
		},
		{
			name:                "f32x4",
			rawCommandActionVal: `{"type": "v128", "lane_type": "f32", "value": ["nan:canonical", "nan:arithmetic", "nan:canonical", "nan:arithmetic"]}`,
			exp: []uint64{
				uint64(moremath.F32CanonicalNaNBits) | (uint64(moremath.F32ArithmeticNaNBits) << 32),
				uint64(moremath.F32CanonicalNaNBits) | (uint64(moremath.F32ArithmeticNaNBits) << 32),
			},
		},
		{
			name:                "f64x2",
			rawCommandActionVal: `{"type": "v128", "lane_type": "f64", "value": ["9223372036854775808", "9223372036854775808"]}`,
			exp:                 []uint64{9223372036854775808, 9223372036854775808},
		},
		{
			name:                "f64x2",
			rawCommandActionVal: `{"type": "v128", "lane_type": "f64", "value": ["nan:canonical", "nan:arithmetic"]}`,
			exp:                 []uint64{moremath.F64CanonicalNaNBits, moremath.F64ArithmeticNaNBits},
		},
		{
			name:                "i8x16",
			rawCommandActionVal: `{"type": "v128", "lane_type": "i8", "value": ["128", "129", "130", "131", "253", "254", "255", "0", "0", "1", "2", "127", "128", "253", "254", "255"]}`,
			exp: []uint64{
				128 | (129 << 8) | (130 << 16) | (131 << 24) | (253 << 32) | (254 << 40) | (255 << 48),
				1<<8 | 2<<16 | 127<<24 | 128<<32 | 253<<40 | 254<<48 | 255<<56,
			},
		},
		{
			name:                "i16x8",
			rawCommandActionVal: `{"type": "v128", "lane_type": "i16", "value": ["256", "770", "1284", "1798", "2312", "2826", "3340", "3854"]}`,
			exp: []uint64{
				256 | 770<<16 | 1284<<32 | 1798<<48,
				2312 | 2826<<16 | 3340<<32 | 3854<<48,
			},
		},
		{
			name:                "i32x4",
			rawCommandActionVal: `{"type": "v128", "lane_type": "i32", "value": ["123", "32766", "32766", "40000"]}`,
			exp: []uint64{
				123 | 32766<<32,
				32766 | 40000<<32,
			},
		},
		{
			name:                "i64x2",
			rawCommandActionVal: `{"type": "v128", "lane_type": "i64", "value": ["18446744073709551615", "123124"]}`,
			exp: []uint64{
				18446744073709551615,
				123124,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var c commandActionVal
			err := json.Unmarshal([]byte(tc.rawCommandActionVal), &c)
			require.NoError(t, err)
			actual := c.toUint64s()
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCommand_getAssertReturnArgsExps(t *testing.T) {
	tests := []struct {
		name       string
		rawCommand string
		args, exps []uint64
	}{
		{
			name: "1",
			rawCommand: `
{
  "type": "assert_return",
  "line": 148,
  "action": {
    "type": "invoke", "field": "f32x4.min",
    "args": [
      {"type": "v128", "lane_type": "f32", "value": ["2147483648", "123", "2147483648", "1"]},
      {"type": "v128", "lane_type": "i8", "value": ["128", "129", "130", "131", "253", "254", "255", "0", "0", "1", "2", "127", "128", "253", "254", "255"]}
    ]
  },
  "expected": [
    {"type": "v128", "lane_type": "f32", "value": ["2147483648", "0", "0", "2147483648"]}
  ]
}`,
			args: []uint64{
				123<<32 | 2147483648,
				1<<32 | 2147483648,
				128 | (129 << 8) | (130 << 16) | (131 << 24) | (253 << 32) | (254 << 40) | (255 << 48),
				1<<8 | 2<<16 | 127<<24 | 128<<32 | 253<<40 | 254<<48 | 255<<56,
			},
			exps: []uint64{
				2147483648,
				2147483648 << 32,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var c command
			err := json.Unmarshal([]byte(tc.rawCommand), &c)
			require.NoError(t, err)
			actualArgs, actualExps := c.getAssertReturnArgsExps()
			require.Equal(t, tc.args, actualArgs)
			require.Equal(t, tc.exps, actualExps)
		})
	}
}
