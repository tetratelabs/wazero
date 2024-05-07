package interpreter

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestInstructionName ensures that all the operation Kind's stringer is well-defined.
func TestOperationKind_String(t *testing.T) {
	for k := operationKind(0); k < operationKindEnd; k++ {
		require.NotEqual(t, "", k.String())
	}
}

// Test_unionOperation_String ensures that UnionOperation's stringer is well-defined for all supported OpKinds.
func Test_unionOperation_String(t *testing.T) {
	op := unionOperation{}
	for k := operationKind(0); k < operationKindEnd; k++ {
		op.Kind = k
		require.NotEqual(t, "", op.String())
	}
}

func TestLabel(t *testing.T) {
	for k := labelKind(0); k < labelKindNum; k++ {
		label := newLabel(k, 12345)
		require.Equal(t, k, label.Kind())
		require.Equal(t, 12345, label.FrameID())
	}
}
