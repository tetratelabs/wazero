package wazeroir

import "fmt"

type UnsignedInt byte

const (
	UnsignedInt32 UnsignedInt = iota
	UnsignedInt64
)

func (s UnsignedInt) String() (ret string) {
	switch s {
	case UnsignedInt32:
		ret = "i32"
	case UnsignedInt64:
		ret = "i64"
	}
	return
}

type SignedInt byte

const (
	SignedInt32 SignedInt = iota
	SignedInt64
	SignedUint32
	SignedUint64
)

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

type Float byte

const (
	Float32 Float = iota
	Float64
)

func (s Float) String() (ret string) {
	switch s {
	case Float32:
		ret = "f32"
	case Float64:
		ret = "f64"
	}
	return
}

type UnsignedType byte

const (
	UnsignedTypeI32 UnsignedType = iota
	UnsignedTypeI64
	UnsignedTypeF32
	UnsignedTypeF64
	UnsignedTypeV128
	UnsignedTypeUnknown
)

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

type SignedType byte

const (
	SignedTypeInt32 SignedType = iota
	SignedTypeUint32
	SignedTypeInt64
	SignedTypeUint64
	SignedTypeFloat32
	SignedTypeFloat64
)

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

type Operation interface {
	Kind() OperationKind
}

type OperationKind uint16

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
	case OperationKindSwap:
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
	default:
		panic(fmt.Errorf("unknown operation %d", o))
	}
	return
}

const (
	OperationKindUnreachable OperationKind = iota
	OperationKindLabel
	OperationKindBr
	OperationKindBrIf
	OperationKindBrTable
	OperationKindCall
	OperationKindCallIndirect
	OperationKindDrop
	OperationKindSelect
	OperationKindPick
	OperationKindSwap
	OperationKindGlobalGet
	OperationKindGlobalSet
	OperationKindLoad
	OperationKindLoad8
	OperationKindLoad16
	OperationKindLoad32
	OperationKindStore
	OperationKindStore8
	OperationKindStore16
	OperationKindStore32
	OperationKindMemorySize
	OperationKindMemoryGrow
	OperationKindConstI32
	OperationKindConstI64
	OperationKindConstF32
	OperationKindConstF64
	OperationKindEq
	OperationKindNe
	OperationKindEqz
	OperationKindLt
	OperationKindGt
	OperationKindLe
	OperationKindGe
	OperationKindAdd
	OperationKindSub
	OperationKindMul
	OperationKindClz
	OperationKindCtz
	OperationKindPopcnt
	OperationKindDiv
	OperationKindRem
	OperationKindAnd
	OperationKindOr
	OperationKindXor
	OperationKindShl
	OperationKindShr
	OperationKindRotl
	OperationKindRotr
	OperationKindAbs
	OperationKindNeg
	OperationKindCeil
	OperationKindFloor
	OperationKindTrunc
	OperationKindNearest
	OperationKindSqrt
	OperationKindMin
	OperationKindMax
	OperationKindCopysign
	OperationKindI32WrapFromI64
	OperationKindITruncFromF
	OperationKindFConvertFromI
	OperationKindF32DemoteFromF64
	OperationKindF64PromoteFromF32
	OperationKindI32ReinterpretFromF32
	OperationKindI64ReinterpretFromF64
	OperationKindF32ReinterpretFromI32
	OperationKindF64ReinterpretFromI64
	OperationKindExtend
	OperationKindSignExtend32From8
	OperationKindSignExtend32From16
	OperationKindSignExtend64From8
	OperationKindSignExtend64From16
	OperationKindSignExtend64From32
	OperationKindMemoryInit
	OperationKindDataDrop
	OperationKindMemoryCopy
	OperationKindMemoryFill
	OperationKindTableInit
	OperationKindElemDrop
	OperationKindTableCopy
	OperationKindRefFunc
	OperationKindTableGet
	OperationKindTableSet
	OperationKindTableSize
	OperationKindTableGrow
	OperationKindTableFill

	// Vector value related instructions are prefixed by V128.

	OperationKindV128Const
	OperationKindV128Add
	OperationKindV128Sub
	OperationKindV128Load
	OperationKindV128LoadLane
	OperationKindV128Store
	OperationKindV128StoreLane
	OperationKindV128ExtractLane
	OperationKindV128ReplaceLane
	OperationKindV128Splat
	OperationKindV128Shuffle
	OperationKindV128Swizzle
	OperationKindV128AnyTrue
	OperationKindV128AllTrue
	OperationKindV128BitMask
	OperationKindV128And
	OperationKindV128Not
	OperationKindV128Or
	OperationKindV128Xor
	OperationKindV128Bitselect
	OperationKindV128AndNot
	OperationKindV128Shl
	OperationKindV128Shr
	OperationKindV128Cmp

	// operationKindEnd is always placed at the bottom of this iota definition to be used in the test.
	operationKindEnd
)

