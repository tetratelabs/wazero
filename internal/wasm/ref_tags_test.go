package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestPackI31_MasksHighBit(t *testing.T) {
	tests := []struct {
		in   uint32
		want uint32
	}{
		{0, 0},
		{1, 1},
		{0x7FFFFFFF, 0x7FFFFFFF},
		{0x80000000, 0},          // bit 31 stripped
		{0xFFFFFFFF, 0x7FFFFFFF}, // all bits set => bit 31 stripped
		{0xCAFEBABE, 0x4AFEBABE}, // bit 31 stripped, rest preserved
	}
	for _, tt := range tests {
		packed := PackI31(tt.in)
		require.True(t, IsTaggedI31(packed))
		require.Equal(t, tt.want, UnpackI31Unsigned(packed))
	}
}

func TestUnpackI31Signed_SignExtend(t *testing.T) {
	tests := []struct {
		name string
		in   uint32
		want int32
	}{
		{"zero", 0, 0},
		{"one", 1, 1},
		{"max positive 31-bit (bit 30 unset)", 0x3FFFFFFF, 0x3FFFFFFF},
		{"min negative 31-bit (bit 30 set)", 0x40000000, -0x40000000},
		{"-1 (all 31 bits set)", 0x7FFFFFFF, -1},
		{"input has bit 31 set, masked off then sign-extended", 0xC0000000, -0x40000000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, UnpackI31Signed(PackI31(tt.in)))
		})
	}
}

func TestUnpackI31Unsigned_NoSignExtend(t *testing.T) {
	require.Equal(t, uint32(0x7FFFFFFF), UnpackI31Unsigned(PackI31(0x7FFFFFFF)))
	require.Equal(t, uint32(0x7FFFFFFF), UnpackI31Unsigned(PackI31(0xFFFFFFFF)))
	require.Equal(t, uint32(0x40000000), UnpackI31Unsigned(PackI31(0x40000000)))
}

func TestPackI31_FromNegativeInt32(t *testing.T) {
	require.Equal(t, int32(-1), UnpackI31Signed(PackI31(uint32(0xFFFFFFFF))))
	require.Equal(t, int32(-0x40000000), UnpackI31Signed(PackI31(uint32(0xC0000000))))

	var bitPattern uint32 = 0x80000000
	require.Equal(t, int32(0), UnpackI31Signed(PackI31(bitPattern)))

	bitPattern = 0x80000001
	require.Equal(t, int32(1), UnpackI31Signed(PackI31(bitPattern)))
}
