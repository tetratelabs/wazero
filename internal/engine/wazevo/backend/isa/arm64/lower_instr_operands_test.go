package arm64

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_asImm12(t *testing.T) {
	v, shift, ok := asImm12(0xfff)
	require.True(t, ok)
	require.True(t, shift == 0)
	require.Equal(t, uint16(0xfff), v)

	_, _, ok = asImm12(0xfff << 1)
	require.False(t, ok)

	v, shift, ok = asImm12(0xabc << 12)
	require.True(t, ok)
	require.True(t, shift == 1)
	require.Equal(t, uint16(0xabc), v)

	_, _, ok = asImm12(0xabc<<12 | 0b1)
	require.False(t, ok)
}

func TestMachine_getOperand_NR(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (def *backend.SSAValueDefinition, mode extMode)
		exp          operand
		instructions []string
	}{
		{
			name: "block param - no extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI64)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeZeroExtend64
			},
			exp: operandNR(regToVReg(x4)),
		},
		{
			name: "block param - zero extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI32)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeZeroExtend64
			},
			exp:          operandNR(regalloc.VReg(1).SetRegType(regalloc.RegTypeInt)),
			instructions: []string{"uxtw x1?, w4"},
		},
		{
			name: "block param - sign extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI32)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeSignExtend64
			},
			exp:          operandNR(regalloc.VReg(1).SetRegType(regalloc.RegTypeInt)),
			instructions: []string{"sxtw x1?, w4"},
		},
		{
			name: "const instr",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				instr := builder.AllocateInstruction()
				instr.AsIconst32(0xf00000f)
				builder.InsertInstruction(instr)
				def = &backend.SSAValueDefinition{Instr: instr, N: 0}
				ctx.vRegCounter = 99
				return
			},
			exp: operandNR(regalloc.VReg(100).SetRegType(regalloc.RegTypeInt)),
			instructions: []string{
				"movz w100?, #0xf, lsl 0",
				"movk w100?, #0xf00, lsl 16",
			},
		},
		{
			name: "non const instr",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				c := builder.AllocateInstruction()
				sig := &ssa.Signature{Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64, ssa.TypeF64}}
				builder.DeclareSignature(sig)
				c.AsCall(ssa.FuncRef(0), sig, nil)
				builder.InsertInstruction(c)
				_, rs := c.Returns()
				ctx.vRegMap[rs[1]] = regalloc.VReg(50)
				def = &backend.SSAValueDefinition{Instr: c, N: 2}
				return
			},
			exp: operandNR(regalloc.VReg(50)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def, mode := tc.setup(ctx, b, m)
			actual := m.getOperand_NR(def, mode)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_getOperand_SR_NR(t *testing.T) {
	ishlWithConstAmount := func(
		ctx *mockCompiler, builder ssa.Builder, m *machine,
	) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
		blk := builder.CurrentBlock()
		// (p1+p2) << amount
		p1 := blk.AddParam(builder, ssa.TypeI64)
		p2 := blk.AddParam(builder, ssa.TypeI64)
		add := builder.AllocateInstruction()
		add.AsIadd(p1, p2)
		builder.InsertInstruction(add)
		addResult := add.Return()

		amount := builder.AllocateInstruction()
		amount.AsIconst32(14)
		builder.InsertInstruction(amount)

		amountVal := amount.Return()

		ishl := builder.AllocateInstruction()
		ishl.AsIshl(addResult, amountVal)
		builder.InsertInstruction(ishl)

		ctx.definitions[p1] = &backend.SSAValueDefinition{BlkParamVReg: regalloc.VReg(1), BlockParamValue: p1}
		ctx.definitions[p2] = &backend.SSAValueDefinition{BlkParamVReg: regalloc.VReg(2), BlockParamValue: p2}
		ctx.definitions[addResult] = &backend.SSAValueDefinition{Instr: add, N: 0}
		ctx.definitions[amountVal] = &backend.SSAValueDefinition{Instr: amount, N: 0}

		ctx.vRegMap[addResult] = regalloc.VReg(1234)
		ctx.vRegMap[ishl.Return()] = regalloc.VReg(10)
		def = &backend.SSAValueDefinition{Instr: ishl, N: 0}
		mode = extModeNone
		verify = func(t *testing.T) {
			_, ok := ctx.lowered[ishl]
			require.True(t, ok)
			_, ok = ctx.lowered[amount]
			require.True(t, ok)
		}
		return
	}

	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T))
		exp          operand
		instructions []string
	}{
		{
			name: "block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI64)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeNone, func(t *testing.T) {}
			},
			exp: operandNR(regToVReg(x4)),
		},
		{
			name: "ishl but not const amount",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
				blk := builder.CurrentBlock()
				// (p1+p2) << p3
				p1 := blk.AddParam(builder, ssa.TypeI64)
				p2 := blk.AddParam(builder, ssa.TypeI64)
				p3 := blk.AddParam(builder, ssa.TypeI64)
				add := builder.AllocateInstruction()
				add.AsIadd(p1, p2)
				builder.InsertInstruction(add)
				addResult := add.Return()

				ishl := builder.AllocateInstruction()
				ishl.AsIshl(addResult, p3)
				builder.InsertInstruction(ishl)

				ctx.definitions[p1] = &backend.SSAValueDefinition{BlkParamVReg: regalloc.VReg(1), BlockParamValue: p1}
				ctx.definitions[p2] = &backend.SSAValueDefinition{BlkParamVReg: regalloc.VReg(2), BlockParamValue: p2}
				ctx.definitions[p3] = &backend.SSAValueDefinition{BlkParamVReg: regalloc.VReg(3), BlockParamValue: p3}
				ctx.definitions[addResult] = &backend.SSAValueDefinition{Instr: add, N: 0}

				ctx.vRegMap[ishl.Return()] = regalloc.VReg(10)
				def = &backend.SSAValueDefinition{Instr: ishl, N: 0}
				return def, extModeNone, func(t *testing.T) {}
			},
			exp: operandNR(regalloc.VReg(10)),
		},
		{
			name:  "ishl with const amount",
			setup: ishlWithConstAmount,
			exp:   operandSR(regalloc.VReg(1234), 14, shiftOpLSL),
		},
		{
			name: "ishl with const amount but group id is different",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
				def, mode, _ = ishlWithConstAmount(ctx, builder, m)
				ctx.currentGID = 1230
				verify = func(t *testing.T) { require.Equal(t, 0, len(ctx.lowered)) }
				return
			},
			exp: operandNR(regalloc.VReg(10)),
		},
		{
			name: "ishl with const amount but ref count is larger than 1",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
				def, mode, _ = ishlWithConstAmount(ctx, builder, m)
				def.RefCount = 10
				verify = func(t *testing.T) { require.Equal(t, 0, len(ctx.lowered)) }
				return
			},
			exp: operandNR(regalloc.VReg(10)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def, mode, verify := tc.setup(ctx, b, m)
			actual := m.getOperand_SR_NR(def, mode)
			require.Equal(t, tc.exp, actual)
			verify(t)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_getOperand_ER_SR_NR(t *testing.T) {
	type testCase struct {
		setup        func(*mockCompiler, ssa.Builder, *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T))
		exp          operand
		instructions []string
	}
	runner := func(tc testCase) {
		ctx, b, m := newSetupWithMockContext()
		def, mode, verify := tc.setup(ctx, b, m)
		actual := m.getOperand_ER_SR_NR(def, mode)
		require.Equal(t, tc.exp, actual)
		verify(t)
		require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
	}

	t.Run("block param", func(t *testing.T) {
		runner(testCase{
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI64)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeZeroExtend64, func(t *testing.T) {}
			},
			exp: operandNR(regToVReg(x4)),
		})
	})

	t.Run("mode none", func(t *testing.T) {
		for _, c := range []struct {
			from, to byte
			signed   bool
			exp      operand
		}{
			{from: 8, to: 32, signed: true, exp: operandER(regalloc.VReg(10), extendOpSXTB, 32)},
			{from: 16, to: 32, signed: true, exp: operandER(regalloc.VReg(10), extendOpSXTH, 32)},
			{from: 8, to: 32, signed: false, exp: operandER(regalloc.VReg(10), extendOpUXTB, 32)},
			{from: 16, to: 32, signed: false, exp: operandER(regalloc.VReg(10), extendOpUXTH, 32)},
			{from: 8, to: 64, signed: true, exp: operandER(regalloc.VReg(10), extendOpSXTB, 64)},
			{from: 16, to: 64, signed: true, exp: operandER(regalloc.VReg(10), extendOpSXTH, 64)},
			{from: 32, to: 64, signed: true, exp: operandER(regalloc.VReg(10), extendOpSXTW, 64)},
			{from: 8, to: 64, signed: false, exp: operandER(regalloc.VReg(10), extendOpUXTB, 64)},
			{from: 16, to: 64, signed: false, exp: operandER(regalloc.VReg(10), extendOpUXTH, 64)},
			{from: 32, to: 64, signed: false, exp: operandER(regalloc.VReg(10), extendOpUXTW, 64)},
		} {
			runner(testCase{
				setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
					blk := builder.CurrentBlock()
					v := blk.AddParam(builder, ssa.TypeI64)
					ext := builder.AllocateInstruction()
					if c.signed {
						ext.AsSExtend(v, c.from, c.to)
					} else {
						ext.AsUExtend(v, c.from, c.to)
					}
					builder.InsertInstruction(ext)
					ctx.vRegMap[ext.Arg()] = regalloc.VReg(10)
					def = &backend.SSAValueDefinition{Instr: ext, N: 0}
					return def, extModeNone, func(t *testing.T) {
						_, ok := ctx.lowered[ext]
						require.True(t, ok)
					}
				},
				exp: c.exp,
			})
		}
	})

	t.Run("valid mode", func(t *testing.T) {
		const argVReg, resultVReg = regalloc.VReg(10), regalloc.VReg(11)
		for _, c := range []struct {
			name     string
			from, to byte
			signed   bool
			mode     extMode
			exp      operand
			lowered  bool
		}{
			{
				name: "8->16->32: signed",
				from: 8, to: 16, signed: true, mode: extModeSignExtend32,
				exp:     operandER(argVReg, extendOpSXTB, 32),
				lowered: true,
			},
			{
				name: "8->16->32: unsigned",
				from: 8, to: 16, signed: false, mode: extModeZeroExtend32,
				exp:     operandER(argVReg, extendOpUXTB, 32),
				lowered: true,
			},
			{
				name: "8->32->64: signed",
				from: 8, to: 32, signed: true, mode: extModeSignExtend64,
				exp:     operandER(argVReg, extendOpSXTB, 64),
				lowered: true,
			},
			{
				name: "16->32->64: signed",
				from: 16, to: 32, signed: true, mode: extModeSignExtend64,
				exp:     operandER(argVReg, extendOpSXTH, 64),
				lowered: true,
			},
			{
				name: "8->32->64: unsigned",
				from: 8, to: 32, signed: false, mode: extModeZeroExtend64,
				exp:     operandER(argVReg, extendOpUXTB, 64),
				lowered: true,
			},
			{
				name: "16->32->64: unsigned",
				from: 16, to: 32, signed: false, mode: extModeZeroExtend64,
				exp:     operandER(argVReg, extendOpUXTH, 64),
				lowered: true,
			},
			{
				name: "8->16->64: signed",
				from: 8, to: 16, signed: true, mode: extModeSignExtend64,
				exp:     operandER(argVReg, extendOpSXTB, 64),
				lowered: true,
			},
			{
				name: "8->16->64: unsigned",
				from: 8, to: 16, signed: false, mode: extModeZeroExtend64,
				exp:     operandER(argVReg, extendOpUXTB, 64),
				lowered: true,
			},
			// Not lowered cases.
			{
				name: "8-signed->16-zero->64",
				from: 8, to: 16, signed: true, mode: extModeZeroExtend64,
				exp:     operandER(resultVReg, extendOpUXTH, 64),
				lowered: false,
			},
			{
				name: "8-signed->32-zero->64",
				from: 8, to: 32, signed: true, mode: extModeZeroExtend64,
				exp:     operandER(resultVReg, extendOpUXTW, 64),
				lowered: false,
			},
			{
				name: "16-signed->32-zero->64",
				from: 16, to: 32, signed: true, mode: extModeZeroExtend64,
				exp:     operandER(resultVReg, extendOpUXTW, 64),
				lowered: false,
			},
			{
				name: "8-zero->16-signed->64",
				from: 8, to: 16, signed: false, mode: extModeSignExtend64,
				exp:     operandER(resultVReg, extendOpSXTH, 64),
				lowered: false,
			},
			{
				name: "8-zero->32-signed->64",
				from: 8, to: 32, signed: false, mode: extModeSignExtend64,
				exp:     operandER(resultVReg, extendOpSXTW, 64),
				lowered: false,
			},
			{
				name: "16-zero->32-signed->64",
				from: 16, to: 32, signed: false, mode: extModeSignExtend64,
				exp:     operandER(resultVReg, extendOpSXTW, 64),
				lowered: false,
			},
		} {
			t.Run(c.name, func(t *testing.T) {
				runner(testCase{
					setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode, verify func(t *testing.T)) {
						blk := builder.CurrentBlock()
						v := blk.AddParam(builder, ssa.TypeI64)
						ext := builder.AllocateInstruction()
						if c.signed {
							ext.AsSExtend(v, c.from, c.to)
						} else {
							ext.AsUExtend(v, c.from, c.to)
						}
						builder.InsertInstruction(ext)
						ctx.vRegMap[ext.Arg()] = argVReg
						ctx.vRegMap[ext.Return()] = resultVReg
						def = &backend.SSAValueDefinition{Instr: ext, N: 0}
						return def, c.mode, func(t *testing.T) {
							_, ok := ctx.lowered[ext]
							require.Equal(t, c.lowered, ok)
						}
					},
					exp: c.exp,
				})
			})
		}
	})
}

