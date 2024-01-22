package amd64

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_asImm32(t *testing.T) {
	v, ok := asImm32(0xffffffff)
	require.True(t, ok)
	require.Equal(t, uint32(0xffffffff), v)

	_, ok = asImm32(0xffffffff << 1)
	require.False(t, ok)
}

func TestMachine_getOperand_Reg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) *backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				return &backend.SSAValueDefinition{BlkParamVReg: raxVReg, Instr: nil, N: 0}
			},
			exp: newOperandReg(raxVReg),
		},

		{
			name: "const instr",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				instr := builder.AllocateInstruction()
				instr.AsIconst32(0xf00000f)
				builder.InsertInstruction(instr)
				ctx.vRegCounter = 99
				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			exp:          newOperandReg(regalloc.VReg(100).SetRegType(regalloc.RegTypeInt)),
			instructions: []string{"movl $251658255, %r100d?"},
		},
		{
			name: "non const instr (single-return)",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				c := builder.AllocateInstruction()
				sig := &ssa.Signature{Results: []ssa.Type{ssa.TypeI64}}
				builder.DeclareSignature(sig)
				c.AsCall(ssa.FuncRef(0), sig, nil)
				builder.InsertInstruction(c)
				r := c.Return()
				ctx.vRegMap[r] = regalloc.VReg(50)
				return &backend.SSAValueDefinition{Instr: c, N: 0}
			},
			exp: newOperandReg(regalloc.VReg(50)),
		},
		{
			name: "non const instr (multi-return)",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				c := builder.AllocateInstruction()
				sig := &ssa.Signature{Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64, ssa.TypeF64}}
				builder.DeclareSignature(sig)
				c.AsCall(ssa.FuncRef(0), sig, nil)
				builder.InsertInstruction(c)
				_, rs := c.Returns()
				ctx.vRegMap[rs[1]] = regalloc.VReg(50)
				return &backend.SSAValueDefinition{Instr: c, N: 2}
			},
			exp: newOperandReg(regalloc.VReg(50)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Reg(def)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_getOperand_Imm32_Reg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) *backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param falls back to getOperand_Reg",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				return &backend.SSAValueDefinition{BlkParamVReg: raxVReg, Instr: nil, N: 0}
			},
			exp: newOperandReg(raxVReg),
		},
		{
			name: "const imm 32",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				instr := builder.AllocateInstruction()
				instr.AsIconst32(0xf00000f)
				builder.InsertInstruction(instr)
				ctx.vRegCounter = 99
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			exp: newOperandImm32(0xf00000f),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Imm32_Reg(def)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func Test_machine_getOperand_Mem_Imm32_Reg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) *backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param falls back to getOperand_Imm32_Reg",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				return &backend.SSAValueDefinition{BlkParamVReg: raxVReg, Instr: nil, N: 0}
			},
			exp: newOperandReg(raxVReg),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Mem_Imm32_Reg(def)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}