type Label struct {
	FrameID uint32
	Kind    LabelKind
}

func (l *Label) String() (ret string) {
	if l == nil {
		// Sometimes String() is called on the nil label which is interpreted
		// as the function return.
		return ""
	}
	switch l.Kind {
	case LabelKindHeader:
		ret = fmt.Sprintf(".L%d", l.FrameID)
	case LabelKindElse:
		ret = fmt.Sprintf(".L%d_else", l.FrameID)
	case LabelKindContinuation:
		ret = fmt.Sprintf(".L%d_cont", l.FrameID)
	}
	return
}

type LabelKind = byte

const (
	LabelKindHeader LabelKind = iota
	LabelKindElse
	LabelKindContinuation
)

func (l *Label) asBranchTarget() *BranchTarget {
	return &BranchTarget{Label: l}
}

func (l *Label) asBranchTargetDrop() *BranchTargetDrop {
	return &BranchTargetDrop{Target: l.asBranchTarget()}
}

type BranchTarget struct {
	Label *Label
}

func (b *BranchTarget) IsReturnTarget() bool {
	return b.Label == nil
}

func (b *BranchTarget) String() (ret string) {
	if b.IsReturnTarget() {
		ret = ".return"
	} else {
		ret = b.Label.String()
	}
	return
}

type BranchTargetDrop struct {
	Target *BranchTarget
	ToDrop *InclusiveRange
}

func (b *BranchTargetDrop) String() (ret string) {
	if b.ToDrop != nil {
		ret = fmt.Sprintf("%s(drop %d..%d)", b.Target, b.ToDrop.Start, b.ToDrop.End)
	} else {
		ret = b.Target.String()
	}
	return
}

type OperationUnreachable struct{}

func (o *OperationUnreachable) Kind() OperationKind {
	return OperationKindUnreachable
}

type OperationLabel struct {
	Label *Label
}

func (o *OperationLabel) Kind() OperationKind {
	return OperationKindLabel
}

type OperationBr struct {
	Target *BranchTarget
}

func (o *OperationBr) Kind() OperationKind {
	return OperationKindBr
}

type OperationBrIf struct {
	Then, Else *BranchTargetDrop
}

func (o *OperationBrIf) Kind() OperationKind {
	return OperationKindBrIf
}

type InclusiveRange struct {
	Start, End int
}

type OperationBrTable struct {
	Targets []*BranchTargetDrop
	Default *BranchTargetDrop
}

func (o *OperationBrTable) Kind() OperationKind {
	return OperationKindBrTable
}

type OperationCall struct {
	FunctionIndex uint32
}

func (o *OperationCall) Kind() OperationKind {
	return OperationKindCall
}

type OperationCallIndirect struct {
	TypeIndex, TableIndex uint32
}

func (o *OperationCallIndirect) Kind() OperationKind {
	return OperationKindCallIndirect
}

type OperationDrop struct {
	// Depths spans across the uint64 value stack at runtime to be dopped by this operation.
	Depth *InclusiveRange
}

func (o *OperationDrop) Kind() OperationKind {
	return OperationKindDrop
}

type OperationSelect struct{}

func (o *OperationSelect) Kind() OperationKind {
	return OperationKindSelect
}

type OperationPick struct {
	// Depth is the location of the pick target in the uint64 value stack at runtime.
	// If IsTargetVector=true, this points to the location of the lower 64-bits of the vector.
	Depth          int
	IsTargetVector bool
}

func (o *OperationPick) Kind() OperationKind {
	return OperationKindPick
}

