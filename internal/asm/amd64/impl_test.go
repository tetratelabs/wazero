package asm_amd64

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/asm"
)

func TestNodeImpl_AssignJumpTarget(t *testing.T) {
	a := uint64(0b1110)
	fmt.Println(-a)
	fmt.Printf("0b%b\n", -a)
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
			in:  &NodeImpl{Instruction: NOP},
			exp: "NOP",
		},
		{
			in:  &NodeImpl{Instruction: SETCC, Types: OperandTypesNoneToRegister, DstReg: REG_AX},
			exp: "SETCC AX",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: REG_AX, DstConst: 100},
			exp: "JMP [AX + 0x64]",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: REG_AX, DstConst: 100, DstMemScale: 8, DstMemIndex: REG_R11},
			exp: "JMP [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToBranch, JumpTarget: &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: REG_AX, DstConst: 100}},
			exp: "JMP {JMP [AX + 0x64]}",
		},
		{
			in:  &NodeImpl{Instruction: IDIVQ, Types: OperandTypesRegisterToNone, SrcReg: REG_DX},
			exp: "IDIVQ DX",
		},
		{
			in:  &NodeImpl{Instruction: ADDL, Types: OperandTypesRegisterToRegister, SrcReg: REG_DX, DstReg: REG_R14},
			exp: "ADDL DX, R14",
		},
		{
			in: &NodeImpl{Instruction: MOVQ, Types: OperandTypesRegisterToMemory,
				SrcReg: REG_DX, DstReg: REG_R14, DstConst: 100},
			exp: "MOVQ DX, [R14 + 0x64]",
		},
		{
			in: &NodeImpl{Instruction: MOVQ, Types: OperandTypesRegisterToMemory,
				SrcReg: REG_DX, DstReg: REG_R14, DstConst: 100, DstMemIndex: REG_CX, DstMemScale: 4},
			exp: "MOVQ DX, [R14 + 0x64 + CX*0x4]",
		},
		{
			in: &NodeImpl{Instruction: CMPL, Types: OperandTypesRegisterToConst,
				SrcReg: REG_DX, DstConst: 100},
			exp: "CMPL DX, 0x64",
		},
		{
			in: &NodeImpl{Instruction: MOVL, Types: OperandTypesMemoryToRegister,
				SrcReg: REG_DX, SrcConst: 1, DstReg: REG_AX},
			exp: "MOVL [DX + 0x1], AX",
		},
		{
			in: &NodeImpl{Instruction: MOVL, Types: OperandTypesMemoryToRegister,
				SrcReg: REG_DX, SrcConst: 1, SrcMemIndex: REG_R12, SrcMemScale: 2,
				DstReg: REG_AX},
			exp: "MOVL [DX + 1 + R12*0x2], AX",
		},
		{
			in: &NodeImpl{Instruction: CMPQ, Types: OperandTypesMemoryToConst,
				SrcReg: REG_DX, SrcConst: 1, SrcMemIndex: REG_R12, SrcMemScale: 2,
				DstConst: 123},
			exp: "CMPQ [DX + 1 + R12*0x2], 0x7b",
		},
		{
			in:  &NodeImpl{Instruction: MOVQ, Types: OperandTypesConstToMemory, SrcConst: 123, DstReg: REG_AX, DstConst: 100, DstMemScale: 8, DstMemIndex: REG_R11},
			exp: "MOVQ 0x7b, [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &NodeImpl{Instruction: MOVQ, Types: OperandTypesConstToRegister, SrcConst: 123, DstReg: REG_AX},
			exp: "MOVQ 0x7b, AX",
		},
	} {
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := NewAssemblerImpl()

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
	a := NewAssemblerImpl()
	actual := a.newNode(ADDL, OperandTypesConstToMemory)
	require.Equal(t, ADDL, actual.Instruction)
	require.Equal(t, OperandTypeConst, actual.Types.src)
	require.Equal(t, OperandTypeMemory, actual.Types.dst)
	require.Equal(t, actual, a.Root)
	require.Equal(t, actual, a.Current)
}

func TestAssemblerImpl_encodeNode(t *testing.T) {
	a := NewAssemblerImpl()
	err := a.EncodeNode(&NodeImpl{
		Types: OperandTypes{OperandTypeBranch, OperandTypeRegister},
	})
	require.EqualError(t, err, "encoder undefined for [from:branch,to:register] operand type")
}

