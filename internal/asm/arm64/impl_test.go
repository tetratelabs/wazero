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
			in:  &NodeImpl{Instruction: BCONDEQ, Types: OperandTypesNoneToRegister, DstReg: RegR1},
			exp: "BCONDEQ R1",
		},
		{
			in:  &NodeImpl{Instruction: BCONDNE, Types: OperandTypesNoneToBranch, JumpTarget: &NodeImpl{Instruction: NOP}},
			exp: "BCONDNE {NOP}",
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
	a.CompileJumpToRegister(BCONDNE, RegR27)
	actualNode := a.Current
	require.Equal(t, BCONDNE, actualNode.Instruction)
	require.Equal(t, RegR27, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
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

func Test_CompileTwoVectorRegistersToVectorRegister(t *testing.T) {
	a := NewAssemblerImpl(RegR10)
	a.CompileTwoVectorRegistersToVectorRegister(VMOV, RegV3, RegV15, RegV10, VectorArrangement1D)
	actualNode := a.Current
	require.Equal(t, VMOV, actualNode.Instruction)
	require.Equal(t, RegV3, actualNode.SrcReg)
	require.Equal(t, RegV15, actualNode.SrcReg2)
	require.Equal(t, RegV10, actualNode.DstReg)
	require.Equal(t, OperandTypeTwoVectorRegisters, actualNode.Types.src)
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

func TestAssemblerImpl_encodeNoneToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		a := NewAssemblerImpl(asm.NilRegister)
		err := a.encodeNoneToNone(&NodeImpl{Instruction: ADD})
		require.EqualError(t, err, "ADD is unsupported for from:none,to:none type")
	})
	t.Run("ok", func(t *testing.T) {
		a := NewAssemblerImpl(asm.NilRegister)
		err := a.encodeNoneToNone(&NodeImpl{Instruction: NOP})
		require.NoError(t, err)

		// NOP must be ignored.
		actual := a.Buf.Bytes()
		require.Zero(t, len(actual))
	})
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
		{
			inst: XTN,
			name: "xtn v10.2s, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0x28, 0xa1, 0xe},
		},
		{
			inst: XTN,
			name: "xtn v10.4h, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0x28, 0x61, 0xe},
		},
		{
			inst: XTN,
			name: "xtn v10.8b, v2.8h",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement8H,
			exp:  []byte{0x4a, 0x28, 0x21, 0xe},
		},
		{
			inst: REV64,
			name: "rev64 v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x8, 0x20, 0x4e},
		},
		{
			inst: REV64,
			name: "rev64 v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0x8, 0xa0, 0x4e},
		},
		{
			inst: VCNT,
			name: "cnt v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x58, 0x20, 0x4e},
		},
		{
			inst: VCNT,
			name: "cnt v10.8b, v2.8b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement8B,
			exp:  []byte{0x4a, 0x58, 0x20, 0xe},
		},
		{
			inst: VNEG,
			name: "neg v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0xb8, 0x20, 0x6e},
		},
		{
			inst: VNEG,
			name: "neg v10.8h, v2.18h",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement8H,
			exp:  []byte{0x4a, 0xb8, 0x60, 0x6e},
		},
		{
			inst: VNEG,
			name: "neg v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0xb8, 0xa0, 0x6e},
		},
		{
			inst: VNEG,
			name: "neg v10.2d, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0xb8, 0xe0, 0x6e},
		},
		{
			inst: VABS,
			name: "abs v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0xb8, 0x20, 0x4e},
		},
		{
			inst: VABS,
			name: "abs v10.8h, v2.18h",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement8H,
			exp:  []byte{0x4a, 0xb8, 0x60, 0x4e},
		},
		{
			inst: VABS,
			name: "abs v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0xb8, 0xa0, 0x4e},
		},
		{
			inst: VABS,
			name: "abs v10.2d, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0xb8, 0xe0, 0x4e},
		},
		{
			inst: ZIP1,
			name: "zip1 v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x39, 0x2, 0x4e},
		},
		{
			inst: ADDV,
			name: "addv b10, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0xb8, 0x31, 0x4e},
		},
		{
			inst: VORR,
			name: "orr v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0xa2, 0x4e},
		},
		{
			inst: VORR,
			name: "orr v10.8b, v10.8b, v2.8b",
			x1:   RegV2,
			x2:   RegV10,
			arr:  VectorArrangement8B,
			exp:  []byte{0x4a, 0x1d, 0xa2, 0xe},
		},
		{
			name: "fadd v10.2d, v10.2d, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFADDD,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0xd5, 0x62, 0x4e},
		},
		{
			name: "fadd v10.4s, v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFADDS,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0xd5, 0x22, 0x4e},
		},
		{
			name: "fsub v10.2d, v10.2d, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFSUBD,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0xd5, 0xe2, 0x4e},
		},
		{
			name: "fsub v10.4s, v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFSUBS,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0xd5, 0xa2, 0x4e},
		},
		{
			name: "ushll v10.8h, v2.8b, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: USHLL,
			exp:  []byte{0x4a, 0xa4, 0x8, 0x2f},
			arr:  VectorArrangement8B,
		},
		{
			name: "ushll v10.8h, v2.8b, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: USHLL,
			exp:  []byte{0x4a, 0xa4, 0xf, 0x2f},
			arr:  VectorArrangement8B,
			c:    7,
		},
		{
			name: "10.8h, v2.8b, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x8, 0x4f},
			arr:  VectorArrangement16B,
			c:    8,
		},
		{
			name: "sshr v10.16b, v2.16b, #3",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0xd, 0x4f},
			arr:  VectorArrangement16B,
			c:    3,
		},
		{
			name: "sshr v10.16b, v2.16b, #1",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0xf, 0x4f},
			arr:  VectorArrangement16B,
			c:    1,
		},
		{
			name: "sshr v10.8b, v2.8b, #3",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0xd, 0xf},
			arr:  VectorArrangement8B,
			c:    3,
		},
		{
			name: "sshr v10.8h, v2.8h, #0x10",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x10, 0x4f},
			arr:  VectorArrangement8H,
			c:    16,
		},
		{
			name: "sshr v10.8h, v2.8h, #0xf",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x11, 0x4f},
			arr:  VectorArrangement8H,
			c:    15,
		},
		{
			name: "sshr v10.8h, v2.8h, #3",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x1d, 0x4f},
			arr:  VectorArrangement8H,
			c:    3,
		},
		{
			name: "sshr v10.4h, v2.4h, #0xf",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x11, 0xf},
			arr:  VectorArrangement4H,
			c:    15,
		},
		{
			name: "sshr v10.2s, v2.2s, #0x20",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x20, 0xf},
			arr:  VectorArrangement2S,
			c:    32,
		},
		{
			name: "sshr v10.2s, v2.2s, #0x1f",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x21, 0xf},
			arr:  VectorArrangement2S,
			c:    31,
		},
		{
			name: "sshr v10.2s, v2.2s, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x39, 0xf},
			arr:  VectorArrangement2S,
			c:    7,
		},
		{
			name: "sshr v10.4s, v2.4s, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x39, 0x4f},
			arr:  VectorArrangement4S,
			c:    7,
		},
		{
			name: "sshr v10.2d, v2.2d, #0x3f",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x41, 0x4f},
			arr:  VectorArrangement2D,
			c:    63,
		},
		{
			name: "sshr v10.2d, v2.2d, #0x21",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x5f, 0x4f},
			arr:  VectorArrangement2D,
			c:    33,
		},
		{
			name: "sshr v10.2d, v2.2d, #1",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHR,
			exp:  []byte{0x4a, 0x4, 0x7f, 0x4f},
			arr:  VectorArrangement2D,
			c:    1,
		},
		{
			name: "sshll v10.8h, v2.8b, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL, exp: []byte{0x4a, 0xa4, 0xf, 0xf},
			arr: VectorArrangement8B,
			c:   7,
		},
		{
			name: "sshll v10.4s, v2.4h, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp:  []byte{0x4a, 0xa4, 0x10, 0xf},
			arr:  VectorArrangement4H,
		},
		{
			name: "sshll v10.4s, v2.4h, #0xf",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp:  []byte{0x4a, 0xa4, 0x1f, 0xf},
			arr:  VectorArrangement4H,
			c:    15,
		},
		{
			name: "sshll v10.2d, v2.2s, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp:  []byte{0x4a, 0xa4, 0x20, 0xf},
			arr:  VectorArrangement2S,
		},
		{
			name: "sshll v10.2d, v2.2s, #0x1f",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL,
			exp:  []byte{0x4a, 0xa4, 0x3f, 0xf},
			arr:  VectorArrangement2S,
			c:    31,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.s[2], v2.s[1]",
			inst:     INSELEM,
			exp:      []byte{0x4a, 0x24, 0x14, 0x6e},
			arr:      VectorArrangementS,
			srcIndex: 1,
			dstIndex: 2,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.s[0], v2.s[3]",
			inst:     INSELEM,
			exp:      []byte{0x4a, 0x64, 0x4, 0x6e},
			arr:      VectorArrangementS,
			srcIndex: 3,
			dstIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.b[0], v2.b[0xf]",
			inst:     INSELEM,
			exp:      []byte{0x4a, 0x7c, 0x1, 0x6e},
			arr:      VectorArrangementB,
			srcIndex: 15,
			dstIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "ins v10.d[1], v2.d[0]",
			inst:     INSELEM,
			exp:      []byte{0x4a, 0x4, 0x18, 0x6e},
			arr:      VectorArrangementD,
			srcIndex: 0,
			dstIndex: 1,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.2d, v2.d[0]",
			inst:     DUPELEM,
			exp:      []byte{0x4a, 0x4, 0x8, 0x4e},
			arr:      VectorArrangementD,
			srcIndex: 0,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.2d, v2.d[1]",
			inst:     DUPELEM,
			exp:      []byte{0x4a, 0x4, 0x18, 0x4e},
			arr:      VectorArrangementD,
			srcIndex: 1,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.4s, v2.s[3]",
			inst:     DUPELEM,
			exp:      []byte{0x4a, 0x4, 0x1c, 0x4e},
			arr:      VectorArrangementS,
			srcIndex: 3,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.8h, v2.h[7]",
			inst:     DUPELEM,
			exp:      []byte{0x4a, 0x4, 0x1e, 0x4e},
			arr:      VectorArrangementH,
			srcIndex: 7,
		},
		{
			x1:       RegV2,
			x2:       RegV10,
			name:     "dup v10.16b, v2.b[0xf]",
			inst:     DUPELEM,
			exp:      []byte{0x4a, 0x4, 0x1f, 0x4e},
			arr:      VectorArrangementB,
			srcIndex: 15,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.16b, v10.16b, v2.16b",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa5, 0x22, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.8h, v10.8h, v2.8h",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa5, 0x62, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "umaxp v10.4s, v10.4s, v2.4s",
			inst: UMAXP,
			exp:  []byte{0x4a, 0xa5, 0xa2, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV11,
			x2:   RegV11,
			name: "addp d11, v11.2d",
			inst: ADDP,
			arr:  VectorArrangement2D,
			exp:  []byte{0x6b, 0xb9, 0xf1, 0x5e},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.16b, v10.16b, v2.16b",
			inst: VADDP,
			exp:  []byte{0x4a, 0xbd, 0x22, 0x4e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.8h, v10.8h, v2.8h",
			inst: VADDP,
			exp:  []byte{0x4a, 0xbd, 0x62, 0x4e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "addp v10.4s, v10.4s, v2.4s",
			inst: VADDP,
			exp:  []byte{0x4a, 0xbd, 0xa2, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv b10, v2.16b",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0x31, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv h10, v2.8h",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0x71, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "uminv s10, v2.4s",
			inst: UMINV,
			exp:  []byte{0x4a, 0xa8, 0xb1, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "cmeq v10.2d, v10.2d, v2.2d",
			arr:  VectorArrangement2D,
			inst: CMEQ,
			exp:  []byte{0x4a, 0x8d, 0xe2, 0x6e},
		},
		{
			x1:   RegRZR,
			x2:   RegV30,
			name: "cmeq v30.2d, v0.2d, #0",
			inst: CMEQZERO,
			arr:  VectorArrangement2D,
			exp:  []byte{0x1e, 0x98, 0xe0, 0x4e},
		},
		{
			name: "tbl v1.8b, {v0.16b}, v1.8b",
			x1:   RegV0,
			x2:   RegV1,
			inst: TBL1,
			arr:  VectorArrangement8B,
			exp:  []byte{0x1, 0x0, 0x1, 0xe},
		},
		{
			name: "tbl v1.16b, {v0.16b}, v1.16b",
			x1:   RegV0,
			x2:   RegV1,
			inst: TBL1,
			arr:  VectorArrangement16B,
			exp:  []byte{0x1, 0x0, 0x1, 0x4e},
		},
		{
			name: "tbl v30.8b, {v0.16b, v1.16b}, v30.8b",
			x1:   RegV0,
			x2:   RegV30,
			inst: TBL2,
			arr:  VectorArrangement8B,
			exp:  []byte{0x1e, 0x20, 0x1e, 0xe},
		},
		{
			name: "tbl v1.16b, {v31.16b, v0.16b}, v1.16b",
			x1:   RegV31,
			x2:   RegV1,
			inst: TBL2,
			arr:  VectorArrangement16B,
			exp:  []byte{0xe1, 0x23, 0x1, 0x4e},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "add v10.4s, v10.4s, v2.4s",
			inst: VADD,
			exp:  []byte{0x4a, 0x85, 0xa2, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "add v10.2d, v10.2d, v2.2d",
			inst: VADD,
			exp:  []byte{0x4a, 0x85, 0xe2, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "sub v10.8h, v10.8h, v2.8h",
			inst: VSUB,
			exp:  []byte{0x4a, 0x85, 0x62, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV29,
			x2:   RegV30,
			name: "sub v30.16b, v30.16b, v29.16b",
			inst: VSUB,
			exp:  []byte{0xde, 0x87, 0x3d, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			name: "bic v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: BIC,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0x62, 0x4e},
		},
		{
			name: "eor v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: EOR,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0x22, 0x6e},
		},
		{
			name: "bsl v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: BSL,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0x62, 0x6e},
		},
		{
			name: "bsl v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: BSL,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0x62, 0x6e},
		},
		{
			name: "and v10.16b, v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: VAND,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x1d, 0x22, 0x4e},
		},
		{
			// mvn is an alias of NOT: https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/MVN--Bitwise-NOT--vector---an-alias-of-NOT-?lang=en
			name: "mvn v10.16b, v2.16b",
			x1:   RegV2,
			x2:   RegV10,
			inst: NOT,
			arr:  VectorArrangement16B,
			exp:  []byte{0x4a, 0x58, 0x20, 0x6e},
		},
		{
			name: "fneg v10.2d, v2.2d",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFNEG,
			arr:  VectorArrangement2D,
			exp:  []byte{0x4a, 0xf8, 0xe0, 0x6e},
		},
		{
			name: "fneg v10.4s, v2.4s",
			x1:   RegV2,
			x2:   RegV10,
			inst: VFNEG,
			arr:  VectorArrangement4S,
			exp:  []byte{0x4a, 0xf8, 0xa0, 0x6e},
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "sshl v10.2d, v10.2d, v2.2d",
			inst: SSHL,
			exp:  []byte{0x4a, 0x45, 0xe2, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sshl v30.4s, v30.4s, v25.4s",
			inst: SSHL,
			exp:  []byte{0xde, 0x47, 0xb9, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV2,
			x2:   RegV10,
			name: "ushl v10.8h, v10.8h, v2.8h",
			inst: USHL,
			exp:  []byte{0x4a, 0x45, 0x62, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "ushl v30.16b, v30.16b, v25.16b",
			inst: USHL,
			exp:  []byte{0xde, 0x47, 0x39, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fabs v30.4s, v25.4s",
			inst: VFABS,
			exp:  []byte{0x3e, 0xfb, 0xa0, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fabs v30.2s, v25.2s",
			inst: VFABS,
			exp:  []byte{0x3e, 0xfb, 0xa0, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fabs v30.2d, v25.2d",
			inst: VFABS,
			exp:  []byte{0x3e, 0xfb, 0xe0, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fsqrt v30.4s, v25.4s",
			inst: VFSQRT,
			exp:  []byte{0x3e, 0xfb, 0xa1, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fsqrt v30.2s, v25.2s",
			inst: VFSQRT,
			exp:  []byte{0x3e, 0xfb, 0xa1, 0x2e},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fsqrt v30.2d, v25.2d",
			inst: VFSQRT,
			exp:  []byte{0x3e, 0xfb, 0xe1, 0x6e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintm v30.4s, v25.4s",
			inst: VFRINTM,
			exp:  []byte{0x3e, 0x9b, 0x21, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintm v30.2s, v25.2s",
			inst: VFRINTM,
			exp:  []byte{0x3e, 0x9b, 0x21, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintm v30.2d, v25.2d",
			inst: VFRINTM,
			exp:  []byte{0x3e, 0x9b, 0x61, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintn v30.4s, v25.4s",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x21, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintn v30.2s, v25.2s",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x21, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintn v30.2d, v25.2d",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x61, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.4s, v25.4s",
			inst: VFRINTP,
			exp:  []byte{0x3e, 0x8b, 0xa1, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.2s, v25.2s",
			inst: VFRINTP,
			exp:  []byte{0x3e, 0x8b, 0xa1, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.2d, v25.2d",
			inst: VFRINTP,
			exp:  []byte{0x3e, 0x8b, 0xe1, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.4s, v25.4s",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x21, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.2s, v25.2s",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x21, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "frintp v30.2d, v25.2d",
			inst: VFRINTN,
			exp:  []byte{0x3e, 0x8b, 0x61, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "shll v30.8h, v25.8b, #8",
			inst: SHLL,
			exp:  []byte{0x3e, 0x3b, 0x21, 0x2e},
			arr:  VectorArrangement8B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "shll v30.4s, v25.4h, #16",
			inst: SHLL,
			exp:  []byte{0x3e, 0x3b, 0x61, 0x2e},
			arr:  VectorArrangement4H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "shll v30.2d, v25.2s, #32",
			inst: SHLL,
			exp:  []byte{0x3e, 0x3b, 0xa1, 0x2e},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uaddlv h30, v25.16b",
			inst: UADDLV,
			exp:  []byte{0x3e, 0x3b, 0x30, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uaddlv s30, v25.8h",
			inst: UADDLV,
			exp:  []byte{0x3e, 0x3b, 0x70, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uaddlv d30, v25.4s",
			inst: UADDLV,
			exp:  []byte{0x3e, 0x3b, 0xb0, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "saddlp v30.2d, v25.4s",
			inst: SADDLP,
			exp:  []byte{0x3e, 0x2b, 0xa0, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "saddlp v30.4s, v25.8h",
			inst: SADDLP,
			exp:  []byte{0x3e, 0x2b, 0x60, 0x4e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uaddlp v30.2d, v25.4s",
			inst: UADDLP,
			exp:  []byte{0x3e, 0x2b, 0xa0, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uaddlp v30.4s, v25.8h",
			inst: UADDLP,
			exp:  []byte{0x3e, 0x2b, 0x60, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			name: "sshll2 v10.8h, v2.16b, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL2,
			exp:  []byte{0x4a, 0xa4, 0xf, 0x4f},
			arr:  VectorArrangement16B,
			c:    7,
		},
		{
			name: "sshll2 v10.4s, v2.8h, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL2,
			exp:  []byte{0x4a, 0xa4, 0x10, 0x4f},
			arr:  VectorArrangement8H,
		},
		{
			name: "sshll2 v10.2d, v2.4s, #0x15",
			x1:   RegV2,
			x2:   RegV10,
			inst: SSHLL2,
			exp:  []byte{0x4a, 0xa4, 0x35, 0x4f},
			arr:  VectorArrangement4S,
			c:    21,
		},
		{
			name: "ushll2 v10.8h, v2.16b, #7",
			x1:   RegV2,
			x2:   RegV10,
			inst: USHLL2,
			exp:  []byte{0x4a, 0xa4, 0xf, 0x6f},
			arr:  VectorArrangement16B,
			c:    7,
		},
		{
			name: "ushll2 v10.4s, v2.8h, #0",
			x1:   RegV2,
			x2:   RegV10,
			inst: USHLL2,
			exp:  []byte{0x4a, 0xa4, 0x10, 0x6f},
			arr:  VectorArrangement8H,
		},
		{
			name: "ushll2 v10.2d, v2.4s, #0x15",
			x1:   RegV2,
			x2:   RegV10,
			inst: USHLL2,
			exp:  []byte{0x4a, 0xa4, 0x35, 0x6f},
			arr:  VectorArrangement4S,
			c:    21,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzs v30.4s, v25.4s",
			inst: VFCVTZS,
			exp:  []byte{0x3e, 0xbb, 0xa1, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzs v30.2s, v25.2s",
			inst: VFCVTZS,
			exp:  []byte{0x3e, 0xbb, 0xa1, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzs v30.2d, v25.2d",
			inst: VFCVTZS,
			exp:  []byte{0x3e, 0xbb, 0xe1, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzu v30.4s, v25.4s",
			inst: VFCVTZU,
			exp:  []byte{0x3e, 0xbb, 0xa1, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzu v30.2s, v25.2s",
			inst: VFCVTZU,
			exp:  []byte{0x3e, 0xbb, 0xa1, 0x2e},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtzu v30.2d, v25.2d",
			inst: VFCVTZU,
			exp:  []byte{0x3e, 0xbb, 0xe1, 0x6e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtn v30.2s, v25.2d",
			inst: SQXTN,
			exp:  []byte{0x3e, 0x4b, 0xa1, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtn v30.4h, v25.4s",
			inst: SQXTN,
			exp:  []byte{0x3e, 0x4b, 0x61, 0xe},
			arr:  VectorArrangement4H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uqxtn v30.2s, v25.2d",
			inst: UQXTN,
			exp:  []byte{0x3e, 0x4b, 0xa1, 0x2e},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "uqxtn v30.4h, v25.4s",
			inst: UQXTN,
			exp:  []byte{0x3e, 0x4b, 0x61, 0x2e},
			arr:  VectorArrangement4H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtn2 v30.16b, v25.8h",
			inst: SQXTN2,
			exp:  []byte{0x3e, 0x4b, 0x21, 0x4e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtn2 v30.8h, v25.4s",
			inst: SQXTN2,
			exp:  []byte{0x3e, 0x4b, 0x61, 0x4e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtn2 v30.4s, v25.2d",
			inst: SQXTN2,
			exp:  []byte{0x3e, 0x4b, 0xa1, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun v30.8b, v25.8h",
			inst: SQXTUN,
			exp:  []byte{0x3e, 0x2b, 0x21, 0x2e},
			arr:  VectorArrangement8B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun v30.4h, v25.4s",
			inst: SQXTUN,
			exp:  []byte{0x3e, 0x2b, 0x61, 0x2e},
			arr:  VectorArrangement4H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun v30.2s, v25.2d",
			inst: SQXTUN,
			exp:  []byte{0x3e, 0x2b, 0xa1, 0x2e},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun2 v30.16b, v25.8h",
			inst: SQXTUN2,
			exp:  []byte{0x3e, 0x2b, 0x21, 0x6e},
			arr:  VectorArrangement16B,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun2 v30.8h, v25.4s",
			inst: SQXTUN2,
			exp:  []byte{0x3e, 0x2b, 0x61, 0x6e},
			arr:  VectorArrangement8H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "sqxtun2 v30.4s, v25.2d",
			inst: SQXTUN2,
			exp:  []byte{0x3e, 0x2b, 0xa1, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "scvtf v30.2d, v25.2d",
			inst: VSCVTF,
			exp:  []byte{0x3e, 0xdb, 0x61, 0x4e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "scvtf v30.4s, v25.4s",
			inst: VSCVTF,
			exp:  []byte{0x3e, 0xdb, 0x21, 0x4e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "ucvtf v30.2d, v25.2d",
			inst: VUCVTF,
			exp:  []byte{0x3e, 0xdb, 0x61, 0x6e},
			arr:  VectorArrangement2D,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "ucvtf v30.4s, v25.4s",
			inst: VUCVTF,
			exp:  []byte{0x3e, 0xdb, 0x21, 0x6e},
			arr:  VectorArrangement4S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtl v30.2d, v25.2s",
			inst: FCVTL,
			exp:  []byte{0x3e, 0x7b, 0x61, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtl v30.4s, v25.4h",
			inst: FCVTL,
			exp:  []byte{0x3e, 0x7b, 0x21, 0xe},
			arr:  VectorArrangement4H,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtn v30.2s, v25.2d",
			inst: FCVTN,
			exp:  []byte{0x3e, 0x6b, 0x61, 0xe},
			arr:  VectorArrangement2S,
		},
		{
			x1:   RegV25,
			x2:   RegV30,
			name: "fcvtn v30.4h, v25.4s",
			inst: FCVTN,
			exp:  []byte{0x3e, 0x6b, 0x21, 0xe},
			arr:  VectorArrangement4H,
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
			actual := a.Buf.Bytes()
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
			name: "umov w10, v0.b[0xf]",
			n: &NodeImpl{
				Instruction:       UMOV,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementB,
				SrcVectorIndex:    15,
			},
			exp: []byte{0xa, 0x3c, 0x1f, 0xe},
		},
		{
			name: "mov w10, v0.s[3]",
			n: &NodeImpl{
				Instruction:       UMOV,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementS,
				SrcVectorIndex:    3,
			},
			exp: []byte{0xa, 0x3c, 0x1c, 0xe},
		},
		{
			name: "mov x5, v30.d[1]",
			n: &NodeImpl{
				Instruction:       UMOV,
				SrcReg:            RegV30,
				DstReg:            RegR5,
				VectorArrangement: VectorArrangementD,
				SrcVectorIndex:    1,
			},
			exp: []byte{0xc5, 0x3f, 0x18, 0x4e},
		},
		{
			name: "smov w10, v0.b[0xf]",
			n: &NodeImpl{
				Instruction:       SMOV32,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementB,
				SrcVectorIndex:    15,
			},
			exp: []byte{0xa, 0x2c, 0x1f, 0xe},
		},
		{
			name: "smov w10, v0.b[0]",
			n: &NodeImpl{
				Instruction:       SMOV32,
				SrcReg:            RegV0,
				DstReg:            RegR10,
				VectorArrangement: VectorArrangementB,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xa, 0x2c, 0x1, 0xe},
		},
		{
			name: "smov w1, v30.h[7]",
			n: &NodeImpl{
				Instruction:       SMOV32,
				SrcReg:            RegV30,
				DstReg:            RegR1,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    7,
			},
			exp: []byte{0xc1, 0x2f, 0x1e, 0xe},
		},
		{
			name: "smov w1, v30.h[0]",
			n: &NodeImpl{
				Instruction:       SMOV32,
				SrcReg:            RegV30,
				DstReg:            RegR1,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xc1, 0x2f, 0x2, 0xe},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeVectorRegisterToRegister(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeLeftShiftedRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *NodeImpl
			expErr string
		}{
			{
				n: &NodeImpl{Instruction: SUB, Types: OperandTypesLeftShiftedRegisterToRegister,
					SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "SUB is unsupported for from:left-shifted-register,to:register type",
			},
			{
				n: &NodeImpl{Instruction: ADD,
					SrcConst: -1, SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "shift amount must fit in unsigned 6-bit integer (0-64) but got -1",
			},
			{
				n: &NodeImpl{Instruction: ADD,
					SrcConst: -1, SrcReg: RegV0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "V0 is not integer",
			},
			{
				n: &NodeImpl{Instruction: ADD,
					SrcConst: -1, SrcReg: RegR0, SrcReg2: RegV0, DstReg: RegR0},
				expErr: "V0 is not integer",
			},
			{
				n: &NodeImpl{Instruction: ADD,
					SrcConst: -1, SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegV0},
				expErr: "V0 is not integer",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeLeftShiftedRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegR0,
				SrcReg2:           RegR29,
				SrcConst:          1,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x15, 0x4, 0x1d, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegR0,
				SrcReg2:           RegR29,
				SrcConst:          2,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x15, 0x8, 0x1d, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegR0,
				SrcReg2:           RegR29,
				SrcConst:          8,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x15, 0x20, 0x1d, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegR29,
				SrcReg2:           RegR0,
				SrcConst:          16,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xb5, 0x43, 0x0, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegR29,
				SrcReg2:           RegR0,
				SrcConst:          64,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xb5, 0x3, 0x0, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegRZR,
				SrcReg2:           RegR0,
				SrcConst:          64,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xf5, 0x3, 0x0, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegRZR,
				SrcReg2:           RegRZR,
				SrcConst:          64,
				DstReg:            RegR21,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xf5, 0x3, 0x1f, 0x8b},
		},
		{
			name: "ADD",
			n: &NodeImpl{
				Instruction:       ADD,
				SrcReg:            RegRZR,
				SrcReg2:           RegRZR,
				SrcConst:          64,
				DstReg:            RegRZR,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xff, 0x3, 0x1f, 0x8b},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeLeftShiftedRegisterToRegister(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_encodeTwoRegistersToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *NodeImpl
			expErr string
		}{
			{
				n: &NodeImpl{Instruction: SUB, Types: OperandTypesTwoRegistersToNone,
					SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "SUB is unsupported for from:two-registers,to:none type",
			},
			{
				n: &NodeImpl{Instruction: CMP,
					SrcReg: RegR0, SrcReg2: RegV0},
				expErr: "V0 is not integer",
			},
			{
				n: &NodeImpl{Instruction: FCMPS,
					SrcReg: RegR0, SrcReg2: RegV0},
				expErr: "R0 is not vector",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeTwoRegistersToNone(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "CMP",
			n: &NodeImpl{
				Instruction:       CMP,
				SrcReg:            RegRZR,
				SrcReg2:           RegRZR,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xff, 0x3, 0x1f, 0xeb},
		},
		{
			name: "CMP",
			n: &NodeImpl{
				Instruction:       CMP,
				SrcReg:            RegRZR,
				SrcReg2:           RegR30,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xdf, 0x3, 0x1f, 0xeb},
		},
		{
			name: "CMP",
			n: &NodeImpl{
				Instruction:       CMP,
				SrcReg:            RegR30,
				SrcReg2:           RegRZR,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xff, 0x3, 0x1e, 0xeb},
		},
		{
			name: "CMP",
			n: &NodeImpl{
				Instruction:       CMP,
				SrcReg:            RegR30,
				SrcReg2:           RegR30,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xdf, 0x3, 0x1e, 0xeb},
		},
		{
			name: "CMPW",
			n: &NodeImpl{
				Instruction:       CMPW,
				SrcReg:            RegRZR,
				SrcReg2:           RegRZR,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xff, 0x3, 0x1f, 0x6b},
		},
		{
			name: "CMPW",
			n: &NodeImpl{
				Instruction:       CMPW,
				SrcReg:            RegRZR,
				SrcReg2:           RegR30,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xdf, 0x3, 0x1f, 0x6b},
		},
		{
			name: "CMPW",
			n: &NodeImpl{
				Instruction:       CMPW,
				SrcReg:            RegR30,
				SrcReg2:           RegRZR,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xff, 0x3, 0x1e, 0x6b},
		},
		{
			name: "CMPW",
			n: &NodeImpl{
				Instruction:       CMPW,
				SrcReg:            RegR30,
				SrcReg2:           RegR30,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xdf, 0x3, 0x1e, 0x6b},
		},
		{
			name: "FCMPD",
			n: &NodeImpl{
				Instruction:       FCMPD,
				SrcReg:            RegV0,
				SrcReg2:           RegV0,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x0, 0x20, 0x60, 0x1e},
		},
		{
			name: "FCMPD",
			n: &NodeImpl{
				Instruction:       FCMPD,
				SrcReg:            RegV0,
				SrcReg2:           RegV31,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xe0, 0x23, 0x60, 0x1e},
		},
		{
			name: "FCMPD",
			n: &NodeImpl{
				Instruction:       FCMPD,
				SrcReg:            RegV31,
				SrcReg2:           RegV0,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x0, 0x20, 0x7f, 0x1e},
		},
		{
			name: "FCMPD",
			n: &NodeImpl{
				Instruction:       FCMPD,
				SrcReg:            RegV31,
				SrcReg2:           RegV31,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xe0, 0x23, 0x7f, 0x1e},
		},
		{
			name: "FCMPS",
			n: &NodeImpl{
				Instruction:       FCMPS,
				SrcReg:            RegV0,
				SrcReg2:           RegV0,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x0, 0x20, 0x20, 0x1e},
		},
		{
			name: "FCMPS",
			n: &NodeImpl{
				Instruction:       FCMPS,
				SrcReg:            RegV0,
				SrcReg2:           RegV31,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xe0, 0x23, 0x20, 0x1e},
		},
		{
			name: "FCMPS",
			n: &NodeImpl{
				Instruction:       FCMPS,
				SrcReg:            RegV31,
				SrcReg2:           RegV0,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0x0, 0x20, 0x3f, 0x1e},
		},
		{
			name: "FCMPS",
			n: &NodeImpl{
				Instruction:       FCMPS,
				SrcReg:            RegV31,
				SrcReg2:           RegV31,
				VectorArrangement: VectorArrangementH,
				SrcVectorIndex:    0,
			},
			exp: []byte{0xe0, 0x23, 0x3f, 0x1e},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeTwoRegistersToNone(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeThreeRegistersToRegister(t *testing.T) {
	const src1, src2, src3, dst = RegR1, RegR10, RegR30, RegR11

	tests := []struct {
		name string
		inst asm.Instruction
		exp  []byte
	}{
		{
			name: "MSUB/src1=R1,src2=R10,src3=R30,dst=R11",
			inst: MSUB,
			exp:  []byte{0xcb, 0xab, 0x1, 0x9b},
		},
		{
			name: "MSUBW/src1=R1,src2=R10,src3=R30,dst=R11",
			inst: MSUBW,
			exp:  []byte{0xcb, 0xab, 0x1, 0x1b},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeThreeRegistersToRegister(&NodeImpl{
				Instruction: tc.inst, SrcReg: src1, SrcReg2: src2, DstReg: src3, DstReg2: dst,
			})
			require.NoError(t, err)

			actual := a.Bytes()
			require.Equal(t, tc.exp, actual[:4])
		})
	}
}

func TestAssemblerImpl_encodeTwoVectorRegistersToVectorRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "orr v30.16b, v10.16b, v1.16b",
			n: &NodeImpl{
				Instruction:       VORR,
				DstReg:            RegV30,
				SrcReg:            RegV1,
				SrcReg2:           RegV10,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x5e, 0x1d, 0xa1, 0x4e},
		},
		{
			name: "orr v30.8b, v10.8b, v1.8b",
			n: &NodeImpl{
				Instruction:       VORR,
				DstReg:            RegV30,
				SrcReg:            RegV1,
				SrcReg2:           RegV10,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0x5e, 0x1d, 0xa1, 0xe},
		},
		{
			name: "bsl v0.8b, v15.8b, v1.8b",
			n: &NodeImpl{
				Instruction:       BSL,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0xe0, 0x1d, 0x61, 0x2e},
		},
		{
			name: "zip1 v0.4s, v15.4s, v1.4s",
			n: &NodeImpl{
				Instruction:       ZIP1,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0xe0, 0x39, 0x81, 0x4e},
		},
		{
			name: "zip1 v0.2d, v15.2d, v1.2d",
			n: &NodeImpl{
				Instruction:       ZIP1,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0xe0, 0x39, 0xc1, 0x4e},
		},
		{
			name: "ext v0.16b, v15.16b, v1.16b, #0xf",
			n: &NodeImpl{
				Instruction:       EXT,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				SrcConst:          0xf,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xe0, 0x79, 0x1, 0x6e},
		},
		{
			name: "ext v0.16b, v15.16b, v1.16b, #8",
			n: &NodeImpl{
				Instruction:       EXT,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				SrcConst:          8,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xe0, 0x41, 0x1, 0x6e},
		},
		{
			name: "ext v0.16b, v15.16b, v1.16b, #0",
			n: &NodeImpl{
				Instruction:       EXT,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				SrcConst:          0,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xe0, 0x1, 0x1, 0x6e},
		},
		{
			name: "ext v0.8b, v15.8b, v1.8b, #7",
			n: &NodeImpl{
				Instruction:       EXT,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				SrcConst:          7,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0xe0, 0x39, 0x1, 0x2e},
		},
		{
			name: "cmeq v0.8b, v15.8b, v1.8b",
			n: &NodeImpl{
				Instruction:       CMEQ,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0xe0, 0x8d, 0x21, 0x2e},
		},
		{
			name: "cmgt v0.16b, v15.16b, v1.16b",
			n: &NodeImpl{
				Instruction:       CMGT,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xe0, 0x35, 0x21, 0x4e},
		},
		{
			name: "cmhi v0.8h, v15.8h, v1.8h",
			n: &NodeImpl{
				Instruction:       CMHI,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0xe0, 0x35, 0x61, 0x6e},
		},
		{
			name: "cmhi v0.4h, v15.4h, v1.4h",
			n: &NodeImpl{
				Instruction:       CMHI,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement4H,
			},
			exp: []byte{0xe0, 0x35, 0x61, 0x2e},
		},
		{
			name: "cmge v0.4s, v15.4s, v1.4s",
			n: &NodeImpl{
				Instruction:       CMGE,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0xe0, 0x3d, 0xa1, 0x4e},
		},
		{
			name: "cmge v0.2s, v15.2s, v1.2s",
			n: &NodeImpl{
				Instruction:       CMGE,
				DstReg:            RegV0,
				SrcReg:            RegV1,
				SrcReg2:           RegV15,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0xe0, 0x3d, 0xa1, 0xe},
		},
		{
			name: "cmhs v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       CMHS,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0x3c, 0xeb, 0x6e},
		},
		{
			name: "fcmeq v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       FCMEQ,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xe4, 0x6b, 0x4e},
		},
		{
			name: "fcmeq v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       FCMEQ,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xe4, 0x2b, 0x4e},
		},
		{
			name: "fcmeq v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       FCMEQ,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xe4, 0x2b, 0xe},
		},
		{
			name: "fcmgt v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       FCMGT,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xe4, 0xeb, 0x6e},
		},
		{
			name: "fcmgt v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       FCMGT,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xe4, 0xab, 0x6e},
		},
		{
			name: "fcmgt v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       FCMGT,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xe4, 0xab, 0x2e},
		},
		{
			name: "fcmge v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       FCMGE,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xe4, 0x6b, 0x6e},
		},
		{
			name: "fcmge v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       FCMGE,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xe4, 0x2b, 0x6e},
		},
		{
			name: "fcmge v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       FCMGE,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xe4, 0x2b, 0x2e},
		},
		{
			name: "fdiv v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VFDIV,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xfc, 0x2b, 0x6e},
		},
		{
			name: "fdiv v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       VFDIV,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xfc, 0x2b, 0x2e},
		},
		{
			name: "fdiv v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       VFDIV,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xfc, 0x6b, 0x6e},
		},
		{
			name: "fmul v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VFMUL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xdc, 0x2b, 0x6e},
		},
		{
			name: "fmul v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       VFMUL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xdc, 0x2b, 0x2e},
		},
		{
			name: "fmul v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       VFMUL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xdc, 0x6b, 0x6e},
		},
		{
			name: "fmin v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VFMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xf4, 0xab, 0x4e},
		},
		{
			name: "fmin v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       VFMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xf4, 0xab, 0xe},
		},
		{
			name: "fmin v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       VFMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xf4, 0xeb, 0x4e},
		},
		{
			name: "fmax v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VFMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xf4, 0x2b, 0x4e},
		},
		{
			name: "fmax v30.2s, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       VFMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xf4, 0x2b, 0xe},
		},
		{
			name: "fmax v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       VFMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xf4, 0x6b, 0x4e},
		},
		{
			name: "mul v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VMUL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0x9c, 0xab, 0x4e},
		},
		{
			name: "mul v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       VMUL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x9c, 0x2b, 0x4e},
		},
		{
			name: "sqadd v30.2d, v4.2d, v11.2d",
			n: &NodeImpl{
				Instruction:       VSQADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x9e, 0xc, 0xeb, 0x4e},
		},
		{
			name: "sqadd v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       VSQADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0xc, 0x6b, 0x4e},
		},
		{
			name: "uqadd v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       VUQADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xc, 0xab, 0x6e},
		},
		{
			name: "uqadd v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       VUQADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0xc, 0x6b, 0x6e},
		},
		{
			name: "smax v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       SMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0x64, 0xab, 0x4e},
		},
		{
			name: "smax v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       SMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0x64, 0x6b, 0x4e},
		},
		{
			name: "smin v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       SMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x6c, 0x2b, 0x4e},
		},
		{
			name: "smin v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       SMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0x6c, 0xab, 0x4e},
		},
		{
			name: "umin v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       UMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x6c, 0x2b, 0x6e},
		},
		{
			name: "umin v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       UMIN,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0x6c, 0xab, 0x6e},
		},
		{
			name: "umax v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       UMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0x64, 0xab, 0x6e},
		},
		{
			name: "umax v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       UMAX,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0x64, 0x6b, 0x6e},
		},
		{
			name: "umax v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       URHADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0x14, 0x6b, 0x6e},
		},
		{
			name: "umax v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       URHADD,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x14, 0x2b, 0x6e},
		},
		{
			name: "sqsub v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       VSQSUB,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x2c, 0x2b, 0x4e},
		},
		{
			name: "sqsub v308hb, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       VSQSUB,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0x2c, 0x6b, 0x4e},
		},
		{
			name: "uqsub v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       VUQSUB,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x2c, 0x2b, 0x6e},
		},
		{
			name: "uqsub v308hb, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       VUQSUB,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0x2c, 0x6b, 0x6e},
		},
		{
			name: "umlal v0.2d, v6.2s, v2.2s",
			n: &NodeImpl{
				Instruction:       VUMLAL,
				DstReg:            RegV0,
				SrcReg:            RegV2,
				SrcReg2:           RegV6,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0xc0, 0x80, 0xa2, 0x2e},
		},
		{
			name: "umlal v0.4s, v6.4h, v2.4h",
			n: &NodeImpl{
				Instruction:       VUMLAL,
				DstReg:            RegV0,
				SrcReg:            RegV2,
				SrcReg2:           RegV6,
				VectorArrangement: VectorArrangement4H,
			},
			exp: []byte{0xc0, 0x80, 0x62, 0x2e},
		},
		{
			name: "umlal v0.8h, v6.8b, v2.8b",
			n: &NodeImpl{
				Instruction:       VUMLAL,
				DstReg:            RegV0,
				SrcReg:            RegV2,
				SrcReg2:           RegV6,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0xc0, 0x80, 0x22, 0x2e},
		},
		{
			name: "bit v30.16b, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       VBIT,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0x1c, 0xab, 0x6e},
		},
		{
			name: "bit v30.8b, v4.8b, v11.8b",
			n: &NodeImpl{
				Instruction:       VBIT,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0x9e, 0x1c, 0xab, 0x2e},
		},
		{
			name: "sqrdmulh v30.8h, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       SQRDMULH,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0xb4, 0x6b, 0x6e},
		},
		{
			name: "sqrdmulh v30.4s, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       SQRDMULH,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xb4, 0xab, 0x6e},
		},
		{
			name: "smull v30.8h, v4.8b, v11.8b",
			n: &NodeImpl{
				Instruction:       SMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0x9e, 0xc0, 0x2b, 0xe},
		},
		{
			name: "smull v30.4s, v4.4h, v11.4h",
			n: &NodeImpl{
				Instruction:       SMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4H,
			},
			exp: []byte{0x9e, 0xc0, 0x6b, 0xe},
		},
		{
			name: "smull v30.2d, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       SMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xc0, 0xab, 0xe},
		},
		{
			name: "smull2 v30.8h, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       SMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0xc0, 0x2b, 0x4e},
		},
		{
			name: "smull2 v30.4s, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       SMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0xc0, 0x6b, 0x4e},
		},
		{
			name: "smull2 v30.2d, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       SMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xc0, 0xab, 0x4e},
		},

		//////////////////////

		{
			name: "umull v30.8h, v4.8b, v11.8b",
			n: &NodeImpl{
				Instruction:       UMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8B,
			},
			exp: []byte{0x9e, 0xc0, 0x2b, 0x2e},
		},
		{
			name: "umull v30.4s, v4.4h, v11.4h",
			n: &NodeImpl{
				Instruction:       UMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4H,
			},
			exp: []byte{0x9e, 0xc0, 0x6b, 0x2e},
		},
		{
			name: "umull v30.2d, v4.2s, v11.2s",
			n: &NodeImpl{
				Instruction:       UMULL,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement2S,
			},
			exp: []byte{0x9e, 0xc0, 0xab, 0x2e},
		},
		{
			name: "umull2 v30.8h, v4.16b, v11.16b",
			n: &NodeImpl{
				Instruction:       UMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x9e, 0xc0, 0x2b, 0x6e},
		},
		{
			name: "umull2 v30.4s, v4.8h, v11.8h",
			n: &NodeImpl{
				Instruction:       UMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x9e, 0xc0, 0x6b, 0x6e},
		},
		{
			name: "umull2 v30.2d, v4.4s, v11.4s",
			n: &NodeImpl{
				Instruction:       UMULL2,
				DstReg:            RegV30,
				SrcReg:            RegV11,
				SrcReg2:           RegV4,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0x9e, 0xc0, 0xab, 0x6e},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeTwoVectorRegistersToVectorRegister(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeConstToRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "and w30, w30, #1",
			n: &NodeImpl{
				Instruction:       ANDIMM32,
				DstReg:            RegR30,
				SrcConst:          1,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x3, 0x0, 0x12},
		},
		{
			name: "and w30, w30, #7",
			n: &NodeImpl{
				Instruction:       ANDIMM32,
				DstReg:            RegR30,
				SrcConst:          0x7,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0xb, 0x0, 0x12},
		},
		{
			name: "and w30, w30, #0xf",
			n: &NodeImpl{
				Instruction:       ANDIMM32,
				DstReg:            RegR30,
				SrcConst:          0xf,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0xf, 0x0, 0x12},
		},
		{
			name: "and w30, w30, #0x1f",
			n: &NodeImpl{
				Instruction:       ANDIMM32,
				DstReg:            RegR30,
				SrcConst:          0x1f,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x13, 0x0, 0x12},
		},
		{
			name: "and w30, w30, #0x3f",
			n: &NodeImpl{
				Instruction:       ANDIMM32,
				DstReg:            RegR30,
				SrcConst:          0x3f,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x17, 0x0, 0x12},
		},
		{
			name: "and x30, x30, #1",
			n: &NodeImpl{
				Instruction:       ANDIMM64,
				DstReg:            RegR30,
				SrcConst:          1,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x3, 0x40, 0x92},
		},
		{
			name: "and x30, x30, #7",
			n: &NodeImpl{
				Instruction:       ANDIMM64,
				DstReg:            RegR30,
				SrcConst:          0x7,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0xb, 0x40, 0x92},
		},
		{
			name: "and x30, x30, #0xf",
			n: &NodeImpl{
				Instruction:       ANDIMM64,
				DstReg:            RegR30,
				SrcConst:          0xf,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0xf, 0x40, 0x92},
		},
		{
			name: "and x30, x30, #0x1f",
			n: &NodeImpl{
				Instruction:       ANDIMM64,
				DstReg:            RegR30,
				SrcConst:          0x1f,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x13, 0x40, 0x92},
		},
		{
			name: "and x30, x30, #0x3f",
			n: &NodeImpl{
				Instruction:       ANDIMM64,
				DstReg:            RegR30,
				SrcConst:          0x3f,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0xde, 0x17, 0x40, 0x92},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeConstToRegister(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
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
			name: "ins v10.d[0], x10",
			n: &NodeImpl{
				Instruction:       INSGEN,
				DstReg:            RegV10,
				SrcReg:            RegR10,
				VectorArrangement: VectorArrangementD,
			},
			exp: []byte{0x4a, 0x1d, 0x8, 0x4e},
		},
		{
			name: "ins v10.d[1], x10",
			n: &NodeImpl{
				Instruction:       INSGEN,
				DstReg:            RegV10,
				SrcReg:            RegR10,
				VectorArrangement: VectorArrangementD,
				DstVectorIndex:    1,
			},
			exp: []byte{0x4a, 0x1d, 0x18, 0x4e},
		},
		{
			name: "dup v10.2d, x10",
			n: &NodeImpl{
				Instruction:       DUPGEN,
				SrcReg:            RegR10,
				DstReg:            RegV10,
				VectorArrangement: VectorArrangement2D,
			},
			exp: []byte{0x4a, 0xd, 0x8, 0x4e},
		},
		{
			name: "dup v1.4s, w30",
			n: &NodeImpl{
				Instruction:       DUPGEN,
				SrcReg:            RegR30,
				DstReg:            RegV1,
				VectorArrangement: VectorArrangement4S,
			},
			exp: []byte{0xc1, 0xf, 0x4, 0x4e},
		},
		{
			name: "dup v30.8h, w1",
			n: &NodeImpl{
				Instruction:       DUPGEN,
				SrcReg:            RegR1,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangement8H,
			},
			exp: []byte{0x3e, 0xc, 0x2, 0x4e},
		},
		{
			name: "dup v30.16b, w1",
			n: &NodeImpl{
				Instruction:       DUPGEN,
				SrcReg:            RegR1,
				DstReg:            RegV30,
				VectorArrangement: VectorArrangement16B,
			},
			exp: []byte{0x3e, 0xc, 0x1, 0x4e},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeRegisterToVectorRegister(tc.n)
			require.NoError(t, err)

			actual := a.Buf.Bytes()
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_maybeFlushConstPool(t *testing.T) {
	tests := []struct {
		name string
		c    []byte
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
			sc := asm.NewStaticConst(tc.c)
			a.pool.AddConst(sc, 0)

			var called bool
			sc.AddOffsetFinalizedCallback(func(uint64) {
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
				staticConst:       asm.NewStaticConst([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
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
				staticConst:       asm.NewStaticConst([]byte{1, 2, 3, 4, 5, 6, 7, 8}),
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
				staticConst:       asm.NewStaticConst([]byte{1, 2, 3, 4}),
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

func TestAssemblerImpl_encodeADR_staticConst(t *testing.T) {
	const beforeADRByteNum uint64 = 2

	tests := []struct {
		name                   string
		reg                    asm.Register
		offsetOfConstInBinary  uint64
		expADRInstructionBytes []byte
	}{
		{
			// #8 = offsetOfConstInBinary - beforeADRByteNum.
			name:                   "adr x12, #8",
			reg:                    RegR12,
			offsetOfConstInBinary:  10,
			expADRInstructionBytes: []byte{0x4c, 0x0, 0x0, 0x10},
		},
		{
			// #0x7fffd = offsetOfConstInBinary - beforeADRByteNum.
			name:                   "adr x12, #0x7fffd",
			reg:                    RegR12,
			offsetOfConstInBinary:  0x7ffff,
			expADRInstructionBytes: []byte{0xec, 0xff, 0x3f, 0x30},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sc := asm.NewStaticConst([]byte{1, 2, 3, 4}) // Arbitrary data is fine.

			a := NewAssemblerImpl(asm.NilRegister)

			a.Buf.Write(make([]byte, beforeADRByteNum))

			err := a.encodeADR(&NodeImpl{Instruction: ADR, DstReg: tc.reg, staticConst: sc})
			require.NoError(t, err)

			require.Equal(t, 1, len(a.pool.Consts))
			require.Equal(t, sc, a.pool.Consts[0])

			require.Equal(t, beforeADRByteNum, *a.pool.FirstUseOffsetInBinary)

			// Finalize the ADR instruction bytes.
			sc.SetOffsetInBinary(tc.offsetOfConstInBinary)

			actualBytes := a.Buf.Bytes()[beforeADRByteNum : beforeADRByteNum+4]
			require.Equal(t, tc.expADRInstructionBytes, actualBytes, hex.EncodeToString(actualBytes))
		})
	}
}

func TestAssemblerImpl_encodeJumpToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *NodeImpl
			expErr string
		}{
			{
				n:      &NodeImpl{Instruction: ADD, Types: OperandTypesNoneToRegister},
				expErr: "ADD is unsupported for from:none,to:register type",
			},
			{
				n:      &NodeImpl{Instruction: RET, DstReg: asm.NilRegister},
				expErr: "invalid destination register: nil is not integer",
			},
			{
				n:      &NodeImpl{Instruction: RET, DstReg: RegV0},
				expErr: "invalid destination register: V0 is not integer",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeJumpToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	tests := []struct {
		name   string
		inst   asm.Instruction
		reg    asm.Register
		expHex string
	}{
		{
			name:   "B",
			inst:   B,
			reg:    RegR0,
			expHex: "00001fd6000000000000000000000000",
		},
		{
			name:   "B",
			inst:   B,
			reg:    RegR5,
			expHex: "a0001fd6000000000000000000000000",
		},
		{
			name:   "B",
			inst:   B,
			reg:    RegR30,
			expHex: "c0031fd6000000000000000000000000",
		},
		{
			name:   "RET",
			inst:   RET,
			reg:    RegR0,
			expHex: "00005fd6000000000000000000000000",
		},
		{
			name:   "RET",
			inst:   RET,
			reg:    RegR5,
			expHex: "a0005fd6000000000000000000000000",
		},
		{
			name:   "RET",
			inst:   RET,
			reg:    RegR30,
			expHex: "c0035fd6000000000000000000000000",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeJumpToRegister(&NodeImpl{Instruction: tc.inst, DstReg: tc.reg})
			require.NoError(t, err)

			actual := a.Bytes()
			require.Equal(t, tc.expHex, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *NodeImpl
			expErr string
		}{
			{
				n: &NodeImpl{Instruction: ADR, Types: OperandTypesRegisterToRegister,
					SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "ADR is unsupported for from:register,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeRegisterToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	tests := []struct {
		name     string
		inst     asm.Instruction
		src, dst asm.Register
		exp      []byte
	}{
		{name: "ADD/src=RZR,dst=RZR", inst: ADD, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x8b}},
		{name: "ADD/src=RZR,dst=R10", inst: ADD, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0x8b}},
		{name: "ADD/src=RZR,dst=R30", inst: ADD, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0x8b}},
		{name: "ADD/src=R10,dst=RZR", inst: ADD, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x8b}},
		{name: "ADD/src=R10,dst=R10", inst: ADD, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0x8b}},
		{name: "ADD/src=R10,dst=R30", inst: ADD, src: RegR10, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0x8b}},
		{name: "ADD/src=R30,dst=RZR", inst: ADD, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x8b}},
		{name: "ADD/src=R30,dst=R10", inst: ADD, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0x8b}},
		{name: "ADD/src=R30,dst=R30", inst: ADD, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0x8b}},
		{name: "ADDW/src=RZR,dst=RZR", inst: ADDW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xb}},
		{name: "ADDW/src=RZR,dst=R10", inst: ADDW, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xb}},
		{name: "ADDW/src=RZR,dst=R30", inst: ADDW, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xb}},
		{name: "ADDW/src=R10,dst=RZR", inst: ADDW, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xb}},
		{name: "ADDW/src=R10,dst=R10", inst: ADDW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xb}},
		{name: "ADDW/src=R10,dst=R30", inst: ADDW, src: RegR10, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xb}},
		{name: "ADDW/src=R30,dst=RZR", inst: ADDW, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xb}},
		{name: "ADDW/src=R30,dst=R10", inst: ADDW, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xb}},
		{name: "ADDW/src=R30,dst=R30", inst: ADDW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xb}},
		{name: "SUB/src=RZR,dst=RZR", inst: SUB, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xcb}},
		{name: "SUB/src=RZR,dst=R10", inst: SUB, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xcb}},
		{name: "SUB/src=RZR,dst=R30", inst: SUB, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xcb}},
		{name: "SUB/src=R10,dst=RZR", inst: SUB, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xcb}},
		{name: "SUB/src=R10,dst=R10", inst: SUB, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xcb}},
		{name: "SUB/src=R10,dst=R30", inst: SUB, src: RegR10, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xcb}},
		{name: "SUB/src=R30,dst=RZR", inst: SUB, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xcb}},
		{name: "SUB/src=R30,dst=R10", inst: SUB, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xcb}},
		{name: "SUB/src=R30,dst=R30", inst: SUB, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xcb}},
		{name: "CLZ/src=RZR,dst=RZR", inst: CLZ, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x13, 0xc0, 0xda}},
		{name: "CLZ/src=RZR,dst=R10", inst: CLZ, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x13, 0xc0, 0xda}},
		{name: "CLZ/src=RZR,dst=R30", inst: CLZ, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x13, 0xc0, 0xda}},
		{name: "CLZ/src=R10,dst=RZR", inst: CLZ, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x11, 0xc0, 0xda}},
		{name: "CLZ/src=R10,dst=R10", inst: CLZ, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x11, 0xc0, 0xda}},
		{name: "CLZ/src=R10,dst=R30", inst: CLZ, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x11, 0xc0, 0xda}},
		{name: "CLZ/src=R30,dst=RZR", inst: CLZ, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x13, 0xc0, 0xda}},
		{name: "CLZ/src=R30,dst=R10", inst: CLZ, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x13, 0xc0, 0xda}},
		{name: "CLZ/src=R30,dst=R30", inst: CLZ, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x13, 0xc0, 0xda}},
		{name: "CLZW/src=RZR,dst=RZR", inst: CLZW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x13, 0xc0, 0x5a}},
		{name: "CLZW/src=RZR,dst=R10", inst: CLZW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x13, 0xc0, 0x5a}},
		{name: "CLZW/src=RZR,dst=R30", inst: CLZW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x13, 0xc0, 0x5a}},
		{name: "CLZW/src=R10,dst=RZR", inst: CLZW, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x11, 0xc0, 0x5a}},
		{name: "CLZW/src=R10,dst=R10", inst: CLZW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x11, 0xc0, 0x5a}},
		{name: "CLZW/src=R10,dst=R30", inst: CLZW, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x11, 0xc0, 0x5a}},
		{name: "CLZW/src=R30,dst=RZR", inst: CLZW, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x13, 0xc0, 0x5a}},
		{name: "CLZW/src=R30,dst=R10", inst: CLZW, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x13, 0xc0, 0x5a}},
		{name: "CLZW/src=R30,dst=R30", inst: CLZW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x13, 0xc0, 0x5a}},
		{name: "CSET/src=COND_EQ,dst=RZR", inst: CSET, src: RegCondEQ, dst: RegRZR, exp: []byte{0xff, 0x17, 0x9f, 0x9a}},
		{name: "CSET/src=COND_EQ,dst=R10", inst: CSET, src: RegCondEQ, dst: RegR10, exp: []byte{0xea, 0x17, 0x9f, 0x9a}},
		{name: "CSET/src=COND_EQ,dst=R30", inst: CSET, src: RegCondEQ, dst: RegR30, exp: []byte{0xfe, 0x17, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NE,dst=RZR", inst: CSET, src: RegCondNE, dst: RegRZR, exp: []byte{0xff, 0x7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NE,dst=R10", inst: CSET, src: RegCondNE, dst: RegR10, exp: []byte{0xea, 0x7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NE,dst=R30", inst: CSET, src: RegCondNE, dst: RegR30, exp: []byte{0xfe, 0x7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HS,dst=RZR", inst: CSET, src: RegCondHS, dst: RegRZR, exp: []byte{0xff, 0x37, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HS,dst=R10", inst: CSET, src: RegCondHS, dst: RegR10, exp: []byte{0xea, 0x37, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HS,dst=R30", inst: CSET, src: RegCondHS, dst: RegR30, exp: []byte{0xfe, 0x37, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LO,dst=RZR", inst: CSET, src: RegCondLO, dst: RegRZR, exp: []byte{0xff, 0x27, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LO,dst=R10", inst: CSET, src: RegCondLO, dst: RegR10, exp: []byte{0xea, 0x27, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LO,dst=R30", inst: CSET, src: RegCondLO, dst: RegR30, exp: []byte{0xfe, 0x27, 0x9f, 0x9a}},
		{name: "CSET/src=COND_MI,dst=RZR", inst: CSET, src: RegCondMI, dst: RegRZR, exp: []byte{0xff, 0x57, 0x9f, 0x9a}},
		{name: "CSET/src=COND_MI,dst=R10", inst: CSET, src: RegCondMI, dst: RegR10, exp: []byte{0xea, 0x57, 0x9f, 0x9a}},
		{name: "CSET/src=COND_MI,dst=R30", inst: CSET, src: RegCondMI, dst: RegR30, exp: []byte{0xfe, 0x57, 0x9f, 0x9a}},
		{name: "CSET/src=COND_PL,dst=RZR", inst: CSET, src: RegCondPL, dst: RegRZR, exp: []byte{0xff, 0x47, 0x9f, 0x9a}},
		{name: "CSET/src=COND_PL,dst=R10", inst: CSET, src: RegCondPL, dst: RegR10, exp: []byte{0xea, 0x47, 0x9f, 0x9a}},
		{name: "CSET/src=COND_PL,dst=R30", inst: CSET, src: RegCondPL, dst: RegR30, exp: []byte{0xfe, 0x47, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VS,dst=RZR", inst: CSET, src: RegCondVS, dst: RegRZR, exp: []byte{0xff, 0x77, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VS,dst=R10", inst: CSET, src: RegCondVS, dst: RegR10, exp: []byte{0xea, 0x77, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VS,dst=R30", inst: CSET, src: RegCondVS, dst: RegR30, exp: []byte{0xfe, 0x77, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VC,dst=RZR", inst: CSET, src: RegCondVC, dst: RegRZR, exp: []byte{0xff, 0x67, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VC,dst=R10", inst: CSET, src: RegCondVC, dst: RegR10, exp: []byte{0xea, 0x67, 0x9f, 0x9a}},
		{name: "CSET/src=COND_VC,dst=R30", inst: CSET, src: RegCondVC, dst: RegR30, exp: []byte{0xfe, 0x67, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HI,dst=RZR", inst: CSET, src: RegCondHI, dst: RegRZR, exp: []byte{0xff, 0x97, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HI,dst=R10", inst: CSET, src: RegCondHI, dst: RegR10, exp: []byte{0xea, 0x97, 0x9f, 0x9a}},
		{name: "CSET/src=COND_HI,dst=R30", inst: CSET, src: RegCondHI, dst: RegR30, exp: []byte{0xfe, 0x97, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LS,dst=RZR", inst: CSET, src: RegCondLS, dst: RegRZR, exp: []byte{0xff, 0x87, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LS,dst=R10", inst: CSET, src: RegCondLS, dst: RegR10, exp: []byte{0xea, 0x87, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LS,dst=R30", inst: CSET, src: RegCondLS, dst: RegR30, exp: []byte{0xfe, 0x87, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GE,dst=RZR", inst: CSET, src: RegCondGE, dst: RegRZR, exp: []byte{0xff, 0xb7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GE,dst=R10", inst: CSET, src: RegCondGE, dst: RegR10, exp: []byte{0xea, 0xb7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GE,dst=R30", inst: CSET, src: RegCondGE, dst: RegR30, exp: []byte{0xfe, 0xb7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LT,dst=RZR", inst: CSET, src: RegCondLT, dst: RegRZR, exp: []byte{0xff, 0xa7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LT,dst=R10", inst: CSET, src: RegCondLT, dst: RegR10, exp: []byte{0xea, 0xa7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LT,dst=R30", inst: CSET, src: RegCondLT, dst: RegR30, exp: []byte{0xfe, 0xa7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GT,dst=RZR", inst: CSET, src: RegCondGT, dst: RegRZR, exp: []byte{0xff, 0xd7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GT,dst=R10", inst: CSET, src: RegCondGT, dst: RegR10, exp: []byte{0xea, 0xd7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_GT,dst=R30", inst: CSET, src: RegCondGT, dst: RegR30, exp: []byte{0xfe, 0xd7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LE,dst=RZR", inst: CSET, src: RegCondLE, dst: RegRZR, exp: []byte{0xff, 0xc7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LE,dst=R10", inst: CSET, src: RegCondLE, dst: RegR10, exp: []byte{0xea, 0xc7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_LE,dst=R30", inst: CSET, src: RegCondLE, dst: RegR30, exp: []byte{0xfe, 0xc7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_AL,dst=RZR", inst: CSET, src: RegCondAL, dst: RegRZR, exp: []byte{0xff, 0xf7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_AL,dst=R10", inst: CSET, src: RegCondAL, dst: RegR10, exp: []byte{0xea, 0xf7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_AL,dst=R30", inst: CSET, src: RegCondAL, dst: RegR30, exp: []byte{0xfe, 0xf7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NV,dst=RZR", inst: CSET, src: RegCondNV, dst: RegRZR, exp: []byte{0xff, 0xe7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NV,dst=R10", inst: CSET, src: RegCondNV, dst: RegR10, exp: []byte{0xea, 0xe7, 0x9f, 0x9a}},
		{name: "CSET/src=COND_NV,dst=R30", inst: CSET, src: RegCondNV, dst: RegR30, exp: []byte{0xfe, 0xe7, 0x9f, 0x9a}},
		{name: "FABSS/src=V0,dst=V0", inst: FABSS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x20, 0x1e}},
		{name: "FABSS/src=V0,dst=V31", inst: FABSS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x20, 0x1e}},
		{name: "FABSS/src=V31,dst=V0", inst: FABSS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x20, 0x1e}},
		{name: "FABSS/src=V31,dst=V31", inst: FABSS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x20, 0x1e}},
		{name: "FABSD/src=V0,dst=V0", inst: FABSD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x60, 0x1e}},
		{name: "FABSD/src=V0,dst=V31", inst: FABSD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x60, 0x1e}},
		{name: "FABSD/src=V31,dst=V0", inst: FABSD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x60, 0x1e}},
		{name: "FABSD/src=V31,dst=V31", inst: FABSD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x60, 0x1e}},
		{name: "FNEGS/src=V0,dst=V0", inst: FNEGS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x21, 0x1e}},
		{name: "FNEGS/src=V0,dst=V31", inst: FNEGS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x21, 0x1e}},
		{name: "FNEGS/src=V31,dst=V0", inst: FNEGS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x21, 0x1e}},
		{name: "FNEGS/src=V31,dst=V31", inst: FNEGS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x21, 0x1e}},
		{name: "FNEGD/src=V0,dst=V0", inst: FNEGD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x61, 0x1e}},
		{name: "FNEGD/src=V0,dst=V31", inst: FNEGD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x61, 0x1e}},
		{name: "FNEGD/src=V31,dst=V0", inst: FNEGD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x61, 0x1e}},
		{name: "FNEGD/src=V31,dst=V31", inst: FNEGD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x61, 0x1e}},
		{name: "FSQRTD/src=V0,dst=V0", inst: FSQRTD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x61, 0x1e}},
		{name: "FSQRTD/src=V0,dst=V31", inst: FSQRTD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x61, 0x1e}},
		{name: "FSQRTD/src=V31,dst=V0", inst: FSQRTD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x61, 0x1e}},
		{name: "FSQRTD/src=V31,dst=V31", inst: FSQRTD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x61, 0x1e}},
		{name: "FSQRTS/src=V0,dst=V0", inst: FSQRTS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x21, 0x1e}},
		{name: "FSQRTS/src=V0,dst=V31", inst: FSQRTS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x21, 0x1e}},
		{name: "FSQRTS/src=V31,dst=V0", inst: FSQRTS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x21, 0x1e}},
		{name: "FSQRTS/src=V31,dst=V31", inst: FSQRTS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x21, 0x1e}},
		{name: "FCVTDS/src=V0,dst=V0", inst: FCVTDS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x62, 0x1e}},
		{name: "FCVTDS/src=V0,dst=V31", inst: FCVTDS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x62, 0x1e}},
		{name: "FCVTDS/src=V31,dst=V0", inst: FCVTDS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x62, 0x1e}},
		{name: "FCVTDS/src=V31,dst=V31", inst: FCVTDS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x62, 0x1e}},
		{name: "FCVTSD/src=V0,dst=V0", inst: FCVTSD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x22, 0x1e}},
		{name: "FCVTSD/src=V0,dst=V31", inst: FCVTSD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x22, 0x1e}},
		{name: "FCVTSD/src=V31,dst=V0", inst: FCVTSD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x22, 0x1e}},
		{name: "FCVTSD/src=V31,dst=V31", inst: FCVTSD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x22, 0x1e}},
		{name: "FRINTMD/src=V0,dst=V0", inst: FRINTMD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x65, 0x1e}},
		{name: "FRINTMD/src=V0,dst=V31", inst: FRINTMD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x65, 0x1e}},
		{name: "FRINTMD/src=V31,dst=V0", inst: FRINTMD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x65, 0x1e}},
		{name: "FRINTMD/src=V31,dst=V31", inst: FRINTMD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x65, 0x1e}},
		{name: "FRINTMS/src=V0,dst=V0", inst: FRINTMS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x25, 0x1e}},
		{name: "FRINTMS/src=V0,dst=V31", inst: FRINTMS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x25, 0x1e}},
		{name: "FRINTMS/src=V31,dst=V0", inst: FRINTMS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x25, 0x1e}},
		{name: "FRINTMS/src=V31,dst=V31", inst: FRINTMS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x25, 0x1e}},
		{name: "FRINTND/src=V0,dst=V0", inst: FRINTND, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x64, 0x1e}},
		{name: "FRINTND/src=V0,dst=V31", inst: FRINTND, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x64, 0x1e}},
		{name: "FRINTND/src=V31,dst=V0", inst: FRINTND, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x64, 0x1e}},
		{name: "FRINTND/src=V31,dst=V31", inst: FRINTND, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x64, 0x1e}},
		{name: "FRINTNS/src=V0,dst=V0", inst: FRINTNS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x24, 0x1e}},
		{name: "FRINTNS/src=V0,dst=V31", inst: FRINTNS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x24, 0x1e}},
		{name: "FRINTNS/src=V31,dst=V0", inst: FRINTNS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x24, 0x1e}},
		{name: "FRINTNS/src=V31,dst=V31", inst: FRINTNS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x24, 0x1e}},
		{name: "FRINTPD/src=V0,dst=V0", inst: FRINTPD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x64, 0x1e}},
		{name: "FRINTPD/src=V0,dst=V31", inst: FRINTPD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x64, 0x1e}},
		{name: "FRINTPD/src=V31,dst=V0", inst: FRINTPD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x64, 0x1e}},
		{name: "FRINTPD/src=V31,dst=V31", inst: FRINTPD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x64, 0x1e}},
		{name: "FRINTPS/src=V0,dst=V0", inst: FRINTPS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x24, 0x1e}},
		{name: "FRINTPS/src=V0,dst=V31", inst: FRINTPS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x24, 0x1e}},
		{name: "FRINTPS/src=V31,dst=V0", inst: FRINTPS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x24, 0x1e}},
		{name: "FRINTPS/src=V31,dst=V31", inst: FRINTPS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x24, 0x1e}},
		{name: "FRINTZD/src=V0,dst=V0", inst: FRINTZD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x65, 0x1e}},
		{name: "FRINTZD/src=V0,dst=V31", inst: FRINTZD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x65, 0x1e}},
		{name: "FRINTZD/src=V31,dst=V0", inst: FRINTZD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x65, 0x1e}},
		{name: "FRINTZD/src=V31,dst=V31", inst: FRINTZD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x65, 0x1e}},
		{name: "FRINTZS/src=V0,dst=V0", inst: FRINTZS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0xc0, 0x25, 0x1e}},
		{name: "FRINTZS/src=V0,dst=V31", inst: FRINTZS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0xc0, 0x25, 0x1e}},
		{name: "FRINTZS/src=V31,dst=V0", inst: FRINTZS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0xc3, 0x25, 0x1e}},
		{name: "FRINTZS/src=V31,dst=V31", inst: FRINTZS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xc3, 0x25, 0x1e}},
		{name: "FDIVS/src=V0,dst=V0", inst: FDIVS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x18, 0x20, 0x1e}},
		{name: "FDIVS/src=V0,dst=V31", inst: FDIVS, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x1b, 0x20, 0x1e}},
		{name: "FDIVS/src=V31,dst=V0", inst: FDIVS, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x18, 0x3f, 0x1e}},
		{name: "FDIVS/src=V31,dst=V31", inst: FDIVS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x1b, 0x3f, 0x1e}},
		{name: "FDIVD/src=V0,dst=V0", inst: FDIVD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x18, 0x60, 0x1e}},
		{name: "FDIVD/src=V0,dst=V31", inst: FDIVD, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x1b, 0x60, 0x1e}},
		{name: "FDIVD/src=V31,dst=V0", inst: FDIVD, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x18, 0x7f, 0x1e}},
		{name: "FDIVD/src=V31,dst=V31", inst: FDIVD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x1b, 0x7f, 0x1e}},
		{name: "FMAXD/src=V0,dst=V0", inst: FMAXD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x48, 0x60, 0x1e}},
		{name: "FMAXD/src=V0,dst=V31", inst: FMAXD, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x4b, 0x60, 0x1e}},
		{name: "FMAXD/src=V31,dst=V0", inst: FMAXD, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x48, 0x7f, 0x1e}},
		{name: "FMAXD/src=V31,dst=V31", inst: FMAXD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x4b, 0x7f, 0x1e}},
		{name: "FMAXS/src=V0,dst=V0", inst: FMAXS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x48, 0x20, 0x1e}},
		{name: "FMAXS/src=V0,dst=V31", inst: FMAXS, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x4b, 0x20, 0x1e}},
		{name: "FMAXS/src=V31,dst=V0", inst: FMAXS, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x48, 0x3f, 0x1e}},
		{name: "FMAXS/src=V31,dst=V31", inst: FMAXS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x4b, 0x3f, 0x1e}},
		{name: "FMIND/src=V0,dst=V0", inst: FMIND, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x58, 0x60, 0x1e}},
		{name: "FMIND/src=V0,dst=V31", inst: FMIND, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x5b, 0x60, 0x1e}},
		{name: "FMIND/src=V31,dst=V0", inst: FMIND, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x58, 0x7f, 0x1e}},
		{name: "FMIND/src=V31,dst=V31", inst: FMIND, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x5b, 0x7f, 0x1e}},
		{name: "FMINS/src=V0,dst=V0", inst: FMINS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x58, 0x20, 0x1e}},
		{name: "FMINS/src=V0,dst=V31", inst: FMINS, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x5b, 0x20, 0x1e}},
		{name: "FMINS/src=V31,dst=V0", inst: FMINS, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x58, 0x3f, 0x1e}},
		{name: "FMINS/src=V31,dst=V31", inst: FMINS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x5b, 0x3f, 0x1e}},
		{name: "FMULS/src=V0,dst=V0", inst: FMULS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x8, 0x20, 0x1e}},
		{name: "FMULS/src=V0,dst=V31", inst: FMULS, src: RegV0, dst: RegV31, exp: []byte{0xff, 0xb, 0x20, 0x1e}},
		{name: "FMULS/src=V31,dst=V0", inst: FMULS, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x8, 0x3f, 0x1e}},
		{name: "FMULS/src=V31,dst=V31", inst: FMULS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xb, 0x3f, 0x1e}},
		{name: "FMULD/src=V0,dst=V0", inst: FMULD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x8, 0x60, 0x1e}},
		{name: "FMULD/src=V0,dst=V31", inst: FMULD, src: RegV0, dst: RegV31, exp: []byte{0xff, 0xb, 0x60, 0x1e}},
		{name: "FMULD/src=V31,dst=V0", inst: FMULD, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x8, 0x7f, 0x1e}},
		{name: "FMULD/src=V31,dst=V31", inst: FMULD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0xb, 0x7f, 0x1e}},
		{name: "FADDD/src=V0,dst=V0", inst: FADDD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x28, 0x60, 0x1e}},
		{name: "FADDD/src=V0,dst=V31", inst: FADDD, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x2b, 0x60, 0x1e}},
		{name: "FADDD/src=V31,dst=V0", inst: FADDD, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x28, 0x7f, 0x1e}},
		{name: "FADDD/src=V31,dst=V31", inst: FADDD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x2b, 0x7f, 0x1e}},
		{name: "FADDS/src=V0,dst=V0", inst: FADDS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x28, 0x20, 0x1e}},
		{name: "FADDS/src=V0,dst=V31", inst: FADDS, src: RegV0, dst: RegV31, exp: []byte{0xff, 0x2b, 0x20, 0x1e}},
		{name: "FADDS/src=V31,dst=V0", inst: FADDS, src: RegV31, dst: RegV0, exp: []byte{0x0, 0x28, 0x3f, 0x1e}},
		{name: "FADDS/src=V31,dst=V31", inst: FADDS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x2b, 0x3f, 0x1e}},
		{name: "FCVTZSD/src=V0,dst=RZR", inst: FCVTZSD, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x78, 0x9e}},
		{name: "FCVTZSD/src=V0,dst=R10", inst: FCVTZSD, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x78, 0x9e}},
		{name: "FCVTZSD/src=V0,dst=R30", inst: FCVTZSD, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x78, 0x9e}},
		{name: "FCVTZSD/src=V31,dst=RZR", inst: FCVTZSD, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x78, 0x9e}},
		{name: "FCVTZSD/src=V31,dst=R10", inst: FCVTZSD, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x78, 0x9e}},
		{name: "FCVTZSD/src=V31,dst=R30", inst: FCVTZSD, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x78, 0x9e}},
		{name: "FCVTZSDW/src=V0,dst=RZR", inst: FCVTZSDW, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x78, 0x1e}},
		{name: "FCVTZSDW/src=V0,dst=R10", inst: FCVTZSDW, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x78, 0x1e}},
		{name: "FCVTZSDW/src=V0,dst=R30", inst: FCVTZSDW, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x78, 0x1e}},
		{name: "FCVTZSDW/src=V31,dst=RZR", inst: FCVTZSDW, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x78, 0x1e}},
		{name: "FCVTZSDW/src=V31,dst=R10", inst: FCVTZSDW, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x78, 0x1e}},
		{name: "FCVTZSDW/src=V31,dst=R30", inst: FCVTZSDW, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x78, 0x1e}},
		{name: "FCVTZSS/src=V0,dst=RZR", inst: FCVTZSS, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x38, 0x9e}},
		{name: "FCVTZSS/src=V0,dst=R10", inst: FCVTZSS, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x38, 0x9e}},
		{name: "FCVTZSS/src=V0,dst=R30", inst: FCVTZSS, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x38, 0x9e}},
		{name: "FCVTZSS/src=V31,dst=RZR", inst: FCVTZSS, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x38, 0x9e}},
		{name: "FCVTZSS/src=V31,dst=R10", inst: FCVTZSS, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x38, 0x9e}},
		{name: "FCVTZSS/src=V31,dst=R30", inst: FCVTZSS, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x38, 0x9e}},
		{name: "FCVTZSSW/src=V0,dst=RZR", inst: FCVTZSSW, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x38, 0x1e}},
		{name: "FCVTZSSW/src=V0,dst=R10", inst: FCVTZSSW, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x38, 0x1e}},
		{name: "FCVTZSSW/src=V0,dst=R30", inst: FCVTZSSW, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x38, 0x1e}},
		{name: "FCVTZSSW/src=V31,dst=RZR", inst: FCVTZSSW, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x38, 0x1e}},
		{name: "FCVTZSSW/src=V31,dst=R10", inst: FCVTZSSW, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x38, 0x1e}},
		{name: "FCVTZSSW/src=V31,dst=R30", inst: FCVTZSSW, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x38, 0x1e}},
		{name: "FCVTZUD/src=V0,dst=RZR", inst: FCVTZUD, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x79, 0x9e}},
		{name: "FCVTZUD/src=V0,dst=R10", inst: FCVTZUD, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x79, 0x9e}},
		{name: "FCVTZUD/src=V0,dst=R30", inst: FCVTZUD, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x79, 0x9e}},
		{name: "FCVTZUD/src=V31,dst=RZR", inst: FCVTZUD, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x79, 0x9e}},
		{name: "FCVTZUD/src=V31,dst=R10", inst: FCVTZUD, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x79, 0x9e}},
		{name: "FCVTZUD/src=V31,dst=R30", inst: FCVTZUD, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x79, 0x9e}},
		{name: "FCVTZUDW/src=V0,dst=RZR", inst: FCVTZUDW, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x79, 0x1e}},
		{name: "FCVTZUDW/src=V0,dst=R10", inst: FCVTZUDW, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x79, 0x1e}},
		{name: "FCVTZUDW/src=V0,dst=R30", inst: FCVTZUDW, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x79, 0x1e}},
		{name: "FCVTZUDW/src=V31,dst=RZR", inst: FCVTZUDW, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x79, 0x1e}},
		{name: "FCVTZUDW/src=V31,dst=R10", inst: FCVTZUDW, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x79, 0x1e}},
		{name: "FCVTZUDW/src=V31,dst=R30", inst: FCVTZUDW, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x79, 0x1e}},
		{name: "FCVTZUS/src=V0,dst=RZR", inst: FCVTZUS, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x39, 0x9e}},
		{name: "FCVTZUS/src=V0,dst=R10", inst: FCVTZUS, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x39, 0x9e}},
		{name: "FCVTZUS/src=V0,dst=R30", inst: FCVTZUS, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x39, 0x9e}},
		{name: "FCVTZUS/src=V31,dst=RZR", inst: FCVTZUS, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x39, 0x9e}},
		{name: "FCVTZUS/src=V31,dst=R10", inst: FCVTZUS, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x39, 0x9e}},
		{name: "FCVTZUS/src=V31,dst=R30", inst: FCVTZUS, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x39, 0x9e}},
		{name: "FCVTZUSW/src=V0,dst=RZR", inst: FCVTZUSW, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x39, 0x1e}},
		{name: "FCVTZUSW/src=V0,dst=R10", inst: FCVTZUSW, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x39, 0x1e}},
		{name: "FCVTZUSW/src=V0,dst=R30", inst: FCVTZUSW, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x39, 0x1e}},
		{name: "FCVTZUSW/src=V31,dst=RZR", inst: FCVTZUSW, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x39, 0x1e}},
		{name: "FCVTZUSW/src=V31,dst=R10", inst: FCVTZUSW, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x39, 0x1e}},
		{name: "FCVTZUSW/src=V31,dst=R30", inst: FCVTZUSW, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x39, 0x1e}},
		{name: "FMOVD/src=V0,dst=V0", inst: FMOVD, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x60, 0x1e}},
		{name: "FMOVD/src=V0,dst=V31", inst: FMOVD, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x60, 0x1e}},
		{name: "FMOVD/src=V31,dst=V0", inst: FMOVD, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x60, 0x1e}},
		{name: "FMOVD/src=V31,dst=V31", inst: FMOVD, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x60, 0x1e}},
		{name: "FMOVS/src=V0,dst=V0", inst: FMOVS, src: RegV0, dst: RegV0, exp: []byte{0x0, 0x40, 0x20, 0x1e}},
		{name: "FMOVS/src=V0,dst=V31", inst: FMOVS, src: RegV0, dst: RegV31, exp: []byte{0x1f, 0x40, 0x20, 0x1e}},
		{name: "FMOVS/src=V31,dst=V0", inst: FMOVS, src: RegV31, dst: RegV0, exp: []byte{0xe0, 0x43, 0x20, 0x1e}},
		{name: "FMOVS/src=V31,dst=V31", inst: FMOVS, src: RegV31, dst: RegV31, exp: []byte{0xff, 0x43, 0x20, 0x1e}},
		{name: "FMOVD/src=RZR,dst=V0", inst: FMOVD, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x67, 0x9e}},
		{name: "FMOVD/src=RZR,dst=V31", inst: FMOVD, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x67, 0x9e}},
		{name: "FMOVD/src=R10,dst=V0", inst: FMOVD, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x67, 0x9e}},
		{name: "FMOVD/src=R10,dst=V31", inst: FMOVD, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x67, 0x9e}},
		{name: "FMOVD/src=R30,dst=V0", inst: FMOVD, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x67, 0x9e}},
		{name: "FMOVD/src=R30,dst=V31", inst: FMOVD, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x67, 0x9e}},
		{name: "FMOVS/src=RZR,dst=V0", inst: FMOVS, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x27, 0x1e}},
		{name: "FMOVS/src=RZR,dst=V31", inst: FMOVS, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x27, 0x1e}},
		{name: "FMOVS/src=R10,dst=V0", inst: FMOVS, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x27, 0x1e}},
		{name: "FMOVS/src=R10,dst=V31", inst: FMOVS, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x27, 0x1e}},
		{name: "FMOVS/src=R30,dst=V0", inst: FMOVS, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x27, 0x1e}},
		{name: "FMOVS/src=R30,dst=V31", inst: FMOVS, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x27, 0x1e}},
		{name: "FMOVD/src=V0,dst=RZR", inst: FMOVD, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x66, 0x9e}},
		{name: "FMOVD/src=V0,dst=R10", inst: FMOVD, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x66, 0x9e}},
		{name: "FMOVD/src=V0,dst=R30", inst: FMOVD, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x66, 0x9e}},
		{name: "FMOVD/src=V31,dst=RZR", inst: FMOVD, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x66, 0x9e}},
		{name: "FMOVD/src=V31,dst=R10", inst: FMOVD, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x66, 0x9e}},
		{name: "FMOVD/src=V31,dst=R30", inst: FMOVD, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x66, 0x9e}},
		{name: "FMOVS/src=V0,dst=RZR", inst: FMOVS, src: RegV0, dst: RegRZR, exp: []byte{0x1f, 0x0, 0x26, 0x1e}},
		{name: "FMOVS/src=V0,dst=R10", inst: FMOVS, src: RegV0, dst: RegR10, exp: []byte{0xa, 0x0, 0x26, 0x1e}},
		{name: "FMOVS/src=V0,dst=R30", inst: FMOVS, src: RegV0, dst: RegR30, exp: []byte{0x1e, 0x0, 0x26, 0x1e}},
		{name: "FMOVS/src=V31,dst=RZR", inst: FMOVS, src: RegV31, dst: RegRZR, exp: []byte{0xff, 0x3, 0x26, 0x1e}},
		{name: "FMOVS/src=V31,dst=R10", inst: FMOVS, src: RegV31, dst: RegR10, exp: []byte{0xea, 0x3, 0x26, 0x1e}},
		{name: "FMOVS/src=V31,dst=R30", inst: FMOVS, src: RegV31, dst: RegR30, exp: []byte{0xfe, 0x3, 0x26, 0x1e}},
		{name: "MOVD/src=RZR,dst=R10", inst: MOVD, src: RegRZR, dst: RegR10, exp: []byte{0xa, 0x0, 0x80, 0xd2}},
		{name: "MOVD/src=RZR,dst=R30", inst: MOVD, src: RegRZR, dst: RegR30, exp: []byte{0x1e, 0x0, 0x80, 0xd2}},
		{name: "MOVD/src=R10,dst=R10", inst: MOVD, src: RegR10, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xaa}},
		{name: "MOVD/src=R10,dst=R30", inst: MOVD, src: RegR10, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xaa}},
		{name: "MOVD/src=R30,dst=R10", inst: MOVD, src: RegR30, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xaa}},
		{name: "MOVD/src=R30,dst=R30", inst: MOVD, src: RegR30, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xaa}},
		{name: "MOVWU/src=RZR,dst=RZR", inst: MOVWU, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "MOVWU/src=RZR,dst=R10", inst: MOVWU, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "MOVWU/src=RZR,dst=R30", inst: MOVWU, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "MOVWU/src=R10,dst=RZR", inst: MOVWU, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x2a}},
		{name: "MOVWU/src=R10,dst=R10", inst: MOVWU, src: RegR10, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x2a}},
		{name: "MOVWU/src=R10,dst=R30", inst: MOVWU, src: RegR10, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x2a}},
		{name: "MOVWU/src=R30,dst=RZR", inst: MOVWU, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x2a}},
		{name: "MOVWU/src=R30,dst=R10", inst: MOVWU, src: RegR30, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x2a}},
		{name: "MOVWU/src=R30,dst=R30", inst: MOVWU, src: RegR30, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x2a}},
		{name: "MRS/src=FPSR,dst=RZR", inst: MRS, src: RegFPSR, dst: RegRZR, exp: []byte{0x3f, 0x44, 0x3b, 0xd5}},
		{name: "MRS/src=FPSR,dst=R10", inst: MRS, src: RegFPSR, dst: RegR10, exp: []byte{0x2a, 0x44, 0x3b, 0xd5}},
		{name: "MRS/src=FPSR,dst=R30", inst: MRS, src: RegFPSR, dst: RegR30, exp: []byte{0x3e, 0x44, 0x3b, 0xd5}},
		{name: "MSR/src=RZR,dst=FPSR", inst: MSR, src: RegRZR, dst: RegFPSR, exp: []byte{0x3f, 0x44, 0x1b, 0xd5}},
		{name: "MSR/src=R10,dst=FPSR", inst: MSR, src: RegR10, dst: RegFPSR, exp: []byte{0x2a, 0x44, 0x1b, 0xd5}},
		{name: "MSR/src=R30,dst=FPSR", inst: MSR, src: RegR30, dst: RegFPSR, exp: []byte{0x3e, 0x44, 0x1b, 0xd5}},
		{name: "MUL/src=RZR,dst=RZR", inst: MUL, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x7f, 0x1f, 0x9b}},
		{name: "MUL/src=RZR,dst=R10", inst: MUL, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x7d, 0x1f, 0x9b}},
		{name: "MUL/src=RZR,dst=R30", inst: MUL, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0x7f, 0x1f, 0x9b}},
		{name: "MUL/src=R10,dst=RZR", inst: MUL, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x7f, 0xa, 0x9b}},
		{name: "MUL/src=R10,dst=R10", inst: MUL, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x7d, 0xa, 0x9b}},
		{name: "MUL/src=R10,dst=R30", inst: MUL, src: RegR10, dst: RegR30, exp: []byte{0xde, 0x7f, 0xa, 0x9b}},
		{name: "MUL/src=R30,dst=RZR", inst: MUL, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x7f, 0x1e, 0x9b}},
		{name: "MUL/src=R30,dst=R10", inst: MUL, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x7d, 0x1e, 0x9b}},
		{name: "MUL/src=R30,dst=R30", inst: MUL, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x7f, 0x1e, 0x9b}},
		{name: "MULW/src=RZR,dst=RZR", inst: MULW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x7f, 0x1f, 0x1b}},
		{name: "MULW/src=RZR,dst=R10", inst: MULW, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x7d, 0x1f, 0x1b}},
		{name: "MULW/src=RZR,dst=R30", inst: MULW, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0x7f, 0x1f, 0x1b}},
		{name: "MULW/src=R10,dst=RZR", inst: MULW, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x7f, 0xa, 0x1b}},
		{name: "MULW/src=R10,dst=R10", inst: MULW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x7d, 0xa, 0x1b}},
		{name: "MULW/src=R10,dst=R30", inst: MULW, src: RegR10, dst: RegR30, exp: []byte{0xde, 0x7f, 0xa, 0x1b}},
		{name: "MULW/src=R30,dst=RZR", inst: MULW, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x7f, 0x1e, 0x1b}},
		{name: "MULW/src=R30,dst=R10", inst: MULW, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x7d, 0x1e, 0x1b}},
		{name: "MULW/src=R30,dst=R30", inst: MULW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x7f, 0x1e, 0x1b}},
		{name: "NEG/src=RZR,dst=RZR", inst: NEG, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xcb}},
		{name: "NEG/src=RZR,dst=R10", inst: NEG, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0xcb}},
		{name: "NEG/src=RZR,dst=R30", inst: NEG, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0xcb}},
		{name: "NEG/src=R10,dst=RZR", inst: NEG, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xcb}},
		{name: "NEG/src=R10,dst=R10", inst: NEG, src: RegR10, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xcb}},
		{name: "NEG/src=R10,dst=R30", inst: NEG, src: RegR10, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xcb}},
		{name: "NEG/src=R30,dst=RZR", inst: NEG, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xcb}},
		{name: "NEG/src=R30,dst=R10", inst: NEG, src: RegR30, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xcb}},
		{name: "NEG/src=R30,dst=R30", inst: NEG, src: RegR30, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xcb}},
		{name: "NEGW/src=RZR,dst=RZR", inst: NEGW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x4b}},
		{name: "NEGW/src=RZR,dst=R10", inst: NEGW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x4b}},
		{name: "NEGW/src=RZR,dst=R30", inst: NEGW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x4b}},
		{name: "NEGW/src=R10,dst=RZR", inst: NEGW, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x4b}},
		{name: "NEGW/src=R10,dst=R10", inst: NEGW, src: RegR10, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x4b}},
		{name: "NEGW/src=R10,dst=R30", inst: NEGW, src: RegR10, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x4b}},
		{name: "NEGW/src=R30,dst=RZR", inst: NEGW, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x4b}},
		{name: "NEGW/src=R30,dst=R10", inst: NEGW, src: RegR30, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x4b}},
		{name: "NEGW/src=R30,dst=R30", inst: NEGW, src: RegR30, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x4b}},
		{name: "RBIT/src=RZR,dst=RZR", inst: RBIT, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xc0, 0xda}},
		{name: "RBIT/src=RZR,dst=R10", inst: RBIT, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xc0, 0xda}},
		{name: "RBIT/src=RZR,dst=R30", inst: RBIT, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xc0, 0xda}},
		{name: "RBIT/src=R10,dst=RZR", inst: RBIT, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xc0, 0xda}},
		{name: "RBIT/src=R10,dst=R10", inst: RBIT, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xc0, 0xda}},
		{name: "RBIT/src=R10,dst=R30", inst: RBIT, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xc0, 0xda}},
		{name: "RBIT/src=R30,dst=RZR", inst: RBIT, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xc0, 0xda}},
		{name: "RBIT/src=R30,dst=R10", inst: RBIT, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xc0, 0xda}},
		{name: "RBIT/src=R30,dst=R30", inst: RBIT, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xc0, 0xda}},
		{name: "RBITW/src=RZR,dst=RZR", inst: RBITW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xc0, 0x5a}},
		{name: "RBITW/src=RZR,dst=R10", inst: RBITW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xc0, 0x5a}},
		{name: "RBITW/src=RZR,dst=R30", inst: RBITW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xc0, 0x5a}},
		{name: "RBITW/src=R10,dst=RZR", inst: RBITW, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xc0, 0x5a}},
		{name: "RBITW/src=R10,dst=R10", inst: RBITW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xc0, 0x5a}},
		{name: "RBITW/src=R10,dst=R30", inst: RBITW, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xc0, 0x5a}},
		{name: "RBITW/src=R30,dst=RZR", inst: RBITW, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xc0, 0x5a}},
		{name: "RBITW/src=R30,dst=R10", inst: RBITW, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xc0, 0x5a}},
		{name: "RBITW/src=R30,dst=R30", inst: RBITW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xc0, 0x5a}},
		{name: "SDIV/src=RZR,dst=RZR", inst: SDIV, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xdf, 0x9a}},
		{name: "SDIV/src=RZR,dst=R10", inst: SDIV, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0xd, 0xdf, 0x9a}},
		{name: "SDIV/src=RZR,dst=R30", inst: SDIV, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0xf, 0xdf, 0x9a}},
		{name: "SDIV/src=R10,dst=RZR", inst: SDIV, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0xf, 0xca, 0x9a}},
		{name: "SDIV/src=R10,dst=R10", inst: SDIV, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xca, 0x9a}},
		{name: "SDIV/src=R10,dst=R30", inst: SDIV, src: RegR10, dst: RegR30, exp: []byte{0xde, 0xf, 0xca, 0x9a}},
		{name: "SDIV/src=R30,dst=RZR", inst: SDIV, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0xf, 0xde, 0x9a}},
		{name: "SDIV/src=R30,dst=R10", inst: SDIV, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0xd, 0xde, 0x9a}},
		{name: "SDIV/src=R30,dst=R30", inst: SDIV, src: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xde, 0x9a}},
		{name: "SDIVW/src=RZR,dst=RZR", inst: SDIVW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xdf, 0x1a}},
		{name: "SDIVW/src=RZR,dst=R10", inst: SDIVW, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0xd, 0xdf, 0x1a}},
		{name: "SDIVW/src=RZR,dst=R30", inst: SDIVW, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0xf, 0xdf, 0x1a}},
		{name: "SDIVW/src=R10,dst=RZR", inst: SDIVW, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0xf, 0xca, 0x1a}},
		{name: "SDIVW/src=R10,dst=R10", inst: SDIVW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xca, 0x1a}},
		{name: "SDIVW/src=R10,dst=R30", inst: SDIVW, src: RegR10, dst: RegR30, exp: []byte{0xde, 0xf, 0xca, 0x1a}},
		{name: "SDIVW/src=R30,dst=RZR", inst: SDIVW, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0xf, 0xde, 0x1a}},
		{name: "SDIVW/src=R30,dst=R10", inst: SDIVW, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0xd, 0xde, 0x1a}},
		{name: "SDIVW/src=R30,dst=R30", inst: SDIVW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xde, 0x1a}},
		{name: "UDIV/src=RZR,dst=RZR", inst: UDIV, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xdf, 0x9a}},
		{name: "UDIV/src=RZR,dst=R10", inst: UDIV, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x9, 0xdf, 0x9a}},
		{name: "UDIV/src=RZR,dst=R30", inst: UDIV, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0xb, 0xdf, 0x9a}},
		{name: "UDIV/src=R10,dst=RZR", inst: UDIV, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0xb, 0xca, 0x9a}},
		{name: "UDIV/src=R10,dst=R10", inst: UDIV, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xca, 0x9a}},
		{name: "UDIV/src=R10,dst=R30", inst: UDIV, src: RegR10, dst: RegR30, exp: []byte{0xde, 0xb, 0xca, 0x9a}},
		{name: "UDIV/src=R30,dst=RZR", inst: UDIV, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0xb, 0xde, 0x9a}},
		{name: "UDIV/src=R30,dst=R10", inst: UDIV, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x9, 0xde, 0x9a}},
		{name: "UDIV/src=R30,dst=R30", inst: UDIV, src: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xde, 0x9a}},
		{name: "UDIVW/src=RZR,dst=RZR", inst: UDIVW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xdf, 0x1a}},
		{name: "UDIVW/src=RZR,dst=R10", inst: UDIVW, src: RegRZR, dst: RegR10, exp: []byte{0x4a, 0x9, 0xdf, 0x1a}},
		{name: "UDIVW/src=RZR,dst=R30", inst: UDIVW, src: RegRZR, dst: RegR30, exp: []byte{0xde, 0xb, 0xdf, 0x1a}},
		{name: "UDIVW/src=R10,dst=RZR", inst: UDIVW, src: RegR10, dst: RegRZR, exp: []byte{0xff, 0xb, 0xca, 0x1a}},
		{name: "UDIVW/src=R10,dst=R10", inst: UDIVW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xca, 0x1a}},
		{name: "UDIVW/src=R10,dst=R30", inst: UDIVW, src: RegR10, dst: RegR30, exp: []byte{0xde, 0xb, 0xca, 0x1a}},
		{name: "UDIVW/src=R30,dst=RZR", inst: UDIVW, src: RegR30, dst: RegRZR, exp: []byte{0xff, 0xb, 0xde, 0x1a}},
		{name: "UDIVW/src=R30,dst=R10", inst: UDIVW, src: RegR30, dst: RegR10, exp: []byte{0x4a, 0x9, 0xde, 0x1a}},
		{name: "UDIVW/src=R30,dst=R30", inst: UDIVW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xde, 0x1a}},
		{name: "SCVTFD/src=RZR,dst=V0", inst: SCVTFD, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x62, 0x9e}},
		{name: "SCVTFD/src=RZR,dst=V31", inst: SCVTFD, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x62, 0x9e}},
		{name: "SCVTFD/src=R10,dst=V0", inst: SCVTFD, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x62, 0x9e}},
		{name: "SCVTFD/src=R10,dst=V31", inst: SCVTFD, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x62, 0x9e}},
		{name: "SCVTFD/src=R30,dst=V0", inst: SCVTFD, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x62, 0x9e}},
		{name: "SCVTFD/src=R30,dst=V31", inst: SCVTFD, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x62, 0x9e}},
		{name: "SCVTFWD/src=RZR,dst=V0", inst: SCVTFWD, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x62, 0x1e}},
		{name: "SCVTFWD/src=RZR,dst=V31", inst: SCVTFWD, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x62, 0x1e}},
		{name: "SCVTFWD/src=R10,dst=V0", inst: SCVTFWD, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x62, 0x1e}},
		{name: "SCVTFWD/src=R10,dst=V31", inst: SCVTFWD, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x62, 0x1e}},
		{name: "SCVTFWD/src=R30,dst=V0", inst: SCVTFWD, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x62, 0x1e}},
		{name: "SCVTFWD/src=R30,dst=V31", inst: SCVTFWD, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x62, 0x1e}},
		{name: "SCVTFS/src=RZR,dst=V0", inst: SCVTFS, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x22, 0x9e}},
		{name: "SCVTFS/src=RZR,dst=V31", inst: SCVTFS, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x22, 0x9e}},
		{name: "SCVTFS/src=R10,dst=V0", inst: SCVTFS, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x22, 0x9e}},
		{name: "SCVTFS/src=R10,dst=V31", inst: SCVTFS, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x22, 0x9e}},
		{name: "SCVTFS/src=R30,dst=V0", inst: SCVTFS, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x22, 0x9e}},
		{name: "SCVTFS/src=R30,dst=V31", inst: SCVTFS, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x22, 0x9e}},
		{name: "SCVTFWS/src=RZR,dst=V0", inst: SCVTFWS, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x22, 0x1e}},
		{name: "SCVTFWS/src=RZR,dst=V31", inst: SCVTFWS, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x22, 0x1e}},
		{name: "SCVTFWS/src=R10,dst=V0", inst: SCVTFWS, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x22, 0x1e}},
		{name: "SCVTFWS/src=R10,dst=V31", inst: SCVTFWS, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x22, 0x1e}},
		{name: "SCVTFWS/src=R30,dst=V0", inst: SCVTFWS, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x22, 0x1e}},
		{name: "SCVTFWS/src=R30,dst=V31", inst: SCVTFWS, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x22, 0x1e}},
		{name: "UCVTFD/src=RZR,dst=V0", inst: UCVTFD, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x63, 0x9e}},
		{name: "UCVTFD/src=RZR,dst=V31", inst: UCVTFD, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x63, 0x9e}},
		{name: "UCVTFD/src=R10,dst=V0", inst: UCVTFD, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x63, 0x9e}},
		{name: "UCVTFD/src=R10,dst=V31", inst: UCVTFD, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x63, 0x9e}},
		{name: "UCVTFD/src=R30,dst=V0", inst: UCVTFD, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x63, 0x9e}},
		{name: "UCVTFD/src=R30,dst=V31", inst: UCVTFD, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x63, 0x9e}},
		{name: "UCVTFWD/src=RZR,dst=V0", inst: UCVTFWD, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x63, 0x1e}},
		{name: "UCVTFWD/src=RZR,dst=V31", inst: UCVTFWD, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x63, 0x1e}},
		{name: "UCVTFWD/src=R10,dst=V0", inst: UCVTFWD, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x63, 0x1e}},
		{name: "UCVTFWD/src=R10,dst=V31", inst: UCVTFWD, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x63, 0x1e}},
		{name: "UCVTFWD/src=R30,dst=V0", inst: UCVTFWD, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x63, 0x1e}},
		{name: "UCVTFWD/src=R30,dst=V31", inst: UCVTFWD, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x63, 0x1e}},
		{name: "UCVTFS/src=RZR,dst=V0", inst: UCVTFS, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x23, 0x9e}},
		{name: "UCVTFS/src=RZR,dst=V31", inst: UCVTFS, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x23, 0x9e}},
		{name: "UCVTFS/src=R10,dst=V0", inst: UCVTFS, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x23, 0x9e}},
		{name: "UCVTFS/src=R10,dst=V31", inst: UCVTFS, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x23, 0x9e}},
		{name: "UCVTFS/src=R30,dst=V0", inst: UCVTFS, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x23, 0x9e}},
		{name: "UCVTFS/src=R30,dst=V31", inst: UCVTFS, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x23, 0x9e}},
		{name: "UCVTFWS/src=RZR,dst=V0", inst: UCVTFWS, src: RegRZR, dst: RegV0, exp: []byte{0xe0, 0x3, 0x23, 0x1e}},
		{name: "UCVTFWS/src=RZR,dst=V31", inst: UCVTFWS, src: RegRZR, dst: RegV31, exp: []byte{0xff, 0x3, 0x23, 0x1e}},
		{name: "UCVTFWS/src=R10,dst=V0", inst: UCVTFWS, src: RegR10, dst: RegV0, exp: []byte{0x40, 0x1, 0x23, 0x1e}},
		{name: "UCVTFWS/src=R10,dst=V31", inst: UCVTFWS, src: RegR10, dst: RegV31, exp: []byte{0x5f, 0x1, 0x23, 0x1e}},
		{name: "UCVTFWS/src=R30,dst=V0", inst: UCVTFWS, src: RegR30, dst: RegV0, exp: []byte{0xc0, 0x3, 0x23, 0x1e}},
		{name: "UCVTFWS/src=R30,dst=V31", inst: UCVTFWS, src: RegR30, dst: RegV31, exp: []byte{0xdf, 0x3, 0x23, 0x1e}},
		{name: "SXTB/src=RZR,dst=RZR", inst: SXTB, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "SXTB/src=RZR,dst=R10", inst: SXTB, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "SXTB/src=RZR,dst=R30", inst: SXTB, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "SXTB/src=R10,dst=RZR", inst: SXTB, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1d, 0x40, 0x93}},
		{name: "SXTB/src=R10,dst=R10", inst: SXTB, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1d, 0x40, 0x93}},
		{name: "SXTB/src=R10,dst=R30", inst: SXTB, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1d, 0x40, 0x93}},
		{name: "SXTB/src=R30,dst=RZR", inst: SXTB, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x1f, 0x40, 0x93}},
		{name: "SXTB/src=R30,dst=R10", inst: SXTB, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x1f, 0x40, 0x93}},
		{name: "SXTB/src=R30,dst=R30", inst: SXTB, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x1f, 0x40, 0x93}},
		{name: "SXTBW/src=RZR,dst=RZR", inst: SXTBW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "SXTBW/src=RZR,dst=R10", inst: SXTBW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "SXTBW/src=RZR,dst=R30", inst: SXTBW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "SXTBW/src=R10,dst=RZR", inst: SXTBW, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1d, 0x0, 0x13}},
		{name: "SXTBW/src=R10,dst=R10", inst: SXTBW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1d, 0x0, 0x13}},
		{name: "SXTBW/src=R10,dst=R30", inst: SXTBW, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1d, 0x0, 0x13}},
		{name: "SXTBW/src=R30,dst=RZR", inst: SXTBW, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x1f, 0x0, 0x13}},
		{name: "SXTBW/src=R30,dst=R10", inst: SXTBW, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x1f, 0x0, 0x13}},
		{name: "SXTBW/src=R30,dst=R30", inst: SXTBW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x1f, 0x0, 0x13}},
		{name: "SXTH/src=RZR,dst=RZR", inst: SXTH, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "SXTH/src=RZR,dst=R10", inst: SXTH, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "SXTH/src=RZR,dst=R30", inst: SXTH, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "SXTH/src=R10,dst=RZR", inst: SXTH, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x3d, 0x40, 0x93}},
		{name: "SXTH/src=R10,dst=R10", inst: SXTH, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x3d, 0x40, 0x93}},
		{name: "SXTH/src=R10,dst=R30", inst: SXTH, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x3d, 0x40, 0x93}},
		{name: "SXTH/src=R30,dst=RZR", inst: SXTH, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3f, 0x40, 0x93}},
		{name: "SXTH/src=R30,dst=R10", inst: SXTH, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x3f, 0x40, 0x93}},
		{name: "SXTH/src=R30,dst=R30", inst: SXTH, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3f, 0x40, 0x93}},
		{name: "SXTHW/src=RZR,dst=RZR", inst: SXTHW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "SXTHW/src=RZR,dst=R10", inst: SXTHW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "SXTHW/src=RZR,dst=R30", inst: SXTHW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "SXTHW/src=R10,dst=RZR", inst: SXTHW, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x3d, 0x0, 0x13}},
		{name: "SXTHW/src=R10,dst=R10", inst: SXTHW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x3d, 0x0, 0x13}},
		{name: "SXTHW/src=R10,dst=R30", inst: SXTHW, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x3d, 0x0, 0x13}},
		{name: "SXTHW/src=R30,dst=RZR", inst: SXTHW, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3f, 0x0, 0x13}},
		{name: "SXTHW/src=R30,dst=R10", inst: SXTHW, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x3f, 0x0, 0x13}},
		{name: "SXTHW/src=R30,dst=R30", inst: SXTHW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x3f, 0x0, 0x13}},
		{name: "SXTW/src=RZR,dst=RZR", inst: SXTW, src: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "SXTW/src=RZR,dst=R10", inst: SXTW, src: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "SXTW/src=RZR,dst=R30", inst: SXTW, src: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "SXTW/src=R10,dst=RZR", inst: SXTW, src: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x7d, 0x40, 0x93}},
		{name: "SXTW/src=R10,dst=R10", inst: SXTW, src: RegR10, dst: RegR10, exp: []byte{0x4a, 0x7d, 0x40, 0x93}},
		{name: "SXTW/src=R10,dst=R30", inst: SXTW, src: RegR10, dst: RegR30, exp: []byte{0x5e, 0x7d, 0x40, 0x93}},
		{name: "SXTW/src=R30,dst=RZR", inst: SXTW, src: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x7f, 0x40, 0x93}},
		{name: "SXTW/src=R30,dst=R10", inst: SXTW, src: RegR30, dst: RegR10, exp: []byte{0xca, 0x7f, 0x40, 0x93}},
		{name: "SXTW/src=R30,dst=R30", inst: SXTW, src: RegR30, dst: RegR30, exp: []byte{0xde, 0x7f, 0x40, 0x93}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.encodeRegisterToRegister(&NodeImpl{Instruction: tc.inst, SrcReg: tc.src, DstReg: tc.dst})
			require.NoError(t, err)

			actual := a.Bytes()
			require.Equal(t, tc.exp, actual[:4])
		})
	}
}

