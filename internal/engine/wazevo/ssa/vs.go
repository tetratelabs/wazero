package ssa

import (
	"fmt"
	"math"
)

// Variable is a unique identifier for a source program's variable and will correspond to
// multiple ssa Value(s).
//
// For example, `Local 1` is a Variable in WebAssembly, and Value(s) will be created for it
// whenever it executes `local.set 1`.
//
// Variable is useful to track the SSA Values of a variable in the source program, and
// can be used to find the corresponding latest SSA Value via Builder.FindValue.
type Variable uint32

// String implements fmt.Stringer.
func (v Variable) String() string {
	return fmt.Sprintf("var%d", v)
}

// Value represents an SSA value with a type information. The relationship with Variable is 1: N (including 0),
// that means there might be multiple Variable(s) for a Value.
//
// Higher 32-bit is used to store Type for this value.
type Value uint64

// ValueID is the lower 32bit of Value, which is the pure identifier of Value without type info.
type ValueID uint32

const (
	valueIDInvalid ValueID = math.MaxUint32
	ValueInvalid   Value   = Value(valueIDInvalid)
)

// Format creates a debug string for this Value using the data stored in Builder.
func (v Value) Format(b Builder) string {
	if annotation, ok := b.(*builder).valueAnnotations[v.ID()]; ok {
		return annotation
	}
	return fmt.Sprintf("v%d", v.ID())
}

func (v Value) formatWithType(b Builder) string {
	if annotation, ok := b.(*builder).valueAnnotations[v.ID()]; ok {
		return annotation + ":" + v.Type().String()
	} else {
		return fmt.Sprintf("v%d:%s", v.ID(), v.Type())
	}
}

// Valid returns true if this value is valid.
func (v Value) Valid() bool {
	return v.ID() != valueIDInvalid
}

// Type returns the Type of this value.
func (v Value) Type() Type {
	return Type(v >> 32)
}

// ID returns the valueID of this value.
func (v Value) ID() ValueID {
	return ValueID(v)
}

// setType sets a type to this Value and returns the updated Value.
func (v Value) setType(typ Type) Value {
	return v | Value(typ)<<32
}
