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
			in: &NodeImpl{Instruction: VLD1, Types: OperandTypesMemoryToVectorRegister,
				SrcReg: RegR1, DstReg: RegV29, VectorArrangement: VectorArrangement2S},
			exp: "VLD1 [R1], V29.2S",
		},
		{
			in: &NodeImpl{Instruction: VST1, Types: OperandTypesVectorRegisterToMemory,
				DstReg: RegR1, SrcReg: RegV29, VectorArrangement: VectorArrangement2S},
			exp: "VST1 V29.2S, [R1]",
		},
		{
			in: &NodeImpl{Instruction: VMOV, Types: OperandTypesRegisterToVectorRegister,
				SrcReg: RegR1, DstReg: RegV29, VectorArrangement: VectorArrangement2D, VectorIndex: 1},
			exp: "VMOV R1, V29.2D[1]",
		},
		{
			in: &NodeImpl{Instruction: VCNT, Types: OperandTypesVectorRegisterToVectorRegister,
				SrcReg: RegV3, DstReg: RegV29, VectorArrangement: VectorArrangement2D, VectorIndex: 1},
			exp: "VCNT V3.V3, V29.V3",
		},
	}

	for _, tt := range tests {
		tc := tt
		require.Equal(t, tc.exp, tc.in.String())
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
	a.CompileMemoryToVectorRegister(VMOV, RegR10, RegV3, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegR10, actualNode.SrcReg)
	require.Equal(t, RegV3, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
}

func Test_CompileVectorRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileVectorRegisterToMemory(VMOV, RegV3, RegR10, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegR10, actualNode.DstReg)
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
	require.Equal(t, VectorIndex(10), actualNode.VectorIndex)
}

func Test_CompileVectorRegisterToVectorRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileVectorRegisterToVectorRegister(VMOV, RegV3, RegV10, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegV10, actualNode.DstReg)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeVectorRegister, actualNode.Types.dst)
	require.Equal(t, VectorArrangement1D, actualNode.VectorArrangement)
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

func TestAssemblerImpl_EncodeVectorRegisterToVectorRegister(t *testing.T) {
	x1, x2 := RegV2, RegV10
	tests := []struct {
		inst asm.Instruction
		exp  []byte
	}{
		// These are not supported in golang-asm, so test it here instead of integration tests.
		{inst: VFADDD, exp: []byte{
			0x4a, 0xd4, 0x6a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		}},
		{inst: VFADDS, exp: []byte{
			0x4a, 0xd4, 0x2a, 0x4e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.inst), func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToVectorRegister(&NodeImpl{
				Instruction: tc.inst,
				SrcReg:      x1,
				DstReg:      x2,
			})
			require.NoError(t, err)
			actual, err := a.Assemble()
			require.NoError(t, err)

			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}
