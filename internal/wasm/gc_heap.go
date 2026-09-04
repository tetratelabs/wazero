package wasm

import "fmt"

// WasmStruct is the runtime representation of a wasm-gc `struct` instance.
//
// Per the chosen heap strategy (see RATIONALE.md "WebAssembly GC heap
// representation"), struct instances are Go-allocated headers that Go's
// runtime traces and reclaims naturally. The Fields slice holds one entry
// per declared field, ordered as in the type-section schema.
//
// Field values are stored as `any` for uniformity:
//   - Numeric fields (i32, i64, f32, f64) hold the corresponding Go
//     concrete type (int32, int64, float32, float64) for clarity in tests
//     and host code; the interpreter is free to box uint64 raw bits.
//   - Vector fields (v128) hold a [16]byte.
//   - Packed fields (i8, i16) hold a uint8 / uint16 with the narrowed
//     bits. Read instructions (struct.get_s / struct.get_u) extend at
//     read time via the SignExtendI8 / ZeroExtendI8 / SignExtendI16 /
//     ZeroExtendI16 helpers below.
//   - Reference fields hold *WasmStruct / *WasmArray / function
//     instances / host externref values (`any`), or the untyped Go nil
//     for null references. i31 values are stored as tagged uint64
//     (see PackI31 in i31.go).
type WasmStruct struct {
	// TypeID is the engine-wide canonical FunctionTypeID of this struct's
	// type, as assigned by Store.GetFunctionTypeIDs. ref.test / ref.cast
	// use this together with Store.IsSubtype.
	TypeID FunctionTypeID
	// Fields stores the field values in declaration order.
	Fields []any
}

// NewWasmStruct allocates a fresh struct of the given canonical type with
// `fieldCount` zero-valued fields. The caller is responsible for filling
// in defaults appropriate to each field's declared storage type
// (typically via struct.new_default at the interpreter level).
func NewWasmStruct(typeID FunctionTypeID, fieldCount int) *WasmStruct {
	return &WasmStruct{
		TypeID: typeID,
		Fields: make([]any, fieldCount),
	}
}

// NewWasmStructWith allocates a struct populated with the given fields.
func NewWasmStructWith(typeID FunctionTypeID, fields []any) *WasmStruct {
	return &WasmStruct{TypeID: typeID, Fields: fields}
}

// Get returns the field at index i. Returns nil for out-of-range indices —
// the validator guarantees in-range access at runtime, but the helper
// is defensive for misuse outside the interpreter.
func (s *WasmStruct) Get(i int) any {
	if s == nil || i < 0 || i >= len(s.Fields) {
		return nil
	}
	return s.Fields[i]
}

// Set assigns a value to the field at index i. Returns an error if i is
// out of range. No type-check is performed; the interpreter (with
// validator support) is responsible for ensuring the value matches the
// declared storage type.
func (s *WasmStruct) Set(i int, v any) error {
	if s == nil {
		return fmt.Errorf("struct.set on nil struct")
	}
	if i < 0 || i >= len(s.Fields) {
		return fmt.Errorf("struct.set field index %d out of range [0,%d)", i, len(s.Fields))
	}
	s.Fields[i] = v
	return nil
}

// WasmArray is the runtime representation of a wasm-gc `array` instance.
// All elements share the same declared storage type (recorded on the
// array type's schema, not on this header).
type WasmArray struct {
	// TypeID is the engine-wide canonical FunctionTypeID of this array's
	// type. Used by ref.test / ref.cast.
	TypeID FunctionTypeID
	// Elements stores the element values; element type comes from the
	// schema.
	Elements []any
}

// NewWasmArray allocates a fresh array of the given length with zero-
// valued elements. The interpreter fills in the appropriate zero value
// for the element type when array.new_default is executed.
func NewWasmArray(typeID FunctionTypeID, length uint32) *WasmArray {
	return &WasmArray{
		TypeID:   typeID,
		Elements: make([]any, length),
	}
}

