// Package api includes constants and interfaces used by both end-users and internal implementations.
package api

import (
	"context"
	"fmt"
	"math"
)

// ValueType describes a numeric type used in Web Assembly 1.0 (20191205). For example, Function parameters and results are
// only definable as a value type.
//
// The following describes how to convert between Wasm and Golang types:
//  * ValueTypeI32 - uint64(uint32,int32)
//  * ValueTypeI64 - uint64(int64)
//  * ValueTypeF32 - EncodeF32 DecodeF32 from float32
//  * ValueTypeF64 - EncodeF64 DecodeF64 from float64
//
// Ex. Given a Text Format type use (param i64) (result i64), no conversion is necessary.
//
//	results, _ := fn(ctx, input)
//	result := result[0]
//
// Ex. Given a Text Format type use (param f64) (result f64), conversion is necessary.
//
//	results, _ := fn(ctx, api.EncodeF64(input))
//	result := api.DecodeF64(result[0])
//
// Note: This is a type alias as it is easier to encode and decode in the binary format.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-valtype
type ValueType = byte

const (
	// ValueTypeI32 is a 32-bit integer.
	ValueTypeI32 ValueType = 0x7f
	// ValueTypeI64 is a 64-bit integer.
	ValueTypeI64 ValueType = 0x7e
	// ValueTypeF32 is a 32-bit floating point number.
	ValueTypeF32 ValueType = 0x7d
	// ValueTypeF64 is a 32-bit floating point number.
	ValueTypeF64 ValueType = 0x7c
)

// ValueTypeName returns the type name of the given ValueType as a string.
// These type names match the names used in the WebAssembly text format.
//
// Note: This returns "unknown", if an undefined ValueType value is passed.
func ValueTypeName(t ValueType) string {
	switch t {
	case ValueTypeI32:
		return "i32"
	case ValueTypeI64:
		return "i64"
	case ValueTypeF32:
		return "f32"
	case ValueTypeF64:
		return "f64"
	}
	return "unknown"
}

// Module return functions exported in a module, post-instantiation.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#external-types%E2%91%A0
type Module interface {
	fmt.Stringer

	// Name is the name this module was instantiated with. Exported functions can be imported with this name.
	Name() string

	// Close is a convenience that invokes CloseWithExitCode with zero.
	// Note: When `ctx` is nil, it defaults to context.Background.
	Close(ctx context.Context) error

	// CloseWithExitCode releases resources allocated for this Module. Use a non-zero exitCode parameter to indicate a
	// failure to ExportedFunction callers.
	//
	// The error returned here, if present, is about resource de-allocation (such as I/O errors). Only the last error is
	// returned, so a non-nil return means at least one error happened. Regardless of error, this module instance will
	// be removed, making its name available again.
	//
	// Calling this inside a host function is safe, and may cause ExportedFunction callers to receive a sys.ExitError
	// with the exitCode.
	// Note: When `ctx` is nil, it defaults to context.Background.
	CloseWithExitCode(ctx context.Context, exitCode uint32) error

	// Memory returns a memory defined in this module or nil if there are none wasn't.
	Memory() Memory

	// ExportedFunction returns a function exported from this module or nil if it wasn't.
	ExportedFunction(name string) Function

	// TODO: Table

	// ExportedMemory returns a memory exported from this module or nil if it wasn't.
	//
	// Note: WASI modules require exporting a Memory named "memory". This means that a module successfully initialized
	// as a WASI Command or Reactor will never return nil for this name.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	ExportedMemory(name string) Memory

	// ExportedGlobal a global exported from this module or nil if it wasn't.
	ExportedGlobal(name string) Global
}

// Function is a WebAssembly 1.0 (20191205) function exported from an instantiated module (wazero.Runtime InstantiateModule).
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-func
type Function interface {
	// ParamTypes are the possibly empty sequence of value types accepted by a function with this signature.
	// See ValueType documentation for encoding rules.
	ParamTypes() []ValueType

	// ResultTypes are the possibly empty sequence of value types returned by a function with this signature.
	//
	// Note: In WebAssembly 1.0 (20191205), there can be at most one result.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#result-types%E2%91%A0
	// See ValueType documentation for decoding rules.
	ResultTypes() []ValueType

	// Call invokes the function with parameters encoded according to ParamTypes. Up to one result is returned,
	// encoded according to ResultTypes. An error is returned for any failure looking up or invoking the function
	// including signature mismatch.
	//
	// Note: When `ctx` is nil, it defaults to context.Background.
	// Note: If Module.Close or Module.CloseWithExitCode were invoked during this call, the error returned may be a
	// sys.ExitError. Interpreting this is specific to the module. For example, some "main" functions always call a
	// function that exits.
	Call(ctx context.Context, params ...uint64) ([]uint64, error)
}

