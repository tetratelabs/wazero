package wasm

import "errors"

// All the errors are returned by Engine during the execution of Wasm functions,
// and they indicate that the Wasm virtual machine's state is unrecoverable.
var (
	// ErrRuntimeCallStackOverflow indicates that there are too many function calls,
	// and the Engine terminated the execution.
	ErrRuntimeCallStackOverflow = errors.New("callstack overflow")
	// ErrRuntimeInvalidConversionToInteger indicates the Wasm function tries to
	// convert NaN floating point value to integers during trunc variant instructions.
	ErrRuntimeInvalidConversionToInteger = errors.New("invalid conversion to integer")
	// ErrRuntimeIntegerOverflow indicates that an integer arithmetic resulted in
	// overflow value. For example, when the program tried to truncate a float value
	// which doesn't fit in the range of target integer.
	ErrRuntimeIntegerOverflow = errors.New("integer overflow")
	// ErrRuntimeIntegerDivideByZero indicates that an integer div or rem instructions
	// was executed with 0 as the divisor.
	ErrRuntimeIntegerDivideByZero = errors.New("integer divide by zero")
	// ErrRuntimeUnreachable means "unreachable" instruction was executed by the program.
	ErrRuntimeUnreachable = errors.New("unreachable")
	// ErrRuntimeOutOfBoundsMemoryAccess indicates that the program tried to access the
	// region beyond the linear memory.
	ErrRuntimeOutOfBoundsMemoryAccess = errors.New("out of bounds memory access")
	// ErrRuntimeInvalidTableAccess means either offset to the table was out of bounds of table, or
	// the target element in the table was uninitialized during call_indirect instruction.
	ErrRuntimeInvalidTableAccess = errors.New("invalid table access")
	// ErrRuntimeIndirectCallTypeMismatch indicates that the type check failed during call_indirect.
	ErrRuntimeIndirectCallTypeMismatch = errors.New("indirect call type mismatch")
)