type OperationSwap struct {
	// Depth is the location of the pick target in the uint64 value stack at runtime.
	// If IsTargetVector=true, this points the location of the lower 64-bits of the vector.
	Depth          int
	IsTargetVector bool
}

func (o *OperationSwap) Kind() OperationKind {
	return OperationKindSwap
}

type OperationGlobalGet struct{ Index uint32 }

func (o *OperationGlobalGet) Kind() OperationKind {
	return OperationKindGlobalGet
}

type OperationGlobalSet struct{ Index uint32 }

func (o *OperationGlobalSet) Kind() OperationKind {
	return OperationKindGlobalSet
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

type OperationLoad struct {
	Type UnsignedType
	Arg  *MemoryArg
}

func (o *OperationLoad) Kind() OperationKind {
	return OperationKindLoad
}

type OperationLoad8 struct {
	Type SignedInt
	Arg  *MemoryArg
}

func (o *OperationLoad8) Kind() OperationKind {
	return OperationKindLoad8
}

type OperationLoad16 struct {
	Type SignedInt
	Arg  *MemoryArg
}

func (o *OperationLoad16) Kind() OperationKind {
	return OperationKindLoad16
}

type OperationLoad32 struct {
	Signed bool
	Arg    *MemoryArg
}

func (o *OperationLoad32) Kind() OperationKind {
	return OperationKindLoad32
}

type OperationStore struct {
	Type UnsignedType
	Arg  *MemoryArg
}

func (o *OperationStore) Kind() OperationKind {
	return OperationKindStore
}

type OperationStore8 struct {
	// TODO: Semantically Type doesn't affect operation so consider deleting this field.
	Type UnsignedInt
	Arg  *MemoryArg
}

func (o *OperationStore8) Kind() OperationKind {
	return OperationKindStore8
}

type OperationStore16 struct {
	// TODO: Semantically Type doesn't affect operation so consider deleting this field.
	Type UnsignedInt
	Arg  *MemoryArg
}

func (o *OperationStore16) Kind() OperationKind {
	return OperationKindStore16
}

type OperationStore32 struct {
	Arg *MemoryArg
}

// Kind implements Operation.Kind.
func (o *OperationStore32) Kind() OperationKind {
	return OperationKindStore32
}

type OperationMemorySize struct{}

// Kind implements Operation.Kind.
func (o *OperationMemorySize) Kind() OperationKind {
	return OperationKindMemorySize
}

type OperationMemoryGrow struct{ Alignment uint64 }

// Kind implements Operation.Kind.
func (o *OperationMemoryGrow) Kind() OperationKind {
	return OperationKindMemoryGrow
}

type OperationConstI32 struct{ Value uint32 }

// Kind implements Operation.Kind.
func (o *OperationConstI32) Kind() OperationKind {
	return OperationKindConstI32
}

type OperationConstI64 struct{ Value uint64 }

// Kind implements Operation.Kind.
func (o *OperationConstI64) Kind() OperationKind {
	return OperationKindConstI64
}

type OperationConstF32 struct{ Value float32 }

// Kind implements Operation.Kind.
func (o *OperationConstF32) Kind() OperationKind {
	return OperationKindConstF32
}

type OperationConstF64 struct{ Value float64 }

// Kind implements Operation.Kind.
func (o *OperationConstF64) Kind() OperationKind {
	return OperationKindConstF64
}

type OperationEq struct{ Type UnsignedType }

// Kind implements Operation.Kind.
func (o *OperationEq) Kind() OperationKind {
	return OperationKindEq
}

type OperationNe struct{ Type UnsignedType }

// Kind implements Operation.Kind.
func (o *OperationNe) Kind() OperationKind {
	return OperationKindNe
}

type OperationEqz struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationEqz) Kind() OperationKind {
	return OperationKindEqz
}

type OperationLt struct{ Type SignedType }

// Kind implements Operation.Kind.
func (o *OperationLt) Kind() OperationKind {
	return OperationKindLt
}

type OperationGt struct{ Type SignedType }

// Kind implements Operation.Kind.
func (o *OperationGt) Kind() OperationKind {
	return OperationKindGt
}

type OperationLe struct{ Type SignedType }

// Kind implements Operation.Kind.
func (o *OperationLe) Kind() OperationKind {
	return OperationKindLe
}

