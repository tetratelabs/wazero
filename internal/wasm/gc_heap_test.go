package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWasmStruct_NewAndAccessors(t *testing.T) {
	id := FunctionTypeID(7)
	s := NewWasmStruct(id, 3)
	require.Equal(t, id, s.TypeID)
	require.Equal(t, 3, len(s.Fields))

	// Default-zero fields read as nil.
	require.Nil(t, s.Get(0))
	require.Nil(t, s.Get(1))
	require.Nil(t, s.Get(2))

	// Set and read back.
	require.NoError(t, s.Set(0, int32(42)))
	require.NoError(t, s.Set(1, int64(-1)))
	require.NoError(t, s.Set(2, float32(3.5)))

	require.Equal(t, int32(42), s.Get(0))
	require.Equal(t, int64(-1), s.Get(1))
	require.Equal(t, float32(3.5), s.Get(2))
}

func TestWasmStruct_OutOfRange(t *testing.T) {
	s := NewWasmStruct(0, 2)
	require.Nil(t, s.Get(-1))
	require.Nil(t, s.Get(2))
	require.Nil(t, s.Get(99))
	require.Error(t, s.Set(-1, 0))
	require.Error(t, s.Set(2, 0))
	require.Error(t, s.Set(99, 0))
}

func TestWasmStruct_NilReceiver(t *testing.T) {
	var s *WasmStruct
	require.Nil(t, s.Get(0))
	require.Error(t, s.Set(0, 0))
}

func TestNewWasmStructWith(t *testing.T) {
	fields := []any{int32(1), int32(2), int32(3)}
	s := NewWasmStructWith(FunctionTypeID(42), fields)
	require.Equal(t, FunctionTypeID(42), s.TypeID)
	require.Equal(t, 3, len(s.Fields))
	require.Equal(t, int32(2), s.Get(1))
}

func TestWasmStruct_RefField(t *testing.T) {
	// Verify that ref-typed fields naturally hold *WasmStruct pointers
	// and survive Go-managed lifetime (i.e. nothing unsafe is happening).
	inner := NewWasmStructWith(0, []any{int32(99)})
	outer := NewWasmStructWith(1, []any{inner, PackI31(7)})
	got := outer.Get(0).(*WasmStruct)
	require.Equal(t, int32(99), got.Get(0))
	got31 := outer.Get(1).(uint64)
	require.Equal(t, int32(7), UnpackI31Signed(got31))
}

func TestWasmArray_NewAndAccessors(t *testing.T) {
	a := NewWasmArray(FunctionTypeID(11), 5)
	require.Equal(t, FunctionTypeID(11), a.TypeID)
	require.Equal(t, uint32(5), a.Len())
	for i := uint32(0); i < 5; i++ {
		require.Nil(t, a.Get(i))
	}

	require.NoError(t, a.Set(2, int32(123)))
	require.Equal(t, int32(123), a.Get(2))
}

func TestWasmArray_OutOfRange(t *testing.T) {
	a := NewWasmArray(0, 3)
	require.Nil(t, a.Get(3))
	require.Nil(t, a.Get(99))
	require.Error(t, a.Set(3, 0))
	require.Error(t, a.Set(99, 0))
}

func TestWasmArray_NilReceiver(t *testing.T) {
	var a *WasmArray
	require.Equal(t, uint32(0), a.Len())
	require.Nil(t, a.Get(0))
	require.Error(t, a.Set(0, 0))
}

func TestNewWasmArrayWith(t *testing.T) {
	elems := []any{int32(10), int32(20), int32(30)}
	a := NewWasmArrayWith(FunctionTypeID(99), elems)
	require.Equal(t, FunctionTypeID(99), a.TypeID)
	require.Equal(t, uint32(3), a.Len())
	require.Equal(t, int32(20), a.Get(1))
}

// -- packed storage --

