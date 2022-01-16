package wasm

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

// CustomNameSection represent the known custom name subsections defined in the WebAssembly Binary Format
// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
// See https://github.com/tetratelabs/wazero/issues/134 about adding this to Module
type CustomNameSection struct {
	// ModuleName is the symbolic identifier for a module. Ex. math
	ModuleName string
	// FunctionNames is an association of a function index to its symbolic identifier. Ex. add
	//
	// * the key (idx) is in the function namespace, where module defined functions are preceded by imported ones.
	//
	// Ex. Assuming the below text format is the second import, you would expect FunctionNames[1] = "mul"
	//	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	//
	// Note: FunctionNames are a map because the specification requires function indices to be unique. These are sorted
	// during EncodeData
	FunctionNames map[uint32]string

	// LocalNames is an association of a function index to any locals which have a symbolic identifier. Ex. add x
	//
	// * the key (funcIndex) is in the function namespace, where module defined functions are preceded by imported ones.
	// * the second key (idx) is in the local namespace, where parameters precede any function locals.
	//
	// Ex. Assuming the below text format is the second import, you would expect LocalNames[1][1] = "y"
	//	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	//
	// Note: LocalNames are a map because the specification requires both function and local indices to be unique. These
	// are sorted during EncodeData
	LocalNames map[uint32]map[uint32]string
}

// EncodeData serializes the data associated with the "name" key in SectionIDCustom according to the standard:
//
// * ModuleName encode as subsection 0
// * FunctionNames encode as subsection 1 in ascending order by function index
// * LocalNames encode as subsection 2 in ascending order by function and local index
//
// Note: The result can be nil because this does not encode empty subsections
//
// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
func (n *CustomNameSection) EncodeData() (data []byte) {
	if n.ModuleName != "" {
		data = append(data, encodeNameSubsection(uint8(0), encodeSizePrefixed([]byte(n.ModuleName)))...)
	}
	if fd := n.encodeFunctionNameData(); len(fd) > 0 {
		data = append(data, encodeNameSubsection(uint8(1), fd)...)
	}
	if ld := n.encodeLocalNameData(); len(ld) > 0 {
		data = append(data, encodeNameSubsection(uint8(2), ld)...)
	}
	return
}

// encodeFunctionNameData encodes the data for the function name subsection.
// See https://www.w3.org/TR/wasm-core-1/#binary-funcnamesec
func (n *CustomNameSection) encodeFunctionNameData() []byte {
	if len(n.FunctionNames) == 0 {
		return nil
	}
	return encodeSortedAndSizePrefixed(n.FunctionNames)
}

func encodeSortedAndSizePrefixed(m map[uint32]string) []byte {
	count := uint32(len(m))
	data := leb128.EncodeUint32(count)

	// Sort the keys so that they encode in ascending order
	keys := make(uint32Slice, 0, count)
	for k := range m {
		keys = append(keys, k)
	}
	sort.Sort(keys)

	for _, i := range keys {
		data = append(data, encodeNameMapEntry(i, []byte(m[i]))...)
	}
	return data
}

// encodeLocalNameData encodes the data for the local name subsection.
// See https://www.w3.org/TR/wasm-core-1/#binary-localnamesec
func (n *CustomNameSection) encodeLocalNameData() []byte {
	if len(n.LocalNames) == 0 {
		return nil
	}

	funcNameCount := uint32(len(n.LocalNames))
	subsection := leb128.EncodeUint32(funcNameCount)

	// Sort the function indices so that they encode in ascending order
	funcIndex := make(uint32Slice, 0, funcNameCount)
	for k := range n.LocalNames {
		funcIndex = append(funcIndex, k)
	}
	sort.Sort(funcIndex)

	for _, i := range funcIndex {
		locals := encodeSortedAndSizePrefixed(n.LocalNames[i])
		subsection = append(subsection, append(leb128.EncodeUint32(i), locals...)...)
	}
	return subsection
}

// encodeNameSubsection returns a buffer encoding the given subsection
// See https://www.w3.org/TR/wasm-core-1/#subsections%E2%91%A0
func encodeNameSubsection(subsectionID uint8, content []byte) []byte {
	contentSizeInBytes := leb128.EncodeUint32(uint32(len(content)))
	result := []byte{subsectionID}
	result = append(result, contentSizeInBytes...)
	result = append(result, content...)
	return result
}

// encodeNameMapEntry encodes the index and data prefixed by their size.
// See https://www.w3.org/TR/wasm-core-1/#binary-namemap
func encodeNameMapEntry(i uint32, data []byte) []byte {
	return append(leb128.EncodeUint32(i), encodeSizePrefixed(data)...)
}

// encodeSizePrefixed encodes the data prefixed by their size.
func encodeSizePrefixed(data []byte) []byte {
	size := leb128.EncodeUint32(uint32(len(data)))
	return append(size, data...)
}

// uint32Slice implements sort.Interface
type uint32Slice []uint32

func (x uint32Slice) Len() int           { return len(x) }
func (x uint32Slice) Less(i, j int) bool { return x[i] < x[j] }
func (x uint32Slice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// DecodeCustomNameSection deserializes the data associated with the "name" key in SectionIDCustom according to the
// standard:
//
// * ModuleName decode from subsection 0
// * FunctionNames decode from subsection 1
// * LocalNames decode from subsection 2
//
// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
func DecodeCustomNameSection(data []byte) (*CustomNameSection, error) {
	r := bytes.NewReader(data)
	for {
		id, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read subsection ID: %w", err)
		}

		size, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read the size of subsection %d: %w", id, err)
		}

		if id == 1 { // the function name subsection.
			break
		} else { // TODO: ModuleName and LocalNames!
			// Skip other subsections.
			_, err := r.Seek(int64(size), io.SeekCurrent)
			if err != nil {
				return nil, fmt.Errorf("failed to skip subsection %d: %w", id, err)
			}
		}
	}

	nameVectorSize, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read the size of name vector: %w", err)
	}

	funcNames := make(map[uint32]string, nameVectorSize)
	for i := uint32(0); i < nameVectorSize; i++ {
		functionIndex, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read function index: %w", err)
		}

		functionNameSize, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read function name size: %w", err)
		}

		namebuf := make([]byte, functionNameSize)
		_, err = io.ReadFull(r, namebuf)
		if err != nil {
			return nil, fmt.Errorf("failed to read function name: %w", err)
		}
		funcNames[functionIndex] = string(namebuf)
	}

	return &CustomNameSection{FunctionNames: funcNames}, nil
}
