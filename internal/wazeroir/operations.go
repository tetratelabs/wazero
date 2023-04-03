package wazeroir

import (
	"fmt"
	"math"
	"strings"
)

// UnsignedInt represents unsigned 32-bit or 64-bit integers.
type UnsignedInt byte

const (
	UnsignedInt32 UnsignedInt = iota
	UnsignedInt64
)

// String implements fmt.Stringer.
func (s UnsignedInt) String() (ret string) {
	switch s {
	case UnsignedInt32:
		ret = "i32"
	case UnsignedInt64:
		ret = "i64"
	}
	return
}

// SignedInt represents signed or unsigned integers.
type SignedInt byte

const (
	SignedInt32 SignedInt = iota
	SignedInt64
	SignedUint32
	SignedUint64
)

// String implements fmt.Stringer.
func (s SignedInt) String() (ret string) {
	switch s {
	case SignedUint32:
		ret = "u32"
	case SignedUint64:
		ret = "u64"
	case SignedInt32:
		ret = "s32"
	case SignedInt64:
		ret = "s64"
	}
	return
}

// Float represents the scalar double or single precision floating points.
type Float byte

const (
	Float32 Float = iota
	Float64
)

// String implements fmt.Stringer.
func (s Float) String() (ret string) {
	switch s {
	case Float32:
		ret = "f32"
	case Float64:
		ret = "f64"
	}
	return
}

// UnsignedType is the union of UnsignedInt, Float and V128 vector type.
type UnsignedType byte

const (
	UnsignedTypeI32 UnsignedType = iota
	UnsignedTypeI64
	UnsignedTypeF32
	UnsignedTypeF64
	UnsignedTypeV128
	UnsignedTypeUnknown
)

// String implements fmt.Stringer.
func (s UnsignedType) String() (ret string) {
	switch s {
	case UnsignedTypeI32:
		ret = "i32"
	case UnsignedTypeI64:
		ret = "i64"
	case UnsignedTypeF32:
		ret = "f32"
	case UnsignedTypeF64:
		ret = "f64"
	case UnsignedTypeV128:
		ret = "v128"
	case UnsignedTypeUnknown:
		ret = "unknown"
	}
	return
}

// SignedType is the union of SignedInt and Float types.
type SignedType byte

const (
	SignedTypeInt32 SignedType = iota
	SignedTypeUint32
	SignedTypeInt64
	SignedTypeUint64
	SignedTypeFloat32
	SignedTypeFloat64
)

// String implements fmt.Stringer.
func (s SignedType) String() (ret string) {
	switch s {
	case SignedTypeInt32:
		ret = "s32"
	case SignedTypeUint32:
		ret = "u32"
	case SignedTypeInt64:
		ret = "s64"
	case SignedTypeUint64:
		ret = "u64"
	case SignedTypeFloat32:
		ret = "f32"
	case SignedTypeFloat64:
		ret = "f64"
	}
	return
}

// Operation is the interface implemented by each individual operation.
type Operation interface {
	// Kind returns the OpKind of the implementation.
	Kind() OperationKind
	fmt.Stringer
}

// OperationKind is the OpKind of each implementation of Operation interface.
type OperationKind uint16

// String implements fmt.Stringer.
func (o OperationKind) String() (ret string) {
	switch o {
	case OperationKindUnreachable:
		ret = "Unreachable"
	case OperationKindLabel:
		ret = "Label"
	case OperationKindBr:
		ret = "Br"
	case OperationKindBrIf:
		ret = "BrIf"
	case OperationKindBrTable:
		ret = "BrTable"
	case OperationKindCall:
		ret = "Call"
	case OperationKindCallIndirect:
		ret = "CallIndirect"
	case OperationKindDrop:
		ret = "Drop"
	case OperationKindSelect:
		ret = "Select"
	case OperationKindPick:
		ret = "Pick"
	case OperationKindSet:
		ret = "Swap"
	case OperationKindGlobalGet:
		ret = "GlobalGet"
	case OperationKindGlobalSet:
		ret = "GlobalSet"
	case OperationKindLoad:
		ret = "Load"
	case OperationKindLoad8:
		ret = "Load8"
	case OperationKindLoad16:
		ret = "Load16"
	case OperationKindLoad32:
		ret = "Load32"
	case OperationKindStore:
		ret = "Store"
	case OperationKindStore8:
		ret = "Store8"
	case OperationKindStore16:
		ret = "Store16"
	case OperationKindStore32:
		ret = "Store32"
	case OperationKindMemorySize:
		ret = "MemorySize"
	case OperationKindMemoryGrow:
		ret = "MemoryGrow"
	case OperationKindConstI32:
		ret = "ConstI32"
	case OperationKindConstI64:
		ret = "ConstI64"
	case OperationKindConstF32:
		ret = "ConstF32"
	case OperationKindConstF64:
		ret = "ConstF64"
	case OperationKindEq:
		ret = "Eq"
	case OperationKindNe:
		ret = "Ne"
	case OperationKindEqz:
		ret = "Eqz"
	case OperationKindLt:
		ret = "Lt"
	case OperationKindGt:
		ret = "Gt"
	case OperationKindLe:
		ret = "Le"
	case OperationKindGe:
		ret = "Ge"
	case OperationKindAdd:
		ret = "Add"
	case OperationKindSub:
		ret = "Sub"
	case OperationKindMul:
		ret = "Mul"
	case OperationKindClz:
		ret = "Clz"
	case OperationKindCtz:
		ret = "Ctz"
	case OperationKindPopcnt:
		ret = "Popcnt"
	case OperationKindDiv:
		ret = "Div"
	case OperationKindRem:
		ret = "Rem"
	case OperationKindAnd:
		ret = "And"
	case OperationKindOr:
		ret = "Or"
	case OperationKindXor:
		ret = "Xor"
	case OperationKindShl:
		ret = "Shl"
	case OperationKindShr:
		ret = "Shr"
	case OperationKindRotl:
		ret = "Rotl"
	case OperationKindRotr:
		ret = "Rotr"
	case OperationKindAbs:
		ret = "Abs"
	case OperationKindNeg:
		ret = "Neg"
	case OperationKindCeil:
		ret = "Ceil"
	case OperationKindFloor:
		ret = "Floor"
	case OperationKindTrunc:
		ret = "Trunc"
	case OperationKindNearest:
		ret = "Nearest"
	case OperationKindSqrt:
		ret = "Sqrt"
	case OperationKindMin:
		ret = "Min"
	case OperationKindMax:
		ret = "Max"
	case OperationKindCopysign:
		ret = "Copysign"
	case OperationKindI32WrapFromI64:
		ret = "I32WrapFromI64"
	case OperationKindITruncFromF:
		ret = "ITruncFromF"
	case OperationKindFConvertFromI:
		ret = "FConvertFromI"
	case OperationKindF32DemoteFromF64:
		ret = "F32DemoteFromF64"
	case OperationKindF64PromoteFromF32:
		ret = "F64PromoteFromF32"
	case OperationKindI32ReinterpretFromF32:
		ret = "I32ReinterpretFromF32"
	case OperationKindI64ReinterpretFromF64:
		ret = "I64ReinterpretFromF64"
	case OperationKindF32ReinterpretFromI32:
		ret = "F32ReinterpretFromI32"
	case OperationKindF64ReinterpretFromI64:
		ret = "F64ReinterpretFromI64"
	case OperationKindExtend:
		ret = "Extend"
	case OperationKindMemoryInit:
		ret = "MemoryInit"
	case OperationKindDataDrop:
		ret = "DataDrop"
	case OperationKindMemoryCopy:
		ret = "MemoryCopy"
	case OperationKindMemoryFill:
		ret = "MemoryFill"
	case OperationKindTableInit:
		ret = "TableInit"
	case OperationKindElemDrop:
		ret = "ElemDrop"
	case OperationKindTableCopy:
		ret = "TableCopy"
	case OperationKindRefFunc:
		ret = "RefFunc"
	case OperationKindTableGet:
		ret = "TableGet"
	case OperationKindTableSet:
		ret = "TableSet"
	case OperationKindTableSize:
		ret = "TableSize"
	case OperationKindTableGrow:
		ret = "TableGrow"
	case OperationKindTableFill:
		ret = "TableFill"
	case OperationKindV128Const:
		ret = "ConstV128"
	case OperationKindV128Add:
		ret = "V128Add"
	case OperationKindV128Sub:
		ret = "V128Sub"
	case OperationKindV128Load:
		ret = "V128Load"
	case OperationKindV128LoadLane:
		ret = "V128LoadLane"
	case OperationKindV128Store:
		ret = "V128Store"
	case OperationKindV128StoreLane:
		ret = "V128StoreLane"
	case OperationKindV128ExtractLane:
		ret = "V128ExtractLane"
	case OperationKindV128ReplaceLane:
		ret = "V128ReplaceLane"
	case OperationKindV128Splat:
		ret = "V128Splat"
	case OperationKindV128Shuffle:
		ret = "V128Shuffle"
	case OperationKindV128Swizzle:
		ret = "V128Swizzle"
	case OperationKindV128AnyTrue:
		ret = "V128AnyTrue"
	case OperationKindV128AllTrue:
		ret = "V128AllTrue"
	case OperationKindV128And:
		ret = "V128And"
	case OperationKindV128Not:
		ret = "V128Not"
	case OperationKindV128Or:
		ret = "V128Or"
	case OperationKindV128Xor:
		ret = "V128Xor"
	case OperationKindV128Bitselect:
		ret = "V128Bitselect"
	case OperationKindV128AndNot:
		ret = "V128AndNot"
	case OperationKindV128BitMask:
		ret = "V128BitMask"
	case OperationKindV128Shl:
		ret = "V128Shl"
	case OperationKindV128Shr:
		ret = "V128Shr"
	case OperationKindV128Cmp:
		ret = "V128Cmp"
	case OperationKindSignExtend32From8:
		ret = "SignExtend32From8"
	case OperationKindSignExtend32From16:
		ret = "SignExtend32From16"
	case OperationKindSignExtend64From8:
		ret = "SignExtend64From8"
	case OperationKindSignExtend64From16:
		ret = "SignExtend64From16"
	case OperationKindSignExtend64From32:
		ret = "SignExtend64From32"
	case OperationKindV128AddSat:
		ret = "V128AddSat"
	case OperationKindV128SubSat:
		ret = "V128SubSat"
	case OperationKindV128Mul:
		ret = "V128Mul"
	case OperationKindV128Div:
		ret = "V128Div"
	case OperationKindV128Neg:
		ret = "V128Neg"
	case OperationKindV128Sqrt:
		ret = "V128Sqrt"
	case OperationKindV128Abs:
		ret = "V128Abs"
	case OperationKindV128Popcnt:
		ret = "V128Popcnt"
	case OperationKindV128Min:
		ret = "V128Min"
	case OperationKindV128Max:
		ret = "V128Max"
	case OperationKindV128AvgrU:
		ret = "V128AvgrU"
	case OperationKindV128Ceil:
		ret = "V128Ceil"
	case OperationKindV128Floor:
		ret = "V128Floor"
	case OperationKindV128Trunc:
		ret = "V128Trunc"
	case OperationKindV128Nearest:
		ret = "V128Nearest"
	case OperationKindV128Pmin:
		ret = "V128Pmin"
	case OperationKindV128Pmax:
		ret = "V128Pmax"
	case OperationKindV128Extend:
		ret = "V128Extend"
	case OperationKindV128ExtMul:
		ret = "V128ExtMul"
	case OperationKindV128Q15mulrSatS:
		ret = "V128Q15mulrSatS"
	case OperationKindV128ExtAddPairwise:
		ret = "V128ExtAddPairwise"
	case OperationKindV128FloatPromote:
		ret = "V128FloatPromote"
	case OperationKindV128FloatDemote:
		ret = "V128FloatDemote"
	case OperationKindV128FConvertFromI:
		ret = "V128FConvertFromI"
	case OperationKindV128Dot:
		ret = "V128Dot"
	case OperationKindV128Narrow:
		ret = "V128Narrow"
	case OperationKindV128ITruncSatFromF:
		ret = "V128ITruncSatFromF"
	case OperationKindBuiltinFunctionCheckExitCode:
		ret = "BuiltinFunctionCheckExitCode"
	default:
		panic(fmt.Errorf("unknown operation %d", o))
	}
	return
}

