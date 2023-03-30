package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestInstructionName ensures that all the operation OpKind's stringer is well-defined.
func TestOperationKind_String(t *testing.T) {
	for k := OperationKind(0); k < operationKindEnd; k++ {
		require.NotEqual(t, "", k.String())
	}
}

// TestUnionOperation_String ensures that UnionOperation's stringer is well-defined for all supported OpKinds.
func TestUnionOperation_String(t *testing.T) {
	op := UnionOperation{}
	for _, k := range []OperationKind{
		OperationKindUnreachable,
		OperationKindMemorySize,
		OperationKindMemoryGrow,
		OperationKindI32WrapFromI64,
		OperationKindF32DemoteFromF64,
		OperationKindF64PromoteFromF32,
		OperationKindI32ReinterpretFromF32,
		OperationKindI64ReinterpretFromF64,
		OperationKindF32ReinterpretFromI32,
		OperationKindF64ReinterpretFromI64,
		OperationKindSignExtend32From8,
		OperationKindSignExtend32From16,
		OperationKindSignExtend64From8,
		OperationKindSignExtend64From16,
		OperationKindSignExtend64From32,
		OperationKindMemoryCopy,
		OperationKindMemoryFill,
		OperationKindBuiltinFunctionCheckExitCode,
		OperationKindCall,
		OperationKindGlobalGet,
		OperationKindGlobalSet,
		OperationKindConstI32,
		OperationKindConstI64,
		OperationKindConstF32,
		OperationKindConstF64,
	} {
		op.OpKind = k
		require.NotEqual(t, "", op.String())
	}
}