type OperationGe struct{ Type SignedType }

// Kind implements Operation.Kind.
func (o *OperationGe) Kind() OperationKind {
	return OperationKindGe
}

type OperationAdd struct{ Type UnsignedType }

// Kind implements Operation.Kind.
func (o *OperationAdd) Kind() OperationKind {
	return OperationKindAdd
}

type OperationSub struct{ Type UnsignedType }

// Kind implements Operation.Kind.
func (o *OperationSub) Kind() OperationKind {
	return OperationKindSub
}

type OperationMul struct{ Type UnsignedType }

// Kind implements Operation.Kind.
func (o *OperationMul) Kind() OperationKind {
	return OperationKindMul
}

type OperationClz struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationClz) Kind() OperationKind {
	return OperationKindClz
}

type OperationCtz struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationCtz) Kind() OperationKind {
	return OperationKindCtz
}

type OperationPopcnt struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationPopcnt) Kind() OperationKind {
	return OperationKindPopcnt
}

type OperationDiv struct{ Type SignedType }

// Kind implements Operation.Kind.
func (o *OperationDiv) Kind() OperationKind {
	return OperationKindDiv
}

type OperationRem struct{ Type SignedInt }

// Kind implements Operation.Kind.
func (o *OperationRem) Kind() OperationKind {
	return OperationKindRem
}

type OperationAnd struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationAnd) Kind() OperationKind {
	return OperationKindAnd
}

type OperationOr struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationOr) Kind() OperationKind {
	return OperationKindOr
}

type OperationXor struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationXor) Kind() OperationKind {
	return OperationKindXor
}

type OperationShl struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationShl) Kind() OperationKind {
	return OperationKindShl
}

type OperationShr struct{ Type SignedInt }

// Kind implements Operation.Kind.
func (o *OperationShr) Kind() OperationKind {
	return OperationKindShr
}

type OperationRotl struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationRotl) Kind() OperationKind {
	return OperationKindRotl
}

type OperationRotr struct{ Type UnsignedInt }

// Kind implements Operation.Kind.
func (o *OperationRotr) Kind() OperationKind {
	return OperationKindRotr
}

type OperationAbs struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationAbs) Kind() OperationKind {
	return OperationKindAbs
}

type OperationNeg struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationNeg) Kind() OperationKind {
	return OperationKindNeg
}

type OperationCeil struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationCeil) Kind() OperationKind {
	return OperationKindCeil
}

type OperationFloor struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationFloor) Kind() OperationKind {
	return OperationKindFloor
}

type OperationTrunc struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationTrunc) Kind() OperationKind {
	return OperationKindTrunc
}

type OperationNearest struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationNearest) Kind() OperationKind {
	return OperationKindNearest
}

type OperationSqrt struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationSqrt) Kind() OperationKind {
	return OperationKindSqrt
}

type OperationMin struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationMin) Kind() OperationKind {
	return OperationKindMin
}

type OperationMax struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationMax) Kind() OperationKind {
	return OperationKindMax
}

type OperationCopysign struct{ Type Float }

// Kind implements Operation.Kind.
func (o *OperationCopysign) Kind() OperationKind {
	return OperationKindCopysign
}

type OperationI32WrapFromI64 struct{}

// Kind implements Operation.Kind.
func (o *OperationI32WrapFromI64) Kind() OperationKind {
	return OperationKindI32WrapFromI64
}

type OperationITruncFromF struct {
	InputType  Float
	OutputType SignedInt
	// NonTrapping true if this conversion is "nontrapping" in the sense of the
	// https://github.com/WebAssembly/spec/blob/ce4b6c4d47eb06098cc7ab2e81f24748da822f20/proposals/nontrapping-float-to-int-conversion/Overview.md
	NonTrapping bool
}

// Kind implements Operation.Kind.
func (o *OperationITruncFromF) Kind() OperationKind {
	return OperationKindITruncFromF
}

type OperationFConvertFromI struct {
	InputType  SignedInt
	OutputType Float
}

// Kind implements Operation.Kind.
func (o *OperationFConvertFromI) Kind() OperationKind {
	return OperationKindFConvertFromI
}