const (
	// OperationKindUnreachable is the OpKind for OperationUnreachable.
	OperationKindUnreachable OperationKind = iota
	// OperationKindLabel is the OpKind for NewOperationLabel.
	OperationKindLabel
	// OperationKindBr is the OpKind for NewOperationBr.
	OperationKindBr
	// OperationKindBrIf is the OpKind for NewOperationBrIf.
	OperationKindBrIf
	// OperationKindBrTable is the OpKind for NewOperationBrTable.
	OperationKindBrTable
	// OperationKindCall is the OpKind for OperationCall.
	OperationKindCall
	// OperationKindCallIndirect is the OpKind for OperationCallIndirect.
	OperationKindCallIndirect
	// OperationKindDrop is the OpKind for NewOperationDrop.
	OperationKindDrop
	// OperationKindSelect is the OpKind for OperationSelect.
	OperationKindSelect
	// OperationKindPick is the OpKind for OperationPick.
	OperationKindPick
	// OperationKindSet is the OpKind for OperationSet.
	OperationKindSet
	// OperationKindGlobalGet is the OpKind for OperationGlobalGet.
	OperationKindGlobalGet
	// OperationKindGlobalSet is the OpKind for OperationGlobalSet.
	OperationKindGlobalSet
	// OperationKindLoad is the OpKind for OperationLoad.
	OperationKindLoad
	// OperationKindLoad8 is the OpKind for OperationLoad8.
	OperationKindLoad8
	// OperationKindLoad16 is the OpKind for OperationLoad16.
	OperationKindLoad16
	// OperationKindLoad32 is the OpKind for OperationLoad32.
	OperationKindLoad32
	// OperationKindStore is the OpKind for OperationStore.
	OperationKindStore
	// OperationKindStore8 is the OpKind for OperationStore8.
	OperationKindStore8
	// OperationKindStore16 is the OpKind for OperationStore16.
	OperationKindStore16
	// OperationKindStore32 is the OpKind for OperationStore32.
	OperationKindStore32
	// OperationKindMemorySize is the OpKind for OperationMemorySize.
	OperationKindMemorySize
	// OperationKindMemoryGrow is the OpKind for OperationMemoryGrow.
	OperationKindMemoryGrow
	// OperationKindConstI32 is the OpKind for NewOperationConstI32.
	OperationKindConstI32
	// OperationKindConstI64 is the OpKind for NewOperationConstI64.
	OperationKindConstI64
	// OperationKindConstF32 is the OpKind for NewOperationConstF32.
	OperationKindConstF32
	// OperationKindConstF64 is the OpKind for NewOperationConstF64.
	OperationKindConstF64
	// OperationKindEq is the OpKind for OperationEq.
	OperationKindEq
	// OperationKindNe is the OpKind for OperationNe.
	OperationKindNe
	// OperationKindEqz is the OpKind for OperationEqz.
	OperationKindEqz
	// OperationKindLt is the OpKind for OperationLt.
	OperationKindLt
	// OperationKindGt is the OpKind for OperationGt.
	OperationKindGt
	// OperationKindLe is the OpKind for OperationLe.
	OperationKindLe
	// OperationKindGe is the OpKind for OperationGe.
	OperationKindGe
	// OperationKindAdd is the OpKind for OperationAdd.
	OperationKindAdd
	// OperationKindSub is the OpKind for OperationSub.
	OperationKindSub
	// OperationKindMul is the OpKind for OperationMul.
	OperationKindMul
	// OperationKindClz is the OpKind for OperationClz.
	OperationKindClz
	// OperationKindCtz is the OpKind for OperationCtz.
	OperationKindCtz
	// OperationKindPopcnt is the OpKind for OperationPopcnt.
	OperationKindPopcnt
	// OperationKindDiv is the OpKind for OperationDiv.
	OperationKindDiv
	// OperationKindRem is the OpKind for OperationRem.
	OperationKindRem
	// OperationKindAnd is the OpKind for OperationAnd.
	OperationKindAnd
	// OperationKindOr is the OpKind for OperationOr.
	OperationKindOr
	// OperationKindXor is the OpKind for OperationXor.
	OperationKindXor
	// OperationKindShl is the OpKind for OperationShl.
	OperationKindShl
	// OperationKindShr is the OpKind for OperationShr.
	OperationKindShr
	// OperationKindRotl is the OpKind for OperationRotl.
	OperationKindRotl
	// OperationKindRotr is the OpKind for OperationRotr.
	OperationKindRotr
	// OperationKindAbs is the OpKind for OperationAbs.
	OperationKindAbs
	// OperationKindNeg is the OpKind for OperationNeg.
	OperationKindNeg
	// OperationKindCeil is the OpKind for OperationCeil.
	OperationKindCeil
	// OperationKindFloor is the OpKind for OperationFloor.
	OperationKindFloor
	// OperationKindTrunc is the OpKind for OperationTrunc.
	OperationKindTrunc
	// OperationKindNearest is the OpKind for OperationNearest.
	OperationKindNearest
	// OperationKindSqrt is the OpKind for OperationSqrt.
	OperationKindSqrt
	// OperationKindMin is the OpKind for OperationMin.
	OperationKindMin
	// OperationKindMax is the OpKind for OperationMax.
	OperationKindMax
	// OperationKindCopysign is the OpKind for OperationCopysign.
	OperationKindCopysign
	// OperationKindI32WrapFromI64 is the OpKind for OperationI32WrapFromI64.
	OperationKindI32WrapFromI64
	// OperationKindITruncFromF is the OpKind for NewOperationITruncFromF.
	OperationKindITruncFromF
	// OperationKindFConvertFromI is the OpKind for NewOperationFConvertFromI.
	OperationKindFConvertFromI
	// OperationKindF32DemoteFromF64 is the OpKind for OperationF32DemoteFromF64.
	OperationKindF32DemoteFromF64
	// OperationKindF64PromoteFromF32 is the OpKind for OperationF64PromoteFromF32.
	OperationKindF64PromoteFromF32
	// OperationKindI32ReinterpretFromF32 is the OpKind for OperationI32ReinterpretFromF32.
	OperationKindI32ReinterpretFromF32
	// OperationKindI64ReinterpretFromF64 is the OpKind for OperationI64ReinterpretFromF64.
	OperationKindI64ReinterpretFromF64
	// OperationKindF32ReinterpretFromI32 is the OpKind for OperationF32ReinterpretFromI32.
	OperationKindF32ReinterpretFromI32
	// OperationKindF64ReinterpretFromI64 is the OpKind for OperationF64ReinterpretFromI64.
	OperationKindF64ReinterpretFromI64
	// OperationKindExtend is the OpKind for NewOperationExtend.
	OperationKindExtend
	// OperationKindSignExtend32From8 is the OpKind for OperationSignExtend32From8.
	OperationKindSignExtend32From8
	// OperationKindSignExtend32From16 is the OpKind for OperationSignExtend32From16.
	OperationKindSignExtend32From16
	// OperationKindSignExtend64From8 is the OpKind for OperationSignExtend64From8.
	OperationKindSignExtend64From8
	// OperationKindSignExtend64From16 is the OpKind for OperationSignExtend64From16.
	OperationKindSignExtend64From16
	// OperationKindSignExtend64From32 is the OpKind for OperationSignExtend64From32.
	OperationKindSignExtend64From32
	// OperationKindMemoryInit is the OpKind for NewOperationMemoryInit.
	OperationKindMemoryInit
	// OperationKindDataDrop is the OpKind for NewOperationDataDrop.
	OperationKindDataDrop
	// OperationKindMemoryCopy is the OpKind for OperationMemoryCopy.
	OperationKindMemoryCopy
	// OperationKindMemoryFill is the OpKind for OperationMemoryFill.
	OperationKindMemoryFill
	// OperationKindTableInit is the OpKind for NewOperationTableInit.
	OperationKindTableInit
	// OperationKindElemDrop is the OpKind for NewOperationElemDrop.
	OperationKindElemDrop
	// OperationKindTableCopy is the OpKind for NewOperationTableCopy.
	OperationKindTableCopy
	// OperationKindRefFunc is the OpKind for NewOperationRefFunc.
	OperationKindRefFunc
	// OperationKindTableGet is the OpKind for NewOperationTableGet.
	OperationKindTableGet
	// OperationKindTableSet is the OpKind for NewOperationTableSet.
	OperationKindTableSet
	// OperationKindTableSize is the OpKind for NewOperationTableSize.
	OperationKindTableSize
	// OperationKindTableGrow is the OpKind for NewOperationTableGrow.
	OperationKindTableGrow
	// OperationKindTableFill is the OpKind for NewOperationTableFill.
	OperationKindTableFill

	// Vector value related instructions are prefixed by V128.

	// OperationKindV128Const is the OpKind for OperationV128Const.
	OperationKindV128Const
	// OperationKindV128Add is the OpKind for OperationV128Add.
	OperationKindV128Add
	// OperationKindV128Sub is the OpKind for OperationV128Sub.
	OperationKindV128Sub
	// OperationKindV128Load is the OpKind for OperationV128Load.
	OperationKindV128Load
	// OperationKindV128LoadLane is the OpKind for OperationV128LoadLane.
	OperationKindV128LoadLane
	// OperationKindV128Store is the OpKind for NewOperationV128Store.
	OperationKindV128Store
	// OperationKindV128StoreLane is the OpKind for NewOperationV128StoreLane.
	OperationKindV128StoreLane
	// OperationKindV128ExtractLane is the OpKind for NewOperationV128ExtractLane.
	OperationKindV128ExtractLane
	// OperationKindV128ReplaceLane is the OpKind for NewOperationV128ReplaceLane.
	OperationKindV128ReplaceLane
	// OperationKindV128Splat is the OpKind for NewOperationV128Splat.
	OperationKindV128Splat
	// OperationKindV128Shuffle is the OpKind for NewOperationV128Shuffle.
	OperationKindV128Shuffle
	// OperationKindV128Swizzle is the OpKind for OperationV128Swizzle.
	OperationKindV128Swizzle
	// OperationKindV128AnyTrue is the OpKind for OperationV128AnyTrue.
	OperationKindV128AnyTrue
	// OperationKindV128AllTrue is the OpKind for NewOperationV128AllTrue.
	OperationKindV128AllTrue
	// OperationKindV128BitMask is the OpKind for NewOperationV128BitMask.
	OperationKindV128BitMask
	// OperationKindV128And is the OpKind for OperationV128And.
	OperationKindV128And
	// OperationKindV128Not is the OpKind for OperationV128Not.
	OperationKindV128Not
	// OperationKindV128Or is the OpKind for OperationV128Or.
	OperationKindV128Or
	// OperationKindV128Xor is the OpKind for OperationV128Xor.
	OperationKindV128Xor
	// OperationKindV128Bitselect is the OpKind for OperationV128Bitselect.
	OperationKindV128Bitselect
	// OperationKindV128AndNot is the OpKind for OperationV128AndNot.
	OperationKindV128AndNot
	// OperationKindV128Shl is the OpKind for NewOperationV128Shl.
	OperationKindV128Shl
	// OperationKindV128Shr is the OpKind for NewOperationV128Shr.
	OperationKindV128Shr
	// OperationKindV128Cmp is the OpKind for NewOperationV128Cmp.
	OperationKindV128Cmp
	// OperationKindV128AddSat is the OpKind for NewOperationV128AddSat.
	OperationKindV128AddSat
	// OperationKindV128SubSat is the OpKind for NewOperationV128SubSat.
	OperationKindV128SubSat
	// OperationKindV128Mul is the OpKind for NewOperationV128Mul.
	OperationKindV128Mul
	// OperationKindV128Div is the OpKind for NewOperationV128Div.
	OperationKindV128Div
	// OperationKindV128Neg is the OpKind for NewOperationV128Neg.
	OperationKindV128Neg
	// OperationKindV128Sqrt is the OpKind for NewOperationV128Sqrt.
	OperationKindV128Sqrt
	// OperationKindV128Abs is the OpKind for NewOperationV128Abs.
	OperationKindV128Abs
	// OperationKindV128Popcnt is the OpKind for NewOperationV128Popcnt.
	OperationKindV128Popcnt
	// OperationKindV128Min is the OpKind for NewOperationV128Min.
	OperationKindV128Min
	// OperationKindV128Max is the OpKind for NewOperationV128Max.
	OperationKindV128Max
	// OperationKindV128AvgrU is the OpKind for NewOperationV128AvgrU.
	OperationKindV128AvgrU
	// OperationKindV128Pmin is the OpKind for NewOperationV128Pmin.
	OperationKindV128Pmin
	// OperationKindV128Pmax is the OpKind for NewOperationV128Pmax.
	OperationKindV128Pmax
	// OperationKindV128Ceil is the OpKind for NewOperationV128Ceil.
	OperationKindV128Ceil
	// OperationKindV128Floor is the OpKind for NewOperationV128Floor.
	OperationKindV128Floor
	// OperationKindV128Trunc is the OpKind for NewOperationV128Trunc.
	OperationKindV128Trunc
	// OperationKindV128Nearest is the OpKind for NewOperationV128Nearest.
	OperationKindV128Nearest
	// OperationKindV128Extend is the OpKind for NewOperationV128Extend.
	OperationKindV128Extend
	// OperationKindV128ExtMul is the OpKind for NewOperationV128ExtMul.
	OperationKindV128ExtMul
	// OperationKindV128Q15mulrSatS is the OpKind for OperationV128Q15mulrSatS.
	OperationKindV128Q15mulrSatS
	// OperationKindV128ExtAddPairwise is the OpKind for NewOperationV128ExtAddPairwise.
	OperationKindV128ExtAddPairwise
	// OperationKindV128FloatPromote is the OpKind for OperationV128FloatPromote.
	OperationKindV128FloatPromote
	// OperationKindV128FloatDemote is the OpKind for OperationV128FloatDemote.
	OperationKindV128FloatDemote
	// OperationKindV128FConvertFromI is the OpKind for NewOperationV128FConvertFromI.
	OperationKindV128FConvertFromI
	// OperationKindV128Dot is the OpKind for OperationV128Dot.
	OperationKindV128Dot
	// OperationKindV128Narrow is the OpKind for NewOperationV128Narrow.
	OperationKindV128Narrow
	// OperationKindV128ITruncSatFromF is the OpKind for NewOperationV128ITruncSatFromF.
	OperationKindV128ITruncSatFromF

	// OperationKindBuiltinFunctionCheckExitCode is the OpKind for OperationBuiltinFunctionCheckExitCode.
	OperationKindBuiltinFunctionCheckExitCode

	// operationKindEnd is always placed at the bottom of this iota definition to be used in the test.
	operationKindEnd
)

