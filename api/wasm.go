// Package api includes constants and interfaces used by both end-users and internal implementations.
package api

import (
	"context"
	"fmt"
	"math"
	"reflect"
)

// ExternType classifies imports and exports with their respective types.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#external-types%E2%91%A0
type ExternType = byte

const (
	ExternTypeFunc   ExternType = 0x00
	ExternTypeTable  ExternType = 0x01
	ExternTypeMemory ExternType = 0x02
	ExternTypeGlobal ExternType = 0x03
)

// The below are exported to consolidate parsing behavior for external types.
const (
	// ExternTypeFuncName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeFunc.
	ExternTypeFuncName = "func"
	// ExternTypeTableName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeTable.
	ExternTypeTableName = "table"
	// ExternTypeMemoryName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeMemory.
	ExternTypeMemoryName = "memory"
	// ExternTypeGlobalName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeGlobal.
	ExternTypeGlobalName = "global"
)

// ExternTypeName returns the name of the WebAssembly 1.0 (20191205) Text Format field of the given type.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#exports%E2%91%A4
func ExternTypeName(et ExternType) string {
	switch et {
	case ExternTypeFunc:
		return ExternTypeFuncName
	case ExternTypeTable:
		return ExternTypeTableName
	case ExternTypeMemory:
		return ExternTypeMemoryName
	case ExternTypeGlobal:
		return ExternTypeGlobalName
	}
	return fmt.Sprintf("%#x", et)
}

// ValueType describes a numeric type used in Web Assembly 1.0 (20191205). For example, Function parameters and results are
// only definable as a value type.
//
// The following describes how to convert between Wasm and Golang types:
//
//   - ValueTypeI32 - uint64(uint32,int32)
//   - ValueTypeI64 - uint64(int64)
//   - ValueTypeF32 - EncodeF32 DecodeF32 from float32
//   - ValueTypeF64 - EncodeF64 DecodeF64 from float64
//   - ValueTypeExternref - unintptr(unsafe.Pointer(p)) where p is any pointer type in Go (e.g. *string)
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
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-valtype
type ValueType = byte

const (
	// ValueTypeI32 is a 32-bit integer.
	ValueTypeI32 ValueType = 0x7f
	// ValueTypeI64 is a 64-bit integer.
	ValueTypeI64 ValueType = 0x7e
	// ValueTypeF32 is a 32-bit floating point number.
	ValueTypeF32 ValueType = 0x7d
	// ValueTypeF64 is a 64-bit floating point number.
	ValueTypeF64 ValueType = 0x7c

	// ValueTypeExternref is a externref type.
	//
	// Note: in wazero, externref type value are opaque raw 64-bit pointers,
	// and the ValueTypeExternref type in the signature will be translated as
	// uintptr in wazero's API level.
	//
	// For example, given the import function:
	//	(func (import "env" "f") (param externref) (result externref))
	//
	// This can be defined in Go as:
	//  r.NewModuleBuilder("env").ExportFunctions(map[string]interface{}{
	//    "f": func(externref uintptr) (resultExternRef uintptr) { return },
	//  })
	//
	// Note: The usage of this type is toggled with WithFeatureBulkMemoryOperations.
	ValueTypeExternref ValueType = 0x6f
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
	case ValueTypeExternref:
		return "externref"
	}
	return "unknown"
}

// Module return functions exported in a module, post-instantiation.
//
// # Notes
//
//   - Closing the wazero.Runtime closes any Module it instantiated.
//   - This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#external-types%E2%91%A0
type Module interface {
	fmt.Stringer

	// Name is the name this module was instantiated with. Exported functions can be imported with this name.
	Name() string

	// Memory returns a memory defined in this module or nil if there are none wasn't.
	Memory() Memory

	// ExportedFunction returns a function exported from this module or nil if it wasn't.
	ExportedFunction(name string) Function

	// TODO: Table

	// ExportedMemory returns a memory exported from this module or nil if it wasn't.
	//
	// WASI modules require exporting a Memory named "memory". This means that a module successfully initialized
	// as a WASI Command or Reactor will never return nil for this name.
	//
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	ExportedMemory(name string) Memory

	// ExportedGlobal a global exported from this module or nil if it wasn't.
	ExportedGlobal(name string) Global

	// CloseWithExitCode releases resources allocated for this Module. Use a non-zero exitCode parameter to indicate a
	// failure to ExportedFunction callers. When the context is nil, it defaults to context.Background.
	//
	// The error returned here, if present, is about resource de-allocation (such as I/O errors). Only the last error is
	// returned, so a non-nil return means at least one error happened. Regardless of error, this module instance will
	// be removed, making its name available again.
	//
	// Calling this inside a host function is safe, and may cause ExportedFunction callers to receive a sys.ExitError
	// with the exitCode.
	CloseWithExitCode(ctx context.Context, exitCode uint32) error

	// Closer closes this module by delegating to CloseWithExitCode with an exit code of zero.
	Closer
}

