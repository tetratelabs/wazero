package arm64debug

import (
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TODO: Comment why tmpReg is unused.
func newGoasmAssembler(t *testing.T, _ asm.Register) arm64.Assembler {
	a, err := newAssembler(asm.NilRegister)
	require.NoError(t, err)
	a.CompileStandAlone(arm64.NOP)
	return a
}

/*
			cs = append(cs, fmt.Sprintf(`{
	name: "ADD",
	n: &NodeImpl{
		Instruction:        ADD,
		SrcReg:             Reg%s,
		SrcReg2:            Reg%s,
		SrcConst:           %d,
		DstReg: 			Reg%s,
		VectorArrangement: VectorArrangementH,
		SrcVectorIndex:    0,
	},
	exp: %#v,
}`, arm64.RegisterName(tc.srcReg), arm64.RegisterName(tc.shiftedSrcReg), tc.shiftNum, arm64.RegisterName(tc.dstReg),
				expected))

	fmt.Println(strings.Join(cs, ",\n"))


						cs = append(cs, fmt.Sprintf(`{name: "%s", inst: %s, src: Reg%s, dst: Reg%s, exp: %#v}`,
							fmt.Sprintf("%s/src=%s,dst=%s", arm64.InstructionName(tc.inst), arm64.RegisterName(src), arm64.RegisterName(dst)),
							arm64.InstructionName(tc.inst),
							arm64.RegisterName(src), arm64.RegisterName(dst),
							expected[:4]))

							cs = append(cs, fmt.Sprintf(`{name: "%s", inst: %s, src: Reg%s, src2: Reg%s, dst: Reg%s, exp: %#v}`,
								fmt.Sprintf("src=%s,src2=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(src2), arm64.RegisterName(dst)),
								arm64.InstructionName(tc.inst),
								arm64.RegisterName(src),
								arm64.RegisterName(src2),
								arm64.RegisterName(dst),
								expected[:4]))


								css = append(css, fmt.Sprintf(`{name: "%s", n: &NodeImpl{Instruction: %s, DstReg: Reg%s, SrcConst: %d}, exp: %#v}`,
									fmt.Sprintf("%s/dst=%s/%#x", arm64.InstructionName(tc.inst), arm64.RegisterName(r), uint64(c)),
									arm64.InstructionName(tc.inst),
									arm64.RegisterName(r),
									c,
									expected))
*/

func TestAssemblerImpl_EncodeRegisterToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterToMemory},
				expErr: "ADR is unsupported for from:register,to:memory type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToMemory(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	offsets := []int64{
		-1, 0, 1, 2, -2, 4, -4, 0xf, -0xf, 1 << 4, 1<<4 - 1, 1<<4 + 1, -128, -256, 8 * 10, -128,
		255, 4096, 4096 << 1, 32760, 32760 * 2, 32760*2 - 8,
		32760*2 - 16, 1 << 27, 1 << 30, 1<<30 + 8, 1<<30 - 8, 1<<30 + 16, 1<<30 - 16, 1<<31 - 8,
		(1 << 28) + 4,
	}
	intRegs := []asm.Register{
		arm64.RegR0,
		arm64.RegR30,
	}
	floatRegs := []asm.Register{
		arm64.RegV0,
		arm64.RegV30,
	}
	tests := []struct {
		inst    asm.Instruction
		srcRegs []asm.Register
		offsets []int64
	}{
		{inst: arm64.MOVD, srcRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVW, srcRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVWU, srcRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVH, srcRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVB, srcRegs: intRegs, offsets: offsets},
		{inst: arm64.FMOVD, srcRegs: floatRegs, offsets: offsets},
		{inst: arm64.FMOVS, srcRegs: floatRegs, offsets: offsets},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, srcReg := range tc.srcRegs {
				for _, baseReg := range intRegs {
					t.Run("const offset", func(t *testing.T) {
						for _, offset := range tc.offsets {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesRegisterToMemory,
								Instruction: tc.inst, SrcReg: srcReg, DstReg: baseReg, DstConst: offset}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								a := arm64.NewAssemblerImpl(arm64.RegR27)

								for _, assembler := range []arm64.Assembler{goasm, a} {
									assembler.CompileRegisterToMemory(n.Instruction, n.SrcReg, n.DstReg, n.DstConst)
								}

								expected, err := goasm.Assemble()
								require.NoError(t, err)

								actual, err := a.Assemble()
								require.NoError(t, err)

								require.Equal(t, expected, actual)
							})
						}
					})
					t.Run("register offset", func(t *testing.T) {
						for _, offsetReg := range []asm.Register{arm64.RegR8, arm64.RegR18} {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesRegisterToMemory,
								Instruction: tc.inst, SrcReg: srcReg, DstReg: baseReg, DstReg2: offsetReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								goasm.CompileRegisterToMemoryWithRegisterOffset(n.Instruction, n.SrcReg, n.DstReg, n.DstReg2)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.RegR27)
								err = a.EncodeRegisterToMemory(n)
								require.NoError(t, err)
								actual := a.Bytes()
								require.Equal(t, expected, actual)
							})
						}
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeMemoryToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesMemoryToRegister},
				expErr: "SUB is unsupported for from:memory,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeMemoryToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	offsets := []int64{
		-1, 0, 1, 2, -2, 0xf, -0xf, 1 << 4, 1<<4 - 1, 1<<4 + 1, -128, -256, 8 * 10, -128,
		255, 4096, 4096 << 1, 32760, 32760 * 2, 32760*2 - 8,
		32760*2 - 16, 1 << 27, 1 << 30, 1<<30 + 8, 1<<30 - 8, 1<<30 + 16, 1<<30 - 16, 1<<31 - 8,
		(1 << 28) + 4,
		1<<12<<8 + 8,
		1<<12<<8 - 8,
	}
	intRegs := []asm.Register{
		arm64.RegR0, arm64.RegR16,
		arm64.RegR30,
	}
	floatRegs := []asm.Register{
		arm64.RegV0, arm64.RegV10,
		arm64.RegV30,
	}
	tests := []struct {
		inst    asm.Instruction
		dstRegs []asm.Register
		offsets []int64
	}{
		{inst: arm64.MOVD, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVW, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVWU, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVH, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVHU, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVB, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.MOVBU, dstRegs: intRegs, offsets: offsets},
		{inst: arm64.FMOVD, dstRegs: floatRegs, offsets: offsets},
		{inst: arm64.FMOVS, dstRegs: floatRegs, offsets: offsets},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, dstReg := range tc.dstRegs {
				for _, baseReg := range intRegs {
					t.Run("const offset", func(t *testing.T) {
						for _, offset := range tc.offsets {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesMemoryToRegister,
								Instruction: tc.inst, SrcReg: baseReg, SrcConst: offset, DstReg: dstReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								a := arm64.NewAssemblerImpl(arm64.RegR27)

								for _, assembler := range []arm64.Assembler{goasm, a} {
									assembler.CompileMemoryToRegister(n.Instruction, n.SrcReg, n.SrcConst, n.DstReg)
								}

								expected, err := goasm.Assemble()
								require.NoError(t, err)

								actual, err := a.Assemble()
								require.NoError(t, err)

								require.Equal(t, expected, actual)
							})
						}
					})
					t.Run("register offset", func(t *testing.T) {
						for _, offsetReg := range []asm.Register{arm64.RegR8, arm64.RegR18} {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesMemoryToRegister,
								Instruction: tc.inst, SrcReg: baseReg, SrcReg2: offsetReg, DstReg: dstReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								goasm.CompileMemoryWithRegisterOffsetToRegister(n.Instruction, n.SrcReg, n.SrcReg2, n.DstReg)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.RegR27)
								err = a.EncodeMemoryToRegister(n)
								require.NoError(t, err)
								actual := a.Bytes()
								require.Equal(t, expected, actual)
							})
						}
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_encodeReadInstructionAddress(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		const targetBeforeInstruction = arm64.RET
		for _, dstReg := range []asm.Register{arm64.RegR19, arm64.RegR23} {
			dstReg := dstReg
			t.Run(arm64.RegisterName(dstReg), func(t *testing.T) {
				goasm := newGoasmAssembler(t, asm.NilRegister)
				a := arm64.NewAssemblerImpl(asm.NilRegister)

				for _, assembler := range []arm64.Assembler{a, goasm} {
					assembler.CompileReadInstructionAddress(dstReg, targetBeforeInstruction)
					assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10) // Dummy
					assembler.CompileJumpToRegister(targetBeforeInstruction, arm64.RegR25)
					assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10) // Target.
				}

				actual, err := a.Assemble()
				require.NoError(t, err)
				expected, err := goasm.Assemble()
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			})
		}
	})
	t.Run("not found", func(t *testing.T) {
		a := arm64.NewAssemblerImpl(asm.NilRegister)
		a.CompileReadInstructionAddress(arm64.RegR27, arm64.NOP)
		a.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10)
		_, err := a.Assemble()
		require.EqualError(t, err, "BUG: target instruction NOP not found for ADR")
	})
	t.Run("offset too large", func(t *testing.T) {
		a := arm64.NewAssemblerImpl(asm.NilRegister)
		a.CompileReadInstructionAddress(arm64.RegR27, arm64.RET)
		a.CompileJumpToRegister(arm64.RET, arm64.RegR25)
		a.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10)

		for n := a.Root; n != nil; n = n.Next {
			n.OffsetInBinaryField = uint64(a.Buf.Len())

			err := a.EncodeNode(n)
			require.NoError(t, err)
		}

		require.Equal(t, 1, len(a.OnGenerateCallbacks))
		cb := a.OnGenerateCallbacks[0]

		targetNode := a.Current
		targetNode.OffsetInBinaryField = uint64(math.MaxInt64)

		err := cb(nil)
		require.EqualError(t, err, "BUG: too large offset for ADR")
	})
}