// NewOperationBuiltinFunctionCheckExitCode is a constructor for UnionOperation with Kind OperationKindBuiltinFunctionCheckExitCode.
//
// OperationBuiltinFunctionCheckExitCode corresponds to the instruction to check the api.Module is already closed due to
// context.DeadlineExceeded, context.Canceled, or the explicit call of CloseWithExitCode on api.Module.
func NewOperationBuiltinFunctionCheckExitCode() UnionOperation {
	return UnionOperation{OpKind: OperationKindBuiltinFunctionCheckExitCode}
}

// Label is the label of each block in wazeroir where "block" consists of multiple operations,
// and must end with branching operations (e.g. NewOperationBr or NewOperationBrIf).
type Label struct {
	FrameID uint32
	Kind    LabelKind
}

// LabelID is the unique identifiers for blocks in a single function.
type LabelID uint64

// Kind returns the LabelKind encoded in this LabelID.
func (l LabelID) Kind() LabelKind {
	return LabelKind(uint32(l))
}

// FrameID returns the frame id encoded in this LabelID.
func (l LabelID) FrameID() int {
	return int(uint32(l >> 32))
}

// ID returns the LabelID for this Label.
func (l Label) ID() (id LabelID) {
	id = LabelID(l.Kind) | LabelID(l.FrameID)<<32
	return
}

// String implements fmt.Stringer.
func (lid LabelID) String() (ret string) {
	frameID := lid.FrameID()
	switch lid.Kind() {
	case LabelKindHeader:
		ret = fmt.Sprintf(".L%d", frameID)
	case LabelKindElse:
		ret = fmt.Sprintf(".L%d_else", frameID)
	case LabelKindContinuation:
		ret = fmt.Sprintf(".L%d_cont", frameID)
	case LabelKindReturn:
		return ".return"
	}
	return
}

func (l LabelID) IsReturnTarget() bool {
	return l.Kind() == LabelKindReturn
}

// LabelKind is the OpKind of the label.
type LabelKind = byte

const (
	// LabelKindHeader is the header for various blocks. For example, the "then" block of
	// wasm.OpcodeIfName in Wasm has the label of this OpKind.
	LabelKindHeader LabelKind = iota
	// LabelKindElse is the OpKind of label for "else" block of wasm.OpcodeIfName in Wasm.
	LabelKindElse
	// LabelKindContinuation is the OpKind of label which is the continuation of blocks.
	// For example, for wasm text like
	// (func
	//   ....
	//   (if (local.get 0) (then (nop)) (else (nop)))
	//   return
	// )
	// we have the continuation block (of if-block) corresponding to "return" opcode.
	LabelKindContinuation
	LabelKindReturn
	LabelKindNum
)

func (l LabelID) asBranchTargetDrop() BranchTargetDrop {
	return BranchTargetDrop{Target: l}
}

// BranchTargetDrop represents the branch target and the drop range which must be dropped
// before give the control over to the target label.
type BranchTargetDrop struct {
	Target LabelID
	ToDrop *InclusiveRange
}

// String implements fmt.Stringer.
func (b BranchTargetDrop) String() (ret string) {
	if b.ToDrop != nil {
		ret = fmt.Sprintf("%s(drop %d..%d)", b.Target, b.ToDrop.Start, b.ToDrop.End)
	} else {
		ret = b.Target.String()
	}
	return
}

// UnionOperation implements Operation and is the compilation (engine.lowerIR) result of a wazeroir.Operation.
//
// Not all operations result in a UnionOperation, e.g. wazeroir.OperationI32ReinterpretFromF32, and some operations are
// more complex than others, e.g. wazeroir.NewOperationBrTable.
//
// Note: This is a form of union type as it can store fields needed for any operation. Hence, most fields are opaque and
// only relevant when in context of its kind.
type UnionOperation struct {
	// OpKind determines how to interpret the other fields in this struct.
	OpKind   OperationKind
	B1, B2   byte
	B3       bool
	U1, U2   uint64
	Us       []uint64
	Rs       []*InclusiveRange
	SourcePC uint64
}

// String implements fmt.Stringer.
func (o UnionOperation) String() string {
	switch o.OpKind {
	case OperationKindUnreachable,
		OperationKindSelect,
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
		OperationKindMemoryInit,
		OperationKindDataDrop,
		OperationKindMemoryCopy,
		OperationKindMemoryFill,
		OperationKindTableInit,
		OperationKindElemDrop,
		OperationKindTableCopy,
		OperationKindRefFunc,
		OperationKindTableGet,
		OperationKindTableSet,
		OperationKindTableSize,
		OperationKindTableGrow,
		OperationKindTableFill,
		OperationKindBuiltinFunctionCheckExitCode:
		return o.Kind().String()

	case OperationKindCall,
		OperationKindGlobalGet,
		OperationKindGlobalSet:
		return fmt.Sprintf("%s %d", o.Kind(), o.B1)

	case OperationKindLabel:
		return LabelID(o.U1).String()

	case OperationKindBr:
		return fmt.Sprintf("%s %s", o.Kind(), LabelID(o.U1).String())

	case OperationKindBrIf:
		var thenTarget LabelID
		var elseTarget LabelID
		if len(o.Us) > 0 {
			thenTarget = LabelID(o.Us[0])
			elseTarget = LabelID(o.Us[1])
		}
		return fmt.Sprintf("%s %s, %s", o.Kind(), thenTarget, elseTarget)

	case OperationKindBrTable:
		var targets []string
		var defaultLabel LabelID
		if len(o.Us) > 0 {
			targets = make([]string, len(o.Us)-1)
			for i, t := range o.Us[1:] {
				targets[i] = LabelID(t).String()
			}
			defaultLabel = LabelID(o.Us[0])
		}
		return fmt.Sprintf("%s [%s] %s", o.Kind(), strings.Join(targets, ","), defaultLabel)

	case OperationKindCallIndirect:
		return fmt.Sprintf("%s: type=%d, table=%d", o.Kind(), o.U1, o.U2)

	case OperationKindDrop:
		start := -1
		end := -1
		if len(o.Rs) > 0 {
			start = o.Rs[0].Start
			end = o.Rs[0].End
		}
		return fmt.Sprintf("%s %d..%d", o.Kind(), start, end)

	case OperationKindPick, OperationKindSet:
		return fmt.Sprintf("%s %d (is_vector=%v)", o.Kind(), o.U1, o.B3)

	case OperationKindLoad, OperationKindStore:
		return fmt.Sprintf("%s.%s (align=%d, offset=%d)", UnsignedType(o.B1), o.Kind(), o.U1, o.U2)

	case OperationKindLoad8,
		OperationKindLoad16:
		return fmt.Sprintf("%s.%s (align=%d, offset=%d)", SignedType(o.B1), o.Kind(), o.U1, o.U2)

	case OperationKindStore8,
		OperationKindStore16,
		OperationKindStore32:
		return fmt.Sprintf("%s (align=%d, offset=%d)", o.Kind(), o.U1, o.U2)

	case OperationKindLoad32:
		var t string
		if o.B1 == 1 {
			t = "i64"
		} else {
			t = "u64"
		}
		return fmt.Sprintf("%s.%s (align=%d, offset=%d)", t, o.Kind(), o.U1, o.U2)

	case OperationKindEq,
		OperationKindNe,
		OperationKindAdd,
		OperationKindSub,
		OperationKindMul:
		return fmt.Sprintf("%s.%s", UnsignedType(o.B1), o.Kind())

	case OperationKindEqz,
		OperationKindClz,
		OperationKindCtz,
		OperationKindPopcnt,
		OperationKindAnd,
		OperationKindOr,
		OperationKindXor,
		OperationKindShl,
		OperationKindRotl,
		OperationKindRotr:
		return fmt.Sprintf("%s.%s", UnsignedInt(o.B1), o.Kind())

	case OperationKindRem, OperationKindShr:
		return fmt.Sprintf("%s.%s", SignedInt(o.B1), o.Kind())

	case OperationKindLt,
		OperationKindGt,
		OperationKindLe,
		OperationKindGe,
		OperationKindDiv:
		return fmt.Sprintf("%s.%s", SignedType(o.B1), o.Kind())

	case OperationKindAbs,
		OperationKindNeg,
		OperationKindCeil,
		OperationKindFloor,
		OperationKindTrunc,
		OperationKindNearest,
		OperationKindSqrt,
		OperationKindMin,
		OperationKindMax,
		OperationKindCopysign:
		return fmt.Sprintf("%s.%s", Float(o.B1), o.Kind())

	case OperationKindConstI32,
		OperationKindConstI64:
		return fmt.Sprintf("%s %#x", o.Kind(), o.U1)

	case OperationKindConstF32:
		return fmt.Sprintf("%s %f", o.Kind(), math.Float32frombits(uint32(o.U1)))
	case OperationKindConstF64:
		return fmt.Sprintf("%s %f", o.Kind(), math.Float64frombits(o.U1))

	case OperationKindITruncFromF:
		return fmt.Sprintf("%s.%s.%s (non_trapping=%v)", SignedInt(o.B2), o.Kind(), Float(o.B1), o.B3)
	case OperationKindFConvertFromI:
		return fmt.Sprintf("%s.%s.%s", Float(o.B2), o.Kind(), SignedInt(o.B1))
	case OperationKindExtend:
		var in, out string
		if o.B3 {
			in = "i32"
			out = "i64"
		} else {
			in = "u32"
			out = "u64"
		}
		return fmt.Sprintf("%s.%s.%s", out, o.Kind(), in)

	case OperationKindV128Const:
		return fmt.Sprintf("%s [%#x, %#x]", o.Kind(), o.U1, o.U2)
	case OperationKindV128Add,
		OperationKindV128Sub:
		return fmt.Sprintf("%s (shape=%s)", o.Kind(), shapeName(o.B1))
	case OperationKindV128Load,
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
		OperationKindV128Narrow:
		return o.Kind().String()

	case OperationKindV128ITruncSatFromF:
		if o.B3 {
			return fmt.Sprintf("%s.%sS", o.Kind(), shapeName(o.B1))
		} else {
			return fmt.Sprintf("%s.%sU", o.Kind(), shapeName(o.B1))
		}

	default:
		panic(fmt.Sprintf("TODO: %v", o.OpKind))
	}
}

// Kind implements Operation.Kind
func (o UnionOperation) Kind() OperationKind {
	return o.OpKind
}

// NewOperationUnreachable is a constructor for UnionOperation with Kind OperationKindUnreachable
//
// This corresponds to wasm.OpcodeUnreachable.
//
// The engines are expected to exit the execution with wasmruntime.ErrRuntimeUnreachable error.
func NewOperationUnreachable() UnionOperation {
	return UnionOperation{OpKind: OperationKindUnreachable}
}

// NewOperationLabel is a constructor for UnionOperation with Kind OperationKindLabel.
//
// This is used to inform the engines of the beginning of a label.
func NewOperationLabel(labelID LabelID) UnionOperation {
	return UnionOperation{OpKind: OperationKindLabel, U1: uint64(labelID)}
}

// NewOperationBr is a constructor for UnionOperation with Kind OperationKindBr.
//
// The engines are expected to branch into OperationBr.Target (U1) label.
func NewOperationBr(target LabelID) UnionOperation {
	return UnionOperation{OpKind: OperationKindBr, U1: uint64(target)}
}

// NewOperationBrIf is a constructor for UnionOperation with Kind OperationKindBrIf.
//
// The engines are expected to pop a value and branch into OperationBrIf.Then (Us[0]) label if the value equals 1.
// Otherwise, the code branches into OperationBrIf.Else (Us[1]) label.
func NewOperationBrIf(thenTarget, elseTarget BranchTargetDrop) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindBrIf,
		Us:     []uint64{uint64(thenTarget.Target), uint64(elseTarget.Target)},
		Rs:     []*InclusiveRange{thenTarget.ToDrop, elseTarget.ToDrop},
	}
}

// NewOperationBrTable is a constructor for UnionOperation with Kind OperationKindBrTable.
//
// This corresponds to wasm.OpcodeBrTableName except that the label
// here means the wazeroir level, not the ones of Wasm.
//
// The engines are expected to do the br_table operation base on the default (Us[0], Rs[0]) and
// targets (Us[1:], Rs[1:]). More precisely, this pops a value from the stack (called "index")
// and decides which branch we go into next based on the value.
//
// For example, assume we have operations like {default: L_DEFAULT, targets: [L0, L1, L2]}.
// If "index" >= len(defaults), then branch into the L_DEFAULT label.
// Otherwise, we enter label of targets[index].
//
// if targetRanges is nil, it is padded to the length of targetLabels
func NewOperationBrTable(targetLabels []uint64, targetRanges []*InclusiveRange) UnionOperation {
	if targetRanges == nil {
		targetRanges = make([]*InclusiveRange, len(targetLabels))
	}
	return UnionOperation{
		OpKind: OperationKindBrTable,
		Us:     targetLabels,
		Rs:     targetRanges,
	}
}

// NewOperationCall is a constructor for UnionOperation with Kind OperationKindCall.
//
// This corresponds to wasm.OpcodeCallName, and engines are expected to
// enter into a function whose index equals OperationCall.FunctionIndex.
func NewOperationCall(functionIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindCall, U1: uint64(functionIndex)}
}

