package wasm

import "unsafe"

// I31RefMask is the bit-mask that ref.i31 applies to its input.
const I31RefMask uint32 = 0x7FFFFFFF

// Tagged representation of reference values on the interpreter operand stack.
// High bits 61–63 of the uint64 slot encode the tag; Go pointers use at
// most 48–52 bits of address space, so bits 52–63 are always zero for
// real pointers.
//
//	(none)  — function pointer (typed funcref) or null (the zero slot)
//	bit 63  — i31: 31-bit value in bits 0–30, no shift required
//	bit 62  — extern-wrapped-in-anyref: value in bits 0–61
//	bit 61  — GC ref (struct / array): pointer in bits 0–60, clear
//	          bit 61 before dereferencing. The object is kept alive
//	          by ModuleInstance.GCRoots.

const (
	tagI31       uint64 = 1 << 63
	tagExternAny uint64 = 1 << 62
	tagGCRef     uint64 = 1 << 61
	tagGCArray   uint64 = 1 << 60
	tagMask      uint64 = tagI31 | tagExternAny | tagGCRef | tagGCArray
)

// TagGCStructPointer encodes a *WasmStruct pointer for the operand stack
// (bit 61 set, bit 60 clear).
func TagGCStructPointer(ptr unsafe.Pointer) uint64 {
	return uint64(uintptr(ptr)) | tagGCRef
}

// TagGCArrayPointer encodes a *WasmArray pointer for the operand stack
// (bits 61 and 60 set).
func TagGCArrayPointer(ptr unsafe.Pointer) uint64 {
	return uint64(uintptr(ptr)) | tagGCRef | tagGCArray
}

// UntagGCPointer clears tag bits and returns the raw pointer.
// Callers must check IsGCRef first.
func UntagGCPointer(v uint64) unsafe.Pointer {
	return unsafe.Pointer(uintptr(v &^ (tagGCRef | tagGCArray)))
}

// IsGCRef reports whether a slot is a tagged GC-ref pointer (struct or
// array). Bit 61 must be set while bits 62-63 must be clear.
func IsGCRef(slot uint64) bool {
	return slot&(tagI31|tagExternAny|tagGCRef) == tagGCRef
}

// IsGCStructRef reports whether a GC-ref slot is a struct (bit 60 clear).
// Callers must check IsGCRef first.
func IsGCStructRef(slot uint64) bool {
	return slot&tagGCArray == 0
}

// IsGCArrayRef reports whether a GC-ref slot is an array (bit 60 set).
// Callers must check IsGCRef first.
func IsGCArrayRef(slot uint64) bool {
	return slot&tagGCArray != 0
}

// PackI31 returns the tagged uint64 representation of an i31 value. The
// 32-bit input is narrowed to its low 31 bits per the spec for ref.i31.
// Bit 63 is set as the i31 tag; the value occupies bits 0–30.
func PackI31(v uint32) uint64 {
	return uint64(v&I31RefMask) | tagI31
}

// IsTaggedI31 reports whether a uint64 slot is an i31 ref.
func IsTaggedI31(v uint64) bool {
	return v&tagI31 != 0
}

// UnpackI31Signed extracts an i31 value as a sign-extended i32. Callers
// must verify v is a tagged i31 (via IsTaggedI31) first.
func UnpackI31Signed(v uint64) int32 {
	b := uint32(v) & I31RefMask
	if b&0x40000000 != 0 {
		return int32(b | 0x80000000)
	}
	return int32(b)
}

// UnpackI31Unsigned extracts an i31 value as a zero-extended u32.
func UnpackI31Unsigned(v uint64) uint32 {
	return uint32(v) & I31RefMask
}

// PackExternAsAny tags an externref value so it can be stored in an
// anyref slot without colliding with the i31 or GC-ref bit patterns.
// Sets bit 62; the original value is fully preserved in bits 0–61.
// Null passes through unchanged.
func PackExternAsAny(v uint64) uint64 {
	if v == 0 {
		return 0
	}
	return v | tagExternAny
}

// IsTaggedExternAsAny reports whether v was produced by PackExternAsAny.
func IsTaggedExternAsAny(v uint64) bool {
	return v != 0 && v&tagExternAny != 0
}

// UnpackExternAsAny extracts the original externref value by clearing
// bit 62.
func UnpackExternAsAny(v uint64) uint64 {
	if v == 0 {
		return 0
	}
	return v &^ tagExternAny
}
