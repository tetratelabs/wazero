package asm_arm64

import (
	"testing"

	"github.com/heeus/hwazero/internal/asm"
	"github.com/heeus/hwazero/internal/testing/require"
)

func TestNodeImpl_AssignJumpTarget(t *testing.T) {
	n := &NodeImpl{}
	target := &NodeImpl{}
	n.AssignJumpTarget(target)
	require.Equal(t, n.JumpTarget, target)
}

func TestNodeImpl_AssignDestinationConstant(t *testing.T) {
	n := &NodeImpl{}
	n.AssignDestinationConstant(12345)
	require.Equal(t, int64(12345), n.DstConst)
}

func TestNodeImpl_AssignSourceConstant(t *testing.T) {
	n := &NodeImpl{}
	n.AssignSourceConstant(12345)
	require.Equal(t, int64(12345), n.SrcConst)
}

func TestNodeImpl_String(t *testing.T) {
	for _, tc := range []struct {
		in  *NodeImpl
		exp string
	}{
		{
			in:  &NodeImpl{Instruction: NOP, Types: OperandTypesNoneToNone},
			exp: "NOP",
		},
		{
			in:  &NodeImpl{Instruction: BEQ, Types: OperandTypesNoneToRegister, DstReg: REG_R1},
			exp: "BEQ R1",
		},
		{
			in:  &NodeImpl{Instruction: BNE, Types: OperandTypesNoneToMemory, DstReg: REG_R1, DstConst: 0x1234},
			exp: "BNE [R1 + 0x1234]",
		},
		{
			in:  &NodeImpl{Instruction: BNE, Types: OperandTypesNoneToBranch, JumpTarget: &NodeImpl{Instruction: NOP}},
			exp: "BNE {NOP}",
		},
		{
			in:  &NodeImpl{Instruction: ADD, Types: OperandTypesRegisterToRegister, SrcReg: REG_F0, DstReg: REG_F10},
			exp: "ADD F0, F10",
		},
		{
			in: &NodeImpl{Instruction: ADD, Types: OperandTypesLeftShiftedRegisterToRegister,
				SrcReg: REG_R0, SrcReg2: REG_R11, SrcConst: 4, DstReg: REG_R10},
			exp: "ADD (R0, R11 << 4), R10",
		},
		{
			in:  &NodeImpl{Instruction: ADD, Types: OperandTypesTwoRegistersToRegister, SrcReg: REG_R0, SrcReg2: REG_R8, DstReg: REG_R10},
			exp: "ADD (R0, R8), R10",
		},
		{
			in: &NodeImpl{Instruction: MSUB, Types: OperandTypesThreeRegistersToRegister,
				SrcReg: REG_R0, SrcReg2: REG_R8, DstReg: REG_R10, DstReg2: REG_R1},
			exp: "MSUB (R0, R8, R10), R1)",
		},
		{
			in:  &NodeImpl{Instruction: CMPW, Types: OperandTypesTwoRegistersToNone, SrcReg: REG_R0, SrcReg2: REG_R8},
			exp: "CMPW (R0, R8)",
		},
		{
			in:  &NodeImpl{Instruction: CMP, Types: OperandTypesRegisterAndConstToNone, SrcReg: REG_R0, SrcConst: 0x123},
			exp: "CMP (R0, 0x123)",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesRegisterToMemory, SrcReg: REG_R0, DstReg: REG_R8, DstConst: 0x123},
			exp: "MOVD R0, [R8 + 0x123]",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesRegisterToMemory, SrcReg: REG_R0, DstReg: REG_R8, DstReg2: REG_R6},
			exp: "MOVD R0, [R8 + R6]",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesMemoryToRegister, SrcReg: REG_R0, SrcConst: 0x123, DstReg: REG_R8},
			exp: "MOVD [R0 + 0x123], R8",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesMemoryToRegister, SrcReg: REG_R0, SrcReg2: REG_R6, DstReg: REG_R8},
			exp: "MOVD [R0 + R6], R8",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesConstToRegister, SrcConst: 0x123, DstReg: REG_R8},
			exp: "MOVD 0x123, R8",
		},
		{
			in:  &NodeImpl{Instruction: VCNT, Types: OperandTypesSIMDByteToSIMDByte, SrcReg: REG_F1, DstReg: REG_F2},
			exp: "VCNT F1.B8, F2.B8",
		},
		{
			in:  &NodeImpl{Instruction: VUADDLV, Types: OperandTypesSIMDByteToRegister, SrcReg: REG_F1, DstReg: REG_F2},
			exp: "VUADDLV F1.B8, F2",
		},
		{
			in:  &NodeImpl{Instruction: VBIT, Types: OperandTypesTwoSIMDBytesToSIMDByteRegister, SrcReg: REG_F1, SrcReg2: REG_F2, DstReg: REG_F3},
			exp: "VBIT (F1.B8, F2.B8), F3.B8",
		},
	} {
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)

	root := &NodeImpl{}
	a.addNode(root)
	require.Equal(t, a.Root, root)
	require.Equal(t, a.Current, root)
	require.Nil(t, root.Next)

	next := &NodeImpl{}
	a.addNode(next)
	require.Equal(t, a.Root, root)
	require.Equal(t, a.Current, next)
	require.Equal(t, next, root.Next)
	require.Nil(t, next.Next)
}