// Closer closes a resource.
//
// Note: This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
type Closer interface {
	// Close closes the resource. When the context is nil, it defaults to context.Background.
	Close(context.Context) error
}

// FunctionDefinition is a WebAssembly function exported in a module (wazero.CompiledModule).
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#exports%E2%91%A0
type FunctionDefinition interface {
	// ModuleName is the possibly empty name of the module defining this
	// function.
	//
	// Note: This may be different from Module.Name, because a compiled module
	// can be instantiated multiple times as different names.
	ModuleName() string

	// Index is the position in the module's function index namespace, imports
	// first.
	Index() uint32

	// Name is the module-defined name of the function, which is not necessarily
	// the same as its export name.
	Name() string

	// DebugName identifies this function based on its Index or Name in the
	// module. This is used for errors and stack traces. Ex. "env.abort".
	//
	// When the function name is empty, a substitute name is generated by
	// prefixing '$' to its position in the index namespace. Ex ".$0" is the
	// first function (possibly imported) in an unnamed module.
	//
	// The format is dot-delimited module and function name, but there are no
	// restrictions on the module and function name. This means either can be
	// empty or include dots. Ex. "x.x.x" could mean module "x" and name "x.x",
	// or it could mean module "x.x" and name "x".
	//
	// Note: This name is stable regardless of import or export. For example,
	// if Import returns true, the value is still based on the Name or Index
	// and not the imported function name.
	DebugName() string

	// Import returns true with the module and function name when this function
	// is imported. Otherwise, it returns false.
	//
	// Note: Empty string is valid for both the imported module and function
	// name in the WebAssembly specification.
	Import() (moduleName, name string, isImport bool)

	// ExportNames include all exported names for the given function.
	//
	// Note: The empty name is allowed in the WebAssembly specification, so ""
	// is possible.
	ExportNames() []string

	// GoFunc is present when the function was implemented by the embedder
	// (ex via wazero.ModuleBuilder) instead of a wasm binary.
	//
	// This function can be non-deterministic or cause side effects. It also
	// has special properties not defined in the WebAssembly Core
	// specification. Notably, it uses the caller's memory, which might be
	// different from its defining module.
	//
	// See https://www.w3.org/TR/wasm-core-1/#host-functions%E2%91%A0
	GoFunc() *reflect.Value

	// ParamTypes are the possibly empty sequence of value types accepted by a
	// function with this signature.
	//
	// See ValueType documentation for encoding rules.
	ParamTypes() []ValueType

	// ParamNames are index-correlated with ParamTypes or nil if not available
	// for one or more parameters.
	ParamNames() []string

	// ResultTypes are the results of the function.
	//
	// When WebAssembly 1.0 (20191205), there can be at most one result.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#result-types%E2%91%A0
	//
	// See ValueType documentation for encoding rules.
	ResultTypes() []ValueType
}

// Function is a WebAssembly function exported from an instantiated module (wazero.Runtime InstantiateModule).
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-func
type Function interface {
	// Definition is metadata about this function from its defining module.
	Definition() FunctionDefinition

	// Call invokes the function with parameters encoded according to ParamTypes. Up to one result is returned,
	// encoded according to ResultTypes. An error is returned for any failure looking up or invoking the function
	// including signature mismatch. When the context is nil, it defaults to context.Background.
	//
	// If Module.Close or Module.CloseWithExitCode were invoked during this call, the error returned may be a
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

	// Get returns the last known value of this global. When the context is nil, it defaults to context.Background.
	//
	// See Type for how to encode this value from a Go type.
	Get(context.Context) uint64
}

// MutableGlobal is a Global whose value can be updated at runtime (variable).
type MutableGlobal interface {
	Global

	// Set updates the value of this global. When the context is nil, it defaults to context.Background.
	//
	// See Global.Type for how to decode this value to a Go type.
	Set(ctx context.Context, v uint64)
}