func TestAssemblerImpl_EncodeTwoRegistersToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *NodeImpl
			expErr string
		}{
			{
				n: &NodeImpl{Instruction: ADR, Types: OperandTypesTwoRegistersToRegister,
					SrcReg: RegR0, SrcReg2: RegR0, DstReg: RegR0},
				expErr: "ADR is unsupported for from:two-registers,to:register type",
			},
		}

		for _, tt := range tests {
			tc := tt
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoRegistersToRegister(tc.n)
			require.EqualError(t, err, tc.expErr)
		}
	})

	tests := []struct {
		name           string
		inst           asm.Instruction
		src, src2, dst asm.Register
		exp            []byte
	}{
		{name: "src=RZR,src2=RZR,dst=RZR", inst: AND, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x8a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: AND, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x8a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: AND, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: AND, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: AND, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: AND, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: AND, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: AND, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0x8a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: AND, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0x8a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: AND, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x8a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: AND, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x8a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: AND, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x8a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: AND, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0x8a}},
		{name: "src=R10,src2=R10,dst=R10", inst: AND, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0x8a}},
		{name: "src=R10,src2=R10,dst=R30", inst: AND, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0x8a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: AND, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0x8a}},
		{name: "src=R10,src2=R30,dst=R10", inst: AND, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0x8a}},
		{name: "src=R10,src2=R30,dst=R30", inst: AND, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0x8a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: AND, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x8a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: AND, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x8a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: AND, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x8a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: AND, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0x8a}},
		{name: "src=R30,src2=R10,dst=R10", inst: AND, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0x8a}},
		{name: "src=R30,src2=R10,dst=R30", inst: AND, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0x8a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: AND, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0x8a}},
		{name: "src=R30,src2=R30,dst=R10", inst: AND, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0x8a}},
		{name: "src=R30,src2=R30,dst=R30", inst: AND, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0x8a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ANDW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xa}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ANDW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0xa}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ANDW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0xa}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ANDW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0xa}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ANDW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xa}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ANDW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0xa}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ANDW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0xa}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ANDW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0xa}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ANDW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xa}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ANDW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xa}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ANDW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xa}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ANDW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xa}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ANDW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0xa}},
		{name: "src=R10,src2=R10,dst=R10", inst: ANDW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xa}},
		{name: "src=R10,src2=R10,dst=R30", inst: ANDW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0xa}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ANDW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0xa}},
		{name: "src=R10,src2=R30,dst=R10", inst: ANDW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0xa}},
		{name: "src=R10,src2=R30,dst=R30", inst: ANDW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xa}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ANDW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xa}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ANDW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xa}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ANDW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xa}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ANDW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0xa}},
		{name: "src=R30,src2=R10,dst=R10", inst: ANDW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xa}},
		{name: "src=R30,src2=R10,dst=R30", inst: ANDW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0xa}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ANDW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0xa}},
		{name: "src=R30,src2=R30,dst=R10", inst: ANDW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0xa}},
		{name: "src=R30,src2=R30,dst=R30", inst: ANDW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xa}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ORR, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xaa}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ORR, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0xaa}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ORR, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ORR, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ORR, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ORR, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ORR, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ORR, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0xaa}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ORR, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xaa}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ORR, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xaa}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ORR, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xaa}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ORR, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xaa}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ORR, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0xaa}},
		{name: "src=R10,src2=R10,dst=R10", inst: ORR, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xaa}},
		{name: "src=R10,src2=R10,dst=R30", inst: ORR, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0xaa}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ORR, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0xaa}},
		{name: "src=R10,src2=R30,dst=R10", inst: ORR, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0xaa}},
		{name: "src=R10,src2=R30,dst=R30", inst: ORR, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xaa}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ORR, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xaa}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ORR, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xaa}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ORR, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xaa}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ORR, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0xaa}},
		{name: "src=R30,src2=R10,dst=R10", inst: ORR, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xaa}},
		{name: "src=R30,src2=R10,dst=R30", inst: ORR, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0xaa}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ORR, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0xaa}},
		{name: "src=R30,src2=R30,dst=R10", inst: ORR, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0xaa}},
		{name: "src=R30,src2=R30,dst=R30", inst: ORR, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xaa}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ORRW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x2a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ORRW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x2a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ORRW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ORRW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ORRW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ORRW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ORRW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ORRW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0x2a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ORRW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0x2a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ORRW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x2a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ORRW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x2a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ORRW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x2a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ORRW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0x2a}},
		{name: "src=R10,src2=R10,dst=R10", inst: ORRW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0x2a}},
		{name: "src=R10,src2=R10,dst=R30", inst: ORRW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0x2a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ORRW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0x2a}},
		{name: "src=R10,src2=R30,dst=R10", inst: ORRW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0x2a}},
		{name: "src=R10,src2=R30,dst=R30", inst: ORRW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0x2a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ORRW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x2a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ORRW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x2a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ORRW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x2a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ORRW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0x2a}},
		{name: "src=R30,src2=R10,dst=R10", inst: ORRW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0x2a}},
		{name: "src=R30,src2=R10,dst=R30", inst: ORRW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0x2a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ORRW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0x2a}},
		{name: "src=R30,src2=R30,dst=R10", inst: ORRW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0x2a}},
		{name: "src=R30,src2=R30,dst=R30", inst: ORRW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0x2a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: EOR, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xca}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: EOR, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0xca}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: EOR, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0xca}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: EOR, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0xca}},
		{name: "src=RZR,src2=R10,dst=R10", inst: EOR, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xca}},
		{name: "src=RZR,src2=R10,dst=R30", inst: EOR, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0xca}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: EOR, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0xca}},
		{name: "src=RZR,src2=R30,dst=R10", inst: EOR, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0xca}},
		{name: "src=RZR,src2=R30,dst=R30", inst: EOR, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xca}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: EOR, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xca}},
		{name: "src=R10,src2=RZR,dst=R10", inst: EOR, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xca}},
		{name: "src=R10,src2=RZR,dst=R30", inst: EOR, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xca}},
		{name: "src=R10,src2=R10,dst=RZR", inst: EOR, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0xca}},
		{name: "src=R10,src2=R10,dst=R10", inst: EOR, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xca}},
		{name: "src=R10,src2=R10,dst=R30", inst: EOR, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0xca}},
		{name: "src=R10,src2=R30,dst=RZR", inst: EOR, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0xca}},
		{name: "src=R10,src2=R30,dst=R10", inst: EOR, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0xca}},
		{name: "src=R10,src2=R30,dst=R30", inst: EOR, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xca}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: EOR, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xca}},
		{name: "src=R30,src2=RZR,dst=R10", inst: EOR, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xca}},
		{name: "src=R30,src2=RZR,dst=R30", inst: EOR, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xca}},
		{name: "src=R30,src2=R10,dst=RZR", inst: EOR, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0xca}},
		{name: "src=R30,src2=R10,dst=R10", inst: EOR, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xca}},
		{name: "src=R30,src2=R10,dst=R30", inst: EOR, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0xca}},
		{name: "src=R30,src2=R30,dst=RZR", inst: EOR, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0xca}},
		{name: "src=R30,src2=R30,dst=R10", inst: EOR, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0xca}},
		{name: "src=R30,src2=R30,dst=R30", inst: EOR, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xca}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: EORW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x4a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: EORW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x4a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: EORW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: EORW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: EORW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: EORW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: EORW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: EORW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0x4a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: EORW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0x4a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: EORW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x4a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: EORW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x4a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: EORW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x4a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: EORW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0x4a}},
		{name: "src=R10,src2=R10,dst=R10", inst: EORW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0x4a}},
		{name: "src=R10,src2=R10,dst=R30", inst: EORW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0x4a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: EORW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0x4a}},
		{name: "src=R10,src2=R30,dst=R10", inst: EORW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0x4a}},
		{name: "src=R10,src2=R30,dst=R30", inst: EORW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0x4a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: EORW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x4a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: EORW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x4a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: EORW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x4a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: EORW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0x4a}},
		{name: "src=R30,src2=R10,dst=R10", inst: EORW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0x4a}},
		{name: "src=R30,src2=R10,dst=R30", inst: EORW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0x4a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: EORW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0x4a}},
		{name: "src=R30,src2=R30,dst=R10", inst: EORW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0x4a}},
		{name: "src=R30,src2=R30,dst=R30", inst: EORW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0x4a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ASR, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ASR, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ASR, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ASR, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ASR, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ASR, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ASR, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ASR, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ASR, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ASR, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ASR, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ASR, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ASR, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: ASR, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: ASR, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ASR, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: ASR, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: ASR, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ASR, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ASR, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ASR, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ASR, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: ASR, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: ASR, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ASR, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: ASR, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: ASR, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ASRW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ASRW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ASRW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ASRW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ASRW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ASRW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ASRW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ASRW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ASRW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ASRW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ASRW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ASRW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ASRW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: ASRW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: ASRW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ASRW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: ASRW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: ASRW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ASRW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2b, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ASRW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2b, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ASRW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2b, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ASRW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x29, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: ASRW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x29, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: ASRW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x29, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ASRW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2b, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: ASRW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2b, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: ASRW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2b, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: LSL, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: LSL, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: LSL, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: LSL, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: LSL, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: LSL, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: LSL, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: LSL, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: LSL, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: LSL, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: LSL, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: LSL, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: LSL, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: LSL, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: LSL, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: LSL, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: LSL, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: LSL, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: LSL, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: LSL, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: LSL, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: LSL, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: LSL, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: LSL, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: LSL, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: LSL, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: LSL, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: LSLW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: LSLW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: LSLW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: LSLW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: LSLW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: LSLW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: LSLW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: LSLW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: LSLW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: LSLW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: LSLW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: LSLW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: LSLW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: LSLW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: LSLW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: LSLW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: LSLW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: LSLW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: LSLW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x23, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: LSLW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x23, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: LSLW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x23, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: LSLW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x21, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: LSLW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x21, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: LSLW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x21, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: LSLW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x23, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: LSLW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x23, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: LSLW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x23, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: LSR, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: LSR, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: LSR, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: LSR, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: LSR, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: LSR, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: LSR, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: LSR, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: LSR, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: LSR, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: LSR, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: LSR, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: LSR, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: LSR, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: LSR, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: LSR, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: LSR, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: LSR, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: LSR, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: LSR, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: LSR, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: LSR, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: LSR, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: LSR, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: LSR, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: LSR, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: LSR, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: LSRW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: LSRW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: LSRW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: LSRW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: LSRW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: LSRW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: LSRW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: LSRW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: LSRW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: LSRW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: LSRW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: LSRW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: LSRW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: LSRW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: LSRW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: LSRW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: LSRW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: LSRW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: LSRW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x27, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: LSRW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x27, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: LSRW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x27, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: LSRW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x25, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: LSRW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x25, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: LSRW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x25, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: LSRW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x27, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: LSRW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x27, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: LSRW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x27, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: ROR, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: ROR, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: ROR, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: ROR, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: ROR, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: ROR, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: ROR, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: ROR, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: ROR, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: ROR, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: ROR, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: ROR, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: ROR, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: ROR, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: ROR, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: ROR, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: ROR, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: ROR, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: ROR, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: ROR, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: ROR, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: ROR, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: ROR, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: ROR, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: ROR, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: ROR, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: ROR, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: RORW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: RORW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: RORW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: RORW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: RORW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: RORW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: RORW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: RORW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: RORW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: RORW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: RORW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: RORW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: RORW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: RORW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: RORW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: RORW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: RORW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: RORW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: RORW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x2f, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: RORW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x2f, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: RORW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x2f, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: RORW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x2d, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: RORW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x2d, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: RORW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x2d, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: RORW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x2f, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: RORW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x2f, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: RORW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x2f, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: SDIV, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: SDIV, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: SDIV, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: SDIV, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: SDIV, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: SDIV, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: SDIV, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: SDIV, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: SDIV, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: SDIV, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: SDIV, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: SDIV, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: SDIV, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: SDIV, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: SDIV, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: SDIV, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: SDIV, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: SDIV, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: SDIV, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: SDIV, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: SDIV, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: SDIV, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: SDIV, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: SDIV, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: SDIV, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: SDIV, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: SDIV, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: SDIVW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: SDIVW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: SDIVW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: SDIVW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: SDIVW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: SDIVW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: SDIVW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: SDIVW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: SDIVW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: SDIVW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: SDIVW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: SDIVW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: SDIVW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: SDIVW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: SDIVW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: SDIVW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: SDIVW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: SDIVW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: SDIVW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xf, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: SDIVW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xf, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: SDIVW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xf, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: SDIVW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0xd, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: SDIVW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0xd, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: SDIVW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0xd, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: SDIVW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xf, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: SDIVW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xf, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: SDIVW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xf, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: UDIV, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: UDIV, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xdf, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: UDIV, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: UDIV, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: UDIV, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: UDIV, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: UDIV, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: UDIV, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xdf, 0x9a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: UDIV, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xdf, 0x9a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: UDIV, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: UDIV, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xca, 0x9a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: UDIV, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: UDIV, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R10", inst: UDIV, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xca, 0x9a}},
		{name: "src=R10,src2=R10,dst=R30", inst: UDIV, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: UDIV, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R10", inst: UDIV, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xca, 0x9a}},
		{name: "src=R10,src2=R30,dst=R30", inst: UDIV, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xca, 0x9a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: UDIV, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: UDIV, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xde, 0x9a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: UDIV, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: UDIV, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R10", inst: UDIV, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xde, 0x9a}},
		{name: "src=R30,src2=R10,dst=R30", inst: UDIV, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: UDIV, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R10", inst: UDIV, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xde, 0x9a}},
		{name: "src=R30,src2=R30,dst=R30", inst: UDIV, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xde, 0x9a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: UDIVW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: UDIVW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xdf, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: UDIVW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: UDIVW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R10", inst: UDIVW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R10,dst=R30", inst: UDIVW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: UDIVW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R10", inst: UDIVW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xdf, 0x1a}},
		{name: "src=RZR,src2=R30,dst=R30", inst: UDIVW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xdf, 0x1a}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: UDIVW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R10", inst: UDIVW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xca, 0x1a}},
		{name: "src=R10,src2=RZR,dst=R30", inst: UDIVW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=RZR", inst: UDIVW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R10", inst: UDIVW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xca, 0x1a}},
		{name: "src=R10,src2=R10,dst=R30", inst: UDIVW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=RZR", inst: UDIVW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R10", inst: UDIVW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xca, 0x1a}},
		{name: "src=R10,src2=R30,dst=R30", inst: UDIVW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xca, 0x1a}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: UDIVW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0xb, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R10", inst: UDIVW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0xb, 0xde, 0x1a}},
		{name: "src=R30,src2=RZR,dst=R30", inst: UDIVW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0xb, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=RZR", inst: UDIVW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x9, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R10", inst: UDIVW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x9, 0xde, 0x1a}},
		{name: "src=R30,src2=R10,dst=R30", inst: UDIVW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x9, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=RZR", inst: UDIVW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0xb, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R10", inst: UDIVW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0xb, 0xde, 0x1a}},
		{name: "src=R30,src2=R30,dst=R30", inst: UDIVW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0xb, 0xde, 0x1a}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: SUB, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0xcb}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: SUB, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0xcb}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: SUB, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: SUB, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R10,dst=R10", inst: SUB, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R10,dst=R30", inst: SUB, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: SUB, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R30,dst=R10", inst: SUB, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0xcb}},
		{name: "src=RZR,src2=R30,dst=R30", inst: SUB, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0xcb}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: SUB, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0xcb}},
		{name: "src=R10,src2=RZR,dst=R10", inst: SUB, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0xcb}},
		{name: "src=R10,src2=RZR,dst=R30", inst: SUB, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0xcb}},
		{name: "src=R10,src2=R10,dst=RZR", inst: SUB, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0xcb}},
		{name: "src=R10,src2=R10,dst=R10", inst: SUB, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0xcb}},
		{name: "src=R10,src2=R10,dst=R30", inst: SUB, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0xcb}},
		{name: "src=R10,src2=R30,dst=RZR", inst: SUB, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0xcb}},
		{name: "src=R10,src2=R30,dst=R10", inst: SUB, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0xcb}},
		{name: "src=R10,src2=R30,dst=R30", inst: SUB, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0xcb}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: SUB, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0xcb}},
		{name: "src=R30,src2=RZR,dst=R10", inst: SUB, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0xcb}},
		{name: "src=R30,src2=RZR,dst=R30", inst: SUB, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0xcb}},
		{name: "src=R30,src2=R10,dst=RZR", inst: SUB, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0xcb}},
		{name: "src=R30,src2=R10,dst=R10", inst: SUB, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0xcb}},
		{name: "src=R30,src2=R10,dst=R30", inst: SUB, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0xcb}},
		{name: "src=R30,src2=R30,dst=RZR", inst: SUB, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0xcb}},
		{name: "src=R30,src2=R30,dst=R10", inst: SUB, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0xcb}},
		{name: "src=R30,src2=R30,dst=R30", inst: SUB, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0xcb}},
		{name: "src=RZR,src2=RZR,dst=RZR", inst: SUBW, src: RegRZR, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1f, 0x4b}},
		{name: "src=RZR,src2=RZR,dst=R10", inst: SUBW, src: RegRZR, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1f, 0x4b}},
		{name: "src=RZR,src2=RZR,dst=R30", inst: SUBW, src: RegRZR, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R10,dst=RZR", inst: SUBW, src: RegRZR, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R10,dst=R10", inst: SUBW, src: RegRZR, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R10,dst=R30", inst: SUBW, src: RegRZR, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R30,dst=RZR", inst: SUBW, src: RegRZR, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R30,dst=R10", inst: SUBW, src: RegRZR, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1f, 0x4b}},
		{name: "src=RZR,src2=R30,dst=R30", inst: SUBW, src: RegRZR, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1f, 0x4b}},
		{name: "src=R10,src2=RZR,dst=RZR", inst: SUBW, src: RegR10, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0xa, 0x4b}},
		{name: "src=R10,src2=RZR,dst=R10", inst: SUBW, src: RegR10, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0xa, 0x4b}},
		{name: "src=R10,src2=RZR,dst=R30", inst: SUBW, src: RegR10, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0xa, 0x4b}},
		{name: "src=R10,src2=R10,dst=RZR", inst: SUBW, src: RegR10, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0xa, 0x4b}},
		{name: "src=R10,src2=R10,dst=R10", inst: SUBW, src: RegR10, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0xa, 0x4b}},
		{name: "src=R10,src2=R10,dst=R30", inst: SUBW, src: RegR10, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0xa, 0x4b}},
		{name: "src=R10,src2=R30,dst=RZR", inst: SUBW, src: RegR10, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0xa, 0x4b}},
		{name: "src=R10,src2=R30,dst=R10", inst: SUBW, src: RegR10, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0xa, 0x4b}},
		{name: "src=R10,src2=R30,dst=R30", inst: SUBW, src: RegR10, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0xa, 0x4b}},
		{name: "src=R30,src2=RZR,dst=RZR", inst: SUBW, src: RegR30, src2: RegRZR, dst: RegRZR, exp: []byte{0xff, 0x3, 0x1e, 0x4b}},
		{name: "src=R30,src2=RZR,dst=R10", inst: SUBW, src: RegR30, src2: RegRZR, dst: RegR10, exp: []byte{0xea, 0x3, 0x1e, 0x4b}},
		{name: "src=R30,src2=RZR,dst=R30", inst: SUBW, src: RegR30, src2: RegRZR, dst: RegR30, exp: []byte{0xfe, 0x3, 0x1e, 0x4b}},
		{name: "src=R30,src2=R10,dst=RZR", inst: SUBW, src: RegR30, src2: RegR10, dst: RegRZR, exp: []byte{0x5f, 0x1, 0x1e, 0x4b}},
		{name: "src=R30,src2=R10,dst=R10", inst: SUBW, src: RegR30, src2: RegR10, dst: RegR10, exp: []byte{0x4a, 0x1, 0x1e, 0x4b}},
		{name: "src=R30,src2=R10,dst=R30", inst: SUBW, src: RegR30, src2: RegR10, dst: RegR30, exp: []byte{0x5e, 0x1, 0x1e, 0x4b}},
		{name: "src=R30,src2=R30,dst=RZR", inst: SUBW, src: RegR30, src2: RegR30, dst: RegRZR, exp: []byte{0xdf, 0x3, 0x1e, 0x4b}},
		{name: "src=R30,src2=R30,dst=R10", inst: SUBW, src: RegR30, src2: RegR30, dst: RegR10, exp: []byte{0xca, 0x3, 0x1e, 0x4b}},
		{name: "src=R30,src2=R30,dst=R30", inst: SUBW, src: RegR30, src2: RegR30, dst: RegR30, exp: []byte{0xde, 0x3, 0x1e, 0x4b}},
		{name: "src=V0,src2=V0,dst=V0", inst: FSUBD, src: RegV0, src2: RegV0, dst: RegV0, exp: []byte{0x0, 0x38, 0x60, 0x1e}},
		{name: "src=V0,src2=V0,dst=V31", inst: FSUBD, src: RegV0, src2: RegV0, dst: RegV31, exp: []byte{0x1f, 0x38, 0x60, 0x1e}},
		{name: "src=V0,src2=V31,dst=V0", inst: FSUBD, src: RegV0, src2: RegV31, dst: RegV0, exp: []byte{0xe0, 0x3b, 0x60, 0x1e}},
		{name: "src=V0,src2=V31,dst=V31", inst: FSUBD, src: RegV0, src2: RegV31, dst: RegV31, exp: []byte{0xff, 0x3b, 0x60, 0x1e}},
		{name: "src=V31,src2=V0,dst=V0", inst: FSUBD, src: RegV31, src2: RegV0, dst: RegV0, exp: []byte{0x0, 0x38, 0x7f, 0x1e}},
		{name: "src=V31,src2=V0,dst=V31", inst: FSUBD, src: RegV31, src2: RegV0, dst: RegV31, exp: []byte{0x1f, 0x38, 0x7f, 0x1e}},
		{name: "src=V31,src2=V31,dst=V0", inst: FSUBD, src: RegV31, src2: RegV31, dst: RegV0, exp: []byte{0xe0, 0x3b, 0x7f, 0x1e}},
		{name: "src=V31,src2=V31,dst=V31", inst: FSUBD, src: RegV31, src2: RegV31, dst: RegV31, exp: []byte{0xff, 0x3b, 0x7f, 0x1e}},
		{name: "src=V0,src2=V0,dst=V0", inst: FSUBS, src: RegV0, src2: RegV0, dst: RegV0, exp: []byte{0x0, 0x38, 0x20, 0x1e}},
		{name: "src=V0,src2=V0,dst=V31", inst: FSUBS, src: RegV0, src2: RegV0, dst: RegV31, exp: []byte{0x1f, 0x38, 0x20, 0x1e}},
		{name: "src=V0,src2=V31,dst=V0", inst: FSUBS, src: RegV0, src2: RegV31, dst: RegV0, exp: []byte{0xe0, 0x3b, 0x20, 0x1e}},
		{name: "src=V0,src2=V31,dst=V31", inst: FSUBS, src: RegV0, src2: RegV31, dst: RegV31, exp: []byte{0xff, 0x3b, 0x20, 0x1e}},
		{name: "src=V31,src2=V0,dst=V0", inst: FSUBS, src: RegV31, src2: RegV0, dst: RegV0, exp: []byte{0x0, 0x38, 0x3f, 0x1e}},
		{name: "src=V31,src2=V0,dst=V31", inst: FSUBS, src: RegV31, src2: RegV0, dst: RegV31, exp: []byte{0x1f, 0x38, 0x3f, 0x1e}},
		{name: "src=V31,src2=V31,dst=V0", inst: FSUBS, src: RegV31, src2: RegV31, dst: RegV0, exp: []byte{0xe0, 0x3b, 0x3f, 0x1e}},
		{name: "src=V31,src2=V31,dst=V31", inst: FSUBS, src: RegV31, src2: RegV31, dst: RegV31, exp: []byte{0xff, 0x3b, 0x3f, 0x1e}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl(asm.NilRegister)
			err := a.EncodeTwoRegistersToRegister(&NodeImpl{Instruction: tc.inst, SrcReg: tc.src, SrcReg2: tc.src2, DstReg: tc.dst})
			require.NoError(t, err)

			actual := a.Bytes()
			require.Equal(t, tc.exp, actual[:4])
		})
	}
}
