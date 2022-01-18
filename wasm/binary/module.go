package binary

// magic is the 4 byte preamble (literally "\0asm") of the binary format
// See https://www.w3.org/TR/wasm-core-1/#binary-magic
var magic = []byte{0x00, 0x61, 0x73, 0x6D}

// version is format version and doesn't change between known specification versions
// See https://www.w3.org/TR/wasm-core-1/#binary-version
var version = []byte{0x01, 0x00, 0x00, 0x00}

type SectionID = byte

const (
	// SectionIDCustom includes the standard defined NameSection and possibly others not defined in the standard.
	SectionIDCustom   SectionID = 0
	SectionIDType     SectionID = 1
	SectionIDImport   SectionID = 2
	SectionIDFunction SectionID = 3
	SectionIDTable    SectionID = 4
	SectionIDMemory   SectionID = 5
	SectionIDGlobal   SectionID = 6
	SectionIDExport   SectionID = 7
	SectionIDStart    SectionID = 8
	SectionIDElement  SectionID = 9
	SectionIDCode     SectionID = 10
	SectionIDData     SectionID = 11
)
