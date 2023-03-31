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
		OperationKindCall,
		OperationKindGlobalGet,
		OperationKindGlobalSet,
		OperationKindLoad,
		OperationKindLoad8,
		OperationKindLoad16,
		OperationKindLoad32,
		OperationKindMemorySize,
		OperationKindMemoryGrow,
		OperationKindConstI32,
		OperationKindConstI64,
		OperationKindConstF32,
		OperationKindConstF64,
		OperationKindEq,
		OperationKindNe,
		OperationKindEqz,
		OperationKindLt,
		OperationKindGt,
		OperationKindLe,
		OperationKindGe,
		OperationKindAdd,
		OperationKindSub,
		OperationKindMul,
		OperationKindClz,
		OperationKindCtz,
		OperationKindPopcnt,
		OperationKindDiv,
		OperationKindRem,
		OperationKindAnd,
		OperationKindOr,
		OperationKindXor,
		OperationKindShl,
		OperationKindShr,
		OperationKindRotl,
		OperationKindRotr,
		OperationKindAbs,
		OperationKindNeg,
		OperationKindCeil,
		OperationKindFloor,
		OperationKindTrunc,
		OperationKindNearest,
		OperationKindSqrt,
		OperationKindMin,
		OperationKindMax,
		OperationKindCopysign,
		OperationKindBuiltinFunctionCheckExitCode,
	} {
		op.OpKind = k
		require.NotEqual(t, "", op.String())
	}
}

func TestLabelID(t *testing.T) {
	for k := LabelKind(0); k < LabelKindNum; k++ {
		l := Label{Kind: k, FrameID: 12345}
		id := l.ID()
		require.Equal(t, k, id.Kind())
		require.Equal(t, int(l.FrameID), id.FrameID())
	}
}
