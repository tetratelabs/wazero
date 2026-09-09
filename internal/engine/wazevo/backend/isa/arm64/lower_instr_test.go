package arm64

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_LowerConditionalBranch(t *testing.T) {
	cmpInSameGroupFromParams := func(
		brz bool, intCond ssa.IntegerCmpCond, floatCond ssa.FloatCmpCond,
		ctx *mockCompiler, builder ssa.Builder, m *machine,
	) (instr *ssa.Instruction, verify func(t *testing.T)) {
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

		ctx.definitions[val1] = backend.SSAValueDefinition{V: val1}
		ctx.definitions[val2] = backend.SSAValueDefinition{V: val2}
		ctx.definitions[cmpVal] = backend.SSAValueDefinition{Instr: cmpInstr}
		b := builder.AllocateInstruction()
		if brz {
			b.AsBrz(cmpVal, ssa.ValuesNil, builder.AllocateBasicBlock())
		} else {
			b.AsBrnz(cmpVal, ssa.ValuesNil, builder.AllocateBasicBlock())
		}
		builder.InsertInstruction(b)
		return b, func(t *testing.T) {
			require.True(t, cmpInstr.Lowered())
		}
	}

	icmpInSameGroupFromParamAndImm12 := func(brz bool, ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction, verify func(t *testing.T)) {
		entry := builder.CurrentBlock()
		v1 := entry.AddParam(builder, ssa.TypeI32)

		iconst := builder.AllocateInstruction()
		iconst.AsIconst32(0x4d2)
		builder.InsertInstruction(iconst)
		v2 := iconst.Return()

		icmp := builder.AllocateInstruction()
		icmp.AsIcmp(v1, v2, ssa.IntegerCmpCondEqual)
		builder.InsertInstruction(icmp)
		icmpVal := icmp.Return()
		ctx.definitions[v1] = backend.SSAValueDefinition{V: v1}
		ctx.definitions[v2] = backend.SSAValueDefinition{Instr: iconst}
		ctx.definitions[icmpVal] = backend.SSAValueDefinition{Instr: icmp}
		ctx.vRegMap[v1], ctx.vRegMap[v2], ctx.vRegMap[icmpVal] = intToVReg(1), intToVReg(2), intToVReg(3)
		b := builder.AllocateInstruction()
		if brz {
			b.AsBrz(icmpVal, ssa.ValuesNil, builder.AllocateBasicBlock())
		} else {
			b.AsBrnz(icmpVal, ssa.ValuesNil, builder.AllocateBasicBlock())
		}
		builder.InsertInstruction(b)
		return b, func(t *testing.T) {
			require.True(t, icmp.Lowered())
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
				entry := builder.CurrentBlock()
				v1, v2 := entry.AddParam(builder, ssa.TypeI64), entry.AddParam(builder, ssa.TypeI64)

				icmp := builder.AllocateInstruction()
				icmp.AsIcmp(v1, v2, ssa.IntegerCmpCondEqual)
				builder.InsertInstruction(icmp)
				icmpVal := icmp.Return()
				ctx.definitions[icmpVal] = backend.SSAValueDefinition{Instr: icmp, V: icmpVal}
				ctx.vRegMap[v1], ctx.vRegMap[v2], ctx.vRegMap[icmpVal] = intToVReg(1), intToVReg(2), intToVReg(3)

				brz := builder.AllocateInstruction()
				brz.AsBrz(icmpVal, ssa.ValuesNil, builder.AllocateBasicBlock())
				builder.InsertInstruction(brz)

				// Indicate that currently compiling in the different group.
				ctx.currentGID = 1000
				return brz, func(t *testing.T) {
					require.False(t, icmp.Lowered())
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
				jump.AsJump(ssa.ValuesNil, builder.AllocateBasicBlock())
				builder.InsertInstruction(jump)
				jump.AsFallthroughJump()
				return jump
			},
			instructions: []string{}, // Fallthrough jump should be optimized out.
		},
		{
			name: "b",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction) {
				jump := builder.AllocateInstruction()
				jump.AsJump(ssa.ValuesNil, builder.AllocateBasicBlock())
				builder.InsertInstruction(jump)
				return jump
			},
			instructions: []string{"b L1"},
		},
		{
			name: "ret",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (instr *ssa.Instruction) {
				jump := builder.AllocateInstruction()
				jump.AsJump(ssa.ValuesNil, builder.ReturnBlock())
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
		typ         ssa.Type
		instruction string
	}{
		{
			name:        "int",
			src:         regalloc.VReg(1).SetRegType(regalloc.RegTypeInt),
			dst:         regalloc.VReg(2).SetRegType(regalloc.RegTypeInt),
			instruction: "mov x1?, x2?",
			typ:         ssa.TypeI64,
		},
		{
			name:        "float",
			src:         regalloc.VReg(1).SetRegType(regalloc.RegTypeFloat),
			dst:         regalloc.VReg(2).SetRegType(regalloc.RegTypeFloat),
			instruction: "mov v1?.8b, v2?.8b",
			typ:         ssa.TypeF64,
		},
		{
			name:        "vector",
			src:         regalloc.VReg(1).SetRegType(regalloc.RegTypeFloat),
			dst:         regalloc.VReg(2).SetRegType(regalloc.RegTypeFloat),
			instruction: "mov v1?.16b, v2?.16b",
			typ:         ssa.TypeV128,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.InsertMove(tc.src, tc.dst, tc.typ)
			require.Equal(t, tc.instruction, formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_lowerIDiv(t *testing.T) {
	for _, tc := range []struct {
		name   string
		_64bit bool
		signed bool
		exp    string
	}{
		{
			name: "32bit unsigned", _64bit: false, signed: false,
			exp: `
udiv w1?, w2?, w3?
mov x1?, x65535?
cbnz w3?, L1
movz x2?, #0xa, lsl 0
str w2?, [x1?]
mov x3?, sp
str x3?, [x1?, #0x38]
adr x4?, #0x0
str x4?, [x1?, #0x30]
exit_sequence x1?
L1:
`,
		},
		{name: "32bit signed", _64bit: false, signed: true, exp: `
sdiv w1?, w2?, w3?
mov x1?, x65535?
cbnz w3?, L1
movz x2?, #0xa, lsl 0
str w2?, [x1?]
mov x3?, sp
str x3?, [x1?, #0x38]
adr x4?, #0x0
str x4?, [x1?, #0x30]
exit_sequence x1?
L1:
adds wzr, w3?, #0x1
ccmp w2?, #0x1, #0x0, eq
mov x5?, x65535?
b.vc L2
movz x6?, #0xb, lsl 0
str w6?, [x5?]
mov x7?, sp
str x7?, [x5?, #0x38]
adr x8?, #0x0
str x8?, [x5?, #0x30]
exit_sequence x5?
L2:
`},
		{name: "64bit unsigned", _64bit: true, signed: false, exp: `
udiv x1?, x2?, x3?
mov x1?, x65535?
cbnz x3?, L1
movz x2?, #0xa, lsl 0
str w2?, [x1?]
mov x3?, sp
str x3?, [x1?, #0x38]
adr x4?, #0x0
str x4?, [x1?, #0x30]
exit_sequence x1?
L1:
`},
		{name: "64bit signed", _64bit: true, signed: true, exp: `
sdiv x1?, x2?, x3?
mov x1?, x65535?
cbnz x3?, L1
movz x2?, #0xa, lsl 0
str w2?, [x1?]
mov x3?, sp
str x3?, [x1?, #0x38]
adr x4?, #0x0
str x4?, [x1?, #0x30]
exit_sequence x1?
L1:
adds xzr, x3?, #0x1
ccmp x2?, #0x1, #0x0, eq
mov x5?, x65535?
b.vc L2
movz x6?, #0xb, lsl 0
str w6?, [x5?]
mov x7?, sp
str x7?, [x5?, #0x38]
adr x8?, #0x0
str x8?, [x5?, #0x30]
exit_sequence x5?
L2:
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execCtx := regalloc.VReg(0xffff).SetRegType(regalloc.RegTypeInt)
			rd, rn, rm := regalloc.VReg(1).SetRegType(regalloc.RegTypeInt),
				regalloc.VReg(2).SetRegType(regalloc.RegTypeInt),
				regalloc.VReg(3).SetRegType(regalloc.RegTypeInt)
			mc, _, m := newSetupWithMockContext()
			m.maxSSABlockID, m.nextLabel = 1, 1
			mc.typeOf = map[regalloc.VRegID]ssa.Type{execCtx.ID(): ssa.TypeI64, 2: ssa.TypeI64, 3: ssa.TypeI64}
			m.lowerIDiv(execCtx, rd, operandNR(rn), operandNR(rm), tc._64bit, tc.signed)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_exitWithCode(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	m.lowerExitWithCode(x1VReg, wazevoapi.ExitCodeGrowStack)
	m.FlushPendingInstructions()
	m.encode(m.perBlockHead)
	require.Equal(t, `
movz x1?, #0x1, lsl 0
str w1?, [x1]
mov x2?, sp
str x2?, [x1, #0x38]
adr x3?, #0x0
str x3?, [x1, #0x30]
exit_sequence x1
`, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
}

func TestMachine_lowerFpuToInt(t *testing.T) {
	for _, tc := range []struct {
		name        string
		nontrapping bool
		expectedAsm string
	}{
		{
			name:        "trapping",
			nontrapping: false,
			expectedAsm: `
msr fpsr, xzr
fcvtzu w1, s2
mrs x1? fpsr
mov x2?, x15
mov x3?, d2
subs xzr, x1?, #0x1
b.ne L2
fcmp w3?, w3?
mov x4?, x2?
b.vc L1
movz x5?, #0xc, lsl 0
str w5?, [x4?]
mov x6?, sp
str x6?, [x4?, #0x38]
adr x7?, #0x0
str x7?, [x4?, #0x30]
exit_sequence x4?
L1:
movz x8?, #0xb, lsl 0
str w8?, [x2?]
mov x9?, sp
str x9?, [x2?, #0x38]
adr x10?, #0x0
str x10?, [x2?, #0x30]
exit_sequence x2?
L2:
`,
		},
		{
			name:        "nontrapping",
			nontrapping: true,
			expectedAsm: `
fcvtzu w1, s2
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc, _, m := newSetupWithMockContext()
			m.maxSSABlockID, m.nextLabel = 1, 1
			mc.typeOf = map[regalloc.VRegID]ssa.Type{v2VReg.ID(): ssa.TypeI64, x15VReg.ID(): ssa.TypeI64}
			m.lowerFpuToInt(x1VReg, operandNR(v2VReg), x15VReg, false, false, false, tc.nontrapping)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
		})
	}
}

func TestMachine_lowerVIMul(t *testing.T) {
	for _, tc := range []struct {
		name          string
		expectedAsm   string
		arrangement   vecArrangement
		expectedBytes string
	}{
		{
			name:        "2D",
			arrangement: vecArrangement2D,
			expectedAsm: `
rev64 v2?.4s, x15.4s
mul v2?.4s, v2?.4s, x2.4s
xtn v1?.2s, x2.2s
addp v2?.4s, v2?.4s, v2?.4s
xtn v3?.2s, x15.2s
shll v4?.2s, v2?.2s
umlal v4?.2s, v3?.2s, v1?.2s
mov x1.16b, v4?.16b
`,
			expectedBytes: "e009a04e009ca24e4028a10e00bca04ee029a10e0038a12e0080a02e011ca04e",
		},
		{
			name:        "8B",
			arrangement: vecArrangement8B,
			expectedAsm: `
mul x1.8b, x2.8b, x15.8b
`,
			expectedBytes: "419c2f0e",
		},
		{
			name:        "16B",
			arrangement: vecArrangement16B,
			expectedAsm: `
mul x1.16b, x2.16b, x15.16b
`,
			expectedBytes: "419c2f4e",
		},
		{
			name:        "4H",
			arrangement: vecArrangement4H,
			expectedAsm: `
mul x1.4h, x2.4h, x15.4h
`,
			expectedBytes: "419c6f0e",
		},
		{
			name:        "8H",
			arrangement: vecArrangement8H,
			expectedAsm: `
mul x1.8h, x2.8h, x15.8h
`,
			expectedBytes: "419c6f4e",
		},
		{
			name:        "2S",
			arrangement: vecArrangement2S,
			expectedAsm: `
mul x1.2s, x2.2s, x15.2s
`,
			expectedBytes: "419caf0e",
		},
		{
			name:        "4S",
			arrangement: vecArrangement4S,
			expectedAsm: `
mul x1.4s, x2.4s, x15.4s
`,
			expectedBytes: "419caf4e",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.lowerVIMul(x1VReg, operandNR(x2VReg), operandNR(x15VReg), tc.arrangement)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
			buf := m.compiler.Buf()
			require.Equal(t, tc.expectedBytes, hex.EncodeToString(buf))
		})
	}
}

func TestMachine_lowerVcheckTrue(t *testing.T) {
	for _, tc := range []struct {
		name          string
		op            ssa.Opcode
		expectedAsm   string
		arrangement   vecArrangement
		expectedBytes string
	}{
		{
			name: "anyTrue",
			op:   ssa.OpcodeVanyTrue,
			expectedAsm: `
umaxp v1?.16b, x1.16b, x1.16b
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a4216e0f3c084ee0e940faef079f9a",
		},
		{
			name:        "allTrue 2D",
			op:          ssa.OpcodeVallTrue,
			arrangement: vecArrangement2D,
			expectedAsm: `
cmeq v1?.2d, x1.2d, #0
addp v1?.2d, v1?.2d, v1?.2d
fcmp d1?, d1?
cset x15, eq
`,
			expectedBytes: "2098e04e00bce04e0020601eef179f9a",
		},
		{
			name:        "allTrue 8B",
			arrangement: vecArrangement8B,
			op:          ssa.OpcodeVallTrue,
			expectedAsm: `
uminv h1?, x1.8b
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a8312e0f3c084ee0e940faef079f9a",
		},
		{
			name:        "allTrue 16B",
			arrangement: vecArrangement16B,
			op:          ssa.OpcodeVallTrue,
			expectedAsm: `
uminv h1?, x1.16b
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a8316e0f3c084ee0e940faef079f9a",
		},
		{
			name:        "allTrue 4H",
			arrangement: vecArrangement4H,
			op:          ssa.OpcodeVallTrue,
			expectedAsm: `
uminv s1?, x1.4h
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a8712e0f3c084ee0e940faef079f9a",
		},
		{
			name:        "allTrue 8H",
			arrangement: vecArrangement8H,
			op:          ssa.OpcodeVallTrue,
			expectedAsm: `
uminv s1?, x1.8h
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a8716e0f3c084ee0e940faef079f9a",
		},
		{
			name:        "allTrue 4S",
			arrangement: vecArrangement4S,
			op:          ssa.OpcodeVallTrue,
			expectedAsm: `
uminv d1?, x1.4s
mov x15, v1?.d[0]
ccmp x15, #0x0, #0x0, al
cset x15, ne
`,
			expectedBytes: "20a8b16e0f3c084ee0e940faef079f9a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.lowerVcheckTrue(tc.op, operandNR(x1VReg), x15VReg, tc.arrangement)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
			buf := m.compiler.Buf()
			require.Equal(t, tc.expectedBytes, hex.EncodeToString(buf))
		})
	}
}

func TestMachine_lowerVhighBits(t *testing.T) {
	for _, tc := range []struct {
		name          string
		expectedAsm   string
		arrangement   vecArrangement
		expectedBytes string
	}{
		{
			name:        "16B",
			arrangement: vecArrangement16B,
			expectedAsm: `
sshr v3?.16b, x1.16b, #7
movz x1?, #0x201, lsl 0
movk x1?, #0x804, lsl 16
movk x1?, #0x2010, lsl 32
movk x1?, #0x8040, lsl 48
dup v2?.2d, x1?
and v3?.16b, v3?.16b, v2?.16b
ext v2?.16b, v3?.16b, v3?.16b, #8
zip1 v2?.16b, v3?.16b, v2?.16b
addv s2?, v2?.8h
umov w15, v2?.h[0]
`,
			expectedBytes: "2004094f204080d28000a1f20002c4f20008f0f2000c084e001c204e0040006e0038004e00b8714e0f3c020e",
		},
		{
			name:        "8H",
			arrangement: vecArrangement8H,
			expectedAsm: `
sshr v3?.8h, x1.8h, #15
movz x1?, #0x1, lsl 0
movk x1?, #0x2, lsl 16
movk x1?, #0x4, lsl 32
movk x1?, #0x8, lsl 48
dup v2?.2d, x1?
lsl x1?, x1?, 0x4
ins v2?.d[1], x1?
and v2?.16b, v3?.16b, v2?.16b
addv s2?, v2?.8h
umov w15, v2?.h[0]
`,
			expectedBytes: "2004114f200080d24000a0f28000c0f20001e0f2000c084e00ec7cd3001c184e001c204e00b8714e0f3c020e",
		},
		{
			name:        "4S",
			arrangement: vecArrangement4S,
			expectedAsm: `
sshr v3?.4s, x1.4s, #31
movz x1?, #0x1, lsl 0
movk x1?, #0x2, lsl 32
dup v2?.2d, x1?
lsl x1?, x1?, 0x2
ins v2?.d[1], x1?
and v2?.16b, v3?.16b, v2?.16b
addv d2?, v2?.4s
umov w15, v2?.s[0]
`,
			expectedBytes: "2004214f200080d24000c0f2000c084e00f47ed3001c184e001c204e00b8b14e0f3c040e",
		},
		{
			name:        "2D",
			arrangement: vecArrangement2D,
			expectedAsm: `
mov x15, x1.d[0]
mov x1?, x1.d[1]
lsr x1?, x1?, 0x3f
lsr x15, x15, 0x3f
add w15, w15, w1?, lsl #1
`,
			expectedBytes: "2f3c084e203c184e00fc7fd3effd7fd3ef05000b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.lowerVhighBits(operandNR(x1VReg), x15VReg, tc.arrangement)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
			buf := m.compiler.Buf()
			require.Equal(t, tc.expectedBytes, hex.EncodeToString(buf))
		})
	}
}

func TestMachine_lowerShuffle(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lanes         []uint64
		expectedAsm   string
		expectedBytes string
	}{
		{
			name:  "lanes 0..15",
			lanes: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			expectedAsm: `
mov v29.16b, x2.16b
mov v30.16b, x15.16b
ldr q1?, #8; b 32; data.v128  0706050403020100 0f0e0d0c0b0a0908
tbl x1.16b, { v29.16b, v30.16b }, v1?.16b
`,
			expectedBytes: "5d1ca24efe1daf4e4000009c05000014000102030405060708090a0b0c0d0e0fa123004e",
		},
		{
			name:  "lanes 0101...",
			lanes: []uint64{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1},
			expectedAsm: `
mov v29.16b, x2.16b
mov v30.16b, x15.16b
ldr q1?, #8; b 32; data.v128  0100010001000100 0100010001000100
tbl x1.16b, { v29.16b, v30.16b }, v1?.16b
`,
			expectedBytes: "5d1ca24efe1daf4e4000009c0500001400010001000100010001000100010001a123004e",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			lanes := tc.lanes

			// Encode the 16 bytes as 8 bytes in u1, and 8 bytes in u2.
			lane1 := lanes[7]<<56 | lanes[6]<<48 | lanes[5]<<40 | lanes[4]<<32 | lanes[3]<<24 | lanes[2]<<16 | lanes[1]<<8 | lanes[0]
			lane2 := lanes[15]<<56 | lanes[14]<<48 | lanes[13]<<40 | lanes[12]<<32 | lanes[11]<<24 | lanes[10]<<16 | lanes[9]<<8 | lanes[8]

			m.lowerShuffle(x1VReg, operandNR(x2VReg), operandNR(x15VReg), lane1, lane2)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
			buf := m.compiler.Buf()
			require.Equal(t, tc.expectedBytes, hex.EncodeToString(buf))
		})
	}
}

func TestMachine_lowerVShift(t *testing.T) {
	for _, tc := range []struct {
		name          string
		expectedAsm   string
		op            ssa.Opcode
		arrangement   vecArrangement
		expectedBytes string
	}{
		{
			name:        "VIshl",
			op:          ssa.OpcodeVIshl,
			arrangement: vecArrangement16B,
			expectedAsm: `
and x1?, x15, #0x7
dup v2?.16b, x1?
sshl x1.16b, x2.16b, v2?.16b
`,
			expectedBytes: "e0094092000c014e4144204e",
		},
		{
			name:        "VSshr",
			op:          ssa.OpcodeVSshr,
			arrangement: vecArrangement16B,
			expectedAsm: `
and x1?, x15, #0x7
sub x1?, xzr, x1?
dup v2?.16b, x1?
sshl x1.16b, x2.16b, v2?.16b
`,
			expectedBytes: "e0094092e00300cb000c014e4144204e",
		},
		{
			name:        "VUshr",
			op:          ssa.OpcodeVUshr,
			arrangement: vecArrangement16B,
			expectedAsm: `
and x1?, x15, #0x7
sub x1?, xzr, x1?
dup v2?.16b, x1?
ushl x1.16b, x2.16b, v2?.16b
`,
			expectedBytes: "e0094092e00300cb000c014e4144206e",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.lowerVShift(tc.op, x1VReg, operandNR(x2VReg), operandNR(x15VReg), tc.arrangement)
			require.Equal(t, tc.expectedAsm, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")

			m.FlushPendingInstructions()
			m.encode(m.perBlockHead)
			buf := m.compiler.Buf()
			require.Equal(t, tc.expectedBytes, hex.EncodeToString(buf))
		})
	}
}

func TestMachine_lowerSelectVec(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	c := operandNR(m.compiler.AllocateVReg(ssa.TypeI32))
	rn := operandNR(m.compiler.AllocateVReg(ssa.TypeV128))
	rm := operandNR(m.compiler.AllocateVReg(ssa.TypeV128))
	rd := m.compiler.AllocateVReg(ssa.TypeV128)

	require.Equal(t, 1, int(c.reg().ID()))
	require.Equal(t, 2, int(rn.reg().ID()))
	require.Equal(t, 3, int(rm.reg().ID()))
	require.Equal(t, 4, int(rd.ID()))

	m.lowerSelectVec(c, rn, rm, rd)
	require.Equal(t, `
subs wzr, w1?, wzr
csetm x5?, ne
dup v6?.2d, x5?
bsl v6?.16b, v2?.16b, v3?.16b
mov v4?.16b, v6?.16b
`, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
}

func TestMachine_lowerFcopysign(t *testing.T) {
	for _, tc := range []struct {
		_64bit bool
		exp    string
	}{
		{
			_64bit: false,
			exp: `
movz w1?, #0x8000, lsl 16
ins v2?.s[0], w1?
mov v6?.8b, v3?.8b
bit v6?.8b, v4?.8b, v2?.8b
mov v5?.8b, v6?.8b
`,
		},
		{
			_64bit: true,
			exp: `
movz x1?, #0x8000, lsl 48
ins v2?.d[0], x1?
mov v6?.8b, v3?.8b
bit v6?.8b, v4?.8b, v2?.8b
mov v5?.8b, v6?.8b
`,
		},
	} {
		t.Run(fmt.Sprintf("64bit=%v", tc._64bit), func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ, ftyp ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
				ftyp = ssa.TypeF64
			} else {
				typ = ssa.TypeI32
				ftyp = ssa.TypeF32
			}
			tmpI := m.compiler.AllocateVReg(typ)
			tmpF := m.compiler.AllocateVReg(ftyp)
			rn := operandNR(m.compiler.AllocateVReg(ftyp))
			rm := operandNR(m.compiler.AllocateVReg(ftyp))
			rd := m.compiler.AllocateVReg(ftyp)

			require.Equal(t, 1, int(tmpI.ID()))
			require.Equal(t, 2, int(tmpF.ID()))
			require.Equal(t, 3, int(rn.reg().ID()))
			require.Equal(t, 4, int(rm.reg().ID()))
			require.Equal(t, 5, int(rd.ID()))

			m.lowerFcopysignImpl(rd, rn, rm, tmpI, tmpF, tc._64bit)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_lowerRotl(t *testing.T) {
	for _, tc := range []struct {
		_64bit bool
		exp    string
	}{
		{
			_64bit: false,
			exp: `
sub w1?, wzr, w3?
ror w4?, w2?, w1?
`,
		},
		{
			_64bit: true,
			exp: `
sub x1?, xzr, x3?
ror x4?, x2?, x1?
`,
		},
	} {
		t.Run(fmt.Sprintf("64bit=%v", tc._64bit), func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
			} else {
				typ = ssa.TypeI32
			}
			tmpI := m.compiler.AllocateVReg(typ)
			rn := operandNR(m.compiler.AllocateVReg(typ))
			rm := operandNR(m.compiler.AllocateVReg(typ))
			rd := m.compiler.AllocateVReg(typ)

			require.Equal(t, 1, int(tmpI.ID()))
			require.Equal(t, 2, int(rn.reg().ID()))
			require.Equal(t, 3, int(rm.reg().ID()))
			require.Equal(t, 4, int(rd.ID()))

			m.lowerRotlImpl(rd, rn, rm, tmpI, tc._64bit)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_lowerAtomicRmw(t *testing.T) {
	tests := []struct {
		name      string
		op        atomicRmwOp
		negateArg bool
		flipArg   bool
		_64bit    bool
		size      uint64
		exp       string
	}{
		{
			name: "add 32",
			op:   atomicRmwOpAdd,
			size: 4,
			exp: `
ldaddal w3?, w4?, x2?
`,
		},
		{
			name: "add 32_16u",
			op:   atomicRmwOpAdd,
			size: 2,
			exp: `
ldaddalh w3?, w4?, x2?
`,
		},
		{
			name: "add 32_8u",
			op:   atomicRmwOpAdd,
			size: 1,
			exp: `
ldaddalb w3?, w4?, x2?
`,
		},
		{
			name:   "add 64",
			op:     atomicRmwOpAdd,
			size:   8,
			_64bit: true,
			exp: `
ldaddal x3?, x4?, x2?
`,
		},
		{
			name:   "add 64_32u",
			op:     atomicRmwOpAdd,
			size:   4,
			_64bit: true,
			exp: `
ldaddal w3?, w4?, x2?
`,
		},
		{
			name:   "add 64_16u",
			op:     atomicRmwOpAdd,
			size:   2,
			_64bit: true,
			exp: `
ldaddalh w3?, w4?, x2?
`,
		},
		{
			name:   "add 64_8u",
			op:     atomicRmwOpAdd,
			size:   1,
			_64bit: true,
			exp: `
ldaddalb w3?, w4?, x2?
`,
		},
		{
			name:      "sub 32",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      4,
			exp: `
sub w1?, wzr, w3?
ldaddal w1?, w4?, x2?
`,
		},
		{
			name:      "sub 32_16u",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      2,
			exp: `
sub w1?, wzr, w3?
ldaddalh w1?, w4?, x2?
`,
		},
		{
			name:      "sub 32_8u",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      1,
			exp: `
sub w1?, wzr, w3?
ldaddalb w1?, w4?, x2?
`,
		},
		{
			name:      "sub 64",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      8,
			_64bit:    true,
			exp: `
sub x1?, xzr, x3?
ldaddal x1?, x4?, x2?
`,
		},
		{
			name:      "sub 64_32u",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      4,
			_64bit:    true,
			exp: `
sub x1?, xzr, x3?
ldaddal w1?, w4?, x2?
`,
		},
		{
			name:      "sub 64_16u",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      2,
			_64bit:    true,
			exp: `
sub x1?, xzr, x3?
ldaddalh w1?, w4?, x2?
`,
		},
		{
			name:      "sub 64_8u",
			op:        atomicRmwOpAdd,
			negateArg: true,
			size:      1,
			_64bit:    true,
			exp: `
sub x1?, xzr, x3?
ldaddalb w1?, w4?, x2?
`,
		},
		{
			name:    "and 32",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    4,
			exp: `
orn w1?, wzr, w3?
ldclral w1?, w4?, x2?
`,
		},
		{
			name:    "and 32_16u",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    2,
			exp: `
orn w1?, wzr, w3?
ldclralh w1?, w4?, x2?
`,
		},
		{
			name:    "and 32_8u",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    1,
			exp: `
orn w1?, wzr, w3?
ldclralb w1?, w4?, x2?
`,
		},
		{
			name:    "and 64",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    8,
			_64bit:  true,
			exp: `
orn x1?, xzr, x3?
ldclral x1?, x4?, x2?
`,
		},
		{
			name:    "and 64_32u",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    4,
			_64bit:  true,
			exp: `
orn x1?, xzr, x3?
ldclral w1?, w4?, x2?
`,
		},
		{
			name:    "and 64_16u",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    2,
			_64bit:  true,
			exp: `
orn x1?, xzr, x3?
ldclralh w1?, w4?, x2?
`,
		},
		{
			name:    "and 64_8u",
			op:      atomicRmwOpClr,
			flipArg: true,
			size:    1,
			_64bit:  true,
			exp: `
orn x1?, xzr, x3?
ldclralb w1?, w4?, x2?
`,
		},
		{
			name: "or 32",
			op:   atomicRmwOpSet,
			size: 4,
			exp: `
ldsetal w3?, w4?, x2?
`,
		},
		{
			name: "or 32_16u",
			op:   atomicRmwOpSet,
			size: 2,
			exp: `
ldsetalh w3?, w4?, x2?
`,
		},
		{
			name: "or 32_8u",
			op:   atomicRmwOpSet,
			size: 1,
			exp: `
ldsetalb w3?, w4?, x2?
`,
		},
		{
			name:   "or 64",
			op:     atomicRmwOpSet,
			size:   8,
			_64bit: true,
			exp: `
ldsetal x3?, x4?, x2?
`,
		},
		{
			name:   "or 64_32u",
			op:     atomicRmwOpSet,
			size:   4,
			_64bit: true,
			exp: `
ldsetal w3?, w4?, x2?
`,
		},
		{
			name:   "or 64_16u",
			op:     atomicRmwOpSet,
			size:   2,
			_64bit: true,
			exp: `
ldsetalh w3?, w4?, x2?
`,
		},
		{
			name:   "or 64_8u",
			op:     atomicRmwOpSet,
			size:   1,
			_64bit: true,
			exp: `
ldsetalb w3?, w4?, x2?
`,
		},
		{
			name: "xor 32",
			op:   atomicRmwOpEor,
			size: 4,
			exp: `
ldeoral w3?, w4?, x2?
`,
		},
		{
			name: "xor 32_16u",
			op:   atomicRmwOpEor,
			size: 2,
			exp: `
ldeoralh w3?, w4?, x2?
`,
		},
		{
			name: "xor 32_8u",
			op:   atomicRmwOpEor,
			size: 1,
			exp: `
ldeoralb w3?, w4?, x2?
`,
		},
		{
			name:   "xor 64",
			op:     atomicRmwOpEor,
			size:   8,
			_64bit: true,
			exp: `
ldeoral x3?, x4?, x2?
`,
		},
		{
			name:   "xor 64_32u",
			op:     atomicRmwOpEor,
			size:   4,
			_64bit: true,
			exp: `
ldeoral w3?, w4?, x2?
`,
		},
		{
			name:   "xor 64_16u",
			op:     atomicRmwOpEor,
			size:   2,
			_64bit: true,
			exp: `
ldeoralh w3?, w4?, x2?
`,
		},
		{
			name:   "xor 64_8u",
			op:     atomicRmwOpEor,
			size:   1,
			_64bit: true,
			exp: `
ldeoralb w3?, w4?, x2?
`,
		},
		{
			name: "xchg 32",
			op:   atomicRmwOpSwp,
			size: 4,
			exp: `
swpal w3?, w4?, x2?
`,
		},
		{
			name: "xchg 32_16u",
			op:   atomicRmwOpSwp,
			size: 2,
			exp: `
swpalh w3?, w4?, x2?
`,
		},
		{
			name: "xchg 32_8u",
			op:   atomicRmwOpSwp,
			size: 1,
			exp: `
swpalb w3?, w4?, x2?
`,
		},
		{
			name:   "xchg 64",
			op:     atomicRmwOpSwp,
			size:   8,
			_64bit: true,
			exp: `
swpal x3?, x4?, x2?
`,
		},
		{
			name:   "xchg 64_32u",
			op:     atomicRmwOpSwp,
			size:   4,
			_64bit: true,
			exp: `
swpal w3?, w4?, x2?
`,
		},
		{
			name:   "xchg 64_16u",
			op:     atomicRmwOpSwp,
			size:   2,
			_64bit: true,
			exp: `
swpalh w3?, w4?, x2?
`,
		},
		{
			name:   "xchg 64_8u",
			op:     atomicRmwOpSwp,
			size:   1,
			_64bit: true,
			exp: `
swpalb w3?, w4?, x2?
`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
			} else {
				typ = ssa.TypeI32
			}
			tmp := m.compiler.AllocateVReg(typ)
			rn := m.compiler.AllocateVReg(ssa.TypeI64)
			rs := m.compiler.AllocateVReg(typ)
			rt := m.compiler.AllocateVReg(typ)

			require.Equal(t, 1, int(tmp.ID()))
			require.Equal(t, 2, int(rn.ID()))
			require.Equal(t, 3, int(rs.ID()))
			require.Equal(t, 4, int(rt.ID()))

			m.lowerAtomicRmwImpl(tc.op, rn, rs, rt, tmp, tc.size, tc.negateArg, tc.flipArg, tc._64bit)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_lowerAtomicCas(t *testing.T) {
	tests := []struct {
		name   string
		_64bit bool
		size   uint64
		exp    string
	}{
		{
			name: "cas 32",
			size: 4,
			exp: `
casal w2?, w3?, x1?
`,
		},
		{
			name: "cas 32_16u",
			size: 2,
			exp: `
casalh w2?, w3?, x1?
`,
		},
		{
			name: "cas 32_8u",
			size: 1,
			exp: `
casalb w2?, w3?, x1?
`,
		},
		{
			name:   "cas 64",
			size:   8,
			_64bit: true,
			exp: `
casal x2?, x3?, x1?
`,
		},
		{
			name:   "cas 64_32u",
			size:   4,
			_64bit: true,
			exp: `
casal w2?, w3?, x1?
`,
		},
		{
			name:   "cas 64_16u",
			size:   2,
			_64bit: true,
			exp: `
casalh w2?, w3?, x1?
`,
		},
		{
			name:   "cas 64_8u",
			size:   1,
			_64bit: true,
			exp: `
casalb w2?, w3?, x1?
`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
			} else {
				typ = ssa.TypeI32
			}
			rn := m.compiler.AllocateVReg(ssa.TypeI64)
			rs := m.compiler.AllocateVReg(typ)
			rt := m.compiler.AllocateVReg(typ)

			require.Equal(t, 1, int(rn.ID()))
			require.Equal(t, 2, int(rs.ID()))
			require.Equal(t, 3, int(rt.ID()))

			m.lowerAtomicCasImpl(rn, rs, rt, tc.size)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_lowerAtomicLoad(t *testing.T) {
	tests := []struct {
		name   string
		_64bit bool
		size   uint64
		exp    string
	}{
		{
			name: "load 32",
			size: 4,
			exp: `
ldar w2?, x1?
`,
		},
		{
			name: "load 32_16u",
			size: 2,
			exp: `
ldarh w2?, x1?
`,
		},
		{
			name: "load 32_8u",
			size: 1,
			exp: `
ldarb w2?, x1?
`,
		},
		{
			name:   "load 64",
			size:   8,
			_64bit: true,
			exp: `
ldar x2?, x1?
`,
		},
		{
			name:   "load 64_32u",
			size:   4,
			_64bit: true,
			exp: `
ldar w2?, x1?
`,
		},
		{
			name:   "load 64_16u",
			size:   2,
			_64bit: true,
			exp: `
ldarh w2?, x1?
`,
		},
		{
			name:   "load 64_8u",
			size:   1,
			_64bit: true,
			exp: `
ldarb w2?, x1?
`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
			} else {
				typ = ssa.TypeI32
			}
			rn := m.compiler.AllocateVReg(ssa.TypeI64)
			rt := m.compiler.AllocateVReg(typ)

			require.Equal(t, 1, int(rn.ID()))
			require.Equal(t, 2, int(rt.ID()))

			m.lowerAtomicLoadImpl(rn, rt, tc.size)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}

func TestMachine_lowerAtomicStore(t *testing.T) {
	tests := []struct {
		name   string
		_64bit bool
		size   uint64
		exp    string
	}{
		{
			name: "store 32",
			size: 4,
			exp: `
stlr w2?, x1?
`,
		},
		{
			name: "store 32_16u",
			size: 2,
			exp: `
stlrh w2?, x1?
`,
		},
		{
			name: "store 32_8u",
			size: 1,
			exp: `
stlrb w2?, x1?
`,
		},
		{
			name:   "store 64",
			size:   8,
			_64bit: true,
			exp: `
stlr x2?, x1?
`,
		},
		{
			name:   "store 64_32u",
			size:   4,
			_64bit: true,
			exp: `
stlr w2?, x1?
`,
		},
		{
			name:   "store 64_16u",
			size:   2,
			_64bit: true,
			exp: `
stlrh w2?, x1?
`,
		},
		{
			name:   "store 64_8u",
			size:   1,
			_64bit: true,
			exp: `
stlrb w2?, x1?
`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			var typ ssa.Type
			if tc._64bit {
				typ = ssa.TypeI64
			} else {
				typ = ssa.TypeI32
			}
			rn := operandNR(m.compiler.AllocateVReg(ssa.TypeI64))
			rt := operandNR(m.compiler.AllocateVReg(typ))

			require.Equal(t, 1, int(rn.reg().ID()))
			require.Equal(t, 2, int(rt.reg().ID()))

			m.lowerAtomicStoreImpl(rn, rt, tc.size)
			require.Equal(t, tc.exp, "\n"+formatEmittedInstructionsInCurrentBlock(m)+"\n")
		})
	}
}
