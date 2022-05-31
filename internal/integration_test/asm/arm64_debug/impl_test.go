package arm64debug

import (
	"encoding/hex"
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

func TestAssemblerImpl_encodeNoneToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		a := arm64.NewAssemblerImpl(asm.NilRegister)
		err := a.EncodeNoneToNone(&arm64.NodeImpl{Instruction: arm64.ADD})
		require.EqualError(t, err, "ADD is unsupported for from:none,to:none type")
	})
	t.Run("ok", func(t *testing.T) {
		a := arm64.NewAssemblerImpl(asm.NilRegister)
		err := a.EncodeNoneToNone(&arm64.NodeImpl{Instruction: arm64.NOP})
		require.NoError(t, err)

		// NOP must be ignored.
		actual := a.Buf.Bytes()
		require.Zero(t, len(actual))
	})
}

var intRegisters = []asm.Register{
	arm64.RegR0, arm64.RegR1, arm64.RegR2, arm64.RegR3, arm64.RegR4, arm64.RegR5, arm64.RegR6,
	arm64.RegR7, arm64.RegR8, arm64.RegR9, arm64.RegR10, arm64.RegR11, arm64.RegR12, arm64.RegR13,
	arm64.RegR14, arm64.RegR15, arm64.RegR16, arm64.RegR17, arm64.RegR18, arm64.RegR19, arm64.RegR20,
	arm64.RegR21, arm64.RegR22, arm64.RegR23, arm64.RegR24, arm64.RegR25, arm64.RegR26, arm64.RegR27,
	arm64.RegR28, arm64.RegR29, arm64.RegR30,
}