// NewOperationCallIndirect implements Operation.
//
// This corresponds to wasm.OpcodeCallIndirectName, and engines are expected to
// consume the one value from the top of stack (called "offset"),
// and make a function call against the function whose function address equals
// Tables[OperationCallIndirect.TableIndex][offset].
//
// Note: This is called indirect function call in the sense that the target function is indirectly
// determined by the current state (top value) of the stack.
// Therefore, two checks are performed at runtime before entering the target function:
// 1) whether "offset" exceeds the length of table Tables[OperationCallIndirect.TableIndex].
// 2) whether the type of the function table[offset] matches the function type specified by OperationCallIndirect.TypeIndex.
func NewOperationCallIndirect(typeIndex, tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindCallIndirect, U1: uint64(typeIndex), U2: uint64(tableIndex)}
}

// InclusiveRange is the range which spans across the value stack starting from the top to the bottom, and
// both boundary are included in the range.
type InclusiveRange struct {
	Start, End int
}

// NewOperationDrop is a constructor for UnionOperation with Kind OperationKindDrop.
//
// The engines are expected to discard the values selected by NewOperationDrop.Depth which
// starts from the top of the stack to the bottom.
//
// depth spans across the uint64 value stack at runtime to be dropped by this operation.
func NewOperationDrop(depth *InclusiveRange) UnionOperation {
	return UnionOperation{OpKind: OperationKindDrop, Rs: []*InclusiveRange{depth}}
}

// NewOperationSelect is a constructor for UnionOperation with Kind OperationKindSelect.
//
// This corresponds to wasm.OpcodeSelect.
//
// The engines are expected to pop three values, say [..., x2, x1, c], then if the value "c" equals zero,
// "x1" is pushed back onto the stack and, otherwise "x2" is pushed back.
//
// isTargetVector true if the selection target value's type is wasm.ValueTypeV128.
func NewOperationSelect(isTargetVector bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindSelect, B3: isTargetVector}
}

// NewOperationPick is a constructor for UnionOperation with Kind OperationKindPick.
//
// The engines are expected to copy a value pointed by depth, and push the
// copied value onto the top of the stack.
//
// depth is the location of the pick target in the uint64 value stack at runtime.
// If isTargetVector=true, this points to the location of the lower 64-bits of the vector.
func NewOperationPick(depth int, isTargetVector bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindPick, U1: uint64(depth), B3: isTargetVector}
}

// NewOperationSet is a constructor for UnionOperation with Kind OperationKindSet.
//
// The engines are expected to set the top value of the stack to the location specified by
// depth.
//
// depth is the location of the set target in the uint64 value stack at runtime.
// If isTargetVector=true, this points the location of the lower 64-bits of the vector.
func NewOperationSet(depth int, isTargetVector bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindSet, U1: uint64(depth), B3: isTargetVector}
}

// NewOperationGlobalGet is a constructor for UnionOperation with Kind OperationKindGlobalGet.
//
// The engines are expected to read the global value specified by OperationGlobalGet.Index,
// and push the copy of the value onto the stack.
//
// See wasm.OpcodeGlobalGet.
func NewOperationGlobalGet(index uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindGlobalGet, U1: uint64(index)}
}

// NewOperationGlobalSet is a constructor for UnionOperation with Kind OperationKindGlobalSet.
//
// The engines are expected to consume the value from the top of the stack,
// and write the value into the global specified by OperationGlobalSet.Index.
//
// See wasm.OpcodeGlobalSet.
func NewOperationGlobalSet(index uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindGlobalSet, U1: uint64(index)}
}

// MemoryArg is the "memarg" to all memory instructions.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instructions%E2%91%A0
type MemoryArg struct {
	// Alignment the expected alignment (expressed as the exponent of a power of 2). Default to the natural alignment.
	//
	// "Natural alignment" is defined here as the smallest power of two that can hold the size of the value type. Ex
	// wasm.ValueTypeI64 is encoded in 8 little-endian bytes. 2^3 = 8, so the natural alignment is three.
	Alignment uint32

	// Offset is the address offset added to the instruction's dynamic address operand, yielding a 33-bit effective
	// address that is the zero-based index at which the memory is accessed. Default to zero.
	Offset uint32
}

// NewOperationLoad is a constructor for UnionOperation with Kind OperationKindLoad.
//
// This corresponds to wasm.OpcodeI32LoadName wasm.OpcodeI64LoadName wasm.OpcodeF32LoadName and wasm.OpcodeF64LoadName.
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise load the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationLoad(unsignedType UnsignedType, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindLoad, B1: byte(unsignedType), U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationLoad8 is a constructor for UnionOperation with Kind OperationKindLoad8.
//
// This corresponds to wasm.OpcodeI32Load8SName wasm.OpcodeI32Load8UName wasm.OpcodeI64Load8SName wasm.OpcodeI64Load8UName.
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise load the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationLoad8(signedInt SignedInt, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindLoad8, B1: byte(signedInt), U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationLoad16 is a constructor for UnionOperation with Kind OperationKindLoad16.
//
// This corresponds to wasm.OpcodeI32Load16SName wasm.OpcodeI32Load16UName wasm.OpcodeI64Load16SName wasm.OpcodeI64Load16UName.
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise load the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationLoad16(signedInt SignedInt, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindLoad16, B1: byte(signedInt), U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationLoad32 is a constructor for UnionOperation with Kind OperationKindLoad32.
//
// This corresponds to wasm.OpcodeI64Load32SName wasm.OpcodeI64Load32UName.
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise load the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationLoad32(signed bool, arg MemoryArg) UnionOperation {
	sigB := byte(0)
	if signed {
		sigB = 1
	}
	return UnionOperation{OpKind: OperationKindLoad32, B1: sigB, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationStore is a constructor for UnionOperation with Kind OperationKindStore.
//
// # This corresponds to wasm.OpcodeI32StoreName wasm.OpcodeI64StoreName wasm.OpcodeF32StoreName wasm.OpcodeF64StoreName
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise store the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationStore(unsignedType UnsignedType, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindStore, B1: byte(unsignedType), U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationStore8 is a constructor for UnionOperation with Kind OperationKindStore8.
//
// # This corresponds to wasm.OpcodeI32Store8Name wasm.OpcodeI64Store8Name
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise store the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationStore8(arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindStore8, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationStore16 is a constructor for UnionOperation with Kind OperationKindStore16.
//
// # This corresponds to wasm.OpcodeI32Store16Name wasm.OpcodeI64Store16Name
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise store the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationStore16(arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindStore16, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationStore32 is a constructor for UnionOperation with Kind OperationKindStore32.
//
// # This corresponds to wasm.OpcodeI64Store32Name
//
// The engines are expected to check the boundary of memory length, and exit the execution if this exceeds the boundary,
// otherwise store the corresponding value following the semantics of the corresponding WebAssembly instruction.
func NewOperationStore32(arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindStore32, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationMemorySize is a constructor for UnionOperation with Kind OperationKindMemorySize.
//
// This corresponds to wasm.OpcodeMemorySize.
//
// The engines are expected to push the current page size of the memory onto the stack.
func NewOperationMemorySize() UnionOperation {
	return UnionOperation{OpKind: OperationKindMemorySize}
}

// NewOperationMemoryGrow is a constructor for UnionOperation with Kind OperationKindMemoryGrow.
//
// This corresponds to wasm.OpcodeMemoryGrow.
//
// The engines are expected to pop one value from the top of the stack, then
// execute wasm.MemoryInstance Grow with the value, and push the previous
// page size of the memory onto the stack.
func NewOperationMemoryGrow() UnionOperation {
	return UnionOperation{OpKind: OperationKindMemoryGrow}
}

// NewOperationConstI32 is a constructor for UnionOperation with Kind OperationConstI32.
//
// This corresponds to wasm.OpcodeI32Const.
func NewOperationConstI32(value uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindConstI32, U1: uint64(value)}
}

// NewOperationConstI64 is a constructor for UnionOperation with Kind OperationConstI64.
//
// This corresponds to wasm.OpcodeI64Const.
func NewOperationConstI64(value uint64) UnionOperation {
	return UnionOperation{OpKind: OperationKindConstI64, U1: value}
}

// NewOperationConstF32 is a constructor for UnionOperation with Kind OperationConstF32.
//
// This corresponds to wasm.OpcodeF32Const.
func NewOperationConstF32(value float32) UnionOperation {
	return UnionOperation{OpKind: OperationKindConstF32, U1: uint64(math.Float32bits(value))}
}

// NewOperationConstF64 is a constructor for UnionOperation with Kind OperationConstF64.
//
// This corresponds to wasm.OpcodeF64Const.
func NewOperationConstF64(value float64) UnionOperation {
	return UnionOperation{OpKind: OperationKindConstF64, U1: math.Float64bits(value)}
}

// NewOperationEq is a constructor for UnionOperation with Kind OperationKindEq.
//
// This corresponds to wasm.OpcodeI32EqName wasm.OpcodeI64EqName wasm.OpcodeF32EqName wasm.OpcodeF64EqName
func NewOperationEq(b UnsignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindEq, B1: byte(b)}
}

// NewOperationNe is a constructor for UnionOperation with Kind OperationKindNe.
//
// This corresponds to wasm.OpcodeI32NeName wasm.OpcodeI64NeName wasm.OpcodeF32NeName wasm.OpcodeF64NeName
func NewOperationNe(b UnsignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindNe, B1: byte(b)}
}

// NewOperationEqz is a constructor for UnionOperation with Kind OperationKindEqz.
//
// This corresponds to wasm.OpcodeI32EqzName wasm.OpcodeI64EqzName
func NewOperationEqz(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindEqz, B1: byte(b)}
}

// NewOperationLt is a constructor for UnionOperation with Kind OperationKindLt.
//
// This corresponds to wasm.OpcodeI32LtS wasm.OpcodeI32LtU wasm.OpcodeI64LtS wasm.OpcodeI64LtU wasm.OpcodeF32Lt wasm.OpcodeF64Lt
func NewOperationLt(b SignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindLt, B1: byte(b)}
}

// NewOperationGt is a constructor for UnionOperation with Kind OperationKindGt.
//
// This corresponds to wasm.OpcodeI32GtS wasm.OpcodeI32GtU wasm.OpcodeI64GtS wasm.OpcodeI64GtU wasm.OpcodeF32Gt wasm.OpcodeF64Gt
func NewOperationGt(b SignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindGt, B1: byte(b)}
}

// NewOperationLe is a constructor for UnionOperation with Kind OperationKindLe.
//
// This corresponds to wasm.OpcodeI32LeS wasm.OpcodeI32LeU wasm.OpcodeI64LeS wasm.OpcodeI64LeU wasm.OpcodeF32Le wasm.OpcodeF64Le
func NewOperationLe(b SignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindLe, B1: byte(b)}
}

// NewOperationGe is a constructor for UnionOperation with Kind OperationKindGe.
//
// This corresponds to wasm.OpcodeI32GeS wasm.OpcodeI32GeU wasm.OpcodeI64GeS wasm.OpcodeI64GeU wasm.OpcodeF32Ge wasm.OpcodeF64Ge
// NewOperationGe is the constructor for OperationGe
func NewOperationGe(b SignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindGe, B1: byte(b)}
}

// NewOperationAdd is a constructor for UnionOperation with Kind OperationKindAdd.
//
// This corresponds to wasm.OpcodeI32AddName wasm.OpcodeI64AddName wasm.OpcodeF32AddName wasm.OpcodeF64AddName.
func NewOperationAdd(b UnsignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindAdd, B1: byte(b)}
}

// NewOperationSub is a constructor for UnionOperation with Kind OperationKindSub.
//
// This corresponds to wasm.OpcodeI32SubName wasm.OpcodeI64SubName wasm.OpcodeF32SubName wasm.OpcodeF64SubName.
func NewOperationSub(b UnsignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindSub, B1: byte(b)}
}

// NewOperationMul is a constructor for UnionOperation with Kind wperationKindMul.
//
// This corresponds to wasm.OpcodeI32MulName wasm.OpcodeI64MulName wasm.OpcodeF32MulName wasm.OpcodeF64MulName.
// NewOperationMul is the constructor for OperationMul
func NewOperationMul(b UnsignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindMul, B1: byte(b)}
}

// NewOperationClz is a constructor for UnionOperation with Kind OperationKindClz.
//
// This corresponds to wasm.OpcodeI32ClzName wasm.OpcodeI64ClzName.
//
// The engines are expected to count up the leading zeros in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0x00_ff_ff_ff] results in [..., 8].
// See wasm.OpcodeI32Clz wasm.OpcodeI64Clz
func NewOperationClz(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindClz, B1: byte(b)}
}

// NewOperationCtz is a constructor for UnionOperation with Kind OperationKindCtz.
//
// This corresponds to wasm.OpcodeI32CtzName wasm.OpcodeI64CtzName.
//
// The engines are expected to count up the trailing zeros in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0xff_ff_ff_00] results in [..., 8].
func NewOperationCtz(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindCtz, B1: byte(b)}
}

// NewOperationPopcnt is a constructor for UnionOperation with Kind OperationKindPopcnt.
//
// This corresponds to wasm.OpcodeI32PopcntName wasm.OpcodeI64PopcntName.
//
// The engines are expected to count up the number of set bits in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0b00_00_00_11] results in [..., 2].
func NewOperationPopcnt(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindPopcnt, B1: byte(b)}
}

// NewOperationDiv is a constructor for UnionOperation with Kind OperationKindDiv.
//
// This corresponds to wasm.OpcodeI32DivS wasm.OpcodeI32DivU wasm.OpcodeI64DivS
//
//	wasm.OpcodeI64DivU wasm.OpcodeF32Div wasm.OpcodeF64Div.
func NewOperationDiv(b SignedType) UnionOperation {
	return UnionOperation{OpKind: OperationKindDiv, B1: byte(b)}
}

