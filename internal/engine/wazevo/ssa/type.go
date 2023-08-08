package ssa

type Type byte

const (
	typeInvalid Type = 1 + iota

	// TODO: add 8, 16 bit types when it's needed for optimizations.

	// TypeI32 represents an integer type with 32 bits.
	TypeI32

	// TypeI64 represents an integer type with 64 bits.
	TypeI64

	// TypeF32 represents 32-bit floats in the IEEE 754.
	TypeF32

	// TypeF64 represents 64-bit floats in the IEEE 754.
	TypeF64

	// TODO: SIMD, ref types!
)

// String implements fmt.Stringer.
func (t Type) String() (ret string) {
	switch t {
	case typeInvalid:
		return "invalid"
	case TypeI32:
		return "i32"
	case TypeI64:
		return "i64"
	case TypeF32:
		return "f32"
	case TypeF64:
		return "f64"
	default:
		panic(int(t))
	}
}

// IsInt returns true if the type is an integer type.
func (t Type) IsInt() bool {
	return t == TypeI32 || t == TypeI64
}

// Bits returns the number of bits required to represent the type.
func (t Type) Bits() byte {
	switch t {
	case TypeI32, TypeF32:
		return 32
	case TypeI64, TypeF64:
		return 64
	default:
		panic(int(t))
	}
}

// Size returns the number of bytes required to represent the type.
func (t Type) Size() byte {
	return t.Bits() / 8
}

func (t Type) invalid() bool {
	return t == typeInvalid
}
