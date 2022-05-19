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
	arm64.REG_R0, arm64.REG_R1, arm64.REG_R2, arm64.REG_R3, arm64.REG_R4, arm64.REG_R5, arm64.REG_R6,
	arm64.REG_R7, arm64.REG_R8, arm64.REG_R9, arm64.REG_R10, arm64.REG_R11, arm64.REG_R12, arm64.REG_R13,
	arm64.REG_R14, arm64.REG_R15, arm64.REG_R16, arm64.REG_R17, arm64.REG_R18, arm64.REG_R19, arm64.REG_R20,
	arm64.REG_R21, arm64.REG_R22, arm64.REG_R23, arm64.REG_R24, arm64.REG_R25, arm64.REG_R26, arm64.REG_R27,
	arm64.REG_R28, arm64.REG_R29, arm64.REG_R30,
}

func TestAssemblerImpl_EncodeJumpToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
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
				n:      &arm64.NodeImpl{Instruction: arm64.RET, DstReg: arm64.REG_V0},
				expErr: "invalid destination register: V0 is not integer",
			},
		} {
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesLeftShiftedRegisterToRegister,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "SUB is unsupported for from:left-shifted-register,to:register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "shift amount must fit in unsigned 6-bit integer (0-64) but got -1",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.REG_V0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_V0, DstReg: arm64.REG_R0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADD,
					SrcConst: -1, SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_V0},
				expErr: "V0 is not integer",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeLeftShiftedRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.ADD
	for _, tc := range []struct {
		srcReg, shiftedSrcReg, dstReg asm.Register
		shiftNum                      int64
	}{
		{
			srcReg:        arm64.REG_R0,
			shiftedSrcReg: arm64.REG_R29,
			shiftNum:      1,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REG_R0,
			shiftedSrcReg: arm64.REG_R29,
			shiftNum:      2,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REG_R0,
			shiftedSrcReg: arm64.REG_R29,
			shiftNum:      8,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REG_R29,
			shiftedSrcReg: arm64.REG_R0,
			shiftNum:      16,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REG_R29,
			shiftedSrcReg: arm64.REG_R0,
			shiftNum:      64,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REGZERO,
			shiftedSrcReg: arm64.REG_R0,
			shiftNum:      64,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REGZERO,
			shiftedSrcReg: arm64.REGZERO,
			shiftNum:      64,
			dstReg:        arm64.REG_R21,
		},
		{
			srcReg:        arm64.REGZERO,
			shiftedSrcReg: arm64.REGZERO,
			shiftNum:      64,
			dstReg:        arm64.REGZERO,
		},
	} {
		tc := tc
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesTwoRegistersToNone,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "SUB is unsupported for from:two-registers,to:none type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_V0},
				expErr: "V0 is not integer",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.FCMPS,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_V0},
				expErr: "R0 is not float",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoRegistersToNone(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.REGZERO, arm64.REG_R0, arm64.REG_R10, arm64.REG_R30}
	floatRegs := []asm.Register{arm64.REG_V0, arm64.REG_V12, arm64.REG_V31}
	for _, tc := range []struct {
		instruction asm.Instruction
		regs        []asm.Register
	}{
		{instruction: arm64.CMP, regs: intRegs},
		{instruction: arm64.CMPW, regs: intRegs},
		{instruction: arm64.FCMPD, regs: floatRegs},
		{instruction: arm64.FCMPS, regs: floatRegs},
	} {
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
	intRegs := []asm.Register{arm64.REGZERO, arm64.REG_R1, arm64.REG_R10, arm64.REG_R30}
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterToRegister,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "ADR is unsupported for from:register,to:register type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.REGZERO, arm64.REG_R1, arm64.REG_R10, arm64.REG_R30}
	intRegsWithoutZero := intRegs[1:]
	conditionalRegs := []asm.Register{arm64.REG_COND_EQ, arm64.REG_COND_NE, arm64.REG_COND_HS, arm64.REG_COND_LO, arm64.REG_COND_MI, arm64.REG_COND_PL, arm64.REG_COND_VS, arm64.REG_COND_VC, arm64.REG_COND_HI, arm64.REG_COND_LS, arm64.REG_COND_GE, arm64.REG_COND_LT, arm64.REG_COND_GT, arm64.REG_COND_LE, arm64.REG_COND_AL, arm64.REG_COND_NV}
	floatRegs := []asm.Register{arm64.REG_V0, arm64.REG_V15, arm64.REG_V31}

	for _, tc := range []struct {
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
		{inst: arm64.MRS, srcRegs: []asm.Register{arm64.REG_FPSR}, dstRegs: intRegs},
		{inst: arm64.MSR, srcRegs: intRegs, dstRegs: []asm.Register{arm64.REG_FPSR}},
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
	} {

		tc := tc
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesTwoRegistersToRegister,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "ADR is unsupported for from:two-registers,to:register type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeThreeRegistersToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	intRegs := []asm.Register{arm64.REGZERO, arm64.REG_R1, arm64.REG_R10, arm64.REG_R30}
	floatRegs := []asm.Register{arm64.REG_V0, arm64.REG_V15, arm64.REG_V31}

	for _, tc := range []struct {
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
	} {
		tc := tc
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "ADR is unsupported for from:register-and-const,to:none type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.REG_R0, SrcConst: 12345},
				expErr: "immediate for CMP must fit in 0 to 4095 but got 12345",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.CMP, Types: arm64.OperandTypesRegisterAndConstToNone,
					SrcReg: arm64.REGZERO, SrcConst: 123},
				expErr: "zero register is not supported for CMP (immediate)",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterAndConstToNone(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.CMP
	for _, reg := range []asm.Register{arm64.REG_R1, arm64.REG_R10, arm64.REG_R30} {
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesConstToRegister,
					SrcReg: arm64.REG_R0, SrcReg2: arm64.REG_R0, DstReg: arm64.REG_R0},
				expErr: "ADR is unsupported for from:const,to:register type",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.LSR, Types: arm64.OperandTypesConstToRegister, DstReg: arm64.REG_R0},
				expErr: "LSR with zero constant should be optimized out",
			},
			{
				n:      &arm64.NodeImpl{Instruction: arm64.LSL, Types: arm64.OperandTypesConstToRegister, DstReg: arm64.REG_R0},
				expErr: "LSL with zero constant should be optimized out",
			},
		} {
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

	for _, tc := range []struct {
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
	} {
		tc := tc
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, r := range []asm.Register{
				arm64.REG_R0, arm64.REG_R10,
				arm64.REG_R30,
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
								goasm := newGoasmAssembler(t, arm64.REG_R27)
								goasm.CompileConstToRegister(tc.inst, c, r)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesSIMDByteToSIMDByte},
				expErr: "ADR is unsupported for from:simd-byte,to:simd-byte type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeSIMDByteToSIMDByte(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.VCNT
	t.Run(arm64.InstructionName(inst), func(t *testing.T) {
		floatRegs := []asm.Register{arm64.REG_V0, arm64.REG_V10, arm64.REG_V21, arm64.REG_V31}
		for _, src := range floatRegs {
			for _, dst := range floatRegs {
				src, dst := src, dst
				t.Run(fmt.Sprintf("src=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(dst)), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					goasm.CompileSIMDByteToSIMDByte(inst, src, dst)
					expected, err := goasm.Assemble()
					require.NoError(t, err)

					a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesSIMDByteToRegister},
				expErr: "ADR is unsupported for from:simd-byte,to:register type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeSIMDByteToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	const inst = arm64.VUADDLV
	t.Run(arm64.InstructionName(inst), func(t *testing.T) {
		floatRegs := []asm.Register{arm64.REG_V0, arm64.REG_V10, arm64.REG_V21, arm64.REG_V31}
		for _, src := range floatRegs {
			for _, dst := range floatRegs {
				src, dst := src, dst
				t.Run(fmt.Sprintf("src=%s,dst=%s", arm64.RegisterName(src), arm64.RegisterName(dst)), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					goasm.CompileSIMDByteToRegister(inst, src, dst)
					expected, err := goasm.Assemble()
					require.NoError(t, err)

					a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.ADR, Types: arm64.OperandTypesRegisterToMemory},
				expErr: "ADR is unsupported for from:register,to:memory type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToMemory(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	offsets := []int64{
		-1, 0, 1, 2, -2, 4, -4, 0xf, -0xf, 1 << 4, 1<<4 - 1, 1<<4 + 1, -128, -256, 8 * 10, -128,
		255, 4096, 4096 << 1, 32760, 32760 * 2, 32760*2 - 8,
		32760*2 - 16, 1 << 27, 1 << 30, 1<<30 + 8, 1<<30 - 8, 1<<30 + 16, 1<<30 - 16, 1<<31 - 8,
	}
	intRegs := []asm.Register{
		arm64.REG_R0, arm64.REG_R16,
		arm64.REG_R30,
	}
	floatRegs := []asm.Register{
		arm64.REG_V0, arm64.REG_V10,
		arm64.REG_V30,
	}
	for _, tc := range []struct {
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
	} {
		tc := tc
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, srcReg := range tc.srcRegs {
				for _, baseReg := range intRegs {
					t.Run("const offset", func(t *testing.T) {
						for _, offset := range tc.offsets {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesRegisterToMemory,
								Instruction: tc.inst, SrcReg: srcReg, DstReg: baseReg, DstConst: offset}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								a := arm64.NewAssemblerImpl(arm64.REG_R27)

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
						for _, offsetReg := range []asm.Register{arm64.REG_R8, arm64.REG_R18} {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesRegisterToMemory,
								Instruction: tc.inst, SrcReg: srcReg, DstReg: baseReg, DstReg2: offsetReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								goasm.CompileRegisterToMemoryWithRegisterOffset(n.Instruction, n.SrcReg, n.DstReg, n.DstReg2)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.SUB, Types: arm64.OperandTypesMemoryToRegister},
				expErr: "SUB is unsupported for from:memory,to:register type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeMemoryToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	offsets := []int64{
		-1, 0, 1, 2, -2, 0xf, -0xf, 1 << 4, 1<<4 - 1, 1<<4 + 1, -128, -256, 8 * 10, -128,
		255, 4096, 4096 << 1, 32760, 32760 * 2, 32760*2 - 8,
		32760*2 - 16, 1 << 27, 1 << 30, 1<<30 + 8, 1<<30 - 8, 1<<30 + 16, 1<<30 - 16, 1<<31 - 8,
		1<<12<<8 + 8,
		1<<12<<8 - 8,
	}
	intRegs := []asm.Register{
		arm64.REG_R0, arm64.REG_R16,
		arm64.REG_R30,
	}
	floatRegs := []asm.Register{
		arm64.REG_V0, arm64.REG_V10,
		arm64.REG_V30,
	}
	for _, tc := range []struct {
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
	} {
		tc := tc
		t.Run(arm64.InstructionName(tc.inst), func(t *testing.T) {
			for _, dstReg := range tc.dstRegs {
				for _, baseReg := range intRegs {
					t.Run("const offset", func(t *testing.T) {
						for _, offset := range tc.offsets {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesMemoryToRegister,
								Instruction: tc.inst, SrcReg: baseReg, SrcConst: offset, DstReg: dstReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								a := arm64.NewAssemblerImpl(arm64.REG_R27)

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
						for _, offsetReg := range []asm.Register{arm64.REG_R8, arm64.REG_R18} {
							n := &arm64.NodeImpl{Types: arm64.OperandTypesMemoryToRegister,
								Instruction: tc.inst, SrcReg: baseReg, SrcReg2: offsetReg, DstReg: dstReg}
							t.Run(n.String(), func(t *testing.T) {
								goasm := newGoasmAssembler(t, asm.NilRegister)
								goasm.CompileMemoryWithRegisterOffsetToRegister(n.Instruction, n.SrcReg, n.SrcReg2, n.DstReg)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, dstReg := range []asm.Register{arm64.REG_R19, arm64.REG_R23} {
			dstReg := dstReg
			t.Run(arm64.RegisterName(dstReg), func(t *testing.T) {
				goasm := newGoasmAssembler(t, asm.NilRegister)
				a := arm64.NewAssemblerImpl(asm.NilRegister)

				for _, assembler := range []arm64.Assembler{a, goasm} {
					assembler.CompileReadInstructionAddress(dstReg, targetBeforeInstruction)
					assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10) // Dummy
					assembler.CompileJumpToRegister(targetBeforeInstruction, arm64.REG_R25)
					assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10) // Target.
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
		a.CompileReadInstructionAddress(arm64.REG_R27, arm64.NOP)
		a.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10)
		_, err := a.Assemble()
		require.EqualError(t, err, "BUG: target instruction NOP not found for ADR")
	})
	t.Run("offset too large", func(t *testing.T) {
		a := arm64.NewAssemblerImpl(asm.NilRegister)
		a.CompileReadInstructionAddress(arm64.REG_R27, arm64.RET)
		a.CompileJumpToRegister(arm64.RET, arm64.REG_R25)
		a.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10)

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
		for _, tc := range []struct {
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
		} {
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
			for _, tc := range []struct {
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
			} {
				t.Run(fmt.Sprintf("foward=%v(before=%d,after=%d)", tc.forward,
					tc.instructionsBeforeBranch, tc.instructionsAfterBranch), func(t *testing.T) {
					goasm := newGoasmAssembler(t, asm.NilRegister)
					a := arm64.NewAssemblerImpl(asm.NilRegister)

					for _, assembler := range []arm64.Assembler{a, goasm} {
						for i := 0; i < tc.instructionsInPreamble; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10)
						}
						backwardTarget := assembler.CompileStandAlone(arm64.NOP)
						for i := 0; i < tc.instructionsBeforeBranch; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10)
						}
						br := assembler.CompileJump(inst)
						for i := 0; i < tc.instructionsAfterBranch; i++ {
							assembler.CompileConstToRegister(arm64.MOVD, 1000, arm64.REG_R10)
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
	a := arm64.NewAssemblerImpl(arm64.REG_R27)

	for _, assembler := range []arm64.Assembler{a, goasm} {
		for i := 0; i < 10000; i++ {
			// This will be put into const pool, but the callback won't be set for it.
			assembler.CompileRegisterToMemory(arm64.MOVD, arm64.REG_R11, arm64.REG_R12, 0xfff0+int64(i*8))
			// This will also set the call back for it.
			assembler.CompileRegisterToMemory(arm64.MOVD, arm64.REG_R11, arm64.REG_R12, (0xfff0+int64(i*8)<<16+8)%(1<<31))
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.B, Types: arm64.OperandTypesTwoSIMDBytesToSIMDByteRegister},
				expErr: "B is unsupported for from:two-simd-bytes,to:simd-byte type",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoSIMDBytesToSIMDByteRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	for _, inst := range []asm.Instruction{arm64.VBIT} {
		regs := []asm.Register{arm64.REG_V0, arm64.REG_V10, arm64.REG_V30}
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

						a := arm64.NewAssemblerImpl(arm64.REG_R27)
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
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.B,
					SrcReg: arm64.REG_V21,
					DstReg: arm64.REG_V21,
					Types:  arm64.OperandTypesVectorRegisterToVectorRegister,
				},
				expErr: "B is unsupported for from:vector-register,to:vector-register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					SrcReg: arm64.REG_V21,
					DstReg: arm64.REG_V21,
					Types:  arm64.OperandTypesVectorRegisterToVectorRegister,
				},
				expErr: "unsupported arrangement for VMOV: unknown",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VADD,
					SrcReg: arm64.REG_V21,
					DstReg: arm64.REG_V21,
					Types:  arm64.OperandTypesVectorRegisterToVectorRegister,
				},
				expErr: "unsupported arrangement for VADD: unknown",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VADD,
					SrcReg:            arm64.REG_V21,
					DstReg:            arm64.REG_V21,
					Types:             arm64.OperandTypesVectorRegisterToVectorRegister,
					VectorArrangement: arm64.VectorArrangement1D,
				},
				expErr: "unsupported arrangement for VADD: 1D",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToVectorRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	vectorRegs := []asm.Register{arm64.REG_V10, arm64.REG_V2, arm64.REG_V30}
	for _, tc := range []struct {
		inst asm.Instruction
		arr  arm64.VectorArrangement
	}{
		{inst: arm64.VMOV, arr: arm64.VectorArrangement16B},
		{inst: arm64.VADD, arr: arm64.VectorArrangement2D},
		{inst: arm64.VADD, arr: arm64.VectorArrangement4S},
		{inst: arm64.VADD, arr: arm64.VectorArrangement8H},
		{inst: arm64.VADD, arr: arm64.VectorArrangement16B},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s.%s", arm64.InstructionName(tc.inst), tc.arr), func(t *testing.T) {
			for _, src := range vectorRegs {
				for _, dst := range vectorRegs {
					src, dst := src, dst
					t.Run(fmt.Sprintf("src=%s.%s,dst=%s.%s",
						arm64.RegisterName(src), tc.arr, arm64.RegisterName(dst), tc.arr), func(t *testing.T) {
						goasm := newGoasmAssembler(t, asm.NilRegister)
						a := arm64.NewAssemblerImpl(asm.NilRegister)

						for _, assembler := range []arm64.Assembler{goasm, a} {
							assembler.CompileVectorRegisterToVectorRegister(tc.inst, src, dst, tc.arr)
						}

						expected, err := goasm.Assemble()
						require.NoError(t, err)

						actual, err := a.Assemble()
						require.NoError(t, err)
						require.Equal(t, expected, actual, hex.EncodeToString(expected))
					})
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeMemoryToVectorRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.B,
					SrcReg: arm64.REG_R1,
					DstReg: arm64.REG_V21,
					Types:  arm64.OperandTypesMemoryToVectorRegister,
				},
				expErr: "B is unsupported for from:memory,to:vector-register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VLD1,
					SrcReg: arm64.REG_R1,
					DstReg: arm64.REG_V21,
					Types:  arm64.OperandTypesMemoryToVectorRegister,
				},
				expErr: "unsupported arrangement for VLD1: unknown",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeMemoryToVectorRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	regs := []asm.Register{arm64.REG_R0, arm64.REG_R5, arm64.REG_R30}
	vectorRegs := []asm.Register{arm64.REG_V10, arm64.REG_V2}
	arrangements := []arm64.VectorArrangement{
		arm64.VectorArrangement8B,
		arm64.VectorArrangement16B,
		arm64.VectorArrangement4H,
		arm64.VectorArrangement8H,
		arm64.VectorArrangement2S,
		arm64.VectorArrangement4S,
		arm64.VectorArrangement1D,
		arm64.VectorArrangement2D,
	}

	for _, inst := range []asm.Instruction{arm64.VLD1} {
		inst := inst
		t.Run(arm64.InstructionName(inst), func(t *testing.T) {
			for _, arr := range arrangements {
				for _, offsetReg := range regs {
					for _, vr := range vectorRegs {
						arr, offsetReg, vr := arr, offsetReg, vr
						t.Run(fmt.Sprintf("src=%s,dst=%s.%s",
							arm64.RegisterName(offsetReg), arm64.RegisterName(vr), arr), func(t *testing.T) {
							goasm := newGoasmAssembler(t, asm.NilRegister)
							a := arm64.NewAssemblerImpl(asm.NilRegister)

							for _, assembler := range []arm64.Assembler{goasm, a} {
								assembler.CompileMemoryToVectorRegister(inst, offsetReg, vr, arr)
							}

							expected, err := goasm.Assemble()
							require.NoError(t, err)

							actual, err := a.Assemble()
							require.NoError(t, err)
							require.Equal(t, expected, actual, hex.EncodeToString(expected))
						})
					}
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n: &arm64.NodeImpl{Instruction: arm64.B,
					SrcReg: arm64.REG_V21,
					DstReg: arm64.REG_R1,
					Types:  arm64.OperandTypesVectorRegisterToMemory,
				},
				expErr: "B is unsupported for from:vector-register,to:memory type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VST1,
					SrcReg: arm64.REG_V21,
					DstReg: arm64.REG_R1,
					Types:  arm64.OperandTypesVectorRegisterToMemory,
				},
				expErr: "unsupported arrangement for VST1: unknown",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToMemory(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	regs := []asm.Register{arm64.REG_R0, arm64.REG_R5, arm64.REG_R30}
	vectorRegs := []asm.Register{arm64.REG_V10, arm64.REG_V2}
	arrangements := []arm64.VectorArrangement{
		arm64.VectorArrangement8B,
		arm64.VectorArrangement16B,
		arm64.VectorArrangement4H,
		arm64.VectorArrangement8H,
		arm64.VectorArrangement2S,
		arm64.VectorArrangement4S,
		arm64.VectorArrangement1D,
		arm64.VectorArrangement2D,
	}

	for _, inst := range []asm.Instruction{arm64.VST1} {
		inst := inst
		t.Run(arm64.InstructionName(inst), func(t *testing.T) {
			for _, arr := range arrangements {
				for _, offsetReg := range regs {
					for _, vr := range vectorRegs {
						arr, offsetReg, vr := arr, offsetReg, vr
						t.Run(fmt.Sprintf("src=%s,dst=%s.%s",
							arm64.RegisterName(offsetReg), arm64.RegisterName(vr), arr), func(t *testing.T) {
							goasm := newGoasmAssembler(t, asm.NilRegister)
							a := arm64.NewAssemblerImpl(asm.NilRegister)

							for _, assembler := range []arm64.Assembler{goasm, a} {
								assembler.CompileVectorRegisterToMemory(inst, vr, offsetReg, arr)
							}

							expected, err := goasm.Assemble()
							require.NoError(t, err)

							actual, err := a.Assemble()
							require.NoError(t, err)
							require.Equal(t, expected, actual, hex.EncodeToString(expected))
							fmt.Println(hex.EncodeToString(expected))
						})
					}
				}
			}
		})
	}
}

func TestAssemblerImpl_EncodeRegisterToVectorRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *arm64.NodeImpl
			expErr string
		}{
			{
				n:      &arm64.NodeImpl{Instruction: arm64.B, Types: arm64.OperandTypesRegisterToVectorRegister},
				expErr: "B is unsupported for from:register,to:vector-register type",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					Types:       arm64.OperandTypesRegisterToVectorRegister,
					VectorIndex: 100, VectorArrangement: arm64.VectorArrangement1D,
				},
				expErr: "invalid arrangement and index pair: 1D[100]",
			},
			{
				n: &arm64.NodeImpl{Instruction: arm64.VMOV,
					Types:       arm64.OperandTypesRegisterToVectorRegister,
					SrcReg:      arm64.REG_R0,
					DstReg:      arm64.REG_V3,
					VectorIndex: 0, VectorArrangement: arm64.VectorArrangement1D,
				},
				expErr: "unsupported arrangement for VMOV: 1D",
			},
		} {
			a := arm64.NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToVectorRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	regs := []asm.Register{arm64.REG_R0, arm64.REG_R10, arm64.REG_R30}
	vectorRegs := []asm.Register{arm64.REG_V0, arm64.REG_V10, arm64.REG_V30}

	for _, tc := range []struct {
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
	} {
		tc := tc
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

func conditionalRegisterToState(r asm.Register) asm.ConditionalRegisterState {
	switch r {
	case arm64.REG_COND_EQ:
		return arm64.COND_EQ
	case arm64.REG_COND_NE:
		return arm64.COND_NE
	case arm64.REG_COND_HS:
		return arm64.COND_HS
	case arm64.REG_COND_LO:
		return arm64.COND_LO
	case arm64.REG_COND_MI:
		return arm64.COND_MI
	case arm64.REG_COND_PL:
		return arm64.COND_PL
	case arm64.REG_COND_VS:
		return arm64.COND_VS
	case arm64.REG_COND_VC:
		return arm64.COND_VC
	case arm64.REG_COND_HI:
		return arm64.COND_HI
	case arm64.REG_COND_LS:
		return arm64.COND_LS
	case arm64.REG_COND_GE:
		return arm64.COND_GE
	case arm64.REG_COND_LT:
		return arm64.COND_LT
	case arm64.REG_COND_GT:
		return arm64.COND_GT
	case arm64.REG_COND_LE:
		return arm64.COND_LE
	case arm64.REG_COND_AL:
		return arm64.COND_AL
	case arm64.REG_COND_NV:
		return arm64.COND_NV
	}
	return asm.ConditionalRegisterStateUnset
}
