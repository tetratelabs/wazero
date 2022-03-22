package amd64

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	actual := a.newNode(ADDL, operandTypeConst, operandTypeMemory)
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
		require.EqualError(t, err, "encoder undefined for from:branch,to:register")
	})
	// TODO: adds ok cases.
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

	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}
