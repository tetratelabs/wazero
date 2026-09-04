package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeFunctionType(enabledFeatures api.CoreFeatures, r *bytes.Reader, ret *wasm.FunctionType) (err error) {
	if err := decodeSubType(enabledFeatures, r, ret); err != nil {
		return err
	}

	// cache the key for the function type
	_ = ret.String()

	return nil
}

// decodeSubType reads a sub-type entry from the type section, including
// optional supertype declarations and the composite body. Sub-type forms:
//
//	0x4F (sub final) <supertypes> <composite>
//	0x50 (sub)       <supertypes> <composite>
//	0x60 (func shorthand)  -> implicit sub final, zero supertypes
//	0x5F (struct shorthand) -> implicit sub final, zero supertypes
//	0x5E (array shorthand)  -> implicit sub final, zero supertypes
func decodeSubType(enabledFeatures api.CoreFeatures, r *bytes.Reader, ret *wasm.FunctionType) error {
	b, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read leading byte: %w", err)
	}

	switch b {
	case 0x4F: // sub final
		if err := decodeSuperTypes(r, ret); err != nil {
			return err
		}
		return decodeCompositeForm(enabledFeatures, r, ret)

	case 0x50: // sub (non-final)
		if err := decodeSuperTypes(r, ret); err != nil {
			return err
		}
		ret.Open = true
		return decodeCompositeForm(enabledFeatures, r, ret)

	case 0x60, 0x5F, 0x5E:
		// Shorthand: per spec, a bare composite type desugars to
		// (sub final () comptype) — implicit final, no super list.
		if err := r.UnreadByte(); err != nil {
			return err
		}
		return decodeCompositeForm(enabledFeatures, r, ret)
	}
	return fmt.Errorf("%w: %#x", ErrInvalidByte, b)
}

// decodeSuperTypes reads the vec(typeidx) of declared supertypes. The MVP
// permits at most one entry; multiple entries are rejected.
func decodeSuperTypes(r *bytes.Reader, ret *wasm.FunctionType) error {
	n, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("read supertype count: %w", err)
	}
	if n > 1 {
		return fmt.Errorf("at most one supertype allowed, got %d", n)
	}
	if n == 1 {
		idx, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("read supertype index: %w", err)
		}
		ret.SuperTypeIndex = &idx
	}
	return nil
}

// decodeCompositeForm reads the composite-type body, dispatching on the
// form byte: 0x60 (func), 0x5F (struct), 0x5E (array).
func decodeCompositeForm(enabledFeatures api.CoreFeatures, r *bytes.Reader, ret *wasm.FunctionType) error {
	b, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read composite form byte: %w", err)
	}
	switch b {
	case 0x60:
		ret.Form = wasm.CompositeFormFunc
		return decodeFuncBody(enabledFeatures, r, ret)
	case 0x5F:
		ret.Form = wasm.CompositeFormStruct
		return decodeStructBody(r, ret)
	case 0x5E:
		ret.Form = wasm.CompositeFormArray
		return decodeArrayBody(r, ret)
	}
	return fmt.Errorf("invalid composite form byte: %#x", b)
}

// decodeFuncBody reads a func type body — the param vector and result vector.
// The leading 0x60 byte has already been consumed.
func decodeFuncBody(enabledFeatures api.CoreFeatures, r *bytes.Reader, ret *wasm.FunctionType) error {
	paramCount, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("could not read parameter count: %w", err)
	}

	paramTypes, err := decodeValueTypes(r, paramCount)
	if err != nil {
		return fmt.Errorf("could not read parameter types: %w", err)
	}

	resultCount, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("could not read result count: %w", err)
	}

	if resultCount > 1 {
		if err = enabledFeatures.RequireEnabled(api.CoreFeatureMultiValue); err != nil {
			return fmt.Errorf("multiple result types invalid as %v", err)
		}
	}

	resultTypes, err := decodeValueTypes(r, resultCount)
	if err != nil {
		return fmt.Errorf("could not read result types: %w", err)
	}

	ret.Params = paramTypes
	ret.Results = resultTypes
	return nil
}

// decodeStructBody reads a struct type body — a vec(fieldtype). The leading
// 0x5F byte has already been consumed.
func decodeStructBody(r *bytes.Reader, ret *wasm.FunctionType) error {
	fieldCount, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("read struct field count: %w", err)
	}
	if fieldCount == 0 {
		ret.Fields = nil
		return nil
	}
	fields := make([]wasm.FieldType, fieldCount)
	for i := uint32(0); i < fieldCount; i++ {
		ft, err := decodeFieldType(r)
		if err != nil {
			return fmt.Errorf("read struct field[%d]: %w", i, err)
		}
		fields[i] = ft
	}
	ret.Fields = fields
	return nil
}

// decodeArrayBody reads an array type body — a single fieldtype. The leading
// 0x5E byte has already been consumed.
func decodeArrayBody(r *bytes.Reader, ret *wasm.FunctionType) error {
	ft, err := decodeFieldType(r)
	if err != nil {
		return fmt.Errorf("read array element field: %w", err)
	}
	ret.ArrayField = ft
	return nil
}

// decodeFieldType reads a field type: a storage type followed by a
// mutability byte.
func decodeFieldType(r *bytes.Reader) (wasm.FieldType, error) {
	vt, err := decodeStorageType(r)
	if err != nil {
		return 0, err
	}
	mut, err := r.ReadByte()
	if err != nil {
		return 0, fmt.Errorf("read mutability byte: %w", err)
	}
	switch mut {
	case 0x00:
	case 0x01:
		vt = vt.AsMutable()
	default:
		return 0, fmt.Errorf("invalid mutability byte: %#x", mut)
	}
	return vt, nil
}

// decodeStorageType reads a storage type. A storage type is either
// a packed type (0x78 = i8, 0x77 = i16) or any regular value type.
func decodeStorageType(r *bytes.Reader) (wasm.ValueType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, fmt.Errorf("read storage type byte: %w", err)
	}
	switch b {
	case wasm.ValueTypeI8.Kind():
		return wasm.ValueTypeI8, nil
	case wasm.ValueTypeI16.Kind():
		return wasm.ValueTypeI16, nil
	}
	if err := r.UnreadByte(); err != nil {
		return 0, err
	}
	vts, err := decodeValueTypes(r, 1)
	if err != nil {
		return 0, fmt.Errorf("decode field value-type: %w", err)
	}
	return vts[0], nil
}