func TestMachine_getOperand_Imm12_ER_SR_NR(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (def *backend.SSAValueDefinition, mode extMode)
		exp          operand
		instructions []string
	}{
		{
			name: "block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI64)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeZeroExtend64
			},
			exp: operandNR(regToVReg(x4)),
		},
		{
			name: "const imm 12 no shift",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(0xfff)
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp: operandImm12(0xfff, 0),
		},
		{
			name: "const imm 12 with shift bit",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(0xabc_000)
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp: operandImm12(0xabc, 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def, mode := tc.setup(ctx, b, m)
			actual := m.getOperand_Imm12_ER_SR_NR(def, mode)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_getOperand_MaybeNegatedImm12_ER_SR_NR(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) (def *backend.SSAValueDefinition, mode extMode)
		exp          operand
		expNegated   bool
		instructions []string
	}{
		{
			name: "block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				blk := builder.CurrentBlock()
				v := blk.AddParam(builder, ssa.TypeI64)
				def = &backend.SSAValueDefinition{BlkParamVReg: regToVReg(x4), BlockParamValue: v}
				return def, extModeZeroExtend64
			},
			exp: operandNR(regToVReg(x4)),
		},
		{
			name: "const imm 12 no shift",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(0xfff)
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp: operandImm12(0xfff, 0),
		},
		{
			name: "const negative imm 12 no shift",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				c := int32(-1)
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(uint32(c))
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp:        operandImm12(1, 0),
			expNegated: true,
		},
		{
			name: "const imm 12 with shift bit",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(0xabc_000)
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp: operandImm12(0xabc, 1),
		},
		{
			name: "const negated imm 12 with shift bit",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) (def *backend.SSAValueDefinition, mode extMode) {
				c := int32(-0xabc_000)
				cinst := builder.AllocateInstruction()
				cinst.AsIconst32(uint32(c))
				builder.InsertInstruction(cinst)
				def = &backend.SSAValueDefinition{Instr: cinst, N: 0}
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return
			},
			exp:        operandImm12(0xabc, 1),
			expNegated: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def, mode := tc.setup(ctx, b, m)
			actual, negated := m.getOperand_MaybeNegatedImm12_ER_SR_NR(def, mode)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, tc.expNegated, negated)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}
