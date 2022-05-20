package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestCompiler_wasmOpcodeSignature(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		exp  *signature
	}{
		{
			name: "i32.trunc_sat_f32_s",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32S},
			exp:  signature_F32_I32,
		},
		{
			name: "i32.trunc_sat_f32_u",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32U},
			exp:  signature_F32_I32,
		},
		{
			name: "i32.trunc_sat_f64_s",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF64S},
			exp:  signature_F64_I32,
		},
		{
			name: "i32.trunc_sat_f64_u",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF64U},
			exp:  signature_F64_I32,
		},
		{
			name: "i64.trunc_sat_f32_s",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF32S},
			exp:  signature_F32_I64,
		},
		{
			name: "i64.trunc_sat_f32_u",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF32U},
			exp:  signature_F32_I64,
		},
		{
			name: "i64.trunc_sat_f64_s",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF64S},
			exp:  signature_F64_I64,
		},
		{
			name: "i64.trunc_sat_f64_u",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF64U},
			exp:  signature_F64_I64,
		},
		{
			name: "memory.init",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryInit},
			exp:  signature_I32I32I32_None,
		},
		{
			name: "data.drop",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscDataDrop},
			exp:  signature_None_None,
		},
		{
			name: "memory.copy",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryCopy},
			exp:  signature_I32I32I32_None,
		},
		{
			name: "memory.fill",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryFill},
			exp:  signature_I32I32I32_None,
		},
		{
			name: "table.init",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableInit},
			exp:  signature_I32I32I32_None,
		},
		{
			name: "elem.drop",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscElemDrop},
			exp:  signature_None_None,
		},
		{
			name: "table.copy",
			body: []byte{wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableCopy},
			exp:  signature_I32I32I32_None,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			c := &compiler{body: tc.body}
			actual, err := c.wasmOpcodeSignature(tc.body[0], 0)
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual)
		})
	}
}