type OperationF32DemoteFromF64 struct{}

// Kind implements Operation.Kind.
func (o *OperationF32DemoteFromF64) Kind() OperationKind {
	return OperationKindF32DemoteFromF64
}

type OperationF64PromoteFromF32 struct{}

// Kind implements Operation.Kind.
func (o *OperationF64PromoteFromF32) Kind() OperationKind {
	return OperationKindF64PromoteFromF32
}

type OperationI32ReinterpretFromF32 struct{}

// Kind implements Operation.Kind.
func (o *OperationI32ReinterpretFromF32) Kind() OperationKind {
	return OperationKindI32ReinterpretFromF32
}

type OperationI64ReinterpretFromF64 struct{}

// Kind implements Operation.Kind.
func (o *OperationI64ReinterpretFromF64) Kind() OperationKind {
	return OperationKindI64ReinterpretFromF64
}

type OperationF32ReinterpretFromI32 struct{}

// Kind implements Operation.Kind.
func (o *OperationF32ReinterpretFromI32) Kind() OperationKind {
	return OperationKindF32ReinterpretFromI32
}

type OperationF64ReinterpretFromI64 struct{}

// Kind implements Operation.Kind.
func (o *OperationF64ReinterpretFromI64) Kind() OperationKind {
	return OperationKindF64ReinterpretFromI64
}

type OperationExtend struct{ Signed bool }

func (o *OperationExtend) Kind() OperationKind {
	return OperationKindExtend
}

type OperationSignExtend32From8 struct{}

// Kind implements Operation.Kind.
func (o *OperationSignExtend32From8) Kind() OperationKind {
	return OperationKindSignExtend32From8
}

type OperationSignExtend32From16 struct{}

// Kind implements Operation.Kind.
func (o *OperationSignExtend32From16) Kind() OperationKind {
	return OperationKindSignExtend32From16
}

type OperationSignExtend64From8 struct{}

// Kind implements Operation.Kind.
func (o *OperationSignExtend64From8) Kind() OperationKind {
	return OperationKindSignExtend64From8
}

type OperationSignExtend64From16 struct{}

// Kind implements Operation.Kind.
func (o *OperationSignExtend64From16) Kind() OperationKind {
	return OperationKindSignExtend64From16
}

type OperationSignExtend64From32 struct{}

// Kind implements Operation.Kind.
func (o *OperationSignExtend64From32) Kind() OperationKind {
	return OperationKindSignExtend64From32
}

type OperationMemoryInit struct {
	// DataIndex is the index of the data instance in ModuleInstance.DataInstances
	// by which this operation instantiates a part of the memory.
	DataIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationMemoryInit) Kind() OperationKind {
	return OperationKindMemoryInit
}

type OperationDataDrop struct {
	// DataIndex is the index of the data instance in ModuleInstance.DataInstances
	// which this operation drops.
	DataIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationDataDrop) Kind() OperationKind {
	return OperationKindDataDrop
}

type OperationMemoryCopy struct{}

// Kind implements Operation.Kind.
func (o *OperationMemoryCopy) Kind() OperationKind {
	return OperationKindMemoryCopy
}

type OperationMemoryFill struct{}

// Kind implements Operation.Kind.
func (o *OperationMemoryFill) Kind() OperationKind {
	return OperationKindMemoryFill
}

type OperationTableInit struct {
	// ElemIndex is the index of the element by which this operation initializes a part of the table.
	ElemIndex uint32
	// TableIndex is the index of the table on which this operation initialize by the target element.
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableInit) Kind() OperationKind {
	return OperationKindTableInit
}

type OperationElemDrop struct {
	// ElemIndex is the index of the element which this operation drops.
	ElemIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationElemDrop) Kind() OperationKind {
	return OperationKindElemDrop
}

type OperationTableCopy struct {
	SrcTableIndex, DstTableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableCopy) Kind() OperationKind {
	return OperationKindTableCopy
}

// OperationRefFunc corresponds to OpcodeRefFunc, and engines are expected to
// push the opaque pointer value of engine specific func for the given FunctionIndex.
//
// OperationRefFunc implements Operation.
type OperationRefFunc struct {
	FunctionIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationRefFunc) Kind() OperationKind {
	return OperationKindRefFunc
}

