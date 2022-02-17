// Package wasm includes constants and interfaces used by both public and internal APIs.
package wasm

import (
	"context"
)

// ModuleFunctions return functions available in a module, post-instantiation.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// Note: This includes all return types available in WebAssembly 1.0 (MVP).
type ModuleFunctions interface {
	// GetFunctionVoidReturn returns a void-return function for this module or false if it isn't available under that
	// name or has a different return.
	GetFunctionVoidReturn(name string) (FunctionVoidReturn, bool)

	// GetFunctionI32Return returns a ValueTypeI32-return function for this module or false if it isn't available under
	// that name or has a different return.
	GetFunctionI32Return(name string) (FunctionI32Return, bool)

	// GetFunctionI64Return returns a ValueTypeI64-return function for this module or false if it isn't available under
	// that name or has a different return.
	GetFunctionI64Return(name string) (FunctionI64Return, bool)

	// GetFunctionF32Return returns a ValueTypeF32-return function for this module or false if it isn't available under
	// that name or has a different return.
	GetFunctionF32Return(name string) (FunctionF32Return, bool)

	// GetFunctionF64Return returns a ValueTypeF64-return function for this module or false if it isn't available under
	// that name or has a different return.
	GetFunctionF64Return(name string) (FunctionF64Return, bool)
}

type FunctionVoidReturn func(ctx context.Context, params ...uint64) error
type FunctionI32Return func(ctx context.Context, params ...uint64) (uint32, error)
type FunctionI64Return func(ctx context.Context, params ...uint64) (uint64, error)
type FunctionF32Return func(ctx context.Context, params ...uint64) (float32, error)
type FunctionF64Return func(ctx context.Context, params ...uint64) (float64, error)

// HostFunctionCallContext is the first argument of all host functions.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
type HostFunctionCallContext interface {
	// Context returns the host call's context.
	//
	// The returned context is always non-nil; it defaults to the background context.
	Context() context.Context

	// Memory returns a potentially zero memory for the importing module
	Memory() Memory

	// Functions returns the importing module's functions.
	//
	// Note: The main use case for this is callbacks.
	Functions() ModuleFunctions
}

// Memory allows restricted access to a module's memory. Notably, this does not allow growing.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// Note: This includes all value types available in WebAssembly 1.0 (MVP) and all are encoded little-endian.
// See https://www.w3.org/TR/wasm-core-1/#storage%E2%91%A0
type Memory interface {
	// Len returns the size in bytes available. Ex. If the underlying memory has 1 page: 65536
	//
	// Note: this will not grow during a host function call, even if the underlying memory can.  Ex. If the underlying
	// memory has min 0 and max 2 pages, this returns zero.
	Len() uint32

	// ReadUint32Le reads a uint32 in little-endian encoding from the underlying buffer at the offset in or returns
	// false if out of range.
	ReadUint32Le(offset uint32) (uint32, bool)

	// ReadFloat32Le reads a float32 from 32 IEEE 754 little-endian encoded bits in the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float32bits
	ReadFloat32Le(offset uint32) (float32, bool)

	// ReadUint64Le reads a uint64 in little-endian encoding from the underlying buffer at the offset or returns false
	// if out of range.
	ReadUint64Le(offset uint32) (uint64, bool)

	// ReadFloat64Le reads a float64 from 64 IEEE 754 little-endian encoded bits in the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float64bits
	ReadFloat64Le(offset uint32) (float64, bool)

	// Read reads byteCount bytes from the underlying buffer at the offset or returns false if out of range.
	Read(offset, byteCount uint32) ([]byte, bool)

	// WriteUint32Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint32Le(offset, v uint32) bool

	// WriteFloat32Le writes the value in 32 IEEE 754 little-endian encoded bits to the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float32bits
	WriteFloat32Le(offset uint32, v float32) bool

	// WriteUint64Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint64Le(offset uint32, v uint64) bool

	// WriteFloat64Le writes the value in 64 IEEE 754 little-endian encoded bits to the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float64bits
	WriteFloat64Le(offset uint32, v float64) bool

	// Write writes the slice to the underlying buffer at the offset or returns false if out of range.
	Write(offset uint32, v []byte) bool
}
