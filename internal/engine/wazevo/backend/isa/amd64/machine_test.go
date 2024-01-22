package amd64

import (
	"fmt"
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
		{
			name: "amode with block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				blk := builder.CurrentBlock()
				ptr := blk.AddParam(builder, ssa.TypeI64)
				ctx.definitions[ptr] = &backend.SSAValueDefinition{BlockParamValue: ptr, BlkParamVReg: raxVReg}
				instr := builder.AllocateInstruction()
				instr.AsLoad(ptr, 123, ssa.TypeI64).Insert(builder)
				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			exp: newOperandMem(newAmodeImmReg(123, raxVReg)),
		},
		{
			name: "amode with iconst",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				instr := builder.AllocateInstruction()
				instr.AsLoad(iconst.Return(), 123, ssa.TypeI64).Insert(builder)
				ctx.definitions[iconst.Return()] = &backend.SSAValueDefinition{Instr: iconst}
				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			instructions: []string{
				"movabsq $579, %r1?", // r1 := 123+456
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst32(0xffffff).Insert(builder)
				uextend := builder.AllocateInstruction().AsUExtend(iconst.Return(), 32, 64).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(uextend.Return(), 123, ssa.TypeI64).Insert(builder)

				ctx.definitions[uextend.Return()] = &backend.SSAValueDefinition{Instr: uextend}
				ctx.definitions[iconst.Return()] = &backend.SSAValueDefinition{Instr: iconst}

				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 0xffffff+123),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst32(456).Insert(builder)
				uextend := builder.AllocateInstruction().AsUExtend(iconst.Return(), 32, 64).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(uextend.Return(), 123, ssa.TypeI64).Insert(builder)

				ctx.definitions[uextend.Return()] = &backend.SSAValueDefinition{Instr: uextend}
				ctx.definitions[iconst.Return()] = &backend.SSAValueDefinition{Instr: iconst}

				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 456+123),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and add",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				p := builder.CurrentBlock().AddParam(builder, ssa.TypeI64)
				iconst := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				iadd := builder.AllocateInstruction().AsIadd(iconst.Return(), p).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(iadd.Return(), 789, ssa.TypeI64).Insert(builder)

				ctx.definitions[p] = &backend.SSAValueDefinition{BlockParamValue: p, BlkParamVReg: raxVReg}
				ctx.definitions[iconst.Return()] = &backend.SSAValueDefinition{Instr: iconst}
				ctx.definitions[iadd.Return()] = &backend.SSAValueDefinition{Instr: iadd}

				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			exp: newOperandMem(newAmodeImmReg(456+789, raxVReg)),
		},
		{
			name: "amode with iconst, block param and add",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) *backend.SSAValueDefinition {
				iconst1 := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				iconst2 := builder.AllocateInstruction().AsIconst64(123).Insert(builder)
				iadd := builder.AllocateInstruction().AsIadd(iconst1.Return(), iconst2.Return()).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(iadd.Return(), 789, ssa.TypeI64).Insert(builder)

				ctx.definitions[iconst1.Return()] = &backend.SSAValueDefinition{Instr: iconst1}
				ctx.definitions[iconst2.Return()] = &backend.SSAValueDefinition{Instr: iconst2}
				ctx.definitions[iadd.Return()] = &backend.SSAValueDefinition{Instr: iadd}

				return &backend.SSAValueDefinition{Instr: instr, N: 0}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 123+456+789),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
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