// NewOperationRem is a constructor for UnionOperation with Kind OperationKindRem.
//
// This corresponds to wasm.OpcodeI32RemS wasm.OpcodeI32RemU wasm.OpcodeI64RemS wasm.OpcodeI64RemU.
//
// The engines are expected to perform division on the top
// two values of integer type on the stack and puts the remainder of the result
// onto the stack. For example, stack [..., 10, 3] results in [..., 1] where
// the quotient is discarded.
// NewOperationRem is the constructor for OperationRem
func NewOperationRem(b SignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindRem, B1: byte(b)}
}

// NewOperationAnd is a constructor for UnionOperation with Kind OperationKindAnd.
//
// # This corresponds to wasm.OpcodeI32AndName wasm.OpcodeI64AndName
//
// The engines are expected to perform "And" operation on
// top two values on the stack, and pushes the result.
func NewOperationAnd(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindAnd, B1: byte(b)}
}

// NewOperationOr is a constructor for UnionOperation with Kind OperationKindOr.
//
// # This corresponds to wasm.OpcodeI32OrName wasm.OpcodeI64OrName
//
// The engines are expected to perform "Or" operation on
// top two values on the stack, and pushes the result.
func NewOperationOr(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindOr, B1: byte(b)}
}

// NewOperationXor is a constructor for UnionOperation with Kind OperationKindXor.
//
// # This corresponds to wasm.OpcodeI32XorName wasm.OpcodeI64XorName
//
// The engines are expected to perform "Xor" operation on
// top two values on the stack, and pushes the result.
func NewOperationXor(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindXor, B1: byte(b)}
}

// NewOperationShl is a constructor for UnionOperation with Kind OperationKindShl.
//
// # This corresponds to wasm.OpcodeI32ShlName wasm.OpcodeI64ShlName
//
// The engines are expected to perform "Shl" operation on
// top two values on the stack, and pushes the result.
func NewOperationShl(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindShl, B1: byte(b)}
}

// NewOperationShr is a constructor for UnionOperation with Kind OperationKindShr.
//
// # This corresponds to wasm.OpcodeI32ShrSName wasm.OpcodeI32ShrUName wasm.OpcodeI64ShrSName wasm.OpcodeI64ShrUName
//
// If OperationShr.Type is signed integer, then, the engines are expected to perform arithmetic right shift on the two
// top values on the stack, otherwise do the logical right shift.
func NewOperationShr(b SignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindShr, B1: byte(b)}
}

// NewOperationRotl is a constructor for UnionOperation with Kind OperationKindRotl.
//
// # This corresponds to wasm.OpcodeI32RotlName wasm.OpcodeI64RotlName
//
// The engines are expected to perform "Rotl" operation on
// top two values on the stack, and pushes the result.
func NewOperationRotl(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindRotl, B1: byte(b)}
}

// NewOperationRotr is a constructor for UnionOperation with Kind OperationKindRotr.
//
// # This corresponds to wasm.OpcodeI32RotrName wasm.OpcodeI64RotrName
//
// The engines are expected to perform "Rotr" operation on
// top two values on the stack, and pushes the result.
func NewOperationRotr(b UnsignedInt) UnionOperation {
	return UnionOperation{OpKind: OperationKindRotr, B1: byte(b)}
}

// NewOperationAbs is a constructor for UnionOperation with Kind OperationKindAbs.
//
// This corresponds to wasm.OpcodeF32Abs wasm.OpcodeF64Abs
func NewOperationAbs(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindAbs, B1: byte(b)}
}

// NewOperationNeg is a constructor for UnionOperation with Kind OperationKindNeg.
//
// This corresponds to wasm.OpcodeF32Neg wasm.OpcodeF64Neg
func NewOperationNeg(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindNeg, B1: byte(b)}
}

// NewOperationCeil is a constructor for UnionOperation with Kind OperationKindCeil.
//
// This corresponds to wasm.OpcodeF32CeilName wasm.OpcodeF64CeilName
func NewOperationCeil(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindCeil, B1: byte(b)}
}

// NewOperationFloor is a constructor for UnionOperation with Kind OperationKindFloor.
//
// This corresponds to wasm.OpcodeF32FloorName wasm.OpcodeF64FloorName
func NewOperationFloor(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindFloor, B1: byte(b)}
}

// NewOperationTrunc is a constructor for UnionOperation with Kind OperationKindTrunc.
//
// This corresponds to wasm.OpcodeF32TruncName wasm.OpcodeF64TruncName
func NewOperationTrunc(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindTrunc, B1: byte(b)}
}

// NewOperationNearest is a constructor for UnionOperation with Kind OperationKindNearest.
//
// # This corresponds to wasm.OpcodeF32NearestName wasm.OpcodeF64NearestName
//
// Note: this is *not* equivalent to math.Round and instead has the same
// the semantics of LLVM's rint intrinsic. See https://llvm.org/docs/LangRef.html#llvm-rint-intrinsic.
// For example, math.Round(-4.5) produces -5 while we want to produce -4.
func NewOperationNearest(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindNearest, B1: byte(b)}
}

// NewOperationSqrt is a constructor for UnionOperation with Kind OperationKindSqrt.
//
// This corresponds to wasm.OpcodeF32SqrtName wasm.OpcodeF64SqrtName
func NewOperationSqrt(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindSqrt, B1: byte(b)}
}

// NewOperationMin is a constructor for UnionOperation with Kind OperationKindMin.
//
// # This corresponds to wasm.OpcodeF32MinName wasm.OpcodeF64MinName
//
// The engines are expected to pop two values from the stack, and push back the maximum of
// these two values onto the stack. For example, stack [..., 100.1, 1.9] results in [..., 1.9].
//
// Note: WebAssembly specifies that min/max must always return NaN if one of values is NaN,
// which is a different behavior different from math.Min.
func NewOperationMin(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindMin, B1: byte(b)}
}

// NewOperationMax is a constructor for UnionOperation with Kind OperationKindMax.
//
// # This corresponds to wasm.OpcodeF32MaxName wasm.OpcodeF64MaxName
//
// The engines are expected to pop two values from the stack, and push back the maximum of
// these two values onto the stack. For example, stack [..., 100.1, 1.9] results in [..., 100.1].
//
// Note: WebAssembly specifies that min/max must always return NaN if one of values is NaN,
// which is a different behavior different from math.Max.
func NewOperationMax(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindMax, B1: byte(b)}
}

// NewOperationCopysign is a constructor for UnionOperation with Kind OperationKindCopysign.
//
// # This corresponds to wasm.OpcodeF32CopysignName wasm.OpcodeF64CopysignName
//
// The engines are expected to pop two float values from the stack, and copy the signbit of
// the first-popped value to the last one.
// For example, stack [..., 1.213, -5.0] results in [..., -1.213].
func NewOperationCopysign(b Float) UnionOperation {
	return UnionOperation{OpKind: OperationKindCopysign, B1: byte(b)}
}

// NewOperationI32WrapFromI64 is a constructor for UnionOperation with Kind OperationKindI32WrapFromI64.
//
// This corresponds to wasm.OpcodeI32WrapI64 and equivalent to uint64(uint32(v)) in Go.
//
// The engines are expected to replace the 64-bit int on top of the stack
// with the corresponding 32-bit integer.
func NewOperationI32WrapFromI64() UnionOperation {
	return UnionOperation{OpKind: OperationKindI32WrapFromI64}
}

// NewOperationITruncFromF is a constructor for UnionOperation with Kind OperationKindITruncFromF.
//
// This corresponds to
//
//	wasm.OpcodeI32TruncF32SName wasm.OpcodeI32TruncF32UName wasm.OpcodeI32TruncF64SName
//	wasm.OpcodeI32TruncF64UName wasm.OpcodeI64TruncF32SName wasm.OpcodeI64TruncF32UName wasm.OpcodeI64TruncF64SName
//	wasm.OpcodeI64TruncF64UName. wasm.OpcodeI32TruncSatF32SName wasm.OpcodeI32TruncSatF32UName
//	wasm.OpcodeI32TruncSatF64SName wasm.OpcodeI32TruncSatF64UName wasm.OpcodeI64TruncSatF32SName
//	wasm.OpcodeI64TruncSatF32UName wasm.OpcodeI64TruncSatF64SName wasm.OpcodeI64TruncSatF64UName
//
// See [1] and [2] for when we encounter undefined behavior in the WebAssembly specification if NewOperationITruncFromF.NonTrapping == false.
// To summarize, if the source float value is NaN or doesn't fit in the destination range of integers (incl. +=Inf),
// then the runtime behavior is undefined. In wazero, the engines are expected to exit the execution in these undefined cases with
// wasmruntime.ErrRuntimeInvalidConversionToInteger error.
//
// [1] https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefop-trunc-umathrmtruncmathsfu_m-n-z for unsigned integers.
// [2] https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefop-trunc-smathrmtruncmathsfs_m-n-z for signed integers.
//
// nonTrapping true if this conversion is "nontrapping" in the sense of the
// https://github.com/WebAssembly/spec/blob/ce4b6c4d47eb06098cc7ab2e81f24748da822f20/proposals/nontrapping-float-to-int-conversion/Overview.md
func NewOperationITruncFromF(inputType Float, outputType SignedInt, nonTrapping bool) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindITruncFromF,
		B1:     byte(inputType),
		B2:     byte(outputType),
		B3:     nonTrapping,
	}
}

// NewOperationFConvertFromI is a constructor for UnionOperation with Kind OperationKindFConvertFromI.
//
// This corresponds to
//
//	wasm.OpcodeF32ConvertI32SName wasm.OpcodeF32ConvertI32UName wasm.OpcodeF32ConvertI64SName wasm.OpcodeF32ConvertI64UName
//	wasm.OpcodeF64ConvertI32SName wasm.OpcodeF64ConvertI32UName wasm.OpcodeF64ConvertI64SName wasm.OpcodeF64ConvertI64UName
//
// and equivalent to float32(uint32(x)), float32(int32(x)), etc in Go.
func NewOperationFConvertFromI(inputType SignedInt, outputType Float) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindFConvertFromI,
		B1:     byte(inputType),
		B2:     byte(outputType),
	}
}

// NewOperationF32DemoteFromF64 is a constructor for UnionOperation with Kind OperationKindF32DemoteFromF64.
//
// This corresponds to wasm.OpcodeF32DemoteF64 and is equivalent float32(float64(v)).
func NewOperationF32DemoteFromF64() UnionOperation {
	return UnionOperation{OpKind: OperationKindF32DemoteFromF64}
}

// NewOperationF64PromoteFromF32 is a constructor for UnionOperation with Kind OperationKindF64PromoteFromF32.
//
// This corresponds to wasm.OpcodeF64PromoteF32 and is equivalent float64(float32(v)).
func NewOperationF64PromoteFromF32() UnionOperation {
	return UnionOperation{OpKind: OperationKindF64PromoteFromF32}
}

// NewOperationI32ReinterpretFromF32 is a constructor for UnionOperation with Kind OperationKindI32ReinterpretFromF32.
//
// This corresponds to wasm.OpcodeI32ReinterpretF32Name.
func NewOperationI32ReinterpretFromF32() UnionOperation {
	return UnionOperation{OpKind: OperationKindI32ReinterpretFromF32}
}

// NewOperationI64ReinterpretFromF64 is a constructor for UnionOperation with Kind OperationKindI64ReinterpretFromF64.
//
// This corresponds to wasm.OpcodeI64ReinterpretF64Name.
func NewOperationI64ReinterpretFromF64() UnionOperation {
	return UnionOperation{OpKind: OperationKindI64ReinterpretFromF64}
}

// NewOperationF32ReinterpretFromI32 is a constructor for UnionOperation with Kind OperationKindF32ReinterpretFromI32.
//
// This corresponds to wasm.OpcodeF32ReinterpretI32Name.
func NewOperationF32ReinterpretFromI32() UnionOperation {
	return UnionOperation{OpKind: OperationKindF32ReinterpretFromI32}
}

// NewOperationF64ReinterpretFromI64 is a constructor for UnionOperation with Kind OperationKindF64ReinterpretFromI64.
//
// This corresponds to wasm.OpcodeF64ReinterpretI64Name.
func NewOperationF64ReinterpretFromI64() UnionOperation {
	return UnionOperation{OpKind: OperationKindF64ReinterpretFromI64}
}

// NewOperationExtend is a constructor for UnionOperation with Kind OperationKindExtend.
//
// # This corresponds to wasm.OpcodeI64ExtendI32SName wasm.OpcodeI64ExtendI32UName
//
// The engines are expected to extend the 32-bit signed or unsigned int on top of the stack
// as a 64-bit integer of corresponding signedness. For unsigned case, this is just reinterpreting the
// underlying bit pattern as 64-bit integer. For signed case, this is sign-extension which preserves the
// original integer's sign.
func NewOperationExtend(signed bool) UnionOperation {
	op := UnionOperation{OpKind: OperationKindExtend}
	if signed {
		op.B1 = 1
	}
	return op
}

// NewOperationSignExtend32From8 is a constructor for UnionOperation with Kind OperationKindSignExtend32From8.
//
// This corresponds to wasm.OpcodeI32Extend8SName.
//
// The engines are expected to sign-extend the first 8-bits of 32-bit in as signed 32-bit int.
func NewOperationSignExtend32From8() UnionOperation {
	return UnionOperation{OpKind: OperationKindSignExtend32From8}
}