func TestAssemblerImpl_EncodeJumpToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADD, Types: arm64.OperandTypesNoneToRegister},
				expErr: "ADD is unsupported for from:none,to:register type",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.RET, DstReg: asm.NilRegister},
				expErr: "invalid destination register: nil is not integer",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.RET, DstReg: arm64.RegV0},
				expErr: "invalid destination register: V0 is not integer",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeJumpToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{
			arm64.B,
			arm64.RET,
		} {
			t.Run(arm64.InstructionName(inst), func(t *testing.T) {
				for _, r := range intRegisters {
					t.Run(arm64.RegisterName(r), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm := newGoasmAssembler(t, asm.NilRegister)
						if inst == arm64.RET {
							goasm.CompileJumpToRegister(inst, r)
						} else {
							goasm.CompileJumpToMemory(inst, r)
						}

						expected, err := goasm.Assemble()
						require.NoError(t, err)

						a := arm64.NewAssemblerImpl(asm.NilRegister)
						err = a.EncodeJumpToRegister(&arm64.NodeImpl{Instruction: inst, DstReg: r})
						require.NoError(t, err)

						actual := a.Bytes()
						require.Equal(t, expected, actual)
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_EncodeLeftShiftedRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesLeftShiftedRegisterToRegister,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "SUB is unsupported for from:left-shifted-register,to:register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "shift amount must fit in unsigned 6-bit integer (0-64) but got -1",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.RegV0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.RegR0, SrcReg2: arm64.RegV0, DstReg: arm64.RegR0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegV0},
				expErr: "V0 is not integer",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeLeftShiftedRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.ADD
	tests := []struct {
		srcReg, shiftedSrcReg, dstReg asm.Register
		shiftNum                      int64
	}{
		{
			srcReg:        arm64.RegR0,
			shiftedSrcReg: arm64.RegR29,
			shiftNum:      1,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegR0,
			shiftedSrcReg: arm64.RegR29,
			shiftNum:      2,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegR0,
			shiftedSrcReg: arm64.RegR29,
			shiftNum:      8,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegR29,
			shiftedSrcReg: arm64.RegR0,
			shiftNum:      16,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegR29,
			shiftedSrcReg: arm64.RegR0,
			shiftNum:      64,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegRZR,
			shiftedSrcReg: arm64.RegR0,
			shiftNum:      64,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegRZR,
			shiftedSrcReg: arm64.RegRZR,
			shiftNum:      64,
			dstReg:        arm64.RegR21,
		},
		{
			srcReg:        arm64.RegRZR,
			shiftedSrcReg: arm64.RegRZR,
			shiftNum:      64,
			dstReg:        arm64.RegRZR,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("src=%s,shifted_src=%s,shift_num=%d,dst=%s",
			arm64.RegisterName(tc.srcReg), arm64.RegisterName(tc.shiftedSrcReg),
			tc.shiftNum, arm64.RegisterName(tc.srcReg)), func(t *testing.T) {

			// TODO: remove golang-asm dependency in tests.
			goasm := newGoasmAssembler(t, asm.NilRegister)
			goasm.CompileLeftShiftedRegisterToRegister(inst, tc.shiftedSrcReg, tc.shiftNum, tc.srcReg, tc.dstReg)
			expected, err := goasm.Assemble()
			require.NoError(t, err)

			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err = a.EncodeLeftShiftedRegisterToRegister(&arm64.NodeImpl{Instruction: inst,
				SrcReg: tc.srcReg, SrcReg2: tc.shiftedSrcReg, SrcConst: tc.shiftNum,
				DstReg: tc.dstReg,
			})
			require.NoError(t, err)

			actual := a.Bytes()
			require.Equal(t, expected, actual)
		})
	}
}

func TestAssemblerImpl_EncodeTwoRegistersToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesTwoRegistersToNone,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "SUB is unsupported for from:two-registers,to:none type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegV0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.FCMPS,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegV0},
				expErr: "R0 is not vector",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoRegistersToNone(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.RegRZR, arm64.RegR0, arm64.RegR10, arm64.RegR30}
	floatRegs := []asm.Register{arm64.RegV0, arm64.RegV12, arm64.RegV31}
	tests := []struct {
		instruction asm.Instruction
		regs        []asm.Register
	}{
		{instruction: arm64.CMP, regs: intRegs},
		{instruction: arm64.CMPW, regs: intRegs},
		{instruction: arm64.FCMPD, regs: floatRegs},
		{instruction: arm64.FCMPS, regs: floatRegs},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.instruction), func(t *testing.T) {
			for _, src := range tc.regs {
				for _, src2 := range tc.regs {
					t.Run(fmt.Sprintf("src=%s,src2=%s", arm64.RegisterName(src), arm64.RegisterName(src2)), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						goasm.CompileTwoRegistersToNone(tc.instruction, src, src2)
						expected, err := goasm.Assemble()
						require.NoError(t, err)

						a := arm64.NewAssemblerImpl(asm.NilRegister)
						err = a.EncodeTwoRegistersToNone(&arm64.NodeImpl{Instruction: tc.instruction, SrcReg: src, SrcReg2: src2})
						require.NoError(t, err)

						actual := a.Bytes()
						require.Equal(t, expected, actual)
					})

				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeThreeRegistersToRegister(t *testing.T) {
	intRegs := []asm.Register{arm64.RegRZR, arm64.RegR1, arm64.RegR10, arm64.RegR30}
	for _, inst := range []asm.Instruction{arm64.MSUB, arm64.MSUBW} {
		inst := inst
		t.Run(arm64.InstructionName(inst), func(t *testing.T) {
			for _, src1 := range intRegs {
				for _, src2 := range intRegs {
					for _, src3 := range intRegs {
						for _, dst := range intRegs {
							src1, src2, src3, dst := src1, src2, src3, dst
							t.Run(fmt.Sprintf("src1=%s,src2=%s,src3=%s,dst=%s",
								arm64.RegisterName(src1), arm64.RegisterName(src2),
								arm64.RegisterName(src3), arm64.RegisterName(dst)), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								goasm.CompileThreeRegistersToRegister(inst, src1, src2, src3, dst)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(asm.NilRegister)
								err = a.EncodeThreeRegistersToRegister(&arm64.NodeImpl{Instruction: inst, SrcReg: src1, SrcReg2: src2, DstReg: src3, DstReg2: dst})
								require.NoError(t, err)

								actual := a.Bytes()
								require.Equal(t, expected, actual)
							})
						}
					}
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterToRegister,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "ADR is unsupported for from:register,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.RegRZR, arm64.RegR1, arm64.RegR10, arm64.RegR30}
	intRegsWithoutZero := intRegs[1:]
	conditionalRegs := []asm.Register{arm64.RegCondEQ, arm64.RegCondNE,
		arm64.RegCondHS, arm64.RegCondLO, arm64.RegCondMI, arm64.RegCondPL, arm64.RegCondVS, arm64.RegCondVC,
		arm64.RegCondHI, arm64.RegCondLS, arm64.RegCondGE, arm64.RegCondLT, arm64.RegCondGT, arm64.RegCondLE,
		arm64.RegCondAL, arm64.RegCondNV}
	floatRegs := []asm.Register{arm64.RegV0, arm64.RegV15, arm64.RegV31}

	tests := []struct {
		inst             asm.Instruction
		srcRegs, dstRegs []asm.Register
	}{
		{inst: arm64.ADD, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ADDW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SUB, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.CLZ, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.CLZW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.CSET, srcRegs: conditionalRegs, dstRegs: intRegs},
		{inst: arm64.FABSS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FABSD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FNEGS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FNEGD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FSQRTD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FSQRTS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FCVTDS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FCVTSD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTMD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTMS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTND, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTNS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTPD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTPS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTZD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FRINTZS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FDIVS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FDIVD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMAXD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMAXS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMIND, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMINS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMULS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMULD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FADDD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FADDS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FCVTZSD, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZSDW, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZSS, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZSSW, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZUD, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZUDW, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZUS, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FCVTZUSW, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FMOVD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMOVS, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FMOVD, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.FMOVS, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.FMOVD, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.FMOVS, srcRegs: floatRegs, dstRegs: intRegs},
		{inst: arm64.MOVD, srcRegs: intRegs, dstRegs: intRegsWithoutZero},
		{inst: arm64.MOVWU, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.MRS, srcRegs: []asm.Register{arm64.RegFPSR}, dstRegs: intRegs},
		{inst: arm64.MSR, srcRegs: intRegs, dstRegs: []asm.Register{arm64.RegFPSR}},
		{inst: arm64.MUL, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.MULW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.NEG, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.NEGW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.RBIT, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.RBITW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SDIV, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SDIVW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.UDIV, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.UDIVW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SCVTFD, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.SCVTFWD, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.SCVTFS, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.SCVTFWS, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.UCVTFD, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.UCVTFWD, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.UCVTFS, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.UCVTFWS, srcRegs: intRegs, dstRegs: floatRegs},
		{inst: arm64.SXTB, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SXTBW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SXTH, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SXTHW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SXTW, srcRegs: intRegs, dstRegs: intRegs},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, src := range tc.srcRegs {
				for _, dst := range tc.dstRegs {
					src, dst := src, dst
					t.Run(fmt.Sprintf("src=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(dst)), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						if tc.inst == arm64.CSET {

							goasm.CompileConditionalRegisterSet(conditionalRegisterToState(src), dst)
						} else {
							goasm.CompileRegisterToRegister(tc.inst, src, dst)

						}
						expected, err := goasm.Assemble()
						require.NoError(t, err)

						a := arm64.NewAssemblerImpl(asm.NilRegister)
						err = a.EncodeRegisterToRegister(&arm64.NodeImpl{Instruction: tc.inst, SrcReg: src, DstReg: dst})
						require.NoError(t, err)

						actual := a.Bytes()
						require.Equal(t, expected, actual)
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeTwoRegistersToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesTwoRegistersToRegister,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "ADR is unsupported for from:two-registers,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeThreeRegistersToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.RegRZR, arm64.RegR1, arm64.RegR10, arm64.RegR30}
	floatRegs := []asm.Register{arm64.RegV0, arm64.RegV15, arm64.RegV31}

	tests := []struct {
		inst             asm.Instruction
		srcRegs, dstRegs []asm.Register
	}{
		{inst: arm64.AND, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ANDW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ORR, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ORRW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.EOR, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.EORW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ASR, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ASRW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.LSL, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.LSLW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.LSR, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.LSRW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.ROR, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.RORW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SDIV, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SDIVW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.UDIV, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.UDIVW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SUB, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.SUBW, srcRegs: intRegs, dstRegs: intRegs},
		{inst: arm64.FSUBD, srcRegs: floatRegs, dstRegs: floatRegs},
		{inst: arm64.FSUBS, srcRegs: floatRegs, dstRegs: floatRegs},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, src := range tc.srcRegs {
				for _, src2 := range tc.srcRegs {
					for _, dst := range tc.dstRegs {
						src, src2, dst := src, src2, dst
						t.Run(fmt.Sprintf("src=%s,src2=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(src2), arm64.RegisterName(dst)), func(t *testing.T) {
							goasm := newGoasmAssembler(t, asm.NilRegister)
							goasm.CompileTwoRegistersToRegister(tc.inst, src, src2, dst)
							expected, err := goasm.Assemble()
							require.NoError(t, err)

							a := arm64.NewAssemblerImpl(asm.NilRegister)
							err = a.EncodeTwoRegistersToRegister(&arm64.NodeImpl{Instruction: tc.inst, SrcReg: src, SrcReg2: src2, DstReg: dst})
							require.NoError(t, err)

							actual := a.Bytes()
							require.Equal(t, expected, actual)
						})
					}
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeRegisterAndConstToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "ADR is unsupported for from:register-and-const,to:none type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.RegR0, SrcConst: 12345},
				expErr: "immediate for CMP must fit in 0 to 4095 but got 12345",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.RegRZR, SrcConst: 123},
				expErr: "zero register is not supported for CMP (immediate)",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterAndConstToNone(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.CMP
	for _, reg := range []asm.Register{arm64.RegR1, arm64.RegR10, arm64.RegR30} {
		for _, c := range []int64{0, 10, 100, 300, 4095} {
			reg, c := reg, c
			t.Run(fmt.Sprintf("%s, %d", arm64.RegisterName(reg), c), func(t *testing.T) {
				goasm := newGoasmAssembler(t, asm.NilRegister)
				goasm.CompileRegisterAndConstToNone(inst, reg, c)
				expected, err := goasm.Assemble()
				require.NoError(t, err)
				if c == 0 {
					// This case cannot be supported in golang-asm and it results in miscompilation.
					expected[3] = 0b111_10001
				}

				a := arm64.NewAssemblerImpl(asm.NilRegister)
				err = a.EncodeRegisterAndConstToNone(&arm64.NodeImpl{Instruction: inst, SrcReg: reg, SrcConst: c})
				require.NoError(t, err)

				actual := a.Bytes()
				require.Equal(t, expected, actual)
			})
		}
	}
}

func TestAssemblerImpl_EncodeConstToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesConstToRegister,
					SrcReg: arm64.RegR0, SrcReg2: arm64.RegR0, DstReg: arm64.RegR0},
				expErr: "ADR is unsupported for from:const,to:register type",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.LSR, Types: arm64.OperandTypesConstToRegister, DstReg: arm64.RegR0},
				expErr: "LSR with zero constant should be optimized out",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.LSL, Types: arm64.OperandTypesConstToRegister, DstReg: arm64.RegR0},
				expErr: "LSL with zero constant should be optimized out",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeConstToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	consts64 := []int64{
		0x1,
		0xfff,
		0xfff << 12,
		123 << 12,
		(1<<15 + 1),
		(1<<15 + 1) << 16,
		(1<<15 + 1) << 32,
		0x0000_ffff_ffff_ffff,
		-281470681743361, /* = 0xffff_0000_ffff_ffff */
		math.MinInt32 + 1,
		-281474976645121, /* = 0xffff_0000_0000_ffff */
		1<<20 + 1,
		1<<20 - 1,
		1<<23 | 0b01,
		1<<30 + 1,
		1 << 1, 1<<1 + 1, 1<<1 - 1, 1<<1 + 0xf,
		1 << 2, 1<<2 + 1, 1<<2 - 1, 1<<2 + 0xf,
		1 << 3, 1<<3 + 1, 1<<3 - 1, 1<<3 + 0xf,
		1 << 4, 1<<4 + 1, 1<<4 - 1, 1<<4 + 0xf,
		1 << 5, 1<<5 + 1, 1<<5 - 1, 1<<5 + 0xf,
		1 << 6, 1<<6 + 1, 1<<6 - 1, 1<<6 + 0xf,
		0xfff << 1, 0xfff<<1 - 1, 0xfff<<1 + 1,
		0, 1, -1, 2, 3, 10, -10, 123, -123,
		math.MaxInt16, math.MaxInt32, math.MaxUint32, 0b01000000_00000010, 0xffff_0000, 0xffff_0001, 0xf00_000f,
		math.MaxInt16 - 1, math.MaxInt32 - 1, math.MaxUint32 - 1, 0b01000000_00000010 - 1, 0xffff_0000 - 1, 0xffff_0001 - 1, 0xf00_000f - 1,
		math.MaxInt16 + 1, 0b01000000_00001010 - 1, 0xfff_0000 - 1, 0xffe_0001 - 1, 0xe00_000f - 1,
		(1<<15 + 1) << 16, 0b1_00000000_00000010,
		1 << 32, 1 << 34, 1 << 40,
		1<<32 + 1, 1<<34 + 1, 1<<40 + 1,
		1<<32 - 1, 1<<34 - 1, 1<<40 - 1,
		1<<32 + 0xf, 1<<34 + 0xf, 1<<40 + 0xf,
		1<<32 - 0xf, 1<<34 - 0xf, 1<<40 - 0xf,
		math.MaxInt64, math.MinInt64,
		1<<30 + 1,
		0x7000000010000000,
		0x7000000100000000,
		0x7000100000000000,
		87220,
		(math.MaxInt16 + 2) * 8,
		-281471681677793,
		3295005183,
		-8543223759426509416,
		-1000000000,
		0xffffff,
	}

	tests := []struct {
		inst   asm.Instruction
		consts []int64
	}{
		{
			inst:   arm64.ADD,
			consts: consts64,
		},
		{
			inst:   arm64.ADDS,
			consts: consts64,
		},
		{
			inst:   arm64.SUB,
			consts: consts64,
		},
		{
			inst:   arm64.SUBS,
			consts: consts64,
		},
		{
			inst: arm64.MOVW,
			consts: []int64{
				1 << 1, 1<<1 + 1, 1<<1 - 1, 1<<1 + 0xf,
				1 << 2, 1<<2 + 1, 1<<2 - 1, 1<<2 + 0xf,
				1 << 3, 1<<3 + 1, 1<<3 - 1, 1<<3 + 0xf,
				1 << 4, 1<<4 + 1, 1<<4 - 1, 1<<4 + 0xf,
				1 << 5, 1<<5 + 1, 1<<5 - 1, 1<<5 + 0xf,
				1 << 6, 1<<6 + 1, 1<<6 - 1, 1<<6 + 0xf,
				0xfff << 1, 0xfff<<1 - 1, 0xfff<<1 + 1,
				0, 1, -1, 2, 3, 10, -10, 123, -123,
				math.MaxInt16, math.MaxInt32, math.MaxUint32, 0b01000000_00000010, 0xffff_0000, 0xffff_0001, 0xf00_000f,
				math.MaxInt16 - 1, math.MaxInt32 - 1, math.MaxUint32 - 1, 0b01000000_00000010 - 1, 0xffff_0000 - 1, 0xffff_0001 - 1, 0xf00_000f - 1,
				math.MaxInt16 + 1, 0b01000000_00001010 - 1, 0xfff_0000 - 1, 0xffe_0001 - 1, 0xe00_000f - 1,
				(1<<15 + 1) << 16, 0b1_00000000_00000010,
				1 << 30, 1<<30 + 1, 1<<30 - 1, 1<<30 + 0xf, 1<<30 - 0xf,
				0x7fffffffffffffff,
				-(1 << 30),
			},
		},
		{
			inst:   arm64.MOVD,
			consts: consts64,
		},
		{
			inst:   arm64.LSL,
			consts: []int64{1, 2, 4, 16, 31, 32, 63},
		},
		{
			inst:   arm64.LSR,
			consts: []int64{1, 2, 4, 16, 31, 32, 63},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, r := range []asm.Register{
				arm64.RegR0, arm64.RegR10,
				arm64.RegR30,
			} {
				r := r
				t.Run(arm64.RegisterName(r), func(t *testing.T) {
					for _, c := range tc.consts {
						var cs = []int64{c}
						if tc.inst != arm64.LSR && tc.inst != arm64.LSL && c != 0 {
							cs = append(cs, -c)
						}
						for _, c := range cs {
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								goasm := newGoasmAssembler(t, arm64.RegR27)
								goasm.CompileConstToRegister(tc.inst, c, r)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.RegR27)
								err = a.EncodeConstToRegister(&arm64.NodeImpl{Instruction: tc.inst, SrcConst: c, DstReg: r})
								require.NoError(t, err)

								actual := a.Bytes()
								require.Equal(t, expected, actual)
							})
						}
					}
				})
			}
		})
	}
}

func TestAssemblerImpl_EncodeSIMDByteToSIMDByte(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesSIMDByteToSIMDByte},
				expErr: "ADR is unsupported for from:simd-byte,to:simd-byte type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeSIMDByteToSIMDByte(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.VCNT
	t.Run(arm64.InstructionName(inst), func(t *testing.T) {
		floatRegs := []asm.Register{arm64.RegV0, arm64.RegV10, arm64.RegV21, arm64.RegV31}
		for _, src := range floatRegs {
			for _, dst := range floatRegs {
				src, dst := src, dst
				t.Run(fmt.Sprintf("src=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(dst)), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					goasm.CompileSIMDByteToSIMDByte(inst, src, dst)
					expected, err := goasm.Assemble()
					require.NoError(t, err)

					a := arm64.NewAssemblerImpl(arm64.RegR27)
					err = a.EncodeSIMDByteToSIMDByte(&arm64.NodeImpl{Instruction: inst, SrcReg: src, DstReg: dst})
					require.NoError(t, err)

					actual := a.Bytes()
					require.Equal(t, expected, actual)

				})
			}
		}
	})
}

func TestAssemblerImpl_EncodeSIMDByteToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesSIMDByteToRegister},
				expErr: "ADR is unsupported for from:simd-byte,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeSIMDByteToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.VUADDLV
	t.Run(arm64.InstructionName(inst), func(t *testing.T) {
		floatRegs := []asm.Register{arm64.RegV0, arm64.RegV10, arm64.RegV21, arm64.RegV31}
		for _, src := range floatRegs {
			for _, dst := range floatRegs {
				src, dst := src, dst
				t.Run(fmt.Sprintf("src=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(dst)), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					goasm.CompileSIMDByteToRegister(inst, src, dst)
					expected, err := goasm.Assemble()
					require.NoError(t, err)

					a := arm64.NewAssemblerImpl(arm64.RegR27)
					err = a.EncodeSIMDByteToRegister(&arm64.NodeImpl{Instruction: inst, SrcReg: src, DstReg: dst})
					require.NoError(t, err)

					actual := a.Bytes()
					require.Equal(t, expected, actual)

				})
			}
		}
	})
}

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
		arm64.RegR0, arm64.RegR16,
		arm64.RegR30,
	}
	floatRegs := []asm.Register{
		arm64.RegV0, arm64.RegV10,
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
		arm64.B, arm64.BEQ, arm64.BGE, arm64.BGT, arm64.BHI, arm64.BHS,
		arm64.BLE, arm64.BLO, arm64.BLS, arm64.BLT, arm64.BMI, arm64.BNE, arm64.BVS,
		arm64.BPL,
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

// TestAssemblerImpl_multipleLargeOffest ensures that the const pool flushing strategy matches
// the one of Go's assembler.
func TestAssemblerImpl_multipleLargeOffest(t *testing.T) {
	goasm := newGoasmAssembler(t, asm.NilRegister)
	a := arm64.NewAssemblerImpl(arm64.RegR27)

	for _, assembler := range []arm64.Assembler{a, goasm} {
		for i := 0; i < 10000; i++ {
			// This will be put into const pool, but the callback won't be set for it.
			assembler.CompileRegisterToMemory(arm64.MOVD, arm64.RegR11, arm64.RegR12, 0xfff0+int64(i*8))
			// This will also set the call back for it.
			assembler.CompileRegisterToMemory(arm64.MOVD, arm64.RegR11, arm64.RegR12, (0xfff0+int64(i*8)<<16+8)%(1<<31))
		}
	}

	actual, err := a.Assemble()
	require.NoError(t, err)
	expected, err := goasm.Assemble()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestAssemblerImpl_EncodeTwoSIMDBytesToSIMDByteRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.B, Types: arm64.OperandTypesTwoSIMDBytesToSIMDByteRegister},
				expErr: "B is unsupported for from:two-simd-bytes,to:simd-byte type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoSIMDBytesToSIMDByteRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	for _, inst := range []asm.Instruction{arm64.VBIT} {
		regs := []asm.Register{arm64.RegV0, arm64.RegV10, arm64.RegV30}
		for _, src1 := range regs {
			for _, src2 := range regs {
				for _, dst := range regs {
					n := &arm64.NodeImpl{Instruction: inst, SrcReg: src1, SrcReg2: src2, DstReg: dst,
						Types: arm64.OperandTypesTwoSIMDBytesToSIMDByteRegister}
					t.Run(n.String(), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						goasm.CompileTwoSIMDBytesToSIMDByteRegister(n.Instruction, n.SrcReg, n.SrcReg2, n.DstReg)
						expected, err := goasm.Assemble()
						require.NoError(t, err)

						a := arm64.NewAssemblerImpl(arm64.RegR27)
						err = a.EncodeTwoSIMDBytesToSIMDByteRegister(n)
						require.NoError(t, err)
						actual := a.Bytes()
						require.Equal(t, expected, actual)
					})
				}
			}
		}
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToVectorRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.B,
					SrcReg:         arm64.RegV21,
					DstReg:         arm64.RegV21,
					Types:          arm64.OperandTypesVectorRegisterToVectorRegister,
					SrcVectorIndex: arm64.VectorIndexNone,
					DstVectorIndex: arm64.VectorIndexNone,
				},
				expErr: "B is unsupported for from:vector-register,to:vector-register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					SrcReg:         arm64.RegV21,
					DstReg:         arm64.RegV21,
					Types:          arm64.OperandTypesVectorRegisterToVectorRegister,
					SrcVectorIndex: arm64.VectorIndexNone,
					DstVectorIndex: arm64.VectorIndexNone,
				},
				expErr: "unsupported arrangement for VMOV: none",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VADD,
					SrcReg:         arm64.RegV21,
					DstReg:         arm64.RegV21,
					Types:          arm64.OperandTypesVectorRegisterToVectorRegister,
					SrcVectorIndex: arm64.VectorIndexNone,
					DstVectorIndex: arm64.VectorIndexNone,
				},
				expErr: "unsupported arrangement for VADD: none",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VADD,
					SrcReg:            arm64.RegV21,
					DstReg:            arm64.RegV21,
					Types:             arm64.OperandTypesVectorRegisterToVectorRegister,
					VectorArrangement: arm64.VectorArrangement1D,
					SrcVectorIndex:    arm64.VectorIndexNone,
					DstVectorIndex:    arm64.VectorIndexNone,
				},
				expErr: "unsupported arrangement for VADD: 1D",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				a := arm64.NewAssemblerImpl(asm.NilRegister)
				err := a.EncodeVectorRegisterToVectorRegister(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	vectorRegs := []asm.Register{arm64.RegV10, arm64.RegV2, arm64.RegV30}
	tests := []struct {
		name               string
		inst               asm.Instruction
		arr                arm64.VectorArrangement
		needConst          bool
		c                  asm.ConstantValue
		srcIndex, dstIndex arm64.VectorIndex
	}{
		{inst: arm64.VMOV, arr: arm64.VectorArrangement16B, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.VADD, arr: arm64.VectorArrangement2D, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.VADD, arr: arm64.VectorArrangement4S, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.VADD, arr: arm64.VectorArrangement8H, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.VADD, arr: arm64.VectorArrangement16B, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{
			name: "VSUB 2d",
			inst: arm64.VSUB, arr: arm64.VectorArrangement2D, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone,
		},
		{
			name: "VSUB 4s",
			inst: arm64.VSUB, arr: arm64.VectorArrangement4S, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone,
		},
		{
			name: "VSUB 8h",
			inst: arm64.VSUB, arr: arm64.VectorArrangement8H, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone,
		},
		{
			name: "VSUB 16b",
			inst: arm64.VSUB, arr: arm64.VectorArrangement16B, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone,
		},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement8B, needConst: true, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement4H, needConst: true, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement2S, needConst: true, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement8B, needConst: true, c: 7, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement4H, needConst: true, c: 15, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
		{inst: arm64.USHLL, arr: arm64.VectorArrangement2S, needConst: true, c: 31, srcIndex: arm64.VectorIndexNone, dstIndex: arm64.VectorIndexNone},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			for _, src := range vectorRegs {
				for _, dst := range vectorRegs {
					src, dst := src, dst
					t.Run(fmt.Sprintf("src=%s.%s,dst=%s.%s",
						arm64.RegisterName(src), tc.arr, arm64.RegisterName(dst), tc.arr), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						a := arm64.NewAssemblerImpl(asm.NilRegister)

						for _, assembler := range []arm64.Assembler{goasm, a} {
							if tc.needConst {
								assembler.CompileVectorRegisterToVectorRegisterWithConst(tc.inst, src, dst, tc.arr, tc.c)
							} else {
								assembler.CompileVectorRegisterToVectorRegister(tc.inst, src, dst, tc.arr, tc.srcIndex, tc.dstIndex)
							}
						}

						expected, err := goasm.Assemble()
						require.NoError(t, err)

						fmt.Println(hex.EncodeToString(expected))

						actual, err := a.Assemble()
						require.NoError(t, err)
						require.Equal(t, expected, actual, hex.EncodeToString(expected))
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeRegisterToVectorRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n   *arm64.NodeImpl
			exp string
		}{
			{
				n: &arm64.NodeImpl{
					Instruction: arm64.B, Types: arm64.OperandTypesRegisterToVectorRegister,
					SrcReg: arm64.RegR0,
					DstReg: arm64.RegV3,
				},
				exp: "B is unsupported for from:register,to:vector-register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					SrcReg:         arm64.RegR0,
					DstReg:         arm64.RegV3,
					Types:          arm64.OperandTypesRegisterToVectorRegister,
					DstVectorIndex: 100, VectorArrangement: arm64.VectorArrangement1D,
				},
				exp: "invalid arrangement and index pair: 1D[100]",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					Types:          arm64.OperandTypesRegisterToVectorRegister,
					SrcReg:         arm64.RegR0,
					DstReg:         arm64.RegV3,
					DstVectorIndex: 0, VectorArrangement: arm64.VectorArrangement1D,
				},
				exp: "unsupported arrangement for VMOV: 1D",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.exp, func(t *testing.T) {
				a := arm64.NewAssemblerImpl(asm.NilRegister)
				err := a.EncodeRegisterToVectorRegister(tc.n)
				require.EqualError(t, err, tc.exp)
			})
		}
	})

	regs := []asm.Register{arm64.RegR0, arm64.RegR10, arm64.RegR30}
	vectorRegs := []asm.Register{arm64.RegV0, arm64.RegV10, arm64.RegV30}

	tests := []struct {
		inst        asm.Instruction
		arrangement arm64.VectorArrangement
		index       arm64.VectorIndex
	}{
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementD,
			index:       0,
		},
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementD,
			index:       1,
		},
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementB,
			index:       0,
		},
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementB,
			index:       5,
		},
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementH,
			index:       1,
		},
		{
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementH,
			index:       4,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, r := range regs {
				for _, vr := range vectorRegs {
					r, vr := r, vr
					t.Run(fmt.Sprintf("src=%s,dst=%s.%s[%d]",
						arm64.RegisterName(r), arm64.RegisterName(vr), tc.arrangement, tc.index), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						a := arm64.NewAssemblerImpl(asm.NilRegister)

						for _, assembler := range []arm64.Assembler{goasm, a} {
							assembler.CompileRegisterToVectorRegister(tc.inst, r, vr, tc.arrangement, tc.index)
						}

						expected, err := goasm.Assemble()
						require.NoError(t, err)

						actual, err := a.Assemble()
						require.NoError(t, err)
						require.Equal(t, expected, actual)
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.B, Types: arm64.OperandTypesVectorRegisterToRegister,
					SrcReg: arm64.RegV0,
					DstReg: arm64.RegR3,
				},
				expErr: "B is unsupported for from:vector-register,to:register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					Types:          arm64.OperandTypesVectorRegisterToRegister,
					SrcReg:         arm64.RegV0,
					DstReg:         arm64.RegR3,
					SrcVectorIndex: 100, VectorArrangement: arm64.VectorArrangement1D,
				},
				expErr: "invalid arrangement and index pair: 1D[100]",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					Types:          arm64.OperandTypesVectorRegisterToRegister,
					SrcReg:         arm64.RegV0,
					DstReg:         arm64.RegR3,
					SrcVectorIndex: 0, VectorArrangement: arm64.VectorArrangement1D,
				},
				expErr: "unsupported arrangement for VMOV: 1D",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	regs := []asm.Register{arm64.RegR0, arm64.RegR10, arm64.RegR30}
	vectorRegs := []asm.Register{arm64.RegV0, arm64.RegV10, arm64.RegV30}

	tests := []struct {
		name        string
		inst        asm.Instruction
		arrangement arm64.VectorArrangement
		index       arm64.VectorIndex
	}{
		{
			name:        "VMOV D[0]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementD,
			index:       0,
		},
		{
			name:        "VMOV D[1]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementD,
			index:       1,
		},
		{
			name:        "VMOV B[0]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementB,
			index:       0,
		},
		{
			name:        "VMOV B[15]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementB,
			index:       15,
		},
		{
			name:        "VMOV H[1]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementH,
			index:       1,
		},
		{
			name:        "VMOV H[4]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementH,
			index:       7,
		},
		{
			name:        "VMOV S[2]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementS,
			index:       2,
		},
		{
			name:        "VMOV S[3]",
			inst:        arm64.VMOV,
			arrangement: arm64.VectorArrangementS,
			index:       3,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range regs {
				for _, vr := range vectorRegs {
					r, vr := r, vr
					t.Run(fmt.Sprintf("dst=%s,src=%s.%s[%d]",
						arm64.RegisterName(r), arm64.RegisterName(vr), tc.arrangement, tc.index), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						a := arm64.NewAssemblerImpl(asm.NilRegister)

						for _, assembler := range []arm64.Assembler{goasm, a} {
							assembler.CompileVectorRegisterToRegister(tc.inst, vr, r, tc.arrangement, tc.index)
						}

						expected, err := goasm.Assemble()
						require.NoError(t, err)
						actual, err := a.Assemble()
						require.NoError(t, err)
						require.Equal(t, expected, actual)
					})
				}
			}
		})
	}
}

func conditionalRegisterToState(r asm.Register) asm.ConditionalRegisterState {
	switch r {
	case arm64.RegCondEQ:
		return arm64.CondEQ
	case arm64.RegCondNE:
		return arm64.CondNE
	case arm64.RegCondHS:
		return arm64.CondHS
	case arm64.RegCondLO:
		return arm64.CondLO
	case arm64.RegCondMI:
		return arm64.CondMI
	case arm64.RegCondPL:
		return arm64.CondPL
	case arm64.RegCondVS:
		return arm64.CondVS
	case arm64.RegCondVC:
		return arm64.CondVC
	case arm64.RegCondHI:
		return arm64.CondHI
	case arm64.RegCondLS:
		return arm64.CondLS
	case arm64.RegCondGE:
		return arm64.CondGE
	case arm64.RegCondLT:
		return arm64.CondLT
	case arm64.RegCondGT:
		return arm64.CondGT
	case arm64.RegCondLE:
		return arm64.CondLE
	case arm64.RegCondAL:
		return arm64.CondAL
	case arm64.RegCondNV:
		return arm64.CondNV
	}
	return asm.ConditionalRegisterStateUnset
}
