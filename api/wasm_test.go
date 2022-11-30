package api

import (
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestExternTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    ExternType
		expected string
	}{
		{"func", ExternTypeFunc, "func"},
		{"table", ExternTypeTable, "table"},
		{"mem", ExternTypeMemory, "memory"},
		{"global", ExternTypeGlobal, "global"},
		{"unknown", 100, "0x64"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ExternTypeName(tc.input))
		})
	}
}

func TestValueTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    ValueType
		expected string
	}{
		{"i32", ValueTypeI32, "i32"},
		{"i64", ValueTypeI64, "i64"},
		{"f32", ValueTypeF32, "f32"},
		{"f64", ValueTypeF64, "f64"},
		{"externref", ValueTypeExternref, "externref"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ValueTypeName(tc.input))
		})
	}
}

func TestEncodeDecodeExternRef(t *testing.T) {
	for _, v := range []uintptr{
		0, uintptr(unsafe.Pointer(t)),
	} {
		t.Run(fmt.Sprintf("%x", v), func(t *testing.T) {
			encoded := EncodeExternref(v)
			binary := DecodeExternref(encoded)
			require.Equal(t, v, binary)
		})
	}
}

func TestEncodeDecodeF32(t *testing.T) {
	for _, v := range []float32{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			encoded := EncodeF32(v)
			binary := DecodeF32(encoded)
			require.Zero(t, encoded>>32)     // Ensures high bits aren't set
			if math.IsNaN(float64(binary)) { // NaN cannot be compared with themselves, so we have to use IsNaN
				require.True(t, math.IsNaN(float64(binary)))
			} else {
				require.Equal(t, v, binary)
			}
		})
	}
}

func TestEncodeDecodeF64(t *testing.T) {
	for _, v := range []float64{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		6.8719476736e+10,  /* = 1 << 36 */
		1.37438953472e+11, /* = 1 << 37 */
		math.Inf(1), math.Inf(-1), math.NaN(),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			encoded := EncodeF64(v)
			val := DecodeF64(encoded)
			if math.IsNaN(val) { // cannot use require.Equal as NaN by definition doesn't equal itself
				require.True(t, math.IsNaN(val))
			} else {
				require.Equal(t, v, val)
			}
		})
	}
}

func TestEncodeI32(t *testing.T) {
	for _, v := range []int32{
		0, 100, -100, 1, -1,
		math.MaxInt32,
		math.MinInt32,
	} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			encoded := EncodeI32(v)
			require.Zero(t, encoded>>32) // Ensures high bits aren't set
			binary := int32(encoded)
			require.Equal(t, v, binary)
		})
	}
}

func TestDecodeI32(t *testing.T) {
	mini32 := math.MinInt32
	for _, tc := range []struct {
		in  uint64
		exp int32
	}{
		{in: 0, exp: 0},
		{in: 1 << 60, exp: 0},
		{in: 1 << 30, exp: 1 << 30},
		{in: 1<<30 | 1<<60, exp: 1 << 30},
		{in: uint64(uint32(mini32)) | 1<<59, exp: math.MinInt32},
		{in: uint64(uint32(math.MaxInt32)) | 1<<50, exp: math.MaxInt32},
	} {
		decoded := DecodeI32(tc.in)
		require.Equal(t, tc.exp, decoded)
	}
}

func TestEncodeU32(t *testing.T) {
	for _, v := range []uint32{
		0, 100, 1, 1 << 31,
		math.MaxInt32,
		math.MaxUint32,
	} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			encoded := EncodeU32(v)
			require.Zero(t, encoded>>32) // Ensures high bits aren't set
			require.Equal(t, v, uint32(encoded))
		})
	}
}

func TestDecodeU32(t *testing.T) {
	mini32 := math.MinInt32
	for _, tc := range []struct {
		in  uint64
		exp uint32
	}{
		{in: 0, exp: 0},
		{in: 1 << 60, exp: 0},
		{in: 1 << 30, exp: 1 << 30},
		{in: 1<<30 | 1<<60, exp: 1 << 30},
		{in: uint64(uint32(mini32)) | 1<<59, exp: uint32(mini32)},
		{in: uint64(uint32(math.MaxInt32)) | 1<<50, exp: math.MaxInt32},
	} {
		decoded := DecodeU32(tc.in)
		require.Equal(t, tc.exp, decoded)
	}
}

func TestEncodeI64(t *testing.T) {
	for _, v := range []int64{
		0, 100, -100, 1, -1,
		math.MaxInt64,
		math.MinInt64,
	} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			encoded := EncodeI64(v)
			binary := int64(encoded)
			require.Equal(t, v, binary)
		})
	}
}
