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

func Test_funcTypeToIRSignatures(t *testing.T) {
	f := &funcTypeToIRSignatures{
		wasmTypes:     []*wasm.FunctionType{v_v, i32_i32, v_f64f64},
		directCalls:   make([]*signature, 3),
		indirectCalls: make([]*signature, 3),
	}

	require.Equal(t, &signature{in: make([]UnsignedType, 0), out: make([]UnsignedType, 0)}, f.get(0, false))
	require.Equal(t, &signature{in: []UnsignedType{UnsignedTypeI32}, out: make([]UnsignedType, 0)}, f.get(0, true))
	require.NotNil(t, f.directCalls[0])
	require.NotNil(t, f.indirectCalls[0])
	require.Equal(t, &signature{in: []UnsignedType{UnsignedTypeI32}, out: []UnsignedType{UnsignedTypeI32}}, f.get(1, false))
	require.Equal(t, &signature{in: []UnsignedType{UnsignedTypeI32, UnsignedTypeI32}, out: []UnsignedType{UnsignedTypeI32}}, f.get(1, true))
	require.NotNil(t, f.directCalls[1])
	require.NotNil(t, f.indirectCalls[1])
	require.Equal(t, &signature{in: make([]UnsignedType, 0), out: []UnsignedType{UnsignedTypeF64, UnsignedTypeF64}}, f.get(2, false))
	require.Equal(t, &signature{in: []UnsignedType{UnsignedTypeI32}, out: []UnsignedType{UnsignedTypeF64, UnsignedTypeF64}}, f.get(2, true))
	require.NotNil(t, f.directCalls[2])
	require.NotNil(t, f.indirectCalls[2])
}