// Global is a WebAssembly 1.0 (20191205) global exported from an instantiated module (wazero.Runtime InstantiateModule).
//
// Ex. If the value is not mutable, you can read it once:
//
//	offset := module.ExportedGlobal("memory.offset").Get()
//
// Globals are allowed by specification to be mutable. However, this can be disabled by configuration. When in doubt,
// safe cast to find out if the value can change. Ex.
//
//	offset := module.ExportedGlobal("memory.offset")
//	if _, ok := offset.(api.MutableGlobal); ok {
//		// value can change
//	} else {
//		// value is constant
//	}
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#globals%E2%91%A0
type Global interface {
	fmt.Stringer

	// Type describes the numeric type of the global.
	Type() ValueType

	// Get returns the last known value of this global.
	// See Type for how to encode this value from a Go type.
	//
	// Note: When `ctx` is nil, it defaults to context.Background.
	Get(ctx context.Context) uint64
}

// MutableGlobal is a Global whose value can be updated at runtime (variable).
type MutableGlobal interface {
	Global

	// Set updates the value of this global.
	// See Global.Type for how to decode this value to a Go type.
	//
	// Note: When `ctx` is nil, it defaults to context.Background.
	Set(ctx context.Context, v uint64)
}

// Memory allows restricted access to a module's memory. Notably, this does not allow growing.
//
// Note: All functions accept a context.Context, which when nil, default to context.Background.
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// Note: This includes all value types available in WebAssembly 1.0 (20191205) and all are encoded little-endian.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#storage%E2%91%A0
type Memory interface {
	// Size returns the size in bytes available. Ex. If the underlying memory has 1 page: 65536
	//
	// Note: this will not grow during a host function call, even if the underlying memory can.  Ex. If the underlying
	// memory has min 0 and max 2 pages, this returns zero.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefsyntax-instr-memorymathsfmemorysize%E2%91%A0
	Size(ctx context.Context) uint32

	// IndexByte returns the index of the first instance of c in the underlying buffer at the offset or returns false if
	// not found or out of range.
	IndexByte(ctx context.Context, offset uint32, c byte) (uint32, bool)

	// ReadByte reads a single byte from the underlying buffer at the offset or returns false if out of range.
	ReadByte(ctx context.Context, offset uint32) (byte, bool)

	// ReadUint16Le reads a uint16 in little-endian encoding from the underlying buffer at the offset in or returns
	// false if out of range.
	ReadUint16Le(ctx context.Context, offset uint32) (uint16, bool)

	// ReadUint32Le reads a uint32 in little-endian encoding from the underlying buffer at the offset in or returns
	// false if out of range.
	ReadUint32Le(ctx context.Context, offset uint32) (uint32, bool)

	// ReadFloat32Le reads a float32 from 32 IEEE 754 little-endian encoded bits in the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float32bits
	ReadFloat32Le(ctx context.Context, offset uint32) (float32, bool)

	// ReadUint64Le reads a uint64 in little-endian encoding from the underlying buffer at the offset or returns false
	// if out of range.
	ReadUint64Le(ctx context.Context, offset uint32) (uint64, bool)

	// ReadFloat64Le reads a float64 from 64 IEEE 754 little-endian encoded bits in the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float64bits
	ReadFloat64Le(ctx context.Context, offset uint32) (float64, bool)

	// Read reads byteCount bytes from the underlying buffer at the offset or returns false if out of range.
	Read(ctx context.Context, offset, byteCount uint32) ([]byte, bool)

	// WriteByte writes a single byte to the underlying buffer at the offset in or returns false if out of range.
	WriteByte(ctx context.Context, offset uint32, v byte) bool

	// WriteUint16Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint16Le(ctx context.Context, offset uint32, v uint16) bool

	// WriteUint32Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint32Le(ctx context.Context, offset, v uint32) bool

	// WriteFloat32Le writes the value in 32 IEEE 754 little-endian encoded bits to the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float32bits
	WriteFloat32Le(ctx context.Context, offset uint32, v float32) bool

	// WriteUint64Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint64Le(ctx context.Context, offset uint32, v uint64) bool

	// WriteFloat64Le writes the value in 64 IEEE 754 little-endian encoded bits to the underlying buffer at the offset
	// or returns false if out of range.
	// See math.Float64bits
	WriteFloat64Le(ctx context.Context, offset uint32, v float64) bool

	// Write writes the slice to the underlying buffer at the offset or returns false if out of range.
	Write(ctx context.Context, offset uint32, v []byte) bool
}

// EncodeI32 encodes the input as a ValueTypeI32.
func EncodeI32(input int32) uint64 {
	return uint64(uint32(input))
}

// EncodeI64 encodes the input as a ValueTypeI64.
func EncodeI64(input int64) uint64 {
	return uint64(input)
}

// EncodeF32 encodes the input as a ValueTypeF32.
// See DecodeF32
func EncodeF32(input float32) uint64 {
	return uint64(math.Float32bits(input))
}

// DecodeF32 decodes the input as a ValueTypeF32.
// See DecodeF32
func DecodeF32(input uint64) float32 {
	return math.Float32frombits(uint32(input))
}

// EncodeF64 encodes the input as a ValueTypeF64.
// See DecodeF64
func EncodeF64(input float64) uint64 {
	return math.Float64bits(input)
}

// DecodeF64 decodes the input as a ValueTypeF64.
// See EncodeF64
func DecodeF64(input uint64) float64 {
	return math.Float64frombits(input)
}