// OperationTableGet implements Operation.
type OperationTableGet struct {
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableGet) Kind() OperationKind {
	return OperationKindTableGet
}

// OperationTableSet implements Operation.
type OperationTableSet struct {
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableSet) Kind() OperationKind {
	return OperationKindTableSet
}

// OperationTableSize implements Operation.
type OperationTableSize struct {
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableSize) Kind() OperationKind {
	return OperationKindTableSize
}

// OperationTableGrow implements Operation.
type OperationTableGrow struct {
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableGrow) Kind() OperationKind {
	return OperationKindTableGrow
}

// OperationTableFill implements Operation.
type OperationTableFill struct {
	TableIndex uint32
}

// Kind implements Operation.Kind.
func (o *OperationTableFill) Kind() OperationKind {
	return OperationKindTableFill
}

// OperationV128Const implements Operation.
type OperationV128Const struct {
	Lo, Hi uint64
}

// Kind implements Operation.Kind.
func (o *OperationV128Const) Kind() OperationKind {
	return OperationKindV128Const
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

// OperationV128Add implements Operation.
type OperationV128Add struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128Add) Kind() OperationKind {
	return OperationKindV128Add
}

// OperationV128Sub implements Operation.
type OperationV128Sub struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128Sub) Kind() OperationKind {
	return OperationKindV128Sub
}

type LoadV128Type = byte

const (
	// LoadV128Type128 corresponds to wasm.OpcodeVecV128LoadName.
	LoadV128Type128 LoadV128Type = iota
	// LoadV128Type8x8s corresponds to wasm.OpcodeVecV128Load8x8SName.
	LoadV128Type8x8s
	// LoadV128Type8x8u corresponds to wasm.OpcodeVecV128Load8x8UName.
	LoadV128Type8x8u
	// LoadV128Type16x4s corresponds to wasm.OpcodeVecV128Load16x4SName
	LoadV128Type16x4s
	// LoadV128Type16x4u corresponds to wasm.OpcodeVecV128Load16x4UName
	LoadV128Type16x4u
	// LoadV128Type32x2s corresponds to wasm.OpcodeVecV128Load32x2SName
	LoadV128Type32x2s
	// LoadV128Type32x2u corresponds to wasm.OpcodeVecV128Load32x2UName
	LoadV128Type32x2u
	// LoadV128Type8Splat corresponds to wasm.OpcodeVecV128Load8SplatName
	LoadV128Type8Splat
	// LoadV128Type16Splat corresponds to wasm.OpcodeVecV128Load16SplatName
	LoadV128Type16Splat
	// LoadV128Type32Splat corresponds to wasm.OpcodeVecV128Load32SplatName
	LoadV128Type32Splat
	// LoadV128Type64Splat corresponds to wasm.OpcodeVecV128Load64SplatName
	LoadV128Type64Splat
	// LoadV128Type32zero corresponds to wasm.OpcodeVecV128Load32zeroName
	LoadV128Type32zero
	// LoadV128Type64zero corresponds to wasm.OpcodeVecV128Load64zeroName
	LoadV128Type64zero
)

// OperationV128Load implements Operation.
type OperationV128Load struct {
	Type LoadV128Type
	Arg  *MemoryArg
}

// Kind implements Operation.Kind.
func (o *OperationV128Load) Kind() OperationKind {
	return OperationKindV128Load
}

// OperationV128LoadLane implements Operation.
type OperationV128LoadLane struct {
	// LaneIndex is >=0 && <(128/LaneSize).
	LaneIndex byte
	// LaneSize is either 8, 16, 32, or 64.
	LaneSize byte
	Arg      *MemoryArg
}

// Kind implements Operation.Kind.
func (o *OperationV128LoadLane) Kind() OperationKind {
	return OperationKindV128LoadLane
}

// OperationV128Store implements Operation.
type OperationV128Store struct {
	Arg *MemoryArg
}

// Kind implements Operation.Kind.
func (o *OperationV128Store) Kind() OperationKind {
	return OperationKindV128Store
}

