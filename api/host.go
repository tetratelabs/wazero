// Package api only includes constants and interfaces. This ensures no package cycles in implementation.
package api

import "context"

// HostFunctionCallContext is the first argument of all host functions.
type HostFunctionCallContext interface {
	// Context returns the host call's context.
	//
	// The returned context is always non-nil; it defaults to the background context.
	Context() context.Context

	// Memory allows restricted access to an importing module's memory during a host function call.
	Memory() Memory
}

// Memory allows restricted access to a module's memory. Notably, this does not allow growing.
//
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
