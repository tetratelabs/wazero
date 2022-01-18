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
//
// Differences from the specification:
// * The NameSection is decoded, so not present as a key "name" in CustomSections.
type Module struct {
	// TypeSection contains the unique FunctionType of functions imported or defined in this module.
	//
	// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
	TypeSection []*FunctionType
	// ImportSection contains imported functions, tables, memories or globals required for instantiation
	// (Store.Instantiate).
	//
	// Note: there are no unique constraints relating to the two-level namespace of Import.Module and Import.Name.
	//
	// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
	ImportSection []*Import
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
	GlobalSection []*Global
	ExportSection map[string]*Export
	// StartSection is the index of a function to call before returning from Store.Instantiate.
	//
	// Note: The function index space begins with any ImportKindFunc in ImportSection, then the FunctionSection.
	// For example, if there are two imported functions and three defined in this module, the index space is five.
	// Note: This is a pointer to avoid conflating no start section with the valid index zero.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
	StartSection   *uint32
	ElementSection []*ElementSegment
	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	//
	// See https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	CodeSection []*Code
	DataSection []*DataSegment
	// NameSection is set when the custom section "name" was successfully decoded from the binary format.
	//
	// Note: This is the only custom section defined in the WebAssembly 1.0 (MVP) Binary Format. Others are in
	// CustomSections
	//
	// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
	NameSection *NameSection
	// CustomSections is set when at least one non-standard, or otherwise unsupported custom section was found in the
	// binary format.
	//
	// Note: This never contains a "name" because that is standard and parsed into the NameSection.
	//
	// See https://www.w3.org/TR/wasm-core-1/#custom-section%E2%91%A0
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

	ret := &Module{}
	if err := ret.readSections(r); err != nil {
		return nil, fmt.Errorf("readSections failed: %w", err)
	}

	if len(ret.FunctionSection) != len(ret.CodeSection) {
		return nil, fmt.Errorf("function and code section have inconsistent lengths")
	}
	return ret, nil
}
