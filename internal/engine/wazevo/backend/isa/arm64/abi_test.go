package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_insertAddOrSubStackPointer(t *testing.T) {
	t.Run("add/small", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0x10, true)
		require.Equal(t, `add sp, sp, #0x10`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("add/large stack", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0xffffffff_8, true)
		require.Equal(t, `orr x27, xzr, #0xffffffff8
add sp, sp, x27`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("sub/small", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0x10, false)
		require.Equal(t, `sub sp, sp, #0x10`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("sub/large stack", func(t *testing.T) {
		_, _, m := newSetupWithMockContext()
		m.insertAddOrSubStackPointer(spVReg, 0xffffffff_8, false)
		require.Equal(t, `orr x27, xzr, #0xffffffff8
sub sp, sp, x27`, formatEmittedInstructionsInCurrentBlock(m))
	})
}

func TestAbiImpl_callerGenVRegToFunctionArg_constant_inlining(t *testing.T) {
	_, builder, m := newSetupWithMockContext()

	i64 := builder.AllocateInstruction().AsIconst64(10).Insert(builder)
	f64 := builder.AllocateInstruction().AsF64const(3.14).Insert(builder)
	abi := &backend.FunctionABI{}
	abi.Init(&ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeF64}}, intParamResultRegs, floatParamResultRegs)
	m.callerGenVRegToFunctionArg(abi, 0, regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), backend.SSAValueDefinition{Instr: i64, RefCount: 1}, 0)
	m.callerGenVRegToFunctionArg(abi, 1, regalloc.VReg(50).SetRegType(regalloc.RegTypeFloat), backend.SSAValueDefinition{Instr: f64, RefCount: 1}, 0)
	require.Equal(t, `movz x100?, #0xa, lsl 0
mov x0, x100?
ldr d50?, #8; b 16; data.f64 3.140000
mov v0.8b, v50?.8b`, formatEmittedInstructionsInCurrentBlock(m))
}
