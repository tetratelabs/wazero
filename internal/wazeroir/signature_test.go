package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestCompiler_wasmOpcodeSignature(t *testing.T) {
	for _, tc := range []struct {
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
	} {

		t.Run(tc.name, func(t *testing.T) {
			c := &compiler{body: tc.body}
			actual, err := c.wasmOpcodeSignature(tc.body[0], 0)
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual)
		})
	}
}
