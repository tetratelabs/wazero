package wasm

import (
	"bytes"
	"fmt"
	"io"
)

// magic is the 4 byte preamble (literally "\0asm") of the binary format
// See https://www.w3.org/TR/wasm-core-1/#binary-magic
var magic = []byte{0x00, 0x61, 0x73, 0x6D}

// version is format version and doesn't change between known specification versions
// See https://www.w3.org/TR/wasm-core-1/#binary-version
var version = []byte{0x01, 0x00, 0x00, 0x00}

type reader struct {
	binary []byte
	read   int
	buffer *bytes.Buffer
}

func (r *reader) Read(p []byte) (n int, err error) {
	n, err = r.buffer.Read(p)
	r.read += n
	return
}

// Module is a WebAssembly binary representation.
// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A8
type Module struct {
	// TypeSection contains the unique FunctionType of functions imported or defined in this module.
	//
	// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
	TypeSection []*FunctionType
	// ImportSection contains any types, tables, memories or globals imported into this module.
	//
	// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
	ImportSection []*ImportSegment
	// FunctionSection contains the index in TypeSection of each function defined in this module.
	//
	// Function indexes are offset by any imported functions because the function index space begins with imports,
	// followed by ones defined in this module. Moreover, the FunctionSection is index correlated with the CodeSection.
	//
	// For example, if there are two imported functions and three defined in this module, we expect the CodeSection to
	// have a length of three and the function at index 3 to be defined in this module. Its type would be at
	// TypeSection[FunctionSection[0]], while its locals and body are at CodeSection[0].
	//
	// See https://www.w3.org/TR/wasm-core-1/#function-section%E2%91%A0
	FunctionSection []uint32
	// TableSection contains each table defined in this module.
	//
	// Table indexes are offset by any imported tables because the table index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported tables and one defined in this module, the
	// table at index 3 is defined in this module at TableSection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one table definition per module, so the length of
	// the TableSection can be zero or one.
	//
	// See https://www.w3.org/TR/wasm-core-1/#table-section%E2%91%A0
	TableSection []*TableType
	// MemorySection contains each memory defined in this module.
	//
	// Memory indexes are offset by any imported memories because the memory index space begins with imports, followed
	// by ones defined in this module. For example, if there are two imported memories and one defined in this module,
	// the memory at index 3 is defined in this module at MemorySection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one memory definition per module, so the length of
	// the MemorySection can be zero or one.
	//
	// See https://www.w3.org/TR/wasm-core-1/#memory-section%E2%91%A0
	MemorySection []*MemoryType
	// GlobalSection contains each global defined in this module.
	//
	// Global indexes are offset by any imported globals because the global index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported globals and three defined in this module, the
	// global at index 3 is defined in this module at GlobalSection[0].
	//
	// See https://www.w3.org/TR/wasm-core-1/#global-section%E2%91%A0
	GlobalSection []*GlobalSegment
	ExportSection map[string]*ExportSegment
	// StartSection is the index of a function to call before returning from Store.Instantiate.
	//
	// Note: The function index space begins with any ImportKindFunction in ImportSection, then the FunctionSection.
	// For example, if there are two imported functions and three defined in this module, the index space is five.
	// Note: This is a pointer to avoid conflating no start section with the valid index zero.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
	StartSection   *uint32
	ElementSection []*ElementSegment
	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	//
	// See https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	CodeSection    []*CodeSegment
	DataSection    []*DataSegment
	CustomSections map[string][]byte
}

// Encode encodes the given module into a byte slice. The result can be used directly or saved as a %.wasm file.
func (m *Module) Encode() (bytes []byte) {
	return m.encodeSections(append(magic, version...))
}

// DecodeModule decodes a `raw` module from io.Reader whose index spaces are yet to be initialized
func DecodeModule(binary []byte) (*Module, error) {
	r := &reader{binary: binary, buffer: bytes.NewBuffer(binary)}

	// Magic number.
	buf := make([]byte, 4)
	if n, err := io.ReadFull(r, buf); err != nil || n != 4 {
		return nil, ErrInvalidMagicNumber
	}
	for i := 0; i < 4; i++ {
		if buf[i] != magic[i] {
			return nil, ErrInvalidMagicNumber
		}
	}

	// Version.
	if n, err := io.ReadFull(r, buf); err != nil || n != 4 {
		return nil, ErrInvalidVersion
	}
	for i := 0; i < 4; i++ {
		if buf[i] != version[i] {
			return nil, ErrInvalidVersion
		}
	}

	ret := &Module{CustomSections: map[string][]byte{}}
	if err := ret.readSections(r); err != nil {
		return nil, fmt.Errorf("readSections failed: %w", err)
	}

	if len(ret.FunctionSection) != len(ret.CodeSection) {
		return nil, fmt.Errorf("function and code section have inconsistent lengths")
	}
	return ret, nil
}
