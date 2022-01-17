package wasm

import (
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

type FunctionType struct {
	Params, Results []ValueType
}

var nullary = []byte{0x60, 0, 0}

var encodedOneParam = buildEncodedFuncTypes(true)
var encodedOneResult = buildEncodedFuncTypes(false)

// buildEncodedFuncTypes is like buildEncodedValTypes except it is parameter or result specific and includes the func
// type prefix 0x60.
//
// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A4
func buildEncodedFuncTypes(param bool) (encodedTypes [valTypeRange][]byte) {
	if param {
		encodedTypes[ValueTypeI32-valTypeFloor] = []byte{0x60, 1, ValueTypeI32, 0}
		encodedTypes[ValueTypeI64-valTypeFloor] = []byte{0x60, 1, ValueTypeI64, 0}
		encodedTypes[ValueTypeF32-valTypeFloor] = []byte{0x60, 1, ValueTypeF32, 0}
		encodedTypes[ValueTypeF64-valTypeFloor] = []byte{0x60, 1, ValueTypeF64, 0}
		return
	}
	encodedTypes[ValueTypeI32-valTypeFloor] = []byte{0x60, 0, 1, ValueTypeI32}
	encodedTypes[ValueTypeI64-valTypeFloor] = []byte{0x60, 0, 1, ValueTypeI64}
	encodedTypes[ValueTypeF32-valTypeFloor] = []byte{0x60, 0, 1, ValueTypeF32}
	encodedTypes[ValueTypeF64-valTypeFloor] = []byte{0x60, 0, 1, ValueTypeF64}
	return
}

func (t *FunctionType) String() (ret string) {
	for _, b := range t.Params {
		ret += formatValueType(b)
	}
	if len(t.Params) == 0 {
		ret += "null"
	}
	ret += "_"
	for _, b := range t.Results {
		ret += formatValueType(b)
	}
	if len(t.Results) == 0 {
		ret += "null"
	}
	return
}

// encode returns a byte slice in WebAssembly 1.0 (MVP) Binary Format.
//
// Note: Function types are encoded by the byte 0x60 followed by the respective vectors of parameter and result types.
// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A4
func (t *FunctionType) encode() []byte {
	paramCount, resultCount := len(t.Params), len(t.Results)
	if paramCount == 0 && resultCount == 0 {
		return nullary
	}
	if resultCount == 0 {
		if paramCount == 1 {
			return encodedOneParam[t.Params[0]-valTypeFloor]
		}
		return append(append([]byte{0x60}, encodeValTypes(t.Params)...), 0)
	} else if resultCount == 1 {
		if paramCount == 0 {
			return encodedOneResult[t.Results[0]-valTypeFloor]
		}
		return append(append([]byte{0x60}, encodeValTypes(t.Params)...), 1, t.Results[0])
	}
	// This branch should never be reaches as WebAssembly 1.0 (MVP) supports at most 1 result
	data := append([]byte{0x60}, encodeValTypes(t.Params)...)
	return append(data, encodeValTypes(t.Results)...)
}

func readFunctionType(r io.Reader) (*FunctionType, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read leading byte: %w", err)
	}

	if b[0] != 0x60 {
		return nil, fmt.Errorf("%w: %#x != 0x60", ErrInvalidByte, b[0])
	}

	s, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size of input value types: %w", err)
	}

	paramTypes, err := readValueTypes(r, s)
	if err != nil {
		return nil, fmt.Errorf("read value types of inputs: %w", err)
	}

	s, _, err = leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size of output value types: %w", err)
	} else if s > 1 {
		return nil, fmt.Errorf("multi value results not supported")
	}

	resultTypes, err := readValueTypes(r, s)
	if err != nil {
		return nil, fmt.Errorf("read value types of outputs: %w", err)
	}

	return &FunctionType{
		Params:  paramTypes,
		Results: resultTypes,
	}, nil
}

type LimitsType struct {
	Min uint32
	Max *uint32
}

func readLimitsType(r io.Reader) (*LimitsType, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read leading byte: %v", err)
	}

	ret := &LimitsType{}
	switch b[0] {
	case 0x00:
		ret.Min, _, err = leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read min of limit: %v", err)
		}
	case 0x01:
		ret.Min, _, err = leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read min of limit: %v", err)
		}
		m, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read min of limit: %v", err)
		}
		ret.Max = &m
	default:
		return nil, fmt.Errorf("%v for limits: %#x != 0x00 or 0x01", ErrInvalidByte, b[0])
	}
	return ret, nil
}

type TableType struct {
	ElemType byte
	Limit    *LimitsType
}

func readTableType(r io.Reader) (*TableType, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read leading byte: %v", err)
	}

	if b[0] != 0x70 {
		return nil, fmt.Errorf("%w: invalid element type %#x != %#x", ErrInvalidByte, b[0], 0x70)
	}

	lm, err := readLimitsType(r)
	if err != nil {
		return nil, fmt.Errorf("read limits: %v", err)
	}

	return &TableType{
		ElemType: 0x70, // funcref
		Limit:    lm,
	}, nil
}

type MemoryType = LimitsType

func readMemoryType(r io.Reader) (*MemoryType, error) {
	ret, err := readLimitsType(r)
	if err != nil {
		return nil, err
	}
	if ret.Min > uint32(PageSize) {
		return nil, fmt.Errorf("memory min must be at most 65536 pages (4GiB)")
	}
	if ret.Max != nil {
		if *ret.Max < ret.Min {
			return nil, fmt.Errorf("memory size minimum must not be greater than maximum")
		} else if *ret.Max > uint32(PageSize) {
			return nil, fmt.Errorf("memory max must be at most 65536 pages (4GiB)")
		}
	}
	return ret, nil
}

type GlobalType struct {
	ValType ValueType
	Mutable bool
}

func readGlobalType(r io.Reader) (*GlobalType, error) {
	vt, err := readValueTypes(r, 1)
	if err != nil {
		return nil, fmt.Errorf("read value type: %w", err)
	}

	ret := &GlobalType{
		ValType: vt[0],
	}

	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read mutablity: %w", err)
	}

	switch mut := b[0]; mut {
	case 0x00:
	case 0x01:
		ret.Mutable = true
	default:
		return nil, fmt.Errorf("%w for mutability: %#x != 0x00 or 0x01", ErrInvalidByte, mut)
	}
	return ret, nil
}
