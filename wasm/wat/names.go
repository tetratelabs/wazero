package wat

import (
	"github.com/tetratelabs/wazero/wasm/leb128"
)

// encodeNameSection encodes a possibly empty buffer representing the "name" wasm.Module CustomSection.
// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
func encodeNameSection(m *module) []byte {
	funcNameCount := uint32(0)
	var funcNameEntries []byte
	for _, f := range m.importFuncs {
		if f.funcName != "" {
			funcNameCount = funcNameCount + 1
			funcNameEntries = append(funcNameEntries, encodeNameMapEntry(uint32(f.importIndex), f.funcName)...)
		}
	}
	var nameSection []byte
	if m.name != "" {
		// See https://www.w3.org/TR/wasm-core-1/#binary-modulenamesec
		nameSection = append(nameSection, encodeNameSubsection(uint8(0), encodeName(m.name))...)
	}
	if funcNameCount > 0 {
		// See https://www.w3.org/TR/wasm-core-1/#binary-funcnamesec
		content := leb128.EncodeUint32(funcNameCount)
		content = append(content, funcNameEntries...)
		nameSection = append(nameSection, encodeNameSubsection(uint8(1), content)...)
	}
	return nameSection
}

// This returns a buffer encoding the given subsection
// See https://www.w3.org/TR/wasm-core-1/#subsections%E2%91%A0
func encodeNameSubsection(subsectionID uint8, content []byte) []byte {
	contentSizeInBytes := leb128.EncodeUint32(uint32(len(content)))
	result := []byte{subsectionID}
	result = append(result, contentSizeInBytes...)
	result = append(result, content...)
	return result
}

// encodeNameMapEntry encodes the index and name prefixed by their size.
// See https://www.w3.org/TR/wasm-core-1/#binary-namemap
func encodeNameMapEntry(i uint32, name string) []byte {
	indexBytes := leb128.EncodeUint32(i)
	nameBytes := []byte(name)
	nameSize := leb128.EncodeUint32(uint32(len(nameBytes)))

	content := append(indexBytes, nameSize...)
	content = append(content, nameBytes...)
	return content
}

// encodeName encodes the name prefixed by its size
func encodeName(name string) []byte {
	nameBytes := []byte(name)
	nameSize := leb128.EncodeUint32(uint32(len(nameBytes)))
	content := append(nameSize, nameBytes...)
	return content
}