// NewOperationSignExtend32From16 is a constructor for UnionOperation with Kind OperationKindSignExtend32From16.
//
// This corresponds to wasm.OpcodeI32Extend16SName.
//
// The engines are expected to sign-extend the first 16-bits of 32-bit in as signed 32-bit int.
func NewOperationSignExtend32From16() UnionOperation {
	return UnionOperation{OpKind: OperationKindSignExtend32From16}
}

// NewOperationSignExtend64From8 is a constructor for UnionOperation with Kind OperationKindSignExtend64From8.
//
// This corresponds to wasm.OpcodeI64Extend8SName.
//
// The engines are expected to sign-extend the first 8-bits of 64-bit in as signed 32-bit int.
func NewOperationSignExtend64From8() UnionOperation {
	return UnionOperation{OpKind: OperationKindSignExtend64From8}
}

// NewOperationSignExtend64From16 is a constructor for UnionOperation with Kind OperationKindSignExtend64From16.
//
// This corresponds to wasm.OpcodeI64Extend16SName.
//
// The engines are expected to sign-extend the first 16-bits of 64-bit in as signed 32-bit int.
func NewOperationSignExtend64From16() UnionOperation {
	return UnionOperation{OpKind: OperationKindSignExtend64From16}
}

// NewOperationSignExtend64From32 is a constructor for UnionOperation with Kind OperationKindSignExtend64From32.
//
// This corresponds to wasm.OpcodeI64Extend32SName.
//
// The engines are expected to sign-extend the first 32-bits of 64-bit in as signed 32-bit int.
func NewOperationSignExtend64From32() UnionOperation {
	return UnionOperation{OpKind: OperationKindSignExtend64From32}
}

// NewOperationMemoryInit is a constructor for UnionOperation with Kind OperationKindMemoryInit.
//
// This corresponds to wasm.OpcodeMemoryInitName.
//
// dataIndex is the index of the data instance in ModuleInstance.DataInstances
// by which this operation instantiates a part of the memory.
func NewOperationMemoryInit(dataIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindMemoryInit, U1: uint64(dataIndex)}
}

// NewOperationDataDrop implements Operation.
//
// This corresponds to wasm.OpcodeDataDropName.
//
// dataIndex is the index of the data instance in ModuleInstance.DataInstances
// which this operation drops.
func NewOperationDataDrop(dataIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindDataDrop, U1: uint64(dataIndex)}
}

// NewOperationMemoryCopy is a consuctor for UnionOperation with Kind OperationKindMemoryCopy.
//
// This corresponds to wasm.OpcodeMemoryCopyName.
func NewOperationMemoryCopy() UnionOperation {
	return UnionOperation{OpKind: OperationKindMemoryCopy}
}

// NewOperationMemoryFill is a consuctor for UnionOperation with Kind OperationKindMemoryFill.
func NewOperationMemoryFill() UnionOperation {
	return UnionOperation{OpKind: OperationKindMemoryFill}
}

// NewOperationTableInit is a constructor for UnionOperation with Kind OperationKindTableInit.
//
// This corresponds to wasm.OpcodeTableInitName.
//
// elemIndex is the index of the element by which this operation initializes a part of the table.
// tableIndex is the index of the table on which this operation initialize by the target element.
func NewOperationTableInit(elemIndex, tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableInit, U1: uint64(elemIndex), U2: uint64(tableIndex)}
}

// NewOperationElemDrop is a constructor for UnionOperation with Kind OperationKindElemDrop.
//
// This corresponds to wasm.OpcodeElemDropName.
//
// elemIndex is the index of the element which this operation drops.
func NewOperationElemDrop(elemIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindElemDrop, U1: uint64(elemIndex)}
}

// NewOperationTableCopy implements Operation.
//
// This corresponds to wasm.OpcodeTableCopyName.
func NewOperationTableCopy(srcTableIndex, dstTableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableCopy, U1: uint64(srcTableIndex), U2: uint64(dstTableIndex)}
}

// NewOperationRefFunc constructor for UnionOperation with Kind OperationKindRefFunc.
//
// This corresponds to wasm.OpcodeRefFuncName, and engines are expected to
// push the opaque pointer value of engine specific func for the given FunctionIndex.
//
// Note: in wazero, we express any reference types (funcref or externref) as opaque pointers which is uint64.
// Therefore, the engine implementations emit instructions to push the address of *function onto the stack.
func NewOperationRefFunc(functionIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindRefFunc, U1: uint64(functionIndex)}
}

// NewOperationTableGet constructor for UnionOperation with Kind OperationKindTableGet.
//
// This corresponds to wasm.OpcodeTableGetName.
func NewOperationTableGet(tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableGet, U1: uint64(tableIndex)}
}

// NewOperationTableSet constructor for UnionOperation with Kind OperationKindTableSet.
//
// This corresponds to wasm.OpcodeTableSetName.
func NewOperationTableSet(tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableSet, U1: uint64(tableIndex)}
}

// NewOperationTableSize constructor for UnionOperation with Kind OperationKindTableSize.
//
// This corresponds to wasm.OpcodeTableSizeName.
func NewOperationTableSize(tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableSize, U1: uint64(tableIndex)}
}

// NewOperationTableGrow constructor for UnionOperation with Kind OperationKindTableGrow.
//
// This corresponds to wasm.OpcodeTableGrowName.
func NewOperationTableGrow(tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableGrow, U1: uint64(tableIndex)}
}

// NewOperationTableFill constructor for UnionOperation with Kind OperationKindTableFill.
//
// This corresponds to wasm.OpcodeTableFillName.
func NewOperationTableFill(tableIndex uint32) UnionOperation {
	return UnionOperation{OpKind: OperationKindTableFill, U1: uint64(tableIndex)}
}

// NewOperationV128Const constructor for UnionOperation with Kind OperationKindV128Const
func NewOperationV128Const(lo, hi uint64) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Const, U1: lo, U2: hi}
}

// Shape corresponds to a shape of v128 values.
// https://webassembly.github.io/spec/core/syntax/instructions.html#syntax-shape
type Shape = byte

const (
	ShapeI8x16 Shape = iota
	ShapeI16x8
	ShapeI32x4
	ShapeI64x2
	ShapeF32x4
	ShapeF64x2
)

func shapeName(s Shape) (ret string) {
	switch s {
	case ShapeI8x16:
		ret = "I8x16"
	case ShapeI16x8:
		ret = "I16x8"
	case ShapeI32x4:
		ret = "I32x4"
	case ShapeI64x2:
		ret = "I64x2"
	case ShapeF32x4:
		ret = "F32x4"
	case ShapeF64x2:
		ret = "F64x2"
	}
	return
}

// NewOperationV128Add constructor for UnionOperation with Kind OperationKindV128Add.
//
// This corresponds to wasm.OpcodeVecI8x16AddName wasm.OpcodeVecI16x8AddName wasm.OpcodeVecI32x4AddName
//
//	wasm.OpcodeVecI64x2AddName wasm.OpcodeVecF32x4AddName wasm.OpcodeVecF64x2AddName
func NewOperationV128Add(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Add, B1: shape}
}

// NewOperationV128Sub constructor for UnionOperation with Kind OperationKindV128Sub.
//
// This corresponds to wasm.OpcodeVecI8x16SubName wasm.OpcodeVecI16x8SubName wasm.OpcodeVecI32x4SubName
//
//	wasm.OpcodeVecI64x2SubName wasm.OpcodeVecF32x4SubName wasm.OpcodeVecF64x2SubName
func NewOperationV128Sub(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Sub, B1: shape}
}

// V128LoadType represents a type of wasm.OpcodeVecV128Load* instructions.
type V128LoadType = byte

const (
	// V128LoadType128 corresponds to wasm.OpcodeVecV128LoadName.
	V128LoadType128 V128LoadType = iota
	// V128LoadType8x8s corresponds to wasm.OpcodeVecV128Load8x8SName.
	V128LoadType8x8s
	// V128LoadType8x8u corresponds to wasm.OpcodeVecV128Load8x8UName.
	V128LoadType8x8u
	// V128LoadType16x4s corresponds to wasm.OpcodeVecV128Load16x4SName
	V128LoadType16x4s
	// V128LoadType16x4u corresponds to wasm.OpcodeVecV128Load16x4UName
	V128LoadType16x4u
	// V128LoadType32x2s corresponds to wasm.OpcodeVecV128Load32x2SName
	V128LoadType32x2s
	// V128LoadType32x2u corresponds to wasm.OpcodeVecV128Load32x2UName
	V128LoadType32x2u
	// V128LoadType8Splat corresponds to wasm.OpcodeVecV128Load8SplatName
	V128LoadType8Splat
	// V128LoadType16Splat corresponds to wasm.OpcodeVecV128Load16SplatName
	V128LoadType16Splat
	// V128LoadType32Splat corresponds to wasm.OpcodeVecV128Load32SplatName
	V128LoadType32Splat
	// V128LoadType64Splat corresponds to wasm.OpcodeVecV128Load64SplatName
	V128LoadType64Splat
	// V128LoadType32zero corresponds to wasm.OpcodeVecV128Load32zeroName
	V128LoadType32zero
	// V128LoadType64zero corresponds to wasm.OpcodeVecV128Load64zeroName
	V128LoadType64zero
)

// NewOperationV128Load is a constructor for UnionOperation with Kind OperationKindV128Load.
//
// This corresponds to
//
//	wasm.OpcodeVecV128LoadName wasm.OpcodeVecV128Load8x8SName wasm.OpcodeVecV128Load8x8UName
//	wasm.OpcodeVecV128Load16x4SName wasm.OpcodeVecV128Load16x4UName wasm.OpcodeVecV128Load32x2SName
//	wasm.OpcodeVecV128Load32x2UName wasm.OpcodeVecV128Load8SplatName wasm.OpcodeVecV128Load16SplatName
//	wasm.OpcodeVecV128Load32SplatName wasm.OpcodeVecV128Load64SplatName wasm.OpcodeVecV128Load32zeroName
//	wasm.OpcodeVecV128Load64zeroName
func NewOperationV128Load(loadType V128LoadType, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Load, B1: loadType, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationV128LoadLane is a constructor for UnionOperation with Kind OperationKindV128LoadLane.
//
// This corresponds to wasm.OpcodeVecV128Load8LaneName wasm.OpcodeVecV128Load16LaneName
//
//	wasm.OpcodeVecV128Load32LaneName wasm.OpcodeVecV128Load64LaneName.
//
// laneIndex is >=0 && <(128/LaneSize).
// laneSize is either 8, 16, 32, or 64.
func NewOperationV128LoadLane(laneIndex, laneSize byte, arg MemoryArg) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128LoadLane, B1: laneSize, B2: laneIndex, U1: uint64(arg.Alignment), U2: uint64(arg.Offset)}
}

// NewOperationV128Store is a constructor for UnionOperation with Kind OperationKindV128Store.
//
// This corresponds to wasm.OpcodeVecV128Load8LaneName wasm.OpcodeVecV128Load16LaneName
//
//	wasm.OpcodeVecV128Load32LaneName wasm.OpcodeVecV128Load64LaneName.
func NewOperationV128Store(arg MemoryArg) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindV128Store,
		U1:     uint64(arg.Alignment),
		U2:     uint64(arg.Offset),
	}
}

// NewOperationV128StoreLane implements Operation.
//
// This corresponds to wasm.OpcodeVecV128Load8LaneName wasm.OpcodeVecV128Load16LaneName
//
//	wasm.OpcodeVecV128Load32LaneName wasm.OpcodeVecV128Load64LaneName.
//
// laneIndex is >=0 && <(128/LaneSize).
// laneSize is either 8, 16, 32, or 64.
func NewOperationV128StoreLane(laneIndex byte, laneSize byte, arg MemoryArg) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindV128StoreLane,
		B1:     laneSize,
		B2:     laneIndex,
		U1:     uint64(arg.Alignment),
		U2:     uint64(arg.Offset),
	}
}

// NewOperationV128ExtractLane is a constructor for UnionOperation with Kind OperationKindV128ExtractLane.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16ExtractLaneSName wasm.OpcodeVecI8x16ExtractLaneUName
//	wasm.OpcodeVecI16x8ExtractLaneSName wasm.OpcodeVecI16x8ExtractLaneUName
//	wasm.OpcodeVecI32x4ExtractLaneName wasm.OpcodeVecI64x2ExtractLaneName
//	wasm.OpcodeVecF32x4ExtractLaneName wasm.OpcodeVecF64x2ExtractLaneName.
//
// laneIndex is >=0 && <M where shape = NxM.
// signed is used when shape is either i8x16 or i16x2 to specify whether to sign-extend or not.
func NewOperationV128ExtractLane(laneIndex byte, signed bool, shape Shape) UnionOperation {
	return UnionOperation{
		OpKind: OperationKindV128ExtractLane,
		B1:     shape,
		B2:     laneIndex,
		B3:     signed,
	}
}

// NewOperationV128ReplaceLane is a constructor for UnionOperation with Kind OperationKindV128ReplaceLane.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16ReplaceLaneName wasm.OpcodeVecI16x8ReplaceLaneName
//	wasm.OpcodeVecI32x4ReplaceLaneName wasm.OpcodeVecI64x2ReplaceLaneName
//	wasm.OpcodeVecF32x4ReplaceLaneName wasm.OpcodeVecF64x2ReplaceLaneName.
//
// laneIndex is >=0 && <M where shape = NxM.
func NewOperationV128ReplaceLane(laneIndex byte, shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128ReplaceLane, B1: shape, B2: laneIndex}
}

