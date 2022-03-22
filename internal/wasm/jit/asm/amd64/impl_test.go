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
		require.EqualError(t, err, "encoder undefined for branch[from]_register[to]")
	})
}
