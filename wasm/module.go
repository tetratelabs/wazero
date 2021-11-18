package wasm

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

var (
	magic   = []byte{0x00, 0x61, 0x73, 0x6D}
	version = []byte{0x01, 0x00, 0x00, 0x00}
)

type Reader struct {
	binary []byte
	read   int
	buffer *bytes.Buffer
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.buffer.Read(p)
	r.read += n
	return
}

var _ io.Reader = &Reader{}

type (
	// Static binary representations.
	Module struct {
		TypeSection     []*FunctionType
		ImportSection   []*ImportSegment
		FunctionSection []uint32
		TableSection    []*TableType
		MemorySection   []*MemoryType
		GlobalSection   []*GlobalSegment
		ExportSection   map[string]*ExportSegment
		StartSection    *uint32
		ElementSection  []*ElementSegment
		CodeSection     []*CodeSegment
		DataSection     []*DataSegment
		CustomSections  map[string][]byte
	}
)

// DecodeModule decodes a `raw` module from io.Reader whose index spaces are yet to be initialized
func DecodeModule(binary []byte) (*Module, error) {
	reader := &Reader{binary: binary, buffer: bytes.NewBuffer(binary)}

	// Magic number.
	buf := make([]byte, 4)
	if n, err := io.ReadFull(reader, buf); err != nil || n != 4 {
		return nil, ErrInvalidMagicNumber
	}
	for i := 0; i < 4; i++ {
		if buf[i] != magic[i] {
			return nil, ErrInvalidMagicNumber
		}
	}

	// Version.
	if n, err := io.ReadFull(reader, buf); err != nil || n != 4 {
		return nil, ErrInvalidVersion
	}
	for i := 0; i < 4; i++ {
		if buf[i] != version[i] {
			return nil, ErrInvalidVersion
		}
	}

	ret := &Module{CustomSections: map[string][]byte{}}
	if err := ret.readSections(reader); err != nil {
		return nil, fmt.Errorf("readSections failed: %w", err)
	}

	if len(ret.FunctionSection) != len(ret.CodeSection) {
		return nil, fmt.Errorf("function and code section have inconsistent lengths")
	}
	return ret, nil
}

func (m *Module) GetFunctionNames() (map[uint32]string, error) {
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
