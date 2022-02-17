package wasm

import (
	"context"
)

// SectionID identifies the sections of a Module in the WebAssembly 1.0 (MVP) Binary Format.
//
// Note: these are defined in the wasm package, instead of the binary package, as a key per section is needed regardless
// of format, and deferring to the binary type avoids confusion.
//
// See https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
type SectionID = byte

const (
	// SectionIDCustom includes the standard defined NameSection and possibly others not defined in the standard.
	SectionIDCustom SectionID = iota // don't add anything not in https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
	SectionIDType
	SectionIDImport
	SectionIDFunction
	SectionIDTable
	SectionIDMemory
	SectionIDGlobal
	SectionIDExport
	SectionIDStart
	SectionIDElement
	SectionIDCode
	SectionIDData
)

// SectionIDName returns the canonical name of a module section.
// https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
func SectionIDName(sectionID SectionID) string {
	switch sectionID {
	case SectionIDCustom:
		return "custom"
	case SectionIDType:
		return "type"
	case SectionIDImport:
		return "import"
	case SectionIDFunction:
		return "function"
	case SectionIDTable:
		return "table"
	case SectionIDMemory:
		return "memory"
	case SectionIDGlobal:
		return "global"
	case SectionIDExport:
		return "export"
	case SectionIDStart:
		return "start"
	case SectionIDElement:
		return "element"
	case SectionIDCode:
		return "code"
	case SectionIDData:
		return "data"
	}
	return "unknown"
}

// ValueType is the binary encoding of a type such as i32
// See https://www.w3.org/TR/wasm-core-1/#binary-valtype
//
// Note: This is a type alias as it is easier to encode and decode in the binary format.
type ValueType = byte

const (
	ValueTypeI32 ValueType = 0x7f
	ValueTypeI64 ValueType = 0x7e
	ValueTypeF32 ValueType = 0x7d
	ValueTypeF64 ValueType = 0x7c
)

// ValueTypeName returns the type name of the given ValueType as a string.
// These type names match the names used in the WebAssembly text format.
// Note that ValueTypeName returns "unknown", if an undefined ValueType value is passed.
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

// ImportKind indicates which import description is present
// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
type ImportKind = byte

const (
	ImportKindFunc   ImportKind = 0x00
	ImportKindTable  ImportKind = 0x01
	ImportKindMemory ImportKind = 0x02
	ImportKindGlobal ImportKind = 0x03
)

// ExportKind indicates which index Export.Index points to
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
type ExportKind = byte

const (
	ExportKindFunc   ExportKind = 0x00
	ExportKindTable  ExportKind = 0x01
	ExportKindMemory ExportKind = 0x02
	ExportKindGlobal ExportKind = 0x03
)

// ExportKindName returns the canonical name of the exportdesc.
// https://www.w3.org/TR/wasm-core-1/#syntax-exportdesc
func ExportKindName(ek ExportKind) string {
	switch ek {
	case ExportKindFunc:
		return "func"
	case ExportKindTable:
		return "table"
	case ExportKindMemory:
		return "mem"
	case ExportKindGlobal:
		return "global"
	}
	return "unknown"
}

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
