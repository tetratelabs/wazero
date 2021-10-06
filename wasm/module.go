package wasm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	magic   = []byte{0x00, 0x61, 0x73, 0x6D}
	version = []byte{0x01, 0x00, 0x00, 0x00}

	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidVersion     = errors.New("invalid version header")
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

	return ret, nil
}