// Memory allows restricted access to a module's memory. Notably, this does not allow growing.
//
// # Notes
//
//   - All functions accept a context.Context, which when nil, default to context.Background.
//   - This is an interface for decoupling, not third-party implementations. All implementations are in wazero.
//   - This includes all value types available in WebAssembly 1.0 (20191205) and all are encoded little-endian.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#storage%E2%91%A0
type Memory interface {

	// Size returns the size in bytes available. Ex. If the underlying memory has 1 page: 65536
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefsyntax-instr-memorymathsfmemorysize%E2%91%A0
	Size(context.Context) uint32

	// Grow increases memory by the delta in pages (65536 bytes per page).
	// The return val is the previous memory size in pages, or false if the
	// delta was ignored as it exceeds max memory.
	//
	// # Notes
	//
	//   - This is the same as the "memory.grow" instruction defined in the
	//	  WebAssembly Core Specification, except returns false instead of -1.
	//   - When this returns true, any shared views via Read must be refreshed.
	//
	// See MemorySizer Read and https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#grow-mem
	Grow(ctx context.Context, deltaPages uint32) (previousPages uint32, ok bool)

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
	//
	// See math.Float64bits
	ReadFloat64Le(ctx context.Context, offset uint32) (float64, bool)

	// Read reads byteCount bytes from the underlying buffer at the offset or
	// returns false if out of range.
	//
	// For example, to search for a NUL-terminated string:
	//	buf, _ = memory.Read(ctx, offset, byteCount)
	//	n := bytes.IndexByte(buf, 0)
	//	if n < 0 {
	//		// Not found!
	//	}
	//
	// Write-through
	//
	// This returns a view of the underlying memory, not a copy. This means any
	// writes to the slice returned are visible to Wasm, and any updates from
	// Wasm are visible reading the returned slice.
	//
	// For example:
	//	buf, _ = memory.Read(ctx, offset, byteCount)
	//	buf[1] = 'a' // writes through to memory, meaning Wasm code see 'a'.
	//
	// If you don't intend-write through, make a copy of the returned slice.
	//
	// When to refresh Read
	//
	// The returned slice disconnects on any capacity change. For example,
	// `buf = append(buf, 'a')` might result in a slice that is no longer
	// shared. The same exists Wasm side. For example, if Wasm changes its
	// memory capacity, ex via "memory.grow"), the host slice is no longer
	// shared. Those who need a stable view must set Wasm memory min=max, or
	// use wazero.RuntimeConfig WithMemoryCapacityPages to ensure max is always
	// allocated.
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
	//
	// See math.Float32bits
	WriteFloat32Le(ctx context.Context, offset uint32, v float32) bool

	// WriteUint64Le writes the value in little-endian encoding to the underlying buffer at the offset in or returns
	// false if out of range.
	WriteUint64Le(ctx context.Context, offset uint32, v uint64) bool

	// WriteFloat64Le writes the value in 64 IEEE 754 little-endian encoded bits to the underlying buffer at the offset
	// or returns false if out of range.
	//
	// See math.Float64bits
	WriteFloat64Le(ctx context.Context, offset uint32, v float64) bool

	// Write writes the slice to the underlying buffer at the offset or returns false if out of range.
	Write(ctx context.Context, offset uint32, v []byte) bool
}

// EncodeExternref encodes the input as a ValueTypeExternref.
//
// See DecodeExternref
func EncodeExternref(input uintptr) uint64 {
	return uint64(input)
}

// DecodeExternref decodes the input as a ValueTypeExternref.
//
// See EncodeExternref
func DecodeExternref(input uint64) uintptr {
	return uintptr(input)
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
//
// See DecodeF32
func EncodeF32(input float32) uint64 {
	return uint64(math.Float32bits(input))
}

// DecodeF32 decodes the input as a ValueTypeF32.
//
// See EncodeF32
func DecodeF32(input uint64) float32 {
	return math.Float32frombits(uint32(input))
}

// EncodeF64 encodes the input as a ValueTypeF64.
//
// See EncodeF32
func EncodeF64(input float64) uint64 {
	return math.Float64bits(input)
}

// DecodeF64 decodes the input as a ValueTypeF64.
//
// See EncodeF64
func DecodeF64(input uint64) float64 {
	return math.Float64frombits(input)
}

// MemorySizer applies during compilation after a module has been decoded from wasm, but before it is instantiated.
// This determines the amount of memory pages (65536 bytes per page) to use when a memory is instantiated as a []byte.
//
// Ex. Here's how to set the capacity to max instead of min, when set:
//
//	capIsMax := func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
//		if maxPages != nil {
//			return minPages, *maxPages, *maxPages
//		}
//		return minPages, minPages, 65536
//	}
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#grow-mem
type MemorySizer func(minPages uint32, maxPages *uint32) (min, capacity, max uint32)
