package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestInstructionName ensures that all the operation Kind's stringer is well-defined.
func TestOperationKind_String(t *testing.T) {
	for k := OperationKind(0); k < operationKindEnd; k++ {
		require.NotEqual(t, "", k.String())
	}
}

// TestUnionOperation_String ensures that UnionOperation's stringer is well-defined for all supported OpKinds.
func TestUnionOperation_String(t *testing.T) {
	op := UnionOperation{}
	for k := OperationKind(0); k < operationKindEnd; k++ {
		op.Kind = k
		require.NotEqual(t, "", op.String())
	}
}

func TestLabelID(t *testing.T) {
	for k := LabelKind(0); k < LabelKindNum; k++ {
		label := NewLabelID(k, 12345)
		require.Equal(t, k, label.Kind())
		require.Equal(t, 12345, label.FrameID())
	}
}