// NewWasmArrayWith allocates an array populated with the given elements.
func NewWasmArrayWith(typeID FunctionTypeID, elems []any) *WasmArray {
	return &WasmArray{TypeID: typeID, Elements: elems}
}

// Len returns the number of elements. array.len reads this value.
func (a *WasmArray) Len() uint32 {
	if a == nil {
		return 0
	}
	return uint32(len(a.Elements))
}

// Get returns the element at index i. Returns nil for out-of-range
// indices; the interpreter is responsible for emitting a trap when
// array.get is called with an out-of-range index, so this helper does
// not signal the error itself.
func (a *WasmArray) Get(i uint32) any {
	if a == nil || i >= uint32(len(a.Elements)) {
		return nil
	}
	return a.Elements[i]
}

// Set assigns a value to element i. Returns an error if i is out of
// range — the caller (interpreter) is expected to translate that into
// a trap.
func (a *WasmArray) Set(i uint32, v any) error {
	if a == nil {
		return fmt.Errorf("array.set on nil array")
	}
	if i >= uint32(len(a.Elements)) {
		return fmt.Errorf("array.set index %d out of range [0,%d)", i, len(a.Elements))
	}
	a.Elements[i] = v
	return nil
}

// -----------------------------------------------------------------------
// Packed-storage helpers (i8 / i16) used by struct.get_s/u and
// array.get_s/u when the field's declared storage type is PackedTypeI8
// or PackedTypeI16.

// NarrowI8 narrows an i32 value to its low 8 bits, returning the storage
// representation used by struct.set / array.set when writing to a packed
// i8 field.
func NarrowI8(v int32) uint8 {
	return uint8(v & 0xFF)
}

// NarrowI16 narrows an i32 value to its low 16 bits, returning the storage
// representation used when writing to a packed i16 field.
func NarrowI16(v int32) uint16 {
	return uint16(v & 0xFFFF)
}

// SignExtendI8 sign-extends an 8-bit packed value to i32. Implements
// struct.get_s / array.get_s for i8 fields.
func SignExtendI8(v uint8) int32 {
	return int32(int8(v))
}

// ZeroExtendI8 zero-extends an 8-bit packed value to u32. Implements
// struct.get_u / array.get_u for i8 fields.
func ZeroExtendI8(v uint8) uint32 {
	return uint32(v)
}

// SignExtendI16 sign-extends a 16-bit packed value to i32. Implements
// struct.get_s / array.get_s for i16 fields.
func SignExtendI16(v uint16) int32 {
	return int32(int16(v))
}

// ZeroExtendI16 zero-extends a 16-bit packed value to u32. Implements
// struct.get_u / array.get_u for i16 fields.
func ZeroExtendI16(v uint16) uint32 {
	return uint32(v)
}

// DefaultFieldValue returns the zero value for a field of the given
// declared type, as used by struct.new_default and array.new_default.
//
// For numeric types the result is the Go-typed zero (int32(0), int64(0),
// etc.). For vector types it's a zero [16]byte. For packed types it's
// uint8(0) or uint16(0). For reference types — funcref / externref /
// any other ref — the result is the untyped nil (representing a null
// reference). Concrete-ref types whose nullability the schema doesn't
// permit (i.e. `(ref $t)` non-nullable) are not safe to default-construct;
// callers MUST NOT call DefaultFieldValue for those at runtime — the
// validator (Phase 4) is responsible for rejecting struct.new_default /
// array.new_default for types that contain non-nullable ref fields.
func DefaultFieldValue(f FieldType) any {
	switch f.Kind() {
	case ValueTypeI8.Kind():
		return uint8(0)
	case ValueTypeI16.Kind():
		return uint16(0)
	}
	switch f.AsImmutable() {
	case ValueTypeI32:
		return int32(0)
	case ValueTypeI64:
		return int64(0)
	case ValueTypeF32:
		return float32(0)
	case ValueTypeF64:
		return float64(0)
	case ValueTypeV128:
		return [16]byte{}
	}
	// All other shorthand types are nullable abstract references whose
	// default is the null reference (encoded as the zero uint64 by the
	// interpreter's operand-stack representation).
	return uint64(0)
}
