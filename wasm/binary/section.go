package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

func readCustomSectionData(r *bytes.Reader, dataSize uint32) ([]byte, error) {
	data := make([]byte, dataSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("cannot read custom section data: %w", err)
	}
	return data, nil
}

func decodeTypeSection(r io.Reader) ([]*wasm.FunctionType, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.FunctionType, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeFunctionType(r); err != nil {
			return nil, fmt.Errorf("read %d-th type: %v", i, err)
		}
	}
	return result, nil
}

func decodeFunctionType(r io.Reader) (*wasm.FunctionType, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read leading byte: %w", err)
	}

	if b[0] != 0x60 {
		return nil, fmt.Errorf("%w: %#x != 0x60", ErrInvalidByte, b[0])
	}

	s, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("could not read parameter count: %w", err)
	}

	paramTypes, err := decodeValueTypes(r, s)
	if err != nil {
		return nil, fmt.Errorf("could not read parameter types: %w", err)
	}

	s, _, err = leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("could not read result count: %w", err)
	} else if s > 1 {
		return nil, fmt.Errorf("multi value results not supported")
	}

	resultTypes, err := decodeValueTypes(r, s)
	if err != nil {
		return nil, fmt.Errorf("could not read result types: %w", err)
	}

	return &wasm.FunctionType{
		Params:  paramTypes,
		Results: resultTypes,
	}, nil
}

func decodeImportSection(r *bytes.Reader) ([]*wasm.Import, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.Import, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeImport(r); err != nil {
			return nil, fmt.Errorf("read import: %w", err)
		}
	}
	return result, nil
}

func decodeFunctionSection(r *bytes.Reader) ([]uint32, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]uint32, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("get type index: %w", err)
		}
	}
	return result, err
}

func decodeTableSection(r *bytes.Reader) ([]*wasm.TableType, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.TableType, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeTableType(r); err != nil {
			return nil, fmt.Errorf("read table type: %w", err)
		}
	}
	return result, nil
}

func decodeMemorySection(r *bytes.Reader) ([]*wasm.MemoryType, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.MemoryType, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeMemoryType(r); err != nil {
			return nil, fmt.Errorf("read memory type: %w", err)
		}
	}
	return result, nil
}

func decodeGlobalSection(r *bytes.Reader) ([]*wasm.Global, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.Global, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeGlobal(r); err != nil {
			return nil, fmt.Errorf("read global: %v ", err)
		}
	}
	return result, nil
}

func decodeExportSection(r *bytes.Reader) (map[string]*wasm.Export, error) {
	vs, _, sizeErr := leb128.DecodeUint32(r)
	if sizeErr != nil {
		return nil, fmt.Errorf("get size of vector: %v", sizeErr)
	}

	result := make(map[string]*wasm.Export, vs)
	for i := uint32(0); i < vs; i++ {
		export, err := decodeExport(r)
		if err != nil {
			return nil, fmt.Errorf("read export: %w", err)
		}
		if _, ok := result[export.Name]; ok {
			return nil, fmt.Errorf("duplicate export name: %s", export.Name)
		}
		result[export.Name] = export
	}
	return result, nil
}

func decodeStartSection(r *bytes.Reader) (*uint32, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}
	return &vs, nil
}

func decodeElementSection(r *bytes.Reader) ([]*wasm.ElementSegment, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.ElementSegment, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeElementSegment(r); err != nil {
			return nil, fmt.Errorf("read element: %w", err)
		}
	}
	return result, nil
}

func decodeCodeSection(r *bytes.Reader) ([]*wasm.Code, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.Code, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeCode(r); err != nil {
			return nil, fmt.Errorf("read %d-th code segment: %v", i, err)
		}
	}
	return result, nil
}

func decodeDataSection(r *bytes.Reader) ([]*wasm.DataSegment, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.DataSegment, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeDataSegment(r); err != nil {
			return nil, fmt.Errorf("read data segment: %w", err)
		}
	}
	return result, nil
}

// encodeCustomSection encodes the opaque bytes for the given name as a SectionIDCustom
// See https://www.w3.org/TR/wasm-core-1/#binary-customsec
func encodeCustomSection(name string, data []byte) []byte {
	// The contents of a custom section is the non-empty name followed by potentially empty opaque data
	contents := append(encodeSizePrefixed([]byte(name)), data...)
	return encodeSection(SectionIDCustom, contents)
}

// encodeSection encodes the sectionID, the size of its contents in bytes, followed by the contents.
// See https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
func encodeSection(sectionID SectionID, contents []byte) []byte {
	return append([]byte{sectionID}, encodeSizePrefixed(contents)...)
}

// encodeTypeSection encodes a SectionIDType for the given imports in WebAssembly 1.0 (MVP) Binary Format.
//
// See encodeFunctionType
// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
func encodeTypeSection(types []*wasm.FunctionType) []byte {
	contents := leb128.EncodeUint32(uint32(len(types)))
	for _, t := range types {
		contents = append(contents, encodeFunctionType(t)...)
	}
	return encodeSection(SectionIDType, contents)
}

// encodeImportSection encodes a SectionIDImport for the given imports in WebAssembly 1.0 (MVP) Binary Format.
//
// See encodeImport
// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
func encodeImportSection(imports []*wasm.Import) []byte {
	contents := leb128.EncodeUint32(uint32(len(imports)))
	for _, i := range imports {
		contents = append(contents, encodeImport(i)...)
	}
	return encodeSection(SectionIDImport, contents)
}

// encodeExportSection encodes a SectionIDExport for the given exports in WebAssembly 1.0 (MVP) Binary Format.
//
// See encodeExport
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
func encodeExportSection(exports map[string]*wasm.Export) []byte {
	contents := leb128.EncodeUint32(uint32(len(exports)))
	for _, e := range exports {
		contents = append(contents, encodeExport(e)...)
	}
	return encodeSection(SectionIDExport, contents)
}

// encodeStartSection encodes a SectionIDStart for the given function index in WebAssembly 1.0 (MVP) Binary Format.
//
// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
func encodeStartSection(funcidx uint32) []byte {
	return encodeSection(SectionIDStart, leb128.EncodeUint32(funcidx))
}
