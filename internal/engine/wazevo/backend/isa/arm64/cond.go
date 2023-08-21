package arm64

import (
	"strconv"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type (
	cond     uint64
	condKind byte
)

const (
	// condKindRegisterZero represents a condition which checks if the register is zero.
	// This indicates that the instruction must be encoded as CBZ:
	// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/CBZ--Compare-and-Branch-on-Zero-
	condKindRegisterZero condKind = iota
	// condKindRegisterNotZero indicates that the instruction must be encoded as CBNZ:
	// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/CBNZ--Compare-and-Branch-on-Nonzero-
	condKindRegisterNotZero
	// condKindCondFlagSet indicates that the instruction must be encoded as B.cond:
	// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/B-cond--Branch-conditionally-
	condKindCondFlagSet
)

// kind returns the kind of condition which is stored in the first two bits.
func (c cond) kind() condKind {
	return condKind(c & 0b11)
}

func (c cond) asUint64() uint64 {
	return uint64(c)
}

// register returns the register for register conditions.
// This panics if the condition is not a register condition (condKindRegisterZero or condKindRegisterNotZero).
func (c cond) register() regalloc.VReg {
	if c.kind() != condKindRegisterZero && c.kind() != condKindRegisterNotZero {
		panic("condition is not a register")
	}
	return regalloc.VReg(c >> 2)
}

func registerAsRegZeroCond(r regalloc.VReg) cond {
	return cond(r)<<2 | cond(condKindRegisterZero)
}

func registerAsRegNotZeroCond(r regalloc.VReg) cond {
	return cond(r)<<2 | cond(condKindRegisterNotZero)
}

func (c cond) flag() condFlag {
	if c.kind() != condKindCondFlagSet {
		panic("condition is not a flag")
	}
	return condFlag(c >> 2)
}

func (c condFlag) asCond() cond {
	return cond(c)<<2 | cond(condKindCondFlagSet)
}

// condFlag represents a condition flag for conditional branches.
// The value matches the encoding of condition flags in the ARM64 instruction set.
// https://developer.arm.com/documentation/den0024/a/The-A64-instruction-set/Data-processing-instructions/Conditional-instructions
type condFlag uint8

const (
	eq condFlag = iota // eq represents "equal"
	ne                 // ne represents "not equal"
	hs                 // hs represents "higher or same"
	lo                 // lo represents "lower"
	mi                 // mi represents "minus or negative result"
	pl                 // pl represents "plus or positive result"
	vs                 // vs represents "overflow set"
	vc                 // vc represents "overflow clear"
	hi                 // hi represents "higher"
	ls                 // ls represents "lower or same"
	ge                 // ge represents "greater or equal"
	lt                 // lt represents "less than"
	gt                 // gt represents "greater than"
	le                 // le represents "less than or equal"
	al                 // al represents "always"
	nv                 // nv represents "never"
)

// invert returns the inverted condition.
func (c condFlag) invert() condFlag {
	switch c {
	case eq:
		return ne
	case ne:
		return eq
	case hs:
		return lo
	case lo:
		return hs
	case mi:
		return pl
	case pl:
		return mi
	case vs:
		return vc
	case vc:
		return vs
	case hi:
		return ls
	case ls:
		return hi
	case ge:
		return lt
	case lt:
		return ge
	case gt:
		return le
	case le:
		return gt
	case al:
		return nv
	case nv:
		return al
	default:
		panic(c)
	}
}

// String implements fmt.Stringer.
func (c condFlag) String() string {
	switch c {
	case eq:
		return "eq"
	case ne:
		return "ne"
	case hs:
		return "hs"
	case lo:
		return "lo"
	case mi:
		return "mi"
	case pl:
		return "pl"
	case vs:
		return "vs"
	case vc:
		return "vc"
	case hi:
		return "hi"
	case ls:
		return "ls"
	case ge:
		return "ge"
	case lt:
		return "lt"
	case gt:
		return "gt"
	case le:
		return "le"
	case al:
		return "al"
	case nv:
		return "nv"
	default:
		panic(strconv.Itoa(int(c)))
	}
}

// condFlagFromSSAIntegerCmpCond returns the condition flag for the given ssa.IntegerCmpCond.
func condFlagFromSSAIntegerCmpCond(c ssa.IntegerCmpCond) condFlag {
	switch c {
	case ssa.IntegerCmpCondEqual:
		return eq
	case ssa.IntegerCmpCondNotEqual:
		return ne
	case ssa.IntegerCmpCondSignedLessThan:
		return lt
	case ssa.IntegerCmpCondSignedGreaterThanOrEqual:
		return ge
	case ssa.IntegerCmpCondSignedGreaterThan:
		return gt
	case ssa.IntegerCmpCondSignedLessThanOrEqual:
		return le
	case ssa.IntegerCmpCondUnsignedLessThan:
		return lo
	case ssa.IntegerCmpCondUnsignedGreaterThanOrEqual:
		return hs
	case ssa.IntegerCmpCondUnsignedGreaterThan:
		return hi
	case ssa.IntegerCmpCondUnsignedLessThanOrEqual:
		return ls
	default:
		panic(c)
	}
}

// condFlagFromSSAFloatCmpCond returns the condition flag for the given ssa.FloatCmpCond.
func condFlagFromSSAFloatCmpCond(c ssa.FloatCmpCond) condFlag {
	switch c {
	case ssa.FloatCmpCondEqual:
		return eq
	case ssa.FloatCmpCondNotEqual:
		return ne
	case ssa.FloatCmpCondLessThan:
		return mi
	case ssa.FloatCmpCondLessThanOrEqual:
		return ls
	case ssa.FloatCmpCondGreaterThan:
		return gt
	case ssa.FloatCmpCondGreaterThanOrEqual:
		return ge
	default:
		panic(c)
	}
}

// vecArrangement is the arrangement of data within a vector register.
type vecArrangement byte

const (
	// vecArrangementNone is an arrangement indicating no data is stored.
	vecArrangementNone vecArrangement = iota
	// vecArrangement8B is an arrangement of 8 bytes (64-bit vector)
	vecArrangement8B
	// vecArrangement16B is an arrangement of 16 bytes (128-bit vector)
	vecArrangement16B
	// vecArrangement4H is an arrangement of 4 half precisions (64-bit vector)
	vecArrangement4H
	// vecArrangement8H is an arrangement of 8 half precisions (128-bit vector)
	vecArrangement8H
	// vecArrangement2S is an arrangement of 2 single precisions (64-bit vector)
	vecArrangement2S
	// vecArrangement4S is an arrangement of 4 single precisions (128-bit vector)
	vecArrangement4S
	// vecArrangement1D is an arrangement of 1 double precision (64-bit vector)
	vecArrangement1D
	// vecArrangement2D is an arrangement of 2 double precisions (128-bit vector)
	vecArrangement2D

	// Assign each vector size specifier to a vector arrangement ID.
	// Instructions can only have an arrangement or a size specifier, but not both, so it
	// simplifies the internal representation of vector instructions by being able to
	// store either into the same field.

	// vecArrangementB is a size specifier of byte
	vecArrangementB
	// vecArrangementH is a size specifier of word (16-bit)
	vecArrangementH
	// vecArrangementS is a size specifier of double word (32-bit)
	vecArrangementS
	// vecArrangementD is a size specifier of quad word (64-bit)
	vecArrangementD
	// vecArrangementQ is a size specifier of the entire vector (128-bit)
	vecArrangementQ
)

// String implements fmt.Stringer
func (v vecArrangement) String() (ret string) {
	switch v {
	case vecArrangement8B:
		ret = "8B"
	case vecArrangement16B:
		ret = "16B"
	case vecArrangement4H:
		ret = "4H"
	case vecArrangement8H:
		ret = "8H"
	case vecArrangement2S:
		ret = "2S"
	case vecArrangement4S:
		ret = "4S"
	case vecArrangement1D:
		ret = "1D"
	case vecArrangement2D:
		ret = "2D"
	case vecArrangementB:
		ret = "B"
	case vecArrangementH:
		ret = "H"
	case vecArrangementS:
		ret = "S"
	case vecArrangementD:
		ret = "D"
	case vecArrangementQ:
		ret = "Q"
	case vecArrangementNone:
		ret = "none"
	default:
		panic(v)
	}
	return
}

// vecIndex is the index of an element of a vector register
type vecIndex byte

// vecIndexNone indicates no vector index specified.
const vecIndexNone = ^vecIndex(0)