// OperationV128StoreLane implements Operation.
type OperationV128StoreLane struct {
	// LaneIndex is >=0 && <(128/LaneSize).
	LaneIndex byte
	// LaneSize is either 8, 16, 32, or 64.
	LaneSize byte
	Arg      *MemoryArg
}

// Kind implements Operation.Kind.
func (o *OperationV128StoreLane) Kind() OperationKind {
	return OperationKindV128StoreLane
}

// OperationV128ExtractLane implements Operation.
type OperationV128ExtractLane struct {
	// LaneIndex is >=0 && <M where shape = NxM.
	LaneIndex byte
	// Signed is used when shape is either i8x16 or i16x2 to specify whether to sign-extend or not.
	Signed bool
	Shape  Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128ExtractLane) Kind() OperationKind {
	return OperationKindV128ExtractLane
}

// OperationV128ReplaceLane implements Operation.
type OperationV128ReplaceLane struct {
	// LaneIndex is >=0 && <M where shape = NxM.
	LaneIndex byte
	Shape     Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128ReplaceLane) Kind() OperationKind {
	return OperationKindV128ReplaceLane
}

// OperationV128Splat implements Operation.
type OperationV128Splat struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128Splat) Kind() OperationKind {
	return OperationKindV128Splat
}

// OperationV128Shuffle implements Operation.
type OperationV128Shuffle struct {
	Lanes [16]byte
}

// Kind implements Operation.Kind.
func (o *OperationV128Shuffle) Kind() OperationKind {
	return OperationKindV128Shuffle
}

// OperationV128Swizzle implements Operation.
type OperationV128Swizzle struct{}

// Kind implements Operation.Kind.
func (o *OperationV128Swizzle) Kind() OperationKind {
	return OperationKindV128Swizzle
}

// OperationV128AnyTrue implements Operation.
type OperationV128AnyTrue struct{}

// Kind implements Operation.Kind.
func (o *OperationV128AnyTrue) Kind() OperationKind {
	return OperationKindV128AnyTrue
}

// OperationV128AllTrue implements Operation.
type OperationV128AllTrue struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128AllTrue) Kind() OperationKind {
	return OperationKindV128AllTrue
}

// OperationV128BitMask implements Operation.
type OperationV128BitMask struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128BitMask) Kind() OperationKind {
	return OperationKindV128BitMask
}

// OperationV128And implements Operation.
type OperationV128And struct{}

// Kind implements Operation.Kind.
func (o *OperationV128And) Kind() OperationKind {
	return OperationKindV128And
}

// OperationV128Not implements Operation.
type OperationV128Not struct{}

// Kind implements Operation.Kind.
func (o *OperationV128Not) Kind() OperationKind {
	return OperationKindV128Not
}

// OperationV128Or implements Operation.
type OperationV128Or struct{}

// Kind implements Operation.Kind.
func (o *OperationV128Or) Kind() OperationKind {
	return OperationKindV128Or
}

// OperationV128Xor implements Operation.
type OperationV128Xor struct{}

// Kind implements Operation.Kind.
func (o *OperationV128Xor) Kind() OperationKind {
	return OperationKindV128Xor
}

// OperationV128Bitselect implements Operation.
type OperationV128Bitselect struct{}

// Kind implements Operation.Kind.
func (o *OperationV128Bitselect) Kind() OperationKind {
	return OperationKindV128Bitselect
}

// OperationV128AndNot implements Operation.
type OperationV128AndNot struct{}

// Kind implements Operation.Kind.
func (o *OperationV128AndNot) Kind() OperationKind {
	return OperationKindV128AndNot
}

// OperationV128Shl implements Operation.
type OperationV128Shl struct {
	Shape Shape
}

// Kind implements Operation.Kind.
func (o *OperationV128Shl) Kind() OperationKind {
	return OperationKindV128Shl
}

// OperationV128Shr implements Operation.
type OperationV128Shr struct {
	Shape  Shape
	Signed bool
}

// Kind implements Operation.Kind.
func (o *OperationV128Shr) Kind() OperationKind {
	return OperationKindV128Shr
}

// OperationV128Cmp implements Operation.
type OperationV128Cmp struct {
	Type V128CmpType
}

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

//Kind implements Operation.Kind.
func (o *OperationV128Cmp) Kind() OperationKind {
	return OperationKindV128Cmp
}