// NewOperationV128Splat is a constructor for UnionOperation with Kind OperationKindV128Splat.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16SplatName wasm.OpcodeVecI16x8SplatName
//	wasm.OpcodeVecI32x4SplatName wasm.OpcodeVecI64x2SplatName
//	wasm.OpcodeVecF32x4SplatName wasm.OpcodeVecF64x2SplatName.
func NewOperationV128Splat(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Splat, B1: shape}
}

// NewOperationV128Shuffle is a constructor for UnionOperation with Kind OperationKindV128Shuffle.
func NewOperationV128Shuffle(lanes []uint64) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Shuffle, Us: lanes}
}

// NewOperationV128Swizzle is a constructor for UnionOperation with Kind OperationKindV128Swizzle.
//
// This corresponds to wasm.OpcodeVecI8x16SwizzleName.
func NewOperationV128Swizzle() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Swizzle}
}

// NewOperationV128AnyTrue is a constructor for UnionOperation with Kind OperationKindV128AnyTrue.
//
// This corresponds to wasm.OpcodeVecV128AnyTrueName.
func NewOperationV128AnyTrue() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128AnyTrue}
}

// NewOperationV128AllTrue is a constructor for UnionOperation with Kind OperationKindV128AllTrue.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16AllTrueName wasm.OpcodeVecI16x8AllTrueName
//	wasm.OpcodeVecI32x4AllTrueName wasm.OpcodeVecI64x2AllTrueName.
func NewOperationV128AllTrue(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128AllTrue, B1: shape}
}

// NewOperationV128BitMask is a constructor for UnionOperation with Kind OperationKindV128BitMask.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16BitMaskName wasm.OpcodeVecI16x8BitMaskName
//	wasm.OpcodeVecI32x4BitMaskName wasm.OpcodeVecI64x2BitMaskName.
func NewOperationV128BitMask(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128BitMask, B1: shape}
}

// NewOperationV128And is a constructor for UnionOperation with Kind OperationKindV128And.
//
// This corresponds to wasm.OpcodeVecV128And.
func NewOperationV128And() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128And}
}

// NewOperationV128Not is a constructor for UnionOperation with Kind OperationKindV128Not.
//
// This corresponds to wasm.OpcodeVecV128Not.
func NewOperationV128Not() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Not}
}

// NewOperationV128Or is a constructor for UnionOperation with Kind OperationKindV128Or.
//
// This corresponds to wasm.OpcodeVecV128Or.
func NewOperationV128Or() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Or}
}

// NewOperationV128Xor is a constructor for UnionOperation with Kind OperationKindV128Xor.
//
// This corresponds to wasm.OpcodeVecV128Xor.
func NewOperationV128Xor() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Xor}
}

// NewOperationV128Bitselect is a constructor for UnionOperation with Kind OperationKindV128Bitselect.
//
// This corresponds to wasm.OpcodeVecV128Bitselect.
func NewOperationV128Bitselect() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Bitselect}
}

// NewOperationV128AndNot is a constructor for UnionOperation with Kind OperationKindV128AndNot.
//
// This corresponds to wasm.OpcodeVecV128AndNot.
func NewOperationV128AndNot() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128AndNot}
}

// NewOperationV128Shl is a constructor for UnionOperation with Kind OperationKindV128Shl.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16ShlName wasm.OpcodeVecI16x8ShlName
//	wasm.OpcodeVecI32x4ShlName wasm.OpcodeVecI64x2ShlName
func NewOperationV128Shl(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Shl, B1: shape}
}

// NewOperationV128Shr is a constructor for UnionOperation with Kind OperationKindV128Shr.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16ShrSName wasm.OpcodeVecI8x16ShrUName wasm.OpcodeVecI16x8ShrSName
//	wasm.OpcodeVecI16x8ShrUName wasm.OpcodeVecI32x4ShrSName wasm.OpcodeVecI32x4ShrUName.
//	wasm.OpcodeVecI64x2ShrSName wasm.OpcodeVecI64x2ShrUName.
func NewOperationV128Shr(shape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Shr, B1: shape, B3: signed}
}

// NewOperationV128Cmp is a constructor for UnionOperation with Kind OperationKindV128Cmp.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16EqName, wasm.OpcodeVecI8x16NeName, wasm.OpcodeVecI8x16LtSName, wasm.OpcodeVecI8x16LtUName, wasm.OpcodeVecI8x16GtSName,
//	wasm.OpcodeVecI8x16GtUName, wasm.OpcodeVecI8x16LeSName, wasm.OpcodeVecI8x16LeUName, wasm.OpcodeVecI8x16GeSName, wasm.OpcodeVecI8x16GeUName,
//	wasm.OpcodeVecI16x8EqName, wasm.OpcodeVecI16x8NeName, wasm.OpcodeVecI16x8LtSName, wasm.OpcodeVecI16x8LtUName, wasm.OpcodeVecI16x8GtSName,
//	wasm.OpcodeVecI16x8GtUName, wasm.OpcodeVecI16x8LeSName, wasm.OpcodeVecI16x8LeUName, wasm.OpcodeVecI16x8GeSName, wasm.OpcodeVecI16x8GeUName,
//	wasm.OpcodeVecI32x4EqName, wasm.OpcodeVecI32x4NeName, wasm.OpcodeVecI32x4LtSName, wasm.OpcodeVecI32x4LtUName, wasm.OpcodeVecI32x4GtSName,
//	wasm.OpcodeVecI32x4GtUName, wasm.OpcodeVecI32x4LeSName, wasm.OpcodeVecI32x4LeUName, wasm.OpcodeVecI32x4GeSName, wasm.OpcodeVecI32x4GeUName,
//	wasm.OpcodeVecI64x2EqName, wasm.OpcodeVecI64x2NeName, wasm.OpcodeVecI64x2LtSName, wasm.OpcodeVecI64x2GtSName, wasm.OpcodeVecI64x2LeSName,
//	wasm.OpcodeVecI64x2GeSName, wasm.OpcodeVecF32x4EqName, wasm.OpcodeVecF32x4NeName, wasm.OpcodeVecF32x4LtName, wasm.OpcodeVecF32x4GtName,
//	wasm.OpcodeVecF32x4LeName, wasm.OpcodeVecF32x4GeName, wasm.OpcodeVecF64x2EqName, wasm.OpcodeVecF64x2NeName, wasm.OpcodeVecF64x2LtName,
//	wasm.OpcodeVecF64x2GtName, wasm.OpcodeVecF64x2LeName, wasm.OpcodeVecF64x2GeName
func NewOperationV128Cmp(cmpType V128CmpType) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Cmp, B1: cmpType}
}

// V128CmpType represents a type of vector comparison operation.
type V128CmpType = byte

const (
	// V128CmpTypeI8x16Eq corresponds to wasm.OpcodeVecI8x16EqName.
	V128CmpTypeI8x16Eq V128CmpType = iota
	// V128CmpTypeI8x16Ne corresponds to wasm.OpcodeVecI8x16NeName.
	V128CmpTypeI8x16Ne
	// V128CmpTypeI8x16LtS corresponds to wasm.OpcodeVecI8x16LtSName.
	V128CmpTypeI8x16LtS
	// V128CmpTypeI8x16LtU corresponds to wasm.OpcodeVecI8x16LtUName.
	V128CmpTypeI8x16LtU
	// V128CmpTypeI8x16GtS corresponds to wasm.OpcodeVecI8x16GtSName.
	V128CmpTypeI8x16GtS
	// V128CmpTypeI8x16GtU corresponds to wasm.OpcodeVecI8x16GtUName.
	V128CmpTypeI8x16GtU
	// V128CmpTypeI8x16LeS corresponds to wasm.OpcodeVecI8x16LeSName.
	V128CmpTypeI8x16LeS
	// V128CmpTypeI8x16LeU corresponds to wasm.OpcodeVecI8x16LeUName.
	V128CmpTypeI8x16LeU
	// V128CmpTypeI8x16GeS corresponds to wasm.OpcodeVecI8x16GeSName.
	V128CmpTypeI8x16GeS
	// V128CmpTypeI8x16GeU corresponds to wasm.OpcodeVecI8x16GeUName.
	V128CmpTypeI8x16GeU
	// V128CmpTypeI16x8Eq corresponds to wasm.OpcodeVecI16x8EqName.
	V128CmpTypeI16x8Eq
	// V128CmpTypeI16x8Ne corresponds to wasm.OpcodeVecI16x8NeName.
	V128CmpTypeI16x8Ne
	// V128CmpTypeI16x8LtS corresponds to wasm.OpcodeVecI16x8LtSName.
	V128CmpTypeI16x8LtS
	// V128CmpTypeI16x8LtU corresponds to wasm.OpcodeVecI16x8LtUName.
	V128CmpTypeI16x8LtU
	// V128CmpTypeI16x8GtS corresponds to wasm.OpcodeVecI16x8GtSName.
	V128CmpTypeI16x8GtS
	// V128CmpTypeI16x8GtU corresponds to wasm.OpcodeVecI16x8GtUName.
	V128CmpTypeI16x8GtU
	// V128CmpTypeI16x8LeS corresponds to wasm.OpcodeVecI16x8LeSName.
	V128CmpTypeI16x8LeS
	// V128CmpTypeI16x8LeU corresponds to wasm.OpcodeVecI16x8LeUName.
	V128CmpTypeI16x8LeU
	// V128CmpTypeI16x8GeS corresponds to wasm.OpcodeVecI16x8GeSName.
	V128CmpTypeI16x8GeS
	// V128CmpTypeI16x8GeU corresponds to wasm.OpcodeVecI16x8GeUName.
	V128CmpTypeI16x8GeU
	// V128CmpTypeI32x4Eq corresponds to wasm.OpcodeVecI32x4EqName.
	V128CmpTypeI32x4Eq
	// V128CmpTypeI32x4Ne corresponds to wasm.OpcodeVecI32x4NeName.
	V128CmpTypeI32x4Ne
	// V128CmpTypeI32x4LtS corresponds to wasm.OpcodeVecI32x4LtSName.
	V128CmpTypeI32x4LtS
	// V128CmpTypeI32x4LtU corresponds to wasm.OpcodeVecI32x4LtUName.
	V128CmpTypeI32x4LtU
	// V128CmpTypeI32x4GtS corresponds to wasm.OpcodeVecI32x4GtSName.
	V128CmpTypeI32x4GtS
	// V128CmpTypeI32x4GtU corresponds to wasm.OpcodeVecI32x4GtUName.
	V128CmpTypeI32x4GtU
	// V128CmpTypeI32x4LeS corresponds to wasm.OpcodeVecI32x4LeSName.
	V128CmpTypeI32x4LeS
	// V128CmpTypeI32x4LeU corresponds to wasm.OpcodeVecI32x4LeUName.
	V128CmpTypeI32x4LeU
	// V128CmpTypeI32x4GeS corresponds to wasm.OpcodeVecI32x4GeSName.
	V128CmpTypeI32x4GeS
	// V128CmpTypeI32x4GeU corresponds to wasm.OpcodeVecI32x4GeUName.
	V128CmpTypeI32x4GeU
	// V128CmpTypeI64x2Eq corresponds to wasm.OpcodeVecI64x2EqName.
	V128CmpTypeI64x2Eq
	// V128CmpTypeI64x2Ne corresponds to wasm.OpcodeVecI64x2NeName.
	V128CmpTypeI64x2Ne
	// V128CmpTypeI64x2LtS corresponds to wasm.OpcodeVecI64x2LtSName.
	V128CmpTypeI64x2LtS
	// V128CmpTypeI64x2GtS corresponds to wasm.OpcodeVecI64x2GtSName.
	V128CmpTypeI64x2GtS
	// V128CmpTypeI64x2LeS corresponds to wasm.OpcodeVecI64x2LeSName.
	V128CmpTypeI64x2LeS
	// V128CmpTypeI64x2GeS corresponds to wasm.OpcodeVecI64x2GeSName.
	V128CmpTypeI64x2GeS
	// V128CmpTypeF32x4Eq corresponds to wasm.OpcodeVecF32x4EqName.
	V128CmpTypeF32x4Eq
	// V128CmpTypeF32x4Ne corresponds to wasm.OpcodeVecF32x4NeName.
	V128CmpTypeF32x4Ne
	// V128CmpTypeF32x4Lt corresponds to wasm.OpcodeVecF32x4LtName.
	V128CmpTypeF32x4Lt
	// V128CmpTypeF32x4Gt corresponds to wasm.OpcodeVecF32x4GtName.
	V128CmpTypeF32x4Gt
	// V128CmpTypeF32x4Le corresponds to wasm.OpcodeVecF32x4LeName.
	V128CmpTypeF32x4Le
	// V128CmpTypeF32x4Ge corresponds to wasm.OpcodeVecF32x4GeName.
	V128CmpTypeF32x4Ge
	// V128CmpTypeF64x2Eq corresponds to wasm.OpcodeVecF64x2EqName.
	V128CmpTypeF64x2Eq
	// V128CmpTypeF64x2Ne corresponds to wasm.OpcodeVecF64x2NeName.
	V128CmpTypeF64x2Ne
	// V128CmpTypeF64x2Lt corresponds to wasm.OpcodeVecF64x2LtName.
	V128CmpTypeF64x2Lt
	// V128CmpTypeF64x2Gt corresponds to wasm.OpcodeVecF64x2GtName.
	V128CmpTypeF64x2Gt
	// V128CmpTypeF64x2Le corresponds to wasm.OpcodeVecF64x2LeName.
	V128CmpTypeF64x2Le
	// V128CmpTypeF64x2Ge corresponds to wasm.OpcodeVecF64x2GeName.
	V128CmpTypeF64x2Ge
)

