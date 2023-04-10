package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// subsectionIDModuleName contains only the module name.
	subsectionIDModuleName = uint8(0)
	// subsectionIDFunctionNames is a map of indices to function names, in ascending order by function index
	subsectionIDFunctionNames = uint8(1)
	// subsectionIDLocalNames contain a map of function indices to a map of local indices to their names, in ascending
	// order by function and local index
	subsectionIDLocalNames = uint8(2)
)

// EncodeNameSectionData serializes the data for the "name" key in wasm.SectionIDCustom according to the
// standard:
//
// Note: The result can be nil because this does not encode empty subsections
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-namesec
func EncodeNameSectionData(n *wasm.NameSection) (data []byte) {
	if n.ModuleName != "" {
		data = append(data, encodeNameSubsection(subsectionIDModuleName, encodeSizePrefixed([]byte(n.ModuleName)))...)
	}
	if fd := encodeFunctionNameData(n); len(fd) > 0 {
		data = append(data, encodeNameSubsection(subsectionIDFunctionNames, fd)...)
	}
	if ld := encodeLocalNameData(n); len(ld) > 0 {
		data = append(data, encodeNameSubsection(subsectionIDLocalNames, ld)...)
	}
	return
}

// encodeFunctionNameData encodes the data for the function name subsection.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-funcnamesec
func encodeFunctionNameData(n *wasm.NameSection) []byte {
	if len(n.FunctionNames) == 0 {
		return nil
	}

	return encodeNameMap(n.FunctionNames)
}

func encodeNameMap(m wasm.NameMap) []byte {
	count := uint32(len(m))
	data := leb128.EncodeUint32(count)
	for _, na := range m {
		data = append(data, encodeNameAssoc(na)...)
	}
	return data
}

// encodeLocalNameData encodes the data for the local name subsection.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-localnamesec
func encodeLocalNameData(n *wasm.NameSection) []byte {
	if len(n.LocalNames) == 0 {
		return nil
	}

	funcNameCount := uint32(len(n.LocalNames))
	subsection := leb128.EncodeUint32(funcNameCount)

	for _, na := range n.LocalNames {
		locals := encodeNameMap(na.NameMap)
		subsection = append(subsection, append(leb128.EncodeUint32(na.Index), locals...)...)
	}
	return subsection
}

// encodeNameSubsection returns a buffer encoding the given subsection
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#subsections%E2%91%A0
func encodeNameSubsection(subsectionID uint8, content []byte) []byte {
	contentSizeInBytes := leb128.EncodeUint32(uint32(len(content)))
	result := []byte{subsectionID}
	result = append(result, contentSizeInBytes...)
	result = append(result, content...)
	return result
}

// encodeNameAssoc encodes the index and data prefixed by their size.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-namemap
func encodeNameAssoc(na wasm.NameAssoc) []byte {
	return append(leb128.EncodeUint32(na.Index), encodeSizePrefixed([]byte(na.Name))...)
}

// encodeSizePrefixed encodes the data prefixed by their size.
func encodeSizePrefixed(data []byte) []byte {
	size := leb128.EncodeUint32(uint32(len(data)))
	return append(size, data...)
}
