package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestInstructionName ensures that all the operation kind's stringer is well-defined.
func TestOperationKind_String(t *testing.T) {
	for k := OperationKind(0); k < operationKindEnd; k++ {
		require.NotEqual(t, "", k.String())
	}
}
