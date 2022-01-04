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
	return append(leb128.EncodeUint32(i), encodeName(name)...)
}

// encodeName encodes the name prefixed by its size, stripping any leading '$'
//
// The WebAssembly 1.0 specification includes support for naming modules, functions, locals and tables via the custom
// 'name' section: https://www.w3.org/TR/wasm-core-1/#binary-namesec However, how this round-trips between the text and
// binary format is not discussed.
//
// We know that in the text format names must be dollar-sign prefixed to conform with tokenID conventions. However, we
// don't know if the user's intent was a dollar-sign or not. For example, a function written in a higher level language,
// targeting the binary format may end up prefixed with '$' for other reasons.
//
// This round-tripping concern materializes when a function written in the text format is transpiled into the binary
// format (ex via `wat2wasm --debug-names`). For example, if a module name was encoded literally in the binary custom
// 'name' section as "$Math", wabt tools would prefix it again, resulting in "$$Math".
// https://github.com/WebAssembly/wabt/blob/e59cf9369004a521814222afbc05ae6b59446cd5/src/binary-reader-ir.cc#L1279
//
// Until the standard clarifies round-tripping concerns between the text and binary format, we chop off the leading '$'
// when reading any names from the text format. This prevents awkwardness while wabt tools are in use.
func encodeName(name string) []byte {
	nameBytes := []byte(name[1:])
	nameSize := leb128.EncodeUint32(uint32(len(nameBytes)))
	content := append(nameSize, nameBytes...)
	return content
}