func TestAssemblerImpl_EncodeRelativeJump(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.B, Types: arm64.OperandTypesNoneToBranch},
				expErr: "branch target must be set for B",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesNoneToBranch},
				expErr: "SUB is unsupported for from:none,to:branch type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRelativeBranch(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	for _, inst := range []asm.Instruction{
		arm64.B, arm64.BCONDEQ, arm64.BCONDGE, arm64.BCONDGT, arm64.BCONDHI, arm64.BCONDHS,
		arm64.BCONDLE, arm64.BCONDLO, arm64.BCONDLS, arm64.BCONDLT, arm64.BCONDMI, arm64.BCONDNE, arm64.BCONDVS,
		arm64.BCONDPL,
	} {
		inst := inst
		t.Run(arm64.InstructionName(inst), func(t *testing.T) {
			tests := []struct {
				forward                                                                   bool
				instructionsInPreamble, instructionsBeforeBranch, instructionsAfterBranch int
			}{
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 0, instructionsAfterBranch: 10},
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 10, instructionsAfterBranch: 10},
				{forward: true, instructionsInPreamble: 123, instructionsBeforeBranch: 10, instructionsAfterBranch: 10},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 0, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 10, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 10, instructionsAfterBranch: 10},
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 0, instructionsAfterBranch: 1000},
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 1000, instructionsAfterBranch: 1000},
				{forward: true, instructionsInPreamble: 123, instructionsBeforeBranch: 1000, instructionsAfterBranch: 1000},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 0, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 1000, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 1000, instructionsAfterBranch: 1000},
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 0, instructionsAfterBranch: 1234},
				{forward: true, instructionsInPreamble: 0, instructionsBeforeBranch: 1234, instructionsAfterBranch: 1234},
				{forward: true, instructionsInPreamble: 123, instructionsBeforeBranch: 1234, instructionsAfterBranch: 1234},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 0, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 1234, instructionsAfterBranch: 0},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 1234, instructionsAfterBranch: 1234},
				{forward: true, instructionsInPreamble: 123, instructionsBeforeBranch: 123, instructionsAfterBranch: 65536},
				{forward: false, instructionsInPreamble: 123, instructionsBeforeBranch: 65536, instructionsAfterBranch: 0},
			}

			for _, tt := range tests {
				tc := tt
				t.Run(fmt.Sprintf("foward=%v(before=%d,after=%d)", tc.forward,
					tc.instructionsBeforeBranch, tc.instructionsAfterBranch), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					a := arm64.NewAssemblerImpl(asm.NilRegister)

					for _, assembler := range []arm64.Assembler{a, goasm} {
						for i := 0; i < tc.instructionsInPreamble; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10)
						}
						backwardTarget := assembler.CompileStandAlone(arm64.NOP)
						for i := 0; i < tc.instructionsBeforeBranch; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10)
						}
						br := assembler.CompileJump(inst)
						for i := 0; i < tc.instructionsAfterBranch; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.RegR10)
						}
						fowardTarget := assembler.CompileStandAlone(arm64.NOP)

						if tc.forward {
							br.AssignJumpTarget(fowardTarget)
						} else {
							br.AssignJumpTarget(backwardTarget)
						}
					}

					actual, err := a.Assemble()
					require.NoError(t, err)
					expected, err := goasm.Assemble()
					require.NoError(t, err)
					require.Equal(t, expected, actual)
				})
			}
		})
	}
}
