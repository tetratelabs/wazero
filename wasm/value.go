package wasm

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

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
	// valTypeRange allows us to cache common valueType calculations. To determine the array offset, subtract
	// valTypeFloor from the value of interest.
	valTypeRange = ValueTypeI32 - valTypeFloor + 1
	// valTypeFloor is the lowest numeric value of a value type.
	//
	// Note: the floor value changes after WebAssembly 1.0 (MVP)
	valTypeFloor = ValueTypeF64
)

func formatValueType(t ValueType) (ret string) {
	switch t {
	case ValueTypeI32:
		ret = "i32"
	case ValueTypeI64:
		ret = "i64"
	case ValueTypeF32:
		ret = "f32"
	case ValueTypeF64:
		ret = "f64"
	}
	return
}

var noValType = []byte{0}

// encodedValTypes is a cache of size prefixed binary encoding of known val types.
var encodedValTypes = buildEncodedValTypes()

// buildEncodedValTypes builds results for encodeValTypes for known value types.
//
// Note: this is length ValueTypeI32+1 because the largest known val type is that.
func buildEncodedValTypes() (encodedTypes [valTypeRange][]byte) {
	encodedTypes[ValueTypeI32-valTypeFloor] = []byte{1, ValueTypeI32}
	encodedTypes[ValueTypeI64-valTypeFloor] = []byte{1, ValueTypeI64}
	encodedTypes[ValueTypeF32-valTypeFloor] = []byte{1, ValueTypeF32}
	encodedTypes[ValueTypeF64-valTypeFloor] = []byte{1, ValueTypeF64}
	return
}

// encodeValTypes fast paths binary encoding of common value type lengths
func encodeValTypes(vt []ValueType) []byte {
	// Special case nullary and parameter lengths of wasi_snapshot_preview1 to avoid excess allocations
	switch uint32(len(vt)) {
	case 0: // nullary
		return noValType
	case 1: // ex $wasi_snapshot_preview1.fd_close or any result
		return encodedValTypes[vt[0]-valTypeFloor]
	case 2: // ex $wasi_snapshot_preview1.environ_sizes_get
		return []byte{2, vt[0], vt[1]}
	case 4: // ex $wasi_snapshot_preview1.fd_write
		return []byte{4, vt[0], vt[1], vt[2], vt[3]}
	case 9: // ex $wasi_snapshot_preview1.fd_write
		return []byte{9, vt[0], vt[1], vt[2], vt[3], vt[4], vt[5], vt[6], vt[7], vt[8]}
	default: // Slow path others until someone complains with a valid signature
		count := leb128.EncodeUint32(uint32(len(vt)))
		return append(count, vt...)
	}
}

func readValueTypes(r io.Reader, num uint32) ([]ValueType, error) {
	ret := make([]ValueType, num)
	buf := make([]ValueType, num)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	for i, v := range buf {
		switch v {
		case ValueTypeI32, ValueTypeF32, ValueTypeI64, ValueTypeF64:
			ret[i] = v
		default:
			return nil, fmt.Errorf("invalid value type: %d", v)
		}
	}
	return ret, nil
}

func readNameValue(r io.Reader) (string, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return "", fmt.Errorf("read size of name: %v", err)
	}

	buf := make([]byte, vs)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read bytes of name: %v", err)
	}

	if !utf8.Valid(buf) {
		return "", fmt.Errorf("name must be valid as utf8")
	}

	return string(buf), nil
}

func ValueTypesEqual(a []ValueType, b []ValueType) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
