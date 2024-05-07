package interpreter

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
		{
			name: "i32.atomic.load8_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load8U},
			exp:  signature_I32_I32,
		},
		{
			name: "i32.atomic.load16_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load16U},
			exp:  signature_I32_I32,
		},
		{
			name: "i32.atomic.load",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load},
			exp:  signature_I32_I32,
		},
		{
			name: "i64.atomic.load8_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load8U},
			exp:  signature_I32_I64,
		},
		{
			name: "i64.atomic.load16_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load16U},
			exp:  signature_I32_I64,
		},
		{
			name: "i64.atomic.load32_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load32U},
			exp:  signature_I32_I64,
		},
		{
			name: "i64.atomic.load",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load},
			exp:  signature_I32_I64,
		},
		{
			name: "i32.atomic.store8",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store8},
			exp:  signature_I32I32_None,
		},
		{
			name: "i32.atomic.store16_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store16},
			exp:  signature_I32I32_None,
		},
		{
			name: "i32.atomic.store",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store},
			exp:  signature_I32I32_None,
		},
		{
			name: "i64.atomic.store8",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store8},
			exp:  signature_I32I64_None,
		},
		{
			name: "i64.atomic.store16",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store16},
			exp:  signature_I32I64_None,
		},
		{
			name: "i64.atomic.store32",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store32},
			exp:  signature_I32I64_None,
		},
		{
			name: "i64.atomic.store",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store},
			exp:  signature_I32I64_None,
		},
		{
			name: "i32.atomic.rmw8.add_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AddU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.add_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AddU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.add",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAdd},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.add_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AddU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.add_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AddU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.add_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AddU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.add",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAdd},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.sub_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8SubU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.sub_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16SubU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.sub",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwSub},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.sub_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8SubU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.sub_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16SubU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.sub_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32SubU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.sub",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwSub},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.and_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AndU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.and_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AndU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.and",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAnd},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.and_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AndU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.and_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AndU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.and_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AndU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.and",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAnd},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.or_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8OrU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.or_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16OrU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.or",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwOr},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.or_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8OrU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.or_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16OrU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.or_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32OrU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.or",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwOr},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.xor_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XorU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.xor_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XorU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.xor",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXor},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.xor_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XorU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.xor_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XorU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.xor_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XorU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.xor",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXor},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.xchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XchgU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.xchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XchgU},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.xchg",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXchg},
			exp:  signature_I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.xchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XchgU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw16.xchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XchgU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw32.xchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XchgU},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i64.atomic.rmw.xchg",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXchg},
			exp:  signature_I32I64_I64,
		},
		{
			name: "i32.atomic.rmw8.cmpxchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8CmpxchgU},
			exp:  signature_I32I32I32_I32,
		},
		{
			name: "i32.atomic.rmw16.cmpxchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16CmpxchgU},
			exp:  signature_I32I32I32_I32,
		},
		{
			name: "i32.atomic.rmw.cmpxchg",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwCmpxchg},
			exp:  signature_I32I32I32_I32,
		},
		{
			name: "i64.atomic.rmw8.cmpxchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8CmpxchgU},
			exp:  signature_I32I64I64_I64,
		},
		{
			name: "i64.atomic.rmw16.cmpxchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16CmpxchgU},
			exp:  signature_I32I64I64_I64,
		},
		{
			name: "i64.atomic.rmw32.cmpxchg_u",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32CmpxchgU},
			exp:  signature_I32I64I64_I64,
		},
		{
			name: "i64.atomic.rmw.cmpxchg",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwCmpxchg},
			exp:  signature_I32I64I64_I64,
		},
		{
			name: "memory.atomic.wait32",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait32},
			exp:  signature_I32I32I64_I32,
		},
		{
			name: "memory.atomic.wait64",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait64},
			exp:  signature_I32I64I64_I32,
		},
		{
			name: "memory.atomic.notify",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryNotify},
			exp:  signature_I32I32_I32,
		},
		{
			name: "memory.atomic.fence",
			body: []byte{wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicFence},
			exp:  signature_None_None,
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
		wasmTypes:     []wasm.FunctionType{v_v, i32_i32, v_f64f64},
		directCalls:   make([]*signature, 3),
		indirectCalls: make([]*signature, 3),
	}

	require.Equal(t, &signature{in: make([]unsignedType, 0), out: make([]unsignedType, 0)}, f.get(0, false))
	require.Equal(t, &signature{in: []unsignedType{unsignedTypeI32}, out: make([]unsignedType, 0)}, f.get(0, true))
	require.NotNil(t, f.directCalls[0])
	require.NotNil(t, f.indirectCalls[0])
	require.Equal(t, &signature{in: []unsignedType{unsignedTypeI32}, out: []unsignedType{unsignedTypeI32}}, f.get(1, false))
	require.Equal(t, &signature{in: []unsignedType{unsignedTypeI32, unsignedTypeI32}, out: []unsignedType{unsignedTypeI32}}, f.get(1, true))
	require.NotNil(t, f.directCalls[1])
	require.NotNil(t, f.indirectCalls[1])
	require.Equal(t, &signature{in: make([]unsignedType, 0), out: []unsignedType{unsignedTypeF64, unsignedTypeF64}}, f.get(2, false))
	require.Equal(t, &signature{in: []unsignedType{unsignedTypeI32}, out: []unsignedType{unsignedTypeF64, unsignedTypeF64}}, f.get(2, true))
	require.NotNil(t, f.directCalls[2])
	require.NotNil(t, f.indirectCalls[2])
}
