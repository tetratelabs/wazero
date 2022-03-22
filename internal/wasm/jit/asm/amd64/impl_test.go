package amd64

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeImpl_AssignJumpTarget(t *testing.T) {
	n := &nodeImpl{}
	target := &nodeImpl{}
	n.AssignJumpTarget(target)
	require.Equal(t, n.jumpTarget, target)
}

func TestNodeImpl_AssignDestinationConstant(t *testing.T) {
	n := &nodeImpl{}
	n.AssignDestinationConstant(12345)
	require.Equal(t, int64(12345), n.dstConst)
}

func TestNodeImpl_AssignSourceConstant(t *testing.T) {
	n := &nodeImpl{}
	n.AssignSourceConstant(12345)
	require.Equal(t, int64(12345), n.srcConst)
}

func TestNodeImpl_String(t *testing.T) {
	for _, tc := range []struct {
		in  *nodeImpl
		exp string
	}{
		{
			in:  &nodeImpl{instruction: NOP},
			exp: "NOP",
		},
		{
			in:  &nodeImpl{instruction: SETCC, types: operandTypesNoneToRegister, dstReg: REG_AX},
			exp: "SETCC , AX",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100},
			exp: "JMP , [AX + 0x64]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100, dstMemScale: 8, dstMemIndex: REG_R11},
			exp: "JMP , [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToBranch, jumpTarget: &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100}},
			exp: "JMP , {JMP , [AX + 0x64]}",
		},
		{
			in:  &nodeImpl{instruction: IDIVQ, types: operandTypesRegisterToNone, srcReg: REG_DX},
			exp: "IDIVQ DX, ",
		},
		{
			in:  &nodeImpl{instruction: ADDL, types: operandTypesRegisterToRegister, srcReg: REG_DX, dstReg: REG_R14},
			exp: "ADDL DX, R14",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: REG_DX, dstReg: REG_R14, dstConst: 100},
			exp: "MOVQ DX, [R14 + 0x64]",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: REG_DX, dstReg: REG_R14, dstConst: 100, dstMemIndex: REG_CX, dstMemScale: 4},
			exp: "MOVQ DX, [R14 + 0x64 + CX*0x4]",
		},
		{
			in: &nodeImpl{instruction: CMPL, types: operandTypesRegisterToConst,
				srcReg: REG_DX, dstConst: 100},
			exp: "CMPL DX, 0x64",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: REG_DX, srcConst: 1, dstReg: REG_AX},
			exp: "MOVL [DX + 0x1], AX",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: REG_DX, srcConst: 1, srcMemIndex: REG_R12, srcMemScale: 2,
				dstReg: REG_AX},
			exp: "MOVL [DX + 1 + R12*0x2], AX",
		},
		{
			in: &nodeImpl{instruction: CMPQ, types: operandTypesMemoryToConst,
				srcReg: REG_DX, srcConst: 1, srcMemIndex: REG_R12, srcMemScale: 2,
				dstConst: 123},
			exp: "CMPQ [DX + 1 + R12*0x2], 0x7b",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToMemory, srcConst: 123, dstReg: REG_AX, dstConst: 100, dstMemScale: 8, dstMemIndex: REG_R11},
			exp: "MOVQ 0x7b, [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToRegister, srcConst: 123, dstReg: REG_AX},
			exp: "MOVQ 0x7b, AX",
		},
	} {
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := &assemblerImpl{}

	root := &nodeImpl{}
	a.addNode(root)
	require.Equal(t, a.root, root)
	require.Equal(t, a.current, root)
	require.Nil(t, root.next)

	next := &nodeImpl{}
	a.addNode(next)
	require.Equal(t, a.root, root)
	require.Equal(t, a.current, next)
	require.Equal(t, next, root.next)
	require.Nil(t, next.next)
}

func TestAssemblerImpl_newNode(t *testing.T) {
	a := &assemblerImpl{}
	actual := a.newNode(ADDL, operandTypesConstToMemory)
	require.Equal(t, ADDL, actual.instruction)
	require.Equal(t, operandTypeConst, actual.types.src)
	require.Equal(t, operandTypeMemory, actual.types.dst)
	require.Equal(t, actual, a.root)
	require.Equal(t, actual, a.current)
}

func TestAssemblerImpl_encodeNode(t *testing.T) {
	t.Run("encoder undefined", func(t *testing.T) {
		a := &assemblerImpl{}
		err := a.encodeNode(bytes.NewBuffer(nil), &nodeImpl{
			types: operandTypes{operandTypeBranch, operandTypeRegister},
		})
		require.EqualError(t, err, "encoder undefined for [from:branch,to:register] operand type")
	})
	// TODO: adds ok cases.
}

func TestAssemblerImpl_Assemble(t *testing.T) {
	t.Run("callback", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			a := &assemblerImpl{}
			callbacked := false
			a.AddOnGenerateCallBack(func(b []byte) error { callbacked = true; return nil })
			_, err := a.Assemble()
			require.NoError(t, err)
			require.True(t, callbacked)
		})
		t.Run("error", func(t *testing.T) {
			a := &assemblerImpl{}
			a.AddOnGenerateCallBack(func(b []byte) error { return fmt.Errorf("some error") })
			_, err := a.Assemble()
			require.EqualError(t, err, "some error")
		})
	})
	// TODO: adds actual e2e assembling case.
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileStandAlone(RET)
	actualNode := a.current
	require.Equal(t, RET, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileConstToRegister(MOVQ, 1000, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, int64(1000), actualNode.srcConst)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileRegisterToRegister(MOVQ, REG_BX, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileMemoryToRegister(MOVQ, REG_BX, 100, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileRegisterToMemory(MOVQ, REG_BX, REG_AX, 100)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileJump(JMP)
	actualNode := a.current
	require.Equal(t, JMP, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeBranch, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileJumpToRegister(JNE, REG_AX)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileJumpToMemory(JNE, REG_AX, 100)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	t.Skip("TODO: unimplemented")
}

func TestAssemblerImpl_CompileRegisterToRegisterWithMode(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileRegisterToRegisterWithMode(MOVQ, REG_BX, REG_AX, 123)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, byte(123), actualNode.mode)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryWithIndexToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileMemoryWithIndexToRegister(MOVQ, REG_BX, 100, REG_R10, 8, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, REG_R10, actualNode.srcMemIndex)
	require.Equal(t, byte(8), actualNode.srcMemScale)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToConst(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileRegisterToConst(MOVQ, REG_BX, 123)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(123), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToNone(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileRegisterToNone(MOVQ, REG_BX)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToRegister(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileNoneToRegister(MOVQ, REG_BX)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToMemory(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileNoneToMemory(MOVQ, REG_BX, 1234)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToMemory(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileConstToMemory(MOVQ, -9999, REG_BX, 1234)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, int64(-9999), actualNode.srcConst)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToConst(t *testing.T) {
	a := &assemblerImpl{}
	a.CompileMemoryToConst(MOVQ, REG_BX, 1234, -9999)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(1234), actualNode.srcConst)
	require.Equal(t, int64(-9999), actualNode.dstConst)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
}
