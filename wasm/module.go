package wasm

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

const (
	magic   = "\x00asm"
	version = "\x01\x00\x00\x00"
)

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
	// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
	TypeSection []*FunctionType
	// ImportSection contains any types, tables, memories or globals imported into this module.
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
	StartSection   *uint32
	ElementSection []*ElementSegment
	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	// See https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	CodeSection    []*CodeSegment
	DataSection    []*DataSegment
	CustomSections map[string][]byte
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

func (m *Module) DecodeCustomNameSection() (map[uint32]string, error) {
	namesec, ok := m.CustomSections["name"]
	if !ok {
		return nil, fmt.Errorf("'name' %w", ErrCustomSectionNotFound)
	}

	r := bytes.NewReader(namesec)
	for {
		id, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read subsection ID: %w", err)
		}

		size, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read the size of subsection %d: %w", id, err)
		}

		if id == 1 {
			// ID = 1 is the function name subsection.
			break
		} else {
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

	ret := make(map[uint32]string, nameVectorSize)
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
		ret[functionIndex] = string(namebuf)
	}

	return ret, nil
}
