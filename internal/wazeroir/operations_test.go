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
	// TODO: after done with union refactoring, use
	// `for k := OperationKind(0); k < operationKindEnd; k++ { ... }`
	// rather than listing all kinds here manually like TestOperationKind_String.
	for _, k := range []OperationKind{
		OperationKindUnreachable,
		OperationKindCall,
		OperationKindCallIndirect,
		OperationKindSelect,
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

		OperationKindTableGet,
		OperationKindTableSet,
		OperationKindTableSize,
		OperationKindTableGrow,
		OperationKindTableFill,
		OperationKindV128Const,
		OperationKindV128Add,
		OperationKindV128Sub,
		OperationKindV128Load,
		OperationKindV128LoadLane,
		OperationKindV128Store,
		OperationKindV128StoreLane,
		OperationKindV128ExtractLane,
		OperationKindV128ReplaceLane,
		OperationKindV128Splat,
		OperationKindV128Shuffle,
		OperationKindV128Swizzle,
		OperationKindV128AnyTrue,
		OperationKindV128AllTrue,
		OperationKindV128BitMask,
		OperationKindV128And,
		OperationKindV128Not,
		OperationKindV128Or,
		OperationKindV128Xor,
		OperationKindV128Bitselect,
		OperationKindV128AndNot,
		OperationKindV128Shl,
		OperationKindV128Shr,
		OperationKindV128Cmp,
		OperationKindV128AddSat,
		OperationKindV128SubSat,
		OperationKindV128Mul,
		OperationKindV128Div,
		OperationKindV128Neg,
		OperationKindV128Sqrt,
		OperationKindV128Abs,
		OperationKindV128Popcnt,
		OperationKindV128Min,
		OperationKindV128Max,
		OperationKindV128AvgrU,
		OperationKindV128Pmin,
		OperationKindV128Pmax,
		OperationKindV128Ceil,
		OperationKindV128Floor,
		OperationKindV128Trunc,
		OperationKindV128Nearest,
		OperationKindV128Extend,
		OperationKindV128ExtMul,
		OperationKindV128Q15mulrSatS,
		OperationKindV128ExtAddPairwise,
		OperationKindV128FloatPromote,
		OperationKindV128FloatDemote,
		OperationKindV128FConvertFromI,
		OperationKindV128Dot,
		OperationKindV128Narrow,
		OperationKindV128ITruncSatFromF,

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
