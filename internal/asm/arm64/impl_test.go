package arm64

import (
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
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
	tests := []struct {
		in  *NodeImpl
		exp string
	}{
		{
			in:  &NodeImpl{Instruction: NOP, Types: OperandTypesNoneToNone},
			exp: "NOP",
		},
		{
			in:  &NodeImpl{Instruction: BEQ, Types: OperandTypesNoneToRegister, DstReg: RegR1},
			exp: "BEQ R1",
		},
		{
			in:  &NodeImpl{Instruction: BNE, Types: OperandTypesNoneToMemory, DstReg: RegR1, DstConst: 0x1234},
			exp: "BNE [R1 + 0x1234]",
		},
		{
			in:  &NodeImpl{Instruction: BNE, Types: OperandTypesNoneToBranch, JumpTarget: &NodeImpl{Instruction: NOP}},
			exp: "BNE {NOP}",
		},
		{
			in:  &NodeImpl{Instruction: ADD, Types: OperandTypesRegisterToRegister, SrcReg: RegV0, DstReg: RegV10},
			exp: "ADD V0, V10",
		},
		{
			in: &NodeImpl{Instruction: ADD, Types: OperandTypesLeftShiftedRegisterToRegister,
				SrcReg: RegR0, SrcReg2: RegR11, SrcConst: 4, DstReg: RegR10},
			exp: "ADD (R0, R11 << 4), R10",
		},
		{
			in:  &NodeImpl{Instruction: ADD, Types: OperandTypesTwoRegistersToRegister, SrcReg: RegR0, SrcReg2: RegR8, DstReg: RegR10},
			exp: "ADD (R0, R8), R10",
		},
		{
			in: &NodeImpl{Instruction: MSUB, Types: OperandTypesThreeRegistersToRegister,
				SrcReg: RegR0, SrcReg2: RegR8, DstReg: RegR10, DstReg2: RegR1},
			exp: "MSUB (R0, R8, R10), R1)",
		},
		{
			in:  &NodeImpl{Instruction: CMPW, Types: OperandTypesTwoRegistersToNone, SrcReg: RegR0, SrcReg2: RegR8},
			exp: "CMPW (R0, R8)",
		},
		{
			in:  &NodeImpl{Instruction: CMP, Types: OperandTypesRegisterAndConstToNone, SrcReg: RegR0, SrcConst: 0x123},
			exp: "CMP (R0, 0x123)",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesRegisterToMemory, SrcReg: RegR0, DstReg: RegR8, DstConst: 0x123},
			exp: "MOVD R0, [R8 + 0x123]",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesRegisterToMemory, SrcReg: RegR0, DstReg: RegR8, DstReg2: RegR6},
			exp: "MOVD R0, [R8 + R6]",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesMemoryToRegister, SrcReg: RegR0, SrcConst: 0x123, DstReg: RegR8},
			exp: "MOVD [R0 + 0x123], R8",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesMemoryToRegister, SrcReg: RegR0, SrcReg2: RegR6, DstReg: RegR8},
			exp: "MOVD [R0 + R6], R8",
		},
		{
			in:  &NodeImpl{Instruction: MOVD, Types: OperandTypesConstToRegister, SrcConst: 0x123, DstReg: RegR8},
			exp: "MOVD 0x123, R8",
		},
		{
			in:  &NodeImpl{Instruction: VCNT, Types: OperandTypesSIMDByteToSIMDByte, SrcReg: RegV1, DstReg: RegV2},
			exp: "VCNT V1.B8, V2.B8",
		},
		{
			in:  &NodeImpl{Instruction: VUADDLV, Types: OperandTypesSIMDByteToRegister, SrcReg: RegV1, DstReg: RegV2},
			exp: "VUADDLV V1.B8, V2",
		},
		{
			in:  &NodeImpl{Instruction: VBIT, Types: OperandTypesTwoSIMDBytesToSIMDByteRegister, SrcReg: RegV1, SrcReg2: RegV2, DstReg: RegV3},
			exp: "VBIT (V1.B8, V2.B8), V3.B8",
		},
		{
			in: &NodeImpl{Instruction: VMOV, Types: OperandTypesMemoryToVectorRegister,
				SrcReg: RegR1, DstReg: RegV29, VectorArrangement: VectorArrangement2S},
			exp: "VMOV [R1], V29.2S",
		},
		{
			in: &NodeImpl{Instruction: VMOV, Types: OperandTypesVectorRegisterToMemory,
				DstReg: RegR1, SrcReg: RegV29, VectorArrangement: VectorArrangementQ},
			exp: "VMOV V29.Q, [R1]",
		},
		{
			in: &NodeImpl{Instruction: VMOV, Types: OperandTypesRegisterToVectorRegister,
				SrcReg: RegR1, DstReg: RegV29, VectorArrangement: VectorArrangement2D, DstVectorIndex: 1},
			exp: "VMOV R1, V29.2D[1]",
		},
		{
			in: &NodeImpl{Instruction: VCNT, Types: OperandTypesVectorRegisterToVectorRegister,
				SrcReg: RegV3, DstReg: RegV29, VectorArrangement: VectorArrangement2D, SrcVectorIndex: 1},
			exp: "VCNT V3.V3, V29.V3",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.exp, func(t *testing.T) {
			require.Equal(t, tc.exp, tc.in.String())
		})
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := NewAssemblerImpl(RegR10)

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
	a := NewAssemblerImpl(RegR10)
	actual := a.newNode(MOVD, OperandTypesMemoryToRegister)
	require.Equal(t, MOVD, actual.Instruction)
	require.Equal(t, OperandTypeMemory, actual.Types.src)
	require.Equal(t, OperandTypeRegister, actual.Types.dst)
	require.Equal(t, actual, a.Root)
	require.Equal(t, actual, a.Current)
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileStandAlone(RET)
	actualNode := a.Current
	require.Equal(t, RET, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileConstToRegister(MOVD, 1000, RegR10)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, int64(1000), actualNode.SrcConst)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileRegisterToRegister(MOVD, RegR15, RegR27)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR15, actualNode.SrcReg)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileMemoryToRegister(MOVD, RegR15, 100, RegR27)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR15, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileRegisterToMemory(MOVD, RegR15, RegR27, 100)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR15, actualNode.SrcReg)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileJump(B)
	actualNode := a.Current
	require.Equal(t, B, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeBranch, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileJumpToRegister(BNE, RegR27)
	actualNode := a.Current
	require.Equal(t, BNE, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileJumpToMemory(BNE, RegR27)
	actualNode := a.Current
	require.Equal(t, BNE, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileReadInstructionAddress(RegR10, RET)
	actualNode := a.Current
	require.Equal(t, ADR, actualNode.Instruction)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func Test_CompileMemoryWithRegisterOffsetToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileMemoryWithRegisterOffsetToRegister(MOVD, RegR27, RegR10, RegR0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.SrcReg2)
	require.Equal(t, RegR0, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileRegisterToMemoryWithRegisterOffset(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileRegisterToMemoryWithRegisterOffset(MOVD, RegR27, RegR10, RegR0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, RegR0, actualNode.DstReg2)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func Test_CompileTwoRegistersToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileTwoRegistersToRegister(MOVD, RegR27, RegR10, RegR0)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.SrcReg2)
	require.Equal(t, RegR0, actualNode.DstReg)
	require.Equal(t, OperandTypeTwoRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileThreeRegistersToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileThreeRegistersToRegister(MOVD, RegR27, RegR10, RegR0, RegR28)
	actualNode := a.Current
	require.Equal(t, MOVD, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.SrcReg2)
	require.Equal(t, RegR0, actualNode.DstReg)
	require.Equal(t, RegR28, actualNode.DstReg2)
	require.Equal(t, OperandTypeThreeRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileTwoRegistersToNone(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileTwoRegistersToNone(CMP, RegR27, RegR10)
	actualNode := a.Current
	require.Equal(t, CMP, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.SrcReg2)
	require.Equal(t, OperandTypeTwoRegisters, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func Test_CompileRegisterAndConstToNone(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileRegisterAndConstToNone(CMP, RegR27, 10)
	actualNode := a.Current
	require.Equal(t, CMP, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.SrcReg)
	require.Equal(t, int64(10), actualNode.SrcConst)
	require.Equal(t, OperandTypeRegisterAndConst, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func Test_CompileLeftShiftedRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileLeftShiftedRegisterToRegister(ADD, RegR27, 10, RegR28, RegR5)
	actualNode := a.Current
	require.Equal(t, ADD, actualNode.Instruction)
	require.Equal(t, RegR28, actualNode.SrcReg)
	require.Equal(t, RegR27, actualNode.SrcReg2)
	require.Equal(t, int64(10), actualNode.SrcConst)
	require.Equal(t, RegR5, actualNode.DstReg)
	require.Equal(t, OperandTypeLeftShiftedRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileSIMDByteToSIMDByte(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileSIMDByteToSIMDByte(VCNT, RegV0, RegV2)
	actualNode := a.Current
	require.Equal(t, VCNT, actualNode.Instruction)
	require.Equal(t, RegV0, actualNode.SrcReg)
	require.Equal(t, RegV2, actualNode.DstReg)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.src)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.dst)
}

func Test_CompileTwoSIMDBytesToSIMDByteRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileTwoSIMDBytesToSIMDByteRegister(VBIT, RegV0, RegV10, RegV2)
	actualNode := a.Current
	require.Equal(t, VBIT, actualNode.Instruction)
	require.Equal(t, RegV0, actualNode.SrcReg)
	require.Equal(t, RegV10, actualNode.SrcReg2)
	require.Equal(t, RegV2, actualNode.DstReg)
	require.Equal(t, OperandTypeTwoSIMDBytes, actualNode.Types.src)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.dst)
}

func Test_CompileSIMDByteToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileSIMDByteToRegister(VUADDLV, RegV0, RegV10)
	actualNode := a.Current
	require.Equal(t, VUADDLV, actualNode.Instruction)
	require.Equal(t, RegV0, actualNode.SrcReg)
	require.Equal(t, RegV10, actualNode.DstReg)
	require.Equal(t, OperandTypeSIMDByte, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileConditionalRegisterSet(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileConditionalRegisterSet(CondNE, RegR10)
	actualNode := a.Current
	require.Equal(t, CSET, actualNode.Instruction)
	require.Equal(t, RegCondNE, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func Test_CompileMemoryToVectorRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileMemoryToVectorRegister(VMOV, RegR10, 10, RegV3, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegR10, actualNode.SrcReg)
	require.Equal(t, int64(10), actualNode.SrcConst)
	require.Equal(t, RegV3, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
}

func Test_CompileVectorRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileVectorRegisterToMemory(VMOV, RegV3, RegR10, 12, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, int64(12), actualNode.DstConst)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
}

func Test_CompileRegisterToVectorRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileRegisterToVectorRegister(VMOV, RegV3, RegR10, VectorArrangement1D, 10)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
	require.Equal(t, VectorIndex(10), actualNode.DstVectorIndex)
}

func Test_CompileVectorRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileVectorRegisterToRegister(VMOV, RegR10, RegV3, VectorArrangement1D, 10)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegR10, actualNode.SrcReg)
	require.Equal(t, RegV3, actualNode.DstReg)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
	require.Equal(t, VectorIndex(10), actualNode.SrcVectorIndex)
}

func Test_CompileVectorRegisterToVectorRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileVectorRegisterToVectorRegister(VMOV, RegV3, RegV10, VectorArrangement1D, 1, 2)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegV10, actualNode.DstReg)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
	require.Equal(t, VectorIndex(1), actualNode.SrcVectorIndex)
	require.Equal(t, VectorIndex(2), actualNode.DstVectorIndex)
}

func Test_checkRegisterToRegisterType(t *testing.T) {
	tests := []struct {
		src, dst                     asm.Register
		requireSrcInt, requireDstInt bool
		expErr                       string
	}{
		{src: RegR10, dst: RegR30, requireSrcInt: true, requireDstInt: true, expErr: ""},
		{src: RegR10, dst: RegR30, requireSrcInt: false, requireDstInt: true, expErr: "src requires float register but got R10"},
		{src: RegR10, dst: RegR30, requireSrcInt: false, requireDstInt: false, expErr: "src requires float register but got R10"},
		{src: RegR10, dst: RegR30, requireSrcInt: true, requireDstInt: false, expErr: "dst requires float register but got R30"},

		{src: RegR10, dst: RegV30, requireSrcInt: true, requireDstInt: false, expErr: ""},
		{src: RegR10, dst: RegV30, requireSrcInt: false, requireDstInt: true, expErr: "src requires float register but got R10"},
		{src: RegR10, dst: RegV30, requireSrcInt: false, requireDstInt: false, expErr: "src requires float register but got R10"},
		{src: RegR10, dst: RegV30, requireSrcInt: true, requireDstInt: true, expErr: "dst requires int register but got V30"},

		{src: RegV10, dst: RegR30, requireSrcInt: false, requireDstInt: true, expErr: ""},
		{src: RegV10, dst: RegR30, requireSrcInt: true, requireDstInt: true, expErr: "src requires int register but got V10"},
		{src: RegV10, dst: RegR30, requireSrcInt: true, requireDstInt: false, expErr: "src requires int register but got V10"},
		{src: RegV10, dst: RegR30, requireSrcInt: false, requireDstInt: false, expErr: "dst requires float register but got R30"},

		{src: RegV10, dst: RegV30, requireSrcInt: false, requireDstInt: false, expErr: ""},
		{src: RegV10, dst: RegV30, requireSrcInt: true, requireDstInt: false, expErr: "src requires int register but got V10"},
		{src: RegV10, dst: RegV30, requireSrcInt: true, requireDstInt: true, expErr: "src requires int register but got V10"},
		{src: RegV10, dst: RegV30, requireSrcInt: false, requireDstInt: true, expErr: "dst requires int register but got V30"},
	}

	for _, tt := range tests {
		tc := tt
		actual := checkRegisterToRegisterType(tc.src, tc.dst, tc.requireSrcInt, tc.requireDstInt)
		if tc.expErr != "" {
			require.EqualError(t, actual, tc.expErr)
		} else {
			require.NoError(t, actual)
		}
	}
}

func Test_validateMemoryOffset(t *testing.T) {
	tests := []struct {
		offset int64
		expErr string
	}{
		{offset: 0}, {offset: -256}, {offset: 255}, {offset: 123 * 8}, {offset: 123 * 4},
		{offset: -257, expErr: "negative memory offset must be larget than or equal -256 but got -257"},
		{offset: 257, expErr: "large memory offset (>255) must be a multiple of 4 but got 257"},
	}

	for _, tt := range tests {
		tc := tt
		actual := validateMemoryOffset(tc.offset)
		if tc.expErr == "" {
			require.NoError(t, actual)
		} else {
			require.EqualError(t, actual, tc.expErr)
		}
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToMemory(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		// Register offset cases.
		{
			name: "str b11, [x12, x6]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR12,
				DstReg2:           RegR6,
				VectorArrangement: VectorArrangementB,
			},
			exp: []byte{0x8b, 0x69, 0x26, 0x3c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str h11, [x12, x6]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR0,
				DstReg2:           RegR6,
				VectorArrangement: VectorArrangementH,
			},
			exp: []byte{0xb, 0x68, 0x26, 0x7c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str s11, [x29, x6]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR29,
				DstReg2:           RegR6,
				VectorArrangement: VectorArrangementS,
			},
			exp: []byte{0xab, 0x6b, 0x26, 0xbc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str d0, [x0, x0]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV0,
				DstReg:            RegR0,
				DstReg2:           RegR0,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0x0, 0x68, 0x20, 0xfc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str q30, [x30, x29]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV30,
				DstReg:            RegR30,
				DstReg2:           RegR29,
				VectorArrangement: VectorArrangementQ,
			},
			exp: []byte{0xde, 0x6b, 0xbd, 0x3c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		// Constant offset cases.
		{
			name: "str b11, [x12, #0x7b]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR12,
				DstConst:          0x7b,
				VectorArrangement: VectorArrangementB,
			},
			exp: []byte{0x8b, 0xed, 0x1, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr w10, #0xc ; str h11, [x12, x10]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR12,
				DstConst:          1 << 30,
				VectorArrangement: VectorArrangementH,
			},
			exp: []byte{0x6a, 0x0, 0x0, 0x18, 0x8b, 0x69, 0x2a, 0x7c, 0x0, 0x0, 0x0, 0x14, 0x0, 0x0, 0x0, 0x40},
		},
		{
			name: "ldr w10, #0xc ; str s11, [x12, x10]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR12,
				DstConst:          (1 << 28) + 4,
				VectorArrangement: VectorArrangementS,
			},
			exp: []byte{0x6a, 0x0, 0x0, 0x18, 0x8b, 0x69, 0x2a, 0xbc, 0x0, 0x0, 0x0, 0x14, 0x4, 0x0, 0x0, 0x10},
		},
		{
			name: "str d11, [x12, #0x3d8]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV11,
				DstReg:            RegR12,
				DstConst:          0x3d8,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0x8b, 0xed, 0x1, 0xfd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str q1, [x30]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegV1,
				DstReg:            RegR30,
				DstConst:          0,
				VectorArrangement: VectorArrangementQ,
			},
			exp: []byte{0xc1, 0x3, 0x80, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(RegR10)
			err := a.EncodeVectorRegisterToMemory(tc.n)
			require.NoError(t, err)

			a.maybeFlushConstPool(true)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeMemoryToVectorRegister(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		// ldr Register offset cases.
		{
			name: "ldr b11, [x12, x8]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR12,
				SrcReg2:           RegR8,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangementB,
			},
			exp: []byte{0x8b, 0x69, 0x68, 0x3c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr h11, [x30, x0]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR30,
				SrcReg2:           RegR0,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangementH,
			},
			exp: []byte{0xcb, 0x6b, 0x60, 0x7c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr s11, [x0, x30]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR0,
				SrcReg2:           RegR30,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangementS,
			},
			exp: []byte{0xb, 0x68, 0x7e, 0xbc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr d11, [x15, x15]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR15,
				SrcReg2:           RegR15,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0xeb, 0x69, 0x6f, 0xfc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr q30, [x0, x0]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR0,
				SrcReg2:           RegR0,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangementQ,
			},
			exp: []byte{0x1e, 0x68, 0xe0, 0x3c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		// ldr Constant offset cases.
		{
			name: "ldr b11, [x12, #0x7b]",
			n: &NodeImpl{
				Instruction:       VMOV,
				SrcReg:            RegR12,
				SrcConst:          0x7b,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangementB,
			},
			exp: []byte{0x8b, 0xed, 0x41, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "str h11, [x12, w30, uxtw]",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV11,
				SrcReg:            RegR12,
				VectorArrangement: VectorArrangementH,
			},
			exp: []byte{0x8b, 0x1, 0x40, 0x7d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ldr w10, #0xc ; ldr s11, [x12, x10]",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV11,
				SrcReg:            RegR12,
				SrcConst:          1 << 28,
				VectorArrangement: VectorArrangementS,
			},
			exp: []byte{0x6a, 0x0, 0x0, 0x18, 0x8b, 0x69, 0x6a, 0xbc, 0x0, 0x0, 0x0, 0x14, 0x0, 0x0, 0x0, 0x10},
		},
		{
			name: "ldr w10, #0xc ; ldr d11, [x12, x10]",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV11,
				SrcReg:            RegR12,
				SrcConst:          1<<29 + 4,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0x6a, 0x0, 0x0, 0x18, 0x8b, 0x69, 0x6a, 0xfc, 0x0, 0x0, 0x0, 0x14, 0x4, 0x0, 0x0, 0x20},
		},
		{
			name: "ldr w10, #0xc ; ldr q1, [x30, x10]",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV1,
				SrcReg:            RegR30,
				SrcConst:          1<<17 + 4,
				VectorArrangement: VectorArrangementQ,
			},
			exp: []byte{0x6a, 0x0, 0x0, 0x18, 0xc1, 0x6b, 0xea, 0x3c, 0x0, 0x0, 0x0, 0x14, 0x4, 0x0, 0x2, 0x0},
		},
		// LD1R
		{
			name: "ld1r {v11.8b}, [x12]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR12,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0x8b, 0xc1, 0x40, 0xd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v11.16b}, [x12]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR12,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x8b, 0xc1, 0x40, 0x4d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v11.4h}, [x12]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR12,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangement4H,
			},
			exp: []byte{0x8b, 0xc5, 0x40, 0xd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v9.8h}, [x0]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR0,
				DstReg:            RegV0,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x0, 0xc4, 0x40, 0x4d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v11.2s}, [x12]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR12,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x8b, 0xc9, 0x40, 0xd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v0.4s}, [x0]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR0,
				DstReg:            RegV0,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x0, 0xc8, 0x40, 0x4d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v11.1d}, [x12]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR12,
				DstReg:            RegV11,
				VectorArrangement: VectorArrangement1D,
			},
			exp: []byte{0x8b, 0xcd, 0x40, 0xd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "ld1r {v0.2d}, [x0]",
			n: &NodeImpl{
				Instruction:       LD1R,
				SrcReg:            RegR0,
				DstReg:            RegV0,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x0, 0xcc, 0x40, 0x4d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(RegR10)
			err := a.EncodeMemoryToVectorRegister(tc.n)
			require.NoError(t, err)

			a.maybeFlushConstPool(true)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToVectorRegister(t *testing.T) {
	tests := []struct {
		name               string
		x1, x2             asm.Register
		inst               asm.Instruction
		c                  asm.ConstantValue
		arr                VectorArrangement
		srcIndex, dstIndex VectorIndex
		exp                []byte
	}{
		// These are not supported in golang-asm, so test it here instead of integration tests.
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: VFADDD,
			exp: []byte{
				0x4a, 0xd4, 0x6a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: VFADDS,
			exp: []byte{
				0x4a, 0xd4, 0x2a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: VFSUBD,
			exp: []byte{
				0x4a, 0xd4, 0xea, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: VFSUBS,
			exp: []byte{
				0x4a, 0xd4, 0xaa, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp: []byte{
				0x4a, 0xa4, 0x8, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement8B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL, exp: []byte{
				0x4a, 0xa4, 0xf, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement8B,
			c:   7,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp: []byte{
				0x4a, 0xa4, 0x10, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement4H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp: []byte{
				0x4a, 0xa4, 0x1f, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement4H,
			c:   15,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp: []byte{
				0x4a, 0xa4, 0x20, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement2S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp: []byte{
				0x4a, 0xa4, 0x3f, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
			arr: VectorArrangement2S,
			c:   31,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.s[2], v2.s[1]",
			inst:     VMOV,
			exp:      []byte{0x4a, 0x24, 0x14, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementS,
			srcIndex: 1,
			dstIndex: 2,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.s[0], v2.s[3]",
			inst:     VMOV,
			exp:      []byte{0x4a, 0x64, 0x4, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementS,
			srcIndex: 3,
			dstIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.b[0], v2.b[0xf]",
			inst:     VMOV,
			exp:      []byte{0x4a, 0x7c, 0x1, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementB,
			srcIndex: 15,
			dstIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.d[1], v2.d[0]",
			inst:     VMOV,
			exp:      []byte{0x4a, 0x4, 0x18, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementD,
			srcIndex: 0,
			dstIndex: 1,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.2d, v2.d[0]",
			inst:     DUP,
			exp:      []byte{0x4a, 0x4, 0x8, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementD,
			srcIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.2d, v2.d[1]",
			inst:     DUP,
			exp:      []byte{0x4a, 0x4, 0x18, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementD,
			srcIndex: 1,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.4s, v2.s[3]",
			inst:     DUP,
			exp:      []byte{0x4a, 0x4, 0x1c, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementS,
			srcIndex: 3,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.8h, v2.h[7]",
			inst:     DUP,
			exp:      []byte{0x4a, 0x4, 0x1e, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementH,
			srcIndex: 7,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.16b, v2.b[0xf]",
			inst:     DUP,
			exp:      []byte{0x4a, 0x4, 0x1f, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:      VectorArrangementB,
			srcIndex: 15,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.16b, v2.16b, v10.16b",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa4, 0x2a, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.8h, v2.8h, v10.8h",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa4, 0x6a, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.4s, v2.8h, v10.4s",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa4, 0xaa, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV11,
			x2:   RegV11,
			name: "addp d11, v11.2d",
			inst: ADDP,
			exp:  []byte{0x6b, 0xb9, 0xf1, 0x5e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangementD,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.16b, v2.16b, v10.16b",
			inst: ADDP,
			exp:  []byte{0x4a, 0xbc, 0x2a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.8h, v2.8h, v10.8h",
			inst: ADDP,
			exp:  []byte{0x4a, 0xbc, 0x6a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.4s, v2.8h, v10.4s",
			inst: ADDP,
			exp:  []byte{0x4a, 0xbc, 0xaa, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv b10, v2.16b",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0x31, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv h10, v2.8h",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0x71, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv s10, v2.4s",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0xb1, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "cmeq v10.2d, v2.2d, v10.2d",
			inst: CMEQ,
			exp:  []byte{0x4a, 0x8c, 0xea, 0x6e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			x1:   RegRZR,
			x2:   RegV30,
			name: "cmeq v30.2d, v30.2d, #0",
			inst: CMEQ,
			exp:  []byte{0xde, 0x9b, 0xe0, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "tbl v1.8b, {v0.16b}, v1.8b",
			x1:   RegV0,
			x2:   RegV1,
			inst: TBL1,
			arr:  VectorArrangement8B,
			exp:  []byte{0x1, 0x0, 0x1, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "tbl v1.16b, {v0.16b}, v1.16b",
			x1:   RegV0,
			x2:   RegV1,
			inst: TBL1,
			arr:  VectorArrangement16B,
			exp:  []byte{0x1, 0x0, 0x1, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "tbl v30.8b, {v0.16b, v1.16b}, v30.8b",
			x1:   RegV0,
			x2:   RegV30,
			inst: TBL2,
			arr:  VectorArrangement8B,
			exp:  []byte{0x1e, 0x20, 0x1e, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "tbl v1.16b, {v31.16b, v0.16b}, v1.16b",
			x1:   RegV31,
			x2:   RegV1,
			inst: TBL2,
			arr:  VectorArrangement16B,
			exp:  []byte{0xe1, 0x23, 0x1, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToVectorRegister(&NodeImpl{
				Instruction:       tc.inst,
				SrcReg:            tc.x1,
				SrcConst:          tc.c,
				DstReg:            tc.x2,
				VectorArrangement: tc.arr,
				SrcVectorIndex:    tc.srcIndex,
				DstVectorIndex:    tc.dstIndex,
			})
			require.NoError(t, err)
			actual, err := a.Assemble()
			require.NoError(t, err)

			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeVectorRegisterToRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		// These are not supported in golang-asm, so test it here instead of integration tests.
		{
			name: "smov w10, v0.b[0xf]",
			n: &NodeImpl{
				Instruction:       SMOV,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementB,
				SrcVectorIndex:    15,
			},
			exp: []byte{0xa, 0x2c, 0x1f, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "smov w10, v0.b[0]",
			n: &NodeImpl{
				Instruction:       SMOV,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementB,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xa, 0x2c, 0x1, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "smov w1, v30.h[7]",
			n: &NodeImpl{
				Instruction:       SMOV,
				SrcReg:            RegV30,
				DstReg:            RegR1,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    7,
			},
			exp: []byte{0xc1, 0x2f, 0x1e, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "smov w1, v30.h[0]",
			n: &NodeImpl{
				Instruction:       SMOV,
				SrcReg:            RegV30,
				DstReg:            RegR1,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xc1, 0x2f, 0x2, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToRegister(tc.n)
			require.NoError(t, err)
			actual, err := a.Assemble()
			require.NoError(t, err)

			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeRegisterToVectorRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		// These are not supported in golang-asm, so test it here instead of integration tests.
		{
			name: "dup v10.2d, x10",
			n: &NodeImpl{
				Instruction:       DUP,
				SrcReg:            RegR10,
				DstReg:            RegV10,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0x4a, 0xd, 0x8, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "dup v1.4s, w30",
			n: &NodeImpl{
				Instruction:       DUP,
				SrcReg:            RegR30,
				DstReg:            RegV1,
				VectorArrangement: VectorArrangementS,
			},
			exp: []byte{0xc1, 0xf, 0x4, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "dup v30.8h, w1",
			n: &NodeImpl{
				Instruction:       DUP,
				SrcReg:            RegR1,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangementH,
			},
			exp: []byte{0x3e, 0xc, 0x2, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			name: "dup v30.16b, w1",
			n: &NodeImpl{
				Instruction:       DUP,
				SrcReg:            RegR1,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangementB,
			},
			exp: []byte{0x3e, 0xc, 0x1, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToVectorRegister(tc.n)
			require.NoError(t, err)
			actual, err := a.Assemble()
			require.NoError(t, err)

			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_maybeFlushConstPool(t *testing.T) {
	tests := []struct {
		name string
		c    asm.StaticConst
		exp  []byte
	}{
		{
			name: "1 byte consts",
			c:    []byte{1},
			exp: []byte{
				// 0x0:
				// b #0x8
				0x2, 0x0, 0x0, 0x14,
				// 0x4:
				0x1,
				0x0, 0x0, 0x0, // padding to be 4-byte aligned.
				// 0x8: <- branch dst.
			},
		},
		{
			name: "2 byte consts",
			c:    []byte{0xff, 0xfe},
			exp: []byte{
				// 0x0:
				// b #0x8
				0x2, 0x0, 0x0, 0x14,
				// 0x4:
				0xff, 0xfe,
				0x0, 0x0, // padding to be 4-byte aligned.
				// 0x8: <- branch dst.
			},
		},
		{
			name: "3 byte consts",
			c:    []byte{0xff, 0xfe, 0xa},
			exp: []byte{
				// 0x0:
				// b #0x8
				0x2, 0x0, 0x0, 0x14,
				// 0x4:
				0xff, 0xfe, 0xa,
				0x0, // padding to be 4-byte aligned.
				// 0x8: <- branch dst.
			},
		},
		{
			name: "4 byte consts",
			c:    []byte{1, 2, 3, 4},
			exp: []byte{
				// 0x0:
				// b #0x8
				0x2, 0x0, 0x0, 0x14,
				// 0x4:
				0x1, 0x2, 0x3, 0x4,
				// 0x8: <- branch dst.
			},
		},
		{
			name: "12 byte consts",
			c:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			exp: []byte{
				// 0x0:
				// b #0x10
				0x4, 0x0, 0x0, 0x14,
				// 0x4:
				1, 2, 3, 4,
				5, 6, 7, 8,
				9, 10, 11, 12,
				// 0x10: <- branch dst.
			},
		},
		{
			name: "16 byte consts",
			c:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			exp: []byte{
				// 0x0:
				// b #0x14
				0x5, 0x0, 0x0, 0x14,
				// 0x04:
				0x1, 0x2, 0x3, 0x4,
				0x5, 0x6, 0x7, 0x8,
				0x9, 0xa, 0xb, 0xc,
				0xd, 0xe, 0xf, 0x10,
				// 0x14: <- branch dst.
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			a.addConstPool(tc.c, 0)

			var called bool
			a.setConstPoolCallback(tc.c, func(int) {
				called = true
			})

			a.MaxDisplacementForConstantPool = 0
			a.maybeFlushConstPool(false)
			require.True(t, called)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestAssemblerImpl_EncodeStaticConstToVectorRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "ldr q8, #8",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV8,
				VectorArrangement: VectorArrangementQ,
				staticConst:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			},
			exp: []byte{
				// 0x0: ldr q8, #8
				0x48, 0x0, 0x0, 0x9c,
				// Emitted after the end of function.
				// 0x4: br #4  (See AssemblerImpl.maybeFlushConstPool)
				0x0, 0x0, 0x0, 0x14,
				// 0x8: consts.
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf,
				0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			name: "ldr d30, #8",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangementD,
				staticConst:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
			},
			exp: []byte{
				// 0x0: ldr d30, #8
				0x5e, 0x0, 0x0, 0x5c,
				// Emitted after the end of function.
				// 0x4: br #4  (See AssemblerImpl.maybeFlushConstPool)
				0x0, 0x0, 0x0, 0x14,
				// 0x8: consts.
				0x1, 0x2, 0x3, 0x4,
				0x5, 0x6, 0x7, 0x8,
			},
		},
		{
			name: "ldr s8, #8",
			n: &NodeImpl{
				Instruction:       VMOV,
				DstReg:            RegV8,
				VectorArrangement: VectorArrangementS,
				staticConst:       []byte{1, 2, 3, 4},
			},
			exp: []byte{
				// 0x0: ldr s8, #8
				0x48, 0x0, 0x0, 0x1c,
				// Emitted after the end of function.
				// 0x4: br #4  (See AssemblerImpl.maybeFlushConstPool)
				0x0, 0x0, 0x0, 0x14,
				// 0x8: consts.
				0x1, 0x2, 0x3, 0x4,
				0x0, 0x0, 0x0, 0x0,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeStaticConstToVectorRegister(tc.n)
			require.NoError(t, err)
			a.maybeFlushConstPool(true)

			actual, err := a.Assemble()
			require.NoError(t, err)

			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}
