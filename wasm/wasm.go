// Package wasm includes constants and interfaces used by both public and internal APIs.
package wasm

import (
	"context"
	"math"
)

// Store allows access to instantiated modules and host functions
type Store interface {
	// ModuleExports returns exports from an instantiated module or false if it wasn't.
	ModuleExports(moduleName string) (ModuleExports, bool)

	// HostExports returns exported host functions for the moduleName or false there are none.
	HostExports(moduleName string) (HostExports, bool)
}

// ModuleExports return functions exported in a module, post-instantiation.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
type ModuleExports interface {
	// Function returns a function exported from this module or false if it wasn't.
	Function(name string) (Function, bool)
}

// Function is an advanced API allowing efficient invocation of WebAssembly 1.0 (MVP) functions, given predefined
// knowledge about the function signature. An error is returned for any failure looking up or invoking the function including
// signature mismatch.
//
// Web Assembly 1.0 (MVP) Value Type Conversion:
//  * I32 - uint64(uint32,int32,int64)
//  * I64 - uint64
//  * F32 - EncodeF32 DecodeF32 from float32
//  * F64 - EncodeF64 DecodeF64 from float64
//
// Ex. Given a Text Format type use (param i64) (result i64)
//
//	results, _ := fn(ctx, input)
//	result := result[0]
//
// Ex. Given a Text Format type use (param f64) (result f64)
//
//	results, _ := fn(ctx, wasm.EncodeF64(input))
//	result := wasm.DecodeF64(result[0])
//
// Note: The ctx parameter will be the outer-most ancestor of ModuleContext.Context
// ctx will default to context.Background() is nil is passed.
// See https://www.w3.org/TR/wasm-core-1/#binary-valtype
type Function func(ctx context.Context, params ...uint64) ([]uint64, error)

// HostExports return functions defined in Go, a.k.a. "Host Functions" in WebAssembly 1.0 (MVP).
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// See https://www.w3.org/TR/wasm-core-1/#syntax-hostfunc
type HostExports interface {
	// Function returns a function in this module or false if it isn't available under that name.
	Function(name string) (HostFunction, bool)
}

// HostFunction is like a Function, except it is implemented in Go. This is a "Host Function" in WebAssembly 1.0 (MVP).
//
// Note: The usage is the same as Function, except it must be called from an importing module (ctx). The errs if the
// module did not import this function!
// See https://www.w3.org/TR/wasm-core-1/#syntax-hostfunc
type HostFunction func(ctx ModuleContext, params ...uint64) ([]uint64, error)

// ModuleContext is the first argument of a HostFunction.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
type ModuleContext interface {
	// Context returns the host call's context.
	//
	// The returned context is always non-nil; it defaults to the background context.
	Context() context.Context

	// Memory returns a potentially zero memory of the importing module
	Memory() Memory

	// Function returns a function exported from this module or false if it wasn't.
	Function(name string) (Function, bool)
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

// EncodeF32 converts the input so that it can be used as a Function F32 parameter or result.
// See DecodeF32
func EncodeF32(input float32) uint64 {
	return uint64(math.Float32bits(input))
}

// DecodeF32 converts the Function F32 parameter or result to a float32.
// See DecodeF32
func DecodeF32(input uint64) float32 {
	return math.Float32frombits(uint32(input))
}

// EncodeF64 converts the input so that it can be used as a Function F64 parameter or result.
// See DecodeF64
func EncodeF64(input float64) uint64 {
	return math.Float64bits(input)
}

// DecodeF64 converts the Function F64 parameter or result to a float64.
// See EncodeF64
func DecodeF64(input uint64) float64 {
	return math.Float64frombits(input)
}