func TestAssemblerImpl_newNode(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	actual := a.newNode(MOVD, OperandTypesMemoryToRegister)
	require.Equal(t, MOVD, actual.Instruction)
	require.Equal(t, OperandTypeMemory, actual.Types.src)
	require.Equal(t, OperandTypeRegister, actual.Types.dst)
	require.Equal(t, actual, a.Root)
	require.Equal(t, actual, a.Current)
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileStandAlone(RET)
	actualNode := a.Current
	require.Equal(t, RET, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileConstToRegister(MOVD, 1000, REG_R10)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, int64(1000), actualNode.SrcConst)
	require.Equal(t, REG_R10, actualNode.DstReg)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileRegisterToRegister(MOVD, REG_R15, REG_R27)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R15, actualNode.SrcReg)
	require.Equal(t, REG_R27, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileMemoryToRegister(MOVD, REG_R15, 100, REG_R27)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R15, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, REG_R27, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileRegisterToMemory(MOVD, REG_R15, REG_R27, 100)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R15, actualNode.SrcReg)
	require.Equal(t, REG_R27, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileJump(B)
	actualNode := a.Current
	require.Equal(t, B, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeBranch, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileJumpToRegister(BNE, REG_R27)
	actualNode := a.Current
	require.Equal(t, BNE, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileJumpToMemory(BNE, REG_R27)
	actualNode := a.Current
	require.Equal(t, BNE, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileReadInstructionAddress(REG_R10, RET)
	actualNode := a.Current
	require.Equal(t, ADR, actualNode.Instruction)
	require.Equal(t, REG_R10, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func Test_CompileMemoryWithRegisterOffsetToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileMemoryWithRegisterOffsetToRegister(MOVD, REG_R27, REG_R10, REG_R0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.SrcReg2)
	require.Equal(t, REG_R0, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileRegisterToMemoryWithRegisterOffset(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileRegisterToMemoryWithRegisterOffset(MOVD, REG_R27, REG_R10, REG_R0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.DstReg)
	require.Equal(t, REG_R0, actualNode.DstReg2)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func Test_CompileTwoRegistersToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileTwoRegistersToRegister(MOVD, REG_R27, REG_R10, REG_R0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.SrcReg2)
	require.Equal(t, REG_R0, actualNode.DstReg)
	require.Equal(t, OperandTypeTwoRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileThreeRegistersToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileThreeRegistersToRegister(MOVD, REG_R27, REG_R10, REG_R0, REG_R28)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.SrcReg2)
	require.Equal(t, REG_R0, actualNode.DstReg)
	require.Equal(t, REG_R28, actualNode.DstReg2)
	require.Equal(t, OperandTypeThreeRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileTwoRegistersToNone(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileTwoRegistersToNone(CMP, REG_R27, REG_R10)
	actualNode := a.Current
	require.Equal(t, CMP, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.SrcReg2)
	require.Equal(t, OperandTypeTwoRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func Test_CompileRegisterAndConstToNone(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileRegisterAndConstToNone(CMP, REG_R27, 10)
	actualNode := a.Current
	require.Equal(t, CMP, actualNode.Instruction)
	require.Equal(t, REG_R27, actualNode.SrcReg)
	require.Equal(t, int64(10), actualNode.SrcConst)
	require.Equal(t, OperandTypeRegisterAndConst, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func Test_CompileLeftShiftedRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileLeftShiftedRegisterToRegister(ADD, REG_R27, 10, REG_R28, REG_R5)
	actualNode := a.Current
	require.Equal(t, ADD, actualNode.Instruction)
	require.Equal(t, REG_R28, actualNode.SrcReg)
	require.Equal(t, REG_R27, actualNode.SrcReg2)
	require.Equal(t, int64(10), actualNode.SrcConst)
	require.Equal(t, REG_R5, actualNode.DstReg)
	require.Equal(t, OperandTypeLeftShiftedRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileSIMDByteToSIMDByte(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileSIMDByteToSIMDByte(VCNT, REG_F0, REG_F2)
	actualNode := a.Current
	require.Equal(t, VCNT, actualNode.Instruction)
	require.Equal(t, REG_F0, actualNode.SrcReg)
	require.Equal(t, REG_F2, actualNode.DstReg)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.src)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.dst)
}

func Test_CompileTwoSIMDBytesToSIMDByteRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileTwoSIMDBytesToSIMDByteRegister(VBIT, REG_F0, REG_F10, REG_F2)
	actualNode := a.Current
	require.Equal(t, VBIT, actualNode.Instruction)
	require.Equal(t, REG_F0, actualNode.SrcReg)
	require.Equal(t, REG_F10, actualNode.SrcReg2)
	require.Equal(t, REG_F2, actualNode.DstReg)
	require.Equal(t, OperandTypeTwoSIMDBytes, actualNode.Types.src)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.dst)
}

func Test_CompileSIMDByteToRegister(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileSIMDByteToRegister(VUADDLV, REG_F0, REG_F10)
	actualNode := a.Current
	require.Equal(t, VUADDLV, actualNode.Instruction)
	require.Equal(t, REG_F0, actualNode.SrcReg)
	require.Equal(t, REG_F10, actualNode.DstReg)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileConditionalRegisterSet(t *testing.T) {
	a := NewAssemblerImpl(REG_R10)
	a.CompileConditionalRegisterSet(COND_NE, REG_R10)
	actualNode := a.Current
	require.Equal(t, CSET, actualNode.Instruction)
	require.Equal(t, REG_COND_NE, actualNode.SrcReg)
	require.Equal(t, REG_R10, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_checkRegisterToRegisterType(t *testing.T) {
	for _, tc := range []struct {
		src, dst                     asm.Register
		requireSrcInt, requireDstInt bool
		expErr                       string
	}{
		{src: REG_R10, dst: REG_R30, requireSrcInt: true, requireDstInt: true, expErr: ""},
		{src: REG_R10, dst: REG_R30, requireSrcInt: false, requireDstInt: true, expErr: "src requires float register but got R10"},
		{src: REG_R10, dst: REG_R30, requireSrcInt: false, requireDstInt: false, expErr: "src requires float register but got R10"},
		{src: REG_R10, dst: REG_R30, requireSrcInt: true, requireDstInt: false, expErr: "dst requires float register but got R30"},

		{src: REG_R10, dst: REG_F30, requireSrcInt: true, requireDstInt: false, expErr: ""},
		{src: REG_R10, dst: REG_F30, requireSrcInt: false, requireDstInt: true, expErr: "src requires float register but got R10"},
		{src: REG_R10, dst: REG_F30, requireSrcInt: false, requireDstInt: false, expErr: "src requires float register but got R10"},
		{src: REG_R10, dst: REG_F30, requireSrcInt: true, requireDstInt: true, expErr: "dst requires int register but got F30"},

		{src: REG_F10, dst: REG_R30, requireSrcInt: false, requireDstInt: true, expErr: ""},
		{src: REG_F10, dst: REG_R30, requireSrcInt: true, requireDstInt: true, expErr: "src requires int register but got F10"},
		{src: REG_F10, dst: REG_R30, requireSrcInt: true, requireDstInt: false, expErr: "src requires int register but got F10"},
		{src: REG_F10, dst: REG_R30, requireSrcInt: false, requireDstInt: false, expErr: "dst requires float register but got R30"},

		{src: REG_F10, dst: REG_F30, requireSrcInt: false, requireDstInt: false, expErr: ""},
		{src: REG_F10, dst: REG_F30, requireSrcInt: true, requireDstInt: false, expErr: "src requires int register but got F10"},
		{src: REG_F10, dst: REG_F30, requireSrcInt: true, requireDstInt: true, expErr: "src requires int register but got F10"},
		{src: REG_F10, dst: REG_F30, requireSrcInt: false, requireDstInt: true, expErr: "dst requires int register but got F30"},
	} {
		actual := checkRegisterToRegisterType(tc.src, tc.dst, tc.requireSrcInt, tc.requireDstInt)
		if tc.expErr != "" {
			require.EqualError(t, actual, tc.expErr)
		} else {
			require.NoError(t, actual)
		}
	}
}

func Test_validateMemoryOffset(t *testing.T) {
	for _, tc := range []struct {
		offset int64
		expErr string
	}{
		{offset: 0}, {offset: -256}, {offset: 255}, {offset: 123 * 8},
		{offset: -257, expErr: "negative memory offset must be larget than or equal -256 but got -257"},
		{offset: 257, expErr: "large memory offset (>255) must be a multiple of 8 but got 257"},
	} {
		actual := validateMemoryOffset(tc.offset)
		if tc.expErr == "" {
			require.NoError(t, actual)
		} else {
			require.EqualError(t, actual, tc.expErr)
		}
	}
}
