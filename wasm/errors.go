package wasm

import "errors"

var (
	ErrRuntimeCallStackOverflow          = errors.New("callstack overflow")
	ErrRuntimeInvalidConversionToInteger = errors.New("invalid conversion to integer")
	ErrRuntimeIntegerOverflow            = errors.New("integer overflow")
	ErrRuntimeIntegerDivideByZero        = errors.New("integer divide by zero")
	ErrRuntimeUnreachable                = errors.New("unreachable")
	ErrRuntimeOutOfBoundsMemoryAccess    = errors.New("out of bounds memory access")
	ErrRuntimeInvalidTableAcces          = errors.New("invalid table access")
	ErrRuntimeIndirectCallTypeMismatch   = errors.New("indirect call type mismatch")
)