func TestNarrowAndExtendI8(t *testing.T) {
	// Narrowing keeps only the low 8 bits.
	require.Equal(t, uint8(0), NarrowI8(0))
	require.Equal(t, uint8(1), NarrowI8(1))
	require.Equal(t, uint8(0xFF), NarrowI8(-1))    // -1 narrows to 0xFF
	require.Equal(t, uint8(0xFF), NarrowI8(0x1FF)) // high bits dropped
	require.Equal(t, uint8(0x42), NarrowI8(0x12342))

	// Sign-extension treats the byte as int8 and widens to int32.
	require.Equal(t, int32(0), SignExtendI8(0))
	require.Equal(t, int32(1), SignExtendI8(1))
	require.Equal(t, int32(-1), SignExtendI8(0xFF))
	require.Equal(t, int32(-128), SignExtendI8(0x80))
	require.Equal(t, int32(127), SignExtendI8(0x7F))

	// Zero-extension treats the byte as unsigned.
	require.Equal(t, uint32(0), ZeroExtendI8(0))
	require.Equal(t, uint32(0xFF), ZeroExtendI8(0xFF))
	require.Equal(t, uint32(0x80), ZeroExtendI8(0x80))
}

func TestNarrowAndExtendI16(t *testing.T) {
	require.Equal(t, uint16(0), NarrowI16(0))
	require.Equal(t, uint16(0xFFFF), NarrowI16(-1))
	require.Equal(t, uint16(0xFFFF), NarrowI16(0x1FFFF))
	// 0xABCD1234 as int32 = a negative value; its low 16 bits are 0x1234.
	var hiPattern uint32 = 0xABCD1234
	require.Equal(t, uint16(0x1234), NarrowI16(int32(hiPattern)))

	require.Equal(t, int32(0), SignExtendI16(0))
	require.Equal(t, int32(-1), SignExtendI16(0xFFFF))
	require.Equal(t, int32(-32768), SignExtendI16(0x8000))
	require.Equal(t, int32(32767), SignExtendI16(0x7FFF))

	require.Equal(t, uint32(0), ZeroExtendI16(0))
	require.Equal(t, uint32(0xFFFF), ZeroExtendI16(0xFFFF))
	require.Equal(t, uint32(0x8000), ZeroExtendI16(0x8000))
}

func TestPackedRoundTrip(t *testing.T) {
	// Writing then reading back through packed storage preserves the low
	// bits but does NOT preserve high bits (which the spec drops on
	// narrowing). struct.get_s after struct.set is equivalent to
	// sign-extending the low byte/short of the written value.
	tests := []struct {
		in       int32
		wantI8s  int32
		wantI16s int32
	}{
		{0, 0, 0},
		{1, 1, 1},
		{-1, -1, -1},
		{0x1F0, -16, 0x1F0},                      // i8 narrow drops 0x1, sign-extends 0xF0
		{0x10000, 0, 0},                          // i16 narrow drops 0x1, sign-extends 0x0000 = 0
		{int32(0x12345678), int32(0x78), 0x5678}, // both narrowings
	}
	for _, tt := range tests {
		got8 := SignExtendI8(NarrowI8(tt.in))
		require.Equal(t, tt.wantI8s, got8, "i8 sign-extend of NarrowI8(%#x)", tt.in)
		got16 := SignExtendI16(NarrowI16(tt.in))
		require.Equal(t, tt.wantI16s, got16, "i16 sign-extend of NarrowI16(%#x)", tt.in)
	}
}

func TestDefaultFieldValue(t *testing.T) {
	tests := []struct {
		name  string
		field FieldType
		want  any
	}{
		{"i32", ValueTypeI32, int32(0)},
		{"i64", ValueTypeI64, int64(0)},
		{"f32", ValueTypeF32, float32(0)},
		{"f64", ValueTypeF64, float64(0)},
		{"v128", ValueTypeV128, [16]byte{}},
		{"i8 packed", ValueTypeI8, uint8(0)},
		{"i16 packed", ValueTypeI16, uint16(0)},
		{"funcref", ValueTypeFuncref, uint64(0)},
		{"externref", ValueTypeExternref, uint64(0)},
		{"exnref", ValueTypeExnref, uint64(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultFieldValue(tt.field)
			require.Equal(t, tt.want, got)
		})
	}
}
