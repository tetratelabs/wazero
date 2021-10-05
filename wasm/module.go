package wasm

import (
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

type (
	// Static binary representations.
	Module struct {
		SecTypes     []*FunctionType
		SecImports   []*ImportSegment
		SecFunctions []uint32
		SecTables    []*TableType
		SecMemory    []*MemoryType
		SecGlobals   []*GlobalSegment
		SecExports   map[string]*ExportSegment
		SecStart     *uint32
		SecElements  []*ElementSegment
		SecCodes     []*CodeSegment
		SecData      []*DataSegment
	}
)

// DecodeModule decodes a `raw` module from io.Reader whose index spaces are yet to be initialized
func DecodeModule(r io.Reader) (*Module, error) {
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
		panic(err)
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

	return ret, nil
}