func TestAssemblerImpl_padNOP(t *testing.T) {
	for _, tc := range []struct {
		num      int
		expected []byte
	}{
		{num: 1, expected: nopOpcodes[0][:1]},
		{num: 2, expected: nopOpcodes[1][:2]},
		{num: 3, expected: nopOpcodes[2][:3]},
		{num: 4, expected: nopOpcodes[3][:4]},
		{num: 5, expected: nopOpcodes[4][:5]},
		{num: 6, expected: nopOpcodes[5][:6]},
		{num: 7, expected: nopOpcodes[6][:7]},
		{num: 8, expected: nopOpcodes[7][:8]},
		{num: 9, expected: nopOpcodes[8][:9]},
		{num: 10, expected: append(nopOpcodes[8][:9], nopOpcodes[0][:1]...)},
		{num: 11, expected: append(nopOpcodes[8][:9], nopOpcodes[1][:2]...)},
		{num: 12, expected: append(nopOpcodes[8][:9], nopOpcodes[2][:3]...)},
		{num: 13, expected: append(nopOpcodes[8][:9], nopOpcodes[3][:4]...)},
		{num: 14, expected: append(nopOpcodes[8][:9], nopOpcodes[4][:5]...)},
		{num: 15, expected: append(nopOpcodes[8][:9], nopOpcodes[5][:6]...)},
		{num: 16, expected: append(nopOpcodes[8][:9], nopOpcodes[6][:7]...)},
		{num: 17, expected: append(nopOpcodes[8][:9], nopOpcodes[7][:8]...)},
		{num: 18, expected: append(nopOpcodes[8][:9], nopOpcodes[8][:9]...)},
	} {
		tc := tc
		t.Run(strconv.Itoa(tc.num), func(t *testing.T) {
			a := NewAssemblerImpl()
			a.padNOP(tc.num)
			actual := a.Buf.Bytes()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileStandAlone(RET)
	actualNode := a.Current
	require.Equal(t, RET, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileConstToRegister(MOVQ, 1000, REG_AX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, int64(1000), actualNode.SrcConst)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToRegister(MOVQ, REG_BX, REG_AX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryToRegister(MOVQ, REG_BX, 100, REG_AX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToMemory(MOVQ, REG_BX, REG_AX, 100)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJump(JMP)
	actualNode := a.Current
	require.Equal(t, JMP, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeBranch, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJumpToRegister(JNE, REG_AX)
	actualNode := a.Current
	require.Equal(t, JNE, actualNode.Instruction)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJumpToMemory(JNE, REG_AX, 100)
	actualNode := a.Current
	require.Equal(t, JNE, actualNode.Instruction)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileReadInstructionAddress(REG_R10, RET)
	actualNode := a.Current
	require.Equal(t, LEAQ, actualNode.Instruction)
	require.Equal(t, REG_R10, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func TestAssemblerImpl_CompileRegisterToRegisterWithMode(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToRegisterWithMode(MOVQ, REG_BX, REG_AX, 123)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, byte(123), actualNode.Mode)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryWithIndexToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryWithIndexToRegister(MOVQ, REG_BX, 100, REG_R10, 8, REG_AX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, REG_R10, actualNode.SrcMemIndex)
	require.Equal(t, byte(8), actualNode.SrcMemScale)
	require.Equal(t, REG_AX, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToConst(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToConst(MOVQ, REG_BX, 123)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, int64(123), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeConst, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToNone(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToNone(MOVQ, REG_BX)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileNoneToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileNoneToRegister(MOVQ, REG_BX)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileNoneToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileNoneToMemory(MOVQ, REG_BX, 1234)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.DstReg)
	require.Equal(t, int64(1234), actualNode.DstConst)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileConstToMemory(MOVQ, -9999, REG_BX, 1234)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.DstReg)
	require.Equal(t, int64(-9999), actualNode.SrcConst)
	require.Equal(t, int64(1234), actualNode.DstConst)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToConst(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryToConst(MOVQ, REG_BX, 1234, -9999)
	actualNode := a.Current
	require.Equal(t, REG_BX, actualNode.SrcReg)
	require.Equal(t, int64(1234), actualNode.SrcConst)
	require.Equal(t, int64(-9999), actualNode.DstConst)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeConst, actualNode.Types.dst)
}

func TestAssemblerImpl_encodeNoneToNone(t *testing.T) {
	for _, tc := range []struct {
		inst   asm.Instruction
		exp    []byte
		expErr bool
	}{
		{inst: ADDL, expErr: true},
		{inst: CDQ, exp: []byte{0x99}},
		{inst: CQO, exp: []byte{0x48, 0x99}},
		{inst: NOP, exp: nil},
		{inst: RET, exp: []byte{0xc3}},
	} {
		tc := tc
		t.Run(InstructionName(tc.inst), func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.encodeNoneToNone(&NodeImpl{Instruction: tc.inst, Types: OperandTypesNoneToNone})
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.exp, a.Buf.Bytes())
			}
		})
	}
}
