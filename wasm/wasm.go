// Package wasm includes constants and interfaces used by both public and internal APIs.
package wasm

import (
	"context"
	"math"
)

// ValueType describes a numeric type used in Web Assembly 1.0 (20191205). For example, Function parameters and results are
// only definable as a value type.
//
// The following describes how to convert between Wasm and Golang types:
//  * ValueTypeI32 - uint64(uint32,int32,int64)
//  * ValueTypeI64 - uint64
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
//	results, _ := fn(ctx, wasm.EncodeF64(input))
//	result := wasm.DecodeF64(result[0])
//
// Note: This is a type alias as it is easier to encode and decode in the binary format.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-valtype
type ValueType = byte

const (
	ValueTypeI32 ValueType = 0x7f
	ValueTypeI64 ValueType = 0x7e
	ValueTypeF32 ValueType = 0x7d
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

// Store allows access to instantiated modules and host functions
type Store interface {
	// ModuleExports returns exports from an instantiated module or nil if there aren't any.
	ModuleExports(moduleName string) ModuleExports

	// HostExports returns exported host functions for the moduleName or nil if there aren't any.
	HostExports(moduleName string) HostExports
}

// ModuleExports return functions exported in a module, post-instantiation.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
//
// TODO: rename this to InstantiatedModule per https://github.com/tetratelabs/wazero/issues/293.
type ModuleExports interface {
	// Memory returns a memory exported from this module or nil if it wasn't.
	Memory() Memory

	// Function returns a function exported from this module or nil if it wasn't.
	Function(name string) Function

	// TODO
	Global(name string) Global
}

// Function is a WebAssembly 1.0 (20191205) function exported from an instantiated module (wazero.InstantiateModule).
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
	// If the `ctx` is nil, it defaults to the same context as the module was initialized with.
	//
	// To ensure context propagation in a HostFunction body, use or derive `ctx` from ModuleContext.Context:
	//
	//	hostFunction := func(ctx wasm.ModuleContext, offset, byteCount uint32) uint32 {
	//		fn, _ = ctx.Function("__read")
	//		results, err := fn(ctx.Context(), offset, byteCount)
	//	--snip--
	Call(ctx context.Context, params ...uint64) ([]uint64, error)
}

type Global interface {
	Value() uint64
	Type() ValueType
}

// HostExports return functions defined in Go, a.k.a. "Host Functions" in WebAssembly 1.0 (20191205).
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-hostfunc
type HostExports interface {
	// Function returns a host function exported under this module name or nil if it wasn't.
	Function(name string) HostFunction
}

// HostFunction is like a Function, except it is implemented in Go. This is a "Host Function" in WebAssembly 1.0 (20191205).
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-hostfunc
type HostFunction interface {
	// ParamTypes are documented as Function.ParamTypes
	ParamTypes() []ValueType

	// ResultTypes are documented as Function.ResultTypes
	ResultTypes() []ValueType

	// Call is the same as Function.Call, except it must be called from an importing module (ctx). The can also err if
	// the module did not import this function!
	Call(ctx ModuleContext, params ...uint64) ([]uint64, error)
}

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

	// Function returns a function exported from this module or nil if it wasn't.
	Function(name string) Function
}

// Memory allows restricted access to a module's memory. Notably, this does not allow growing.
//
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
	Size() uint32

	// ReadByte reads a single byte from the underlying buffer at the offset in or returns false if out of range.
	ReadByte(offset uint32) (byte, bool)

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

	// WriteByte writes a single byte to the underlying buffer at the offset in or returns false if out of range.
	WriteByte(offset uint32, v byte) bool

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

// HostModuleConfig are WebAssembly 1.0 (20191205) exports from the host bound to a module name used by InstantiateHostModule.
type HostModuleConfig struct {
	// Name is the module name that these exports can be imported with. Ex. wasi.ModuleSnapshotPreview1
	Name string

	// Functions adds functions written in Go, which a WebAssembly Module can import.
	//
	// The key is the name to export and the value is the func. Ex. WASISnapshotPreview1
	//
	// Noting a context exception described later, all parameters or result types must match WebAssembly 1.0 (20191205) value
	// types. This means uint32, uint64, float32 or float64. Up to one result can be returned.
	//
	// Ex. This is a valid host function:
	//
	//	addInts := func(x uint32, uint32) uint32 {
	//		return x + y
	//	}
	//
	// Host functions may also have an initial parameter (param[0]) of type context.Context or wasm.ModuleContext.
	//
	// Ex. This uses a Go Context:
	//
	//	addInts := func(ctx context.Context, x uint32, uint32) uint32 {
	//		// add a little extra if we put some in the context!
	//		return x + y + ctx.Value(extraKey).(uint32)
	//	}
	//
	// The most sophisticated context is wasm.ModuleContext, which allows access to the Go context, but also
	// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
	// data is via writing memory and sharing offsets.
	//
	// Ex. This reads the parameters from!
	//
	//	addInts := func(ctx wasm.ModuleContext, offset uint32) uint32 {
	//		x, _ := ctx.Memory().ReadUint32Le(offset)
	//		y, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
	//		return x + y
	//	}
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#host-functions%E2%91%A2
	Functions map[string]interface{}

	// TODO
	Globals map[string]*HostModuleConfigGlobal
	// TODO
	Table *HostModuleConfigTable
	// TODO
	Memory *HostModuleConfigMemory
}

type HostModuleConfigGlobal struct {
	Type  ValueType
	Value uint64
}

type hostModuleConfigLimit struct {
	Name string
	Min  uint32
	Max  *uint32
}

type HostModuleConfigTable hostModuleConfigLimit
type HostModuleConfigMemory hostModuleConfigLimit
