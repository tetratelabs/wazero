package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAbiImpl_init(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  abiImpl
	}{
		{
			name: "empty sig",
			sig:  &ssa.Signature{},
			exp:  abiImpl{},
		},
		{
			name: "small sig",
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI32, ssa.TypeF32, ssa.TypeI32},
				Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64},
			},
			exp: abiImpl{
				m: nil,
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI32},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI64},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF64},
				},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg},
			},
		},
		{
			name: "regs stack mix and match",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
				Results: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
			},
			exp: abiImpl{
				argStackSize: 128, retStackSize: 128,
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
			},
		},
		{
			name: "all regs",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
				Results: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
			},
			exp: abiImpl{
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			abi := abiImpl{}
			abi.init(tc.sig)
			require.Equal(t, tc.exp.args, abi.args)
			require.Equal(t, tc.exp.rets, abi.rets)
			require.Equal(t, tc.exp.argStackSize, abi.argStackSize)
			require.Equal(t, tc.exp.retStackSize, abi.retStackSize)
			require.Equal(t, tc.exp.retRealRegs, abi.retRealRegs)
			require.Equal(t, tc.exp.argRealRegs, abi.argRealRegs)
		})
	}
}

func TestMachine_insertAddOrSubStackPointer(t *testing.T) {
	t.Run("add/small", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0x10, true, false)
		require.Equal(t, `add sp, sp, #0x10`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("add/large stack", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0xffffffff_8, true, false)
		require.Equal(t, `orr x27, xzr, #0xffffffff8
add sp, sp, x27`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("sub/small", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0x10, false, false)
		require.Equal(t, `sub sp, sp, #0x10`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("sub/large stack", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0xffffffff_8, false, false)
		require.Equal(t, `orr x27, xzr, #0xffffffff8
sub sp, sp, x27`, formatEmittedInstructionsInCurrentBlock(m))
	})
}

func TestAbiImpl_callerGenVRegToFunctionArg_constant_inlining(t *testing.T) {
	_, builder, m := newSetupWithMockContext()

	i64 := builder.AllocateInstruction().AsIconst64(10).Insert(builder)
	f64 := builder.AllocateInstruction().AsF64const(3.14).Insert(builder)
	abi := m.getOrCreateABIImpl(&ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeF64}})
	abi.callerGenVRegToFunctionArg(0, regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), &backend.SSAValueDefinition{Instr: i64, RefCount: 1})
	abi.callerGenVRegToFunctionArg(1, regalloc.VReg(50).SetRegType(regalloc.RegTypeFloat), &backend.SSAValueDefinition{Instr: f64, RefCount: 1})
	require.Equal(t, `movz x100?, #0xa, lsl 0
mov x0, x100?
ldr d50?, #8; b 16; data.f64 3.140000
mov v0.8b, v50?.8b`, formatEmittedInstructionsInCurrentBlock(m))
}
