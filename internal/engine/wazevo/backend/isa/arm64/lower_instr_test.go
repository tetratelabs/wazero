package arm64

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_LowerConditionalBranch(t *testing.T) {
	cmpInSameGroupFromParams := func(
		brz bool, intCond ssa.IntegerCmpCond, floatCond ssa.FloatCmpCond,
		ctx *mockCompiler, builder ssa.Builder, m *machine,
	) (instr *ssa.Instruction, verify func(t *testing.T)) {
		m.StartLoweringFunction(10)
		entry := builder.CurrentBlock()
		isInt := intCond != ssa.IntegerCmpCondInvalid

		var val1, val2 ssa.Value
		if isInt {
			val1 = entry.AddParam(builder, ssa.TypeI64)
			val2 = entry.AddParam(builder, ssa.TypeI64)
			ctx.vRegMap[val1], ctx.vRegMap[val2] = regToVReg(x1).SetRegType(regalloc.RegTypeInt), regToVReg(x2).SetRegType(regalloc.RegTypeInt)
		} else {
			val1 = entry.AddParam(builder, ssa.TypeF64)
			val2 = entry.AddParam(builder, ssa.TypeF64)
			ctx.vRegMap[val1], ctx.vRegMap[val2] = regToVReg(v1).SetRegType(regalloc.RegTypeFloat), regToVReg(v2).SetRegType(regalloc.RegTypeFloat)
		}

		var cmpInstr *ssa.Instruction
		if isInt {
			cmpInstr = builder.AllocateInstruction()
			cmpInstr.AsIcmp(val1, val2, intCond)
			builder.InsertInstruction(cmpInstr)
		} else {
			cmpInstr = builder.AllocateInstruction()
			cmpInstr.AsFcmp(val1, val2, floatCond)
			builder.InsertInstruction(cmpInstr)
		}

		cmpVal := cmpInstr.Return()
		ctx.vRegMap[cmpVal] = 3

		ctx.definitions[val1] = &backend.SSAValueDefinition{BlkParamVReg: ctx.vRegMap[val1], BlockParamValue: val1}
		ctx.definitions[val2] = &backend.SSAValueDefinition{BlkParamVReg: ctx.vRegMap[val2], BlockParamValue: val2}
		ctx.definitions[cmpVal] = &backend.SSAValueDefinition{Instr: cmpInstr}
		b := builder.AllocateInstruction()
		if brz {
			b.AsBrz(cmpVal, nil, builder.AllocateBasicBlock())
		} else {
			b.AsBrnz(cmpVal, nil, builder.AllocateBasicBlock())
		}
		builder.InsertInstruction(b)
		return b, func(t *testing.T) {
			_, ok := ctx.lowered[cmpInstr]
			require.True(t, ok)
		}
	}

	icmpInSameGroupFromParamAndImm12 := func(brz bool, ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
		m.StartLoweringFunction(10)
		entry := builder.CurrentBlock()
		v1 := entry.AddParam(builder, ssa.TypeI32)

		iconst := builder.AllocateInstruction()
		iconst.AsIconst32(0x4d2)
		builder.InsertInstruction(iconst)
		v2 := iconst.Return()

		// Constant can be referenced from different groups because we inline it.
		builder.SetCurrentBlock(builder.AllocateBasicBlock())

		icmp := builder.AllocateInstruction()
		icmp.AsIcmp(v1, v2, ssa.IntegerCmpCondEqual)
		builder.InsertInstruction(icmp)
		icmpVal := icmp.Return()
		ctx.definitions[v1] = &backend.SSAValueDefinition{BlkParamVReg: intToVReg(1), BlockParamValue: v1}
		ctx.definitions[v2] = &backend.SSAValueDefinition{Instr: iconst}
		ctx.definitions[icmpVal] = &backend.SSAValueDefinition{Instr: icmp}
		ctx.vRegMap[v1], ctx.vRegMap[v2], ctx.vRegMap[icmpVal] = intToVReg(1), intToVReg(2), intToVReg(3)
		b := builder.AllocateInstruction()
		if brz {
			b.AsBrz(icmpVal, nil, builder.AllocateBasicBlock())
		} else {
			b.AsBrnz(icmpVal, nil, builder.AllocateBasicBlock())
		}
		builder.InsertInstruction(b)
		return b, func(t *testing.T) {
			_, ok := ctx.lowered[icmp]
			require.True(t, ok)
		}
	}

	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (instr *ssa.Instruction, verify func(t *testing.T))
		instructions []string
	}{
		{
			name: "icmp in different group",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				m.StartLoweringFunction(10)
				entry := builder.CurrentBlock()
				v1, v2 := entry.AddParam(builder, ssa.TypeI64), entry.AddParam(builder, ssa.TypeI64)

				icmp := builder.AllocateInstruction()
				icmp.AsIcmp(v1, v2, ssa.IntegerCmpCondEqual)
				builder.InsertInstruction(icmp)
				icmpVal := icmp.Return()
				ctx.definitions[icmpVal] = &backend.SSAValueDefinition{Instr: icmp}
				ctx.vRegMap[v1], ctx.vRegMap[v2], ctx.vRegMap[icmpVal] = intToVReg(1), intToVReg(2), intToVReg(3)

				brz := builder.AllocateInstruction()
				brz.AsBrz(icmpVal, nil, builder.AllocateBasicBlock())
				builder.InsertInstruction(brz)

				// Indicate that currently compiling in the different group.
				ctx.currentGID = 1000
				return brz, func(t *testing.T) {
					_, ok := ctx.lowered[icmp]
					require.False(t, ok)
				}
			},
			instructions: []string{"cbz w3?, (L1)"},
		},
		{
			name: "brz / icmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return cmpInSameGroupFromParams(true, ssa.IntegerCmpCondUnsignedGreaterThan, ssa.FloatCmpCondInvalid, ctx, builder, m)
			},
			instructions: []string{
				"subs xzr, x1, x2",
				"b.ls L1",
			},
		},
		{
			name: "brnz / icmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return cmpInSameGroupFromParams(false, ssa.IntegerCmpCondEqual, ssa.FloatCmpCondInvalid, ctx, builder, m)
			},
			instructions: []string{
				"subs xzr, x1, x2",
				"b.eq L1",
			},
		},
		{
			name: "brz / fcmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return cmpInSameGroupFromParams(true, ssa.IntegerCmpCondInvalid, ssa.FloatCmpCondEqual, ctx, builder, m)
			},
			instructions: []string{
				"fcmp d1, d2",
				"b.ne L1",
			},
		},
		{
			name: "brnz / fcmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return cmpInSameGroupFromParams(false, ssa.IntegerCmpCondInvalid, ssa.FloatCmpCondGreaterThan, ctx, builder, m)
			},
			instructions: []string{
				"fcmp d1, d2",
				"b.gt L1",
			},
		},
		{
			name: "brz / icmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return icmpInSameGroupFromParamAndImm12(true, ctx, builder, m)
			},
			instructions: []string{
				"subs wzr, w1?, #0x4d2",
				"b.ne L1",
			},
		},
		{
			name: "brz / icmp in the same group / params",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
				return icmpInSameGroupFromParamAndImm12(false, ctx, builder, m)
			},
			instructions: []string{
				"subs wzr, w1?, #0x4d2",
				"b.eq L1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			instr, verify := tc.setup(ctx, b, m)
			m.LowerConditionalBranch(instr)
			verify(t)
			require.Equal(t, strings.Join(tc.instructions, "\n"),
				formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_LowerSingleBranch(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (instr *ssa.Instruction)
		instructions []string
	}{
		{
			name: "jump-fallthrough",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction) {
				jump := builder.AllocateInstruction()
				jump.AsJump(nil, builder.AllocateBasicBlock())
				builder.InsertInstruction(jump)
				jump.AsFallthroughJump()
				return jump
			},
			instructions: []string{}, // Fallthrough jump should be optimized out.
		},
		{
			name: "b",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction) {
				m.StartLoweringFunction(10)
				jump := builder.AllocateInstruction()
				jump.AsJump(nil, builder.AllocateBasicBlock())
				builder.InsertInstruction(jump)
				return jump
			},
			instructions: []string{"b L1"},
		},
		{
			name: "ret",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction) {
				m.StartLoweringFunction(10)
				jump := builder.AllocateInstruction()
				jump.AsJump(nil, builder.ReturnBlock())
				builder.InsertInstruction(jump)
				return jump
			},
			// Jump which targets the return block should be translated as "ret".
			instructions: []string{"ret"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			instr := tc.setup(ctx, b, m)
			m.LowerSingleBranch(instr)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_InsertMove(t *testing.T) {
	for _, tc := range []struct {
		name        string
		src, dst    regalloc.VReg
		instruction string
	}{
		{
			name:        "int",
			src:         regalloc.VReg(1).SetRegType(regalloc.RegTypeInt),
			dst:         regalloc.VReg(2).SetRegType(regalloc.RegTypeInt),
			instruction: "mov x1?, x2?",
		},
		{
			name:        "float",
			src:         regalloc.VReg(1).SetRegType(regalloc.RegTypeFloat),
			dst:         regalloc.VReg(2).SetRegType(regalloc.RegTypeFloat),
			instruction: "mov v1?.8b, v2?.8b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.InsertMove(tc.src, tc.dst)
			require.Equal(t, tc.instruction, formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}