// NewOperationV128AddSat is a constructor for UnionOperation with Kind OperationKindV128AddSat.
//
// This corresponds to wasm.OpcodeVecI8x16AddSatUName wasm.OpcodeVecI8x16AddSatSName
//
//	wasm.OpcodeVecI16x8AddSatUName wasm.OpcodeVecI16x8AddSatSName
//
// shape is either ShapeI8x16 or ShapeI16x8.
func NewOperationV128AddSat(shape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128AddSat, B1: shape, B3: signed}
}

// NewOperationV128SubSat is a constructor for UnionOperation with Kind OperationKindV128SubSat.
//
// This corresponds to wasm.OpcodeVecI8x16SubSatUName wasm.OpcodeVecI8x16SubSatSName
//
//	wasm.OpcodeVecI16x8SubSatUName wasm.OpcodeVecI16x8SubSatSName
//
// shape is either ShapeI8x16 or ShapeI16x8.
func NewOperationV128SubSat(shape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128SubSat, B1: shape, B3: signed}
}

// NewOperationV128Mul is a constructor for UnionOperation with Kind OperationKindV128Mul
//
// This corresponds to wasm.OpcodeVecF32x4MulName wasm.OpcodeVecF64x2MulName
//
//		wasm.OpcodeVecI16x8MulName wasm.OpcodeVecI32x4MulName wasm.OpcodeVecI64x2MulName.
//	 shape is either ShapeI16x8, ShapeI32x4, ShapeI64x2, ShapeF32x4 or ShapeF64x2.
func NewOperationV128Mul(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Mul, B1: shape}
}

// NewOperationV128Div is a constructor for UnionOperation with Kind OperationKindV128Div.
//
// This corresponds to wasm.OpcodeVecF32x4DivName wasm.OpcodeVecF64x2DivName.
// shape is either ShapeF32x4 or ShapeF64x2.
func NewOperationV128Div(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Div, B1: shape}
}

// NewOperationV128Neg is a constructor for UnionOperation with Kind OperationKindV128Neg.
//
// This corresponds to wasm.OpcodeVecI8x16NegName wasm.OpcodeVecI16x8NegName wasm.OpcodeVecI32x4NegName
//
//	wasm.OpcodeVecI64x2NegName wasm.OpcodeVecF32x4NegName wasm.OpcodeVecF64x2NegName.
func NewOperationV128Neg(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Neg, B1: shape}
}

// NewOperationV128Sqrt is a constructor for UnionOperation with Kind 128OperationKindV128Sqrt.
//
// shape is either ShapeF32x4 or ShapeF64x2.
// This corresponds to wasm.OpcodeVecF32x4SqrtName wasm.OpcodeVecF64x2SqrtName.
func NewOperationV128Sqrt(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Sqrt, B1: shape}
}

// NewOperationV128Abs is a constructor for UnionOperation with Kind OperationKindV128Abs.
//
// This corresponds to wasm.OpcodeVecI8x16AbsName wasm.OpcodeVecI16x8AbsName wasm.OpcodeVecI32x4AbsName
//
//	wasm.OpcodeVecI64x2AbsName wasm.OpcodeVecF32x4AbsName wasm.OpcodeVecF64x2AbsName.
func NewOperationV128Abs(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Abs, B1: shape}
}

// NewOperationV128Popcnt is a constructor for UnionOperation with Kind OperationKindV128Popcnt.
//
// This corresponds to wasm.OpcodeVecI8x16PopcntName.
func NewOperationV128Popcnt(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Popcnt, B1: shape}
}

// NewOperationV128Min is a constructor for UnionOperation with Kind OperationKindV128Min.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16MinSName wasm.OpcodeVecI8x16MinUName　wasm.OpcodeVecI16x8MinSName wasm.OpcodeVecI16x8MinUName
//	wasm.OpcodeVecI32x4MinSName wasm.OpcodeVecI32x4MinUName　wasm.OpcodeVecI16x8MinSName wasm.OpcodeVecI16x8MinUName
//	wasm.OpcodeVecF32x4MinName wasm.OpcodeVecF64x2MinName
func NewOperationV128Min(shape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Min, B1: shape, B3: signed}
}

// NewOperationV128Max is a constructor for UnionOperation with Kind OperationKindV128Max.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16MaxSName wasm.OpcodeVecI8x16MaxUName　wasm.OpcodeVecI16x8MaxSName wasm.OpcodeVecI16x8MaxUName
//	wasm.OpcodeVecI32x4MaxSName wasm.OpcodeVecI32x4MaxUName　wasm.OpcodeVecI16x8MaxSName wasm.OpcodeVecI16x8MaxUName
//	wasm.OpcodeVecF32x4MaxName wasm.OpcodeVecF64x2MaxName.
func NewOperationV128Max(shape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Max, B1: shape, B3: signed}
}

// NewOperationV128AvgrU is a constructor for UnionOperation with Kind OperationKindV128AvgrU.
//
// This corresponds to wasm.OpcodeVecI8x16AvgrUName.
func NewOperationV128AvgrU(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128AvgrU, B1: shape}
}

// NewOperationV128Pmin is a constructor for UnionOperation with Kind OperationKindV128Pmin.
//
// This corresponds to wasm.OpcodeVecF32x4PminName wasm.OpcodeVecF64x2PminName.
func NewOperationV128Pmin(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Pmin, B1: shape}
}

// NewOperationV128Pmax is a constructor for UnionOperation with Kind OperationKindV128Pmax.
//
// This corresponds to wasm.OpcodeVecF32x4PmaxName wasm.OpcodeVecF64x2PmaxName.
func NewOperationV128Pmax(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Pmax, B1: shape}
}

// NewOperationV128Ceil is a constructor for UnionOperation with Kind OperationKindV128Ceil.
//
// This corresponds to wasm.OpcodeVecF32x4CeilName wasm.OpcodeVecF64x2CeilName
func NewOperationV128Ceil(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Ceil, B1: shape}
}

// NewOperationV128Floor is a constructor for UnionOperation with Kind OperationKindV128Floor.
//
// This corresponds to wasm.OpcodeVecF32x4FloorName wasm.OpcodeVecF64x2FloorName
func NewOperationV128Floor(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Floor, B1: shape}
}

// NewOperationV128Trunc is a constructor for UnionOperation with Kind OperationKindV128Trunc.
//
// This corresponds to wasm.OpcodeVecF32x4TruncName wasm.OpcodeVecF64x2TruncName
func NewOperationV128Trunc(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Trunc, B1: shape}
}

// NewOperationV128Nearest is a constructor for UnionOperation with Kind OperationKindV128Nearest.
//
// This corresponds to wasm.OpcodeVecF32x4NearestName wasm.OpcodeVecF64x2NearestName
func NewOperationV128Nearest(shape Shape) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Nearest, B1: shape}
}

// NewOperationV128Extend is a constructor for UnionOperation with Kind OperationKindV128Extend.
//
// This corresponds to
//
//	wasm.OpcodeVecI16x8ExtendLowI8x16SName wasm.OpcodeVecI16x8ExtendHighI8x16SName
//	wasm.OpcodeVecI16x8ExtendLowI8x16UName wasm.OpcodeVecI16x8ExtendHighI8x16UName
//	wasm.OpcodeVecI32x4ExtendLowI16x8SName wasm.OpcodeVecI32x4ExtendHighI16x8SName
//	wasm.OpcodeVecI32x4ExtendLowI16x8UName wasm.OpcodeVecI32x4ExtendHighI16x8UName
//	wasm.OpcodeVecI64x2ExtendLowI32x4SName wasm.OpcodeVecI64x2ExtendHighI32x4SName
//	wasm.OpcodeVecI64x2ExtendLowI32x4UName wasm.OpcodeVecI64x2ExtendHighI32x4UName
//
// originShape is the shape of the original lanes for extension which is
// either ShapeI8x16, ShapeI16x8, or ShapeI32x4.
// useLow true if it uses the lower half of vector for extension.
func NewOperationV128Extend(originShape Shape, signed bool, useLow bool) UnionOperation {
	op := UnionOperation{OpKind: OperationKindV128Extend}
	op.B1 = originShape
	if signed {
		op.B2 = 1
	}
	op.B3 = useLow
	return op
}

// NewOperationV128ExtMul is a constructor for UnionOperation with Kind OperationKindV128ExtMul.
//
// This corresponds to
//
//		wasm.OpcodeVecI16x8ExtMulLowI8x16SName wasm.OpcodeVecI16x8ExtMulLowI8x16UName
//		wasm.OpcodeVecI16x8ExtMulHighI8x16SName wasm.OpcodeVecI16x8ExtMulHighI8x16UName
//	 wasm.OpcodeVecI32x4ExtMulLowI16x8SName wasm.OpcodeVecI32x4ExtMulLowI16x8UName
//		wasm.OpcodeVecI32x4ExtMulHighI16x8SName wasm.OpcodeVecI32x4ExtMulHighI16x8UName
//	 wasm.OpcodeVecI64x2ExtMulLowI32x4SName wasm.OpcodeVecI64x2ExtMulLowI32x4UName
//		wasm.OpcodeVecI64x2ExtMulHighI32x4SName wasm.OpcodeVecI64x2ExtMulHighI32x4UName.
//
// originShape is the shape of the original lanes for extension which is
// either ShapeI8x16, ShapeI16x8, or ShapeI32x4.
// useLow true if it uses the lower half of vector for extension.
func NewOperationV128ExtMul(originShape Shape, signed bool, useLow bool) UnionOperation {
	op := UnionOperation{OpKind: OperationKindV128ExtMul}
	op.B1 = originShape
	if signed {
		op.B2 = 1
	}
	op.B3 = useLow
	return op
}

// NewOperationV128Q15mulrSatS is a constructor for UnionOperation with Kind OperationKindV128Q15mulrSatS.
//
// This corresponds to wasm.OpcodeVecI16x8Q15mulrSatSName
func NewOperationV128Q15mulrSatS() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Q15mulrSatS}
}

// NewOperationV128ExtAddPairwise is a constructor for UnionOperation with Kind OperationKindV128ExtAddPairwise.
//
// This corresponds to
//
//	wasm.OpcodeVecI16x8ExtaddPairwiseI8x16SName wasm.OpcodeVecI16x8ExtaddPairwiseI8x16UName
//	wasm.OpcodeVecI32x4ExtaddPairwiseI16x8SName wasm.OpcodeVecI32x4ExtaddPairwiseI16x8UName.
//
// originShape is the shape of the original lanes for extension which is
// either ShapeI8x16, or ShapeI16x8.
func NewOperationV128ExtAddPairwise(originShape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128ExtAddPairwise, B1: originShape, B3: signed}
}

// NewOperationV128FloatPromote is a constructor for UnionOperation with Kind NewOperationV128FloatPromote.
//
// This corresponds to wasm.OpcodeVecF64x2PromoteLowF32x4ZeroName
// This discards the higher 64-bit of a vector, and promotes two
// 32-bit floats in the lower 64-bit as two 64-bit floats.
func NewOperationV128FloatPromote() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128FloatPromote}
}

// NewOperationV128FloatDemote is a constructor for UnionOperation with Kind NewOperationV128FloatDemote.
//
// This corresponds to wasm.OpcodeVecF32x4DemoteF64x2ZeroName.
func NewOperationV128FloatDemote() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128FloatDemote}
}

// NewOperationV128FConvertFromI is a constructor for UnionOperation with Kind NewOperationV128FConvertFromI.
//
// This corresponds to
//
//	wasm.OpcodeVecF32x4ConvertI32x4SName wasm.OpcodeVecF32x4ConvertI32x4UName
//	wasm.OpcodeVecF64x2ConvertLowI32x4SName wasm.OpcodeVecF64x2ConvertLowI32x4UName.
//
// destinationShape is the shape of the destination lanes for conversion which is
// either ShapeF32x4, or ShapeF64x2.
func NewOperationV128FConvertFromI(destinationShape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128FConvertFromI, B1: destinationShape, B3: signed}
}

// NewOperationV128Dot is a constructor for UnionOperation with Kind OperationKindV128Dot.
//
// This corresponds to wasm.OpcodeVecI32x4DotI16x8SName
func NewOperationV128Dot() UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Dot}
}

// NewOperationV128Narrow is a constructor for UnionOperation with Kind OperationKindV128Narrow.
//
// This corresponds to
//
//	wasm.OpcodeVecI8x16NarrowI16x8SName wasm.OpcodeVecI8x16NarrowI16x8UName
//	wasm.OpcodeVecI16x8NarrowI32x4SName wasm.OpcodeVecI16x8NarrowI32x4UName.
//
// originShape is the shape of the original lanes for narrowing which is
// either ShapeI16x8, or ShapeI32x4.
func NewOperationV128Narrow(originShape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128Narrow, B1: originShape, B3: signed}
}

// NewOperationV128ITruncSatFromF is a constructor for UnionOperation with Kind OperationKindV128ITruncSatFromF.
//
// This corresponds to
//
//	wasm.OpcodeVecI32x4TruncSatF64x2UZeroName wasm.OpcodeVecI32x4TruncSatF64x2SZeroName
//	wasm.OpcodeVecI32x4TruncSatF32x4UName wasm.OpcodeVecI32x4TruncSatF32x4SName.
//
// originShape is the shape of the original lanes for truncation which is
// either ShapeF32x4, or ShapeF64x2.
func NewOperationV128ITruncSatFromF(originShape Shape, signed bool) UnionOperation {
	return UnionOperation{OpKind: OperationKindV128ITruncSatFromF, B1: originShape, B3: signed}
}
