package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeTypeSection(r *bytes.Reader) ([]*wasm.FunctionType, error) {
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

func decodeFunctionType(r *bytes.Reader) (*wasm.FunctionType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read leading byte: %w", err)
	}

	if b != 0x60 {
		return nil, fmt.Errorf("%w: %#x != 0x60", ErrInvalidByte, b)
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

func decodeImportSection(r *bytes.Reader, memoryMaxPages uint32) ([]*wasm.Import, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.Import, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeImport(r, i, memoryMaxPages); err != nil {
			return nil, err
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

func decodeTableSection(r *bytes.Reader) (*wasm.Table, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("error reading size")
	}
	if vs > 1 {
		return nil, fmt.Errorf("at most one table allowed in module, but read %d", vs)
	}

	return decodeTable(r)
}

func decodeMemorySection(r *bytes.Reader, memoryMaxPages uint32) (*wasm.Memory, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("error reading size")
	}
	if vs > 1 {
		return nil, fmt.Errorf("at most one memory allowed in module, but read %d", vs)
	}

	return decodeMemory(r, memoryMaxPages)
}

func decodeGlobalSection(r *bytes.Reader) ([]*wasm.Global, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	result := make([]*wasm.Global, vs)
	for i := uint32(0); i < vs; i++ {
		if result[i], err = decodeGlobal(r); err != nil {
			return nil, fmt.Errorf("global[%d]: %w", i, err)
		}
	}
	return result, nil
}

func decodeExportSection(r *bytes.Reader) (map[string]*wasm.Export, error) {
	vs, _, sizeErr := leb128.DecodeUint32(r)
	if sizeErr != nil {
		return nil, fmt.Errorf("get size of vector: %v", sizeErr)
	}

	exportSection := make(map[string]*wasm.Export, vs)
	for i := wasm.Index(0); i < vs; i++ {
		export, err := decodeExport(r)
		if err != nil {
			return nil, fmt.Errorf("read export: %w", err)
		}
		if _, ok := exportSection[export.Name]; ok {
			return nil, fmt.Errorf("export[%d] duplicates name %q", i, export.Name)
		}
		exportSection[export.Name] = export
	}
	return exportSection, nil
}

func decodeStartSection(r *bytes.Reader) (*wasm.Index, error) {
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

// encodeSection encodes the sectionID, the size of its contents in bytes, followed by the contents.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#sections%E2%91%A0
func encodeSection(sectionID wasm.SectionID, contents []byte) []byte {
	return append([]byte{sectionID}, encodeSizePrefixed(contents)...)
}

// encodeTypeSection encodes a wasm.SectionIDType for the given imports in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See encodeFunctionType
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#type-section%E2%91%A0
func encodeTypeSection(types []*wasm.FunctionType) []byte {
	contents := leb128.EncodeUint32(uint32(len(types)))
	for _, t := range types {
		contents = append(contents, encodeFunctionType(t)...)
	}
	return encodeSection(wasm.SectionIDType, contents)
}

// encodeImportSection encodes a wasm.SectionIDImport for the given imports in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See encodeImport
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#import-section%E2%91%A0
func encodeImportSection(imports []*wasm.Import) []byte {
	contents := leb128.EncodeUint32(uint32(len(imports)))
	for _, i := range imports {
		contents = append(contents, encodeImport(i)...)
	}
	return encodeSection(wasm.SectionIDImport, contents)
}

// encodeFunctionSection encodes a wasm.SectionIDFunction for the type indices associated with module-defined
// functions in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-section%E2%91%A0
func encodeFunctionSection(typeIndices []wasm.Index) []byte {
	contents := leb128.EncodeUint32(uint32(len(typeIndices)))
	for _, index := range typeIndices {
		contents = append(contents, leb128.EncodeUint32(index)...)
	}
	return encodeSection(wasm.SectionIDFunction, contents)
}

// encodeCodeSection encodes a wasm.SectionIDCode for the module-defined function in WebAssembly 1.0 (20191205)
// Binary Format.
//
// See encodeCode
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#code-section%E2%91%A0
func encodeCodeSection(code []*wasm.Code) []byte {
	contents := leb128.EncodeUint32(uint32(len(code)))
	for _, i := range code {
		contents = append(contents, encodeCode(i)...)
	}
	return encodeSection(wasm.SectionIDCode, contents)
}

// encodeTableSection encodes a wasm.SectionIDTable for the module-defined function in WebAssembly 1.0
// (20191205) Binary Format.
//
// See encodeTable
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-section%E2%91%A0
func encodeTableSection(table *wasm.Table) []byte {
	contents := append([]byte{1}, encodeTable(table)...)
	return encodeSection(wasm.SectionIDTable, contents)
}

// encodeMemorySection encodes a wasm.SectionIDMemory for the module-defined function in WebAssembly 1.0
// (20191205) Binary Format.
//
// See encodeMemory
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-section%E2%91%A0
func encodeMemorySection(memory *wasm.Memory) []byte {
	contents := append([]byte{1}, encodeMemory(memory)...)
	return encodeSection(wasm.SectionIDMemory, contents)
}

// encodeGlobalSection encodes a wasm.SectionIDGlobal for the given globals in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See encodeGlobal
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-section%E2%91%A0
func encodeGlobalSection(globals []*wasm.Global) []byte {
	contents := leb128.EncodeUint32(uint32(len(globals)))
	for _, g := range globals {
		contents = append(contents, encodeGlobal(g)...)
	}
	return encodeSection(wasm.SectionIDGlobal, contents)
}

// encodeExportSection encodes a wasm.SectionIDExport for the given exports in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See encodeExport
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#export-section%E2%91%A0
func encodeExportSection(exports map[string]*wasm.Export) []byte {
	contents := leb128.EncodeUint32(uint32(len(exports)))
	for _, e := range exports {
		contents = append(contents, encodeExport(e)...)
	}
	return encodeSection(wasm.SectionIDExport, contents)
}

// encodeStartSection encodes a wasm.SectionIDStart for the given function index in WebAssembly 1.0 (20191205)
// Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-section%E2%91%A0
func encodeStartSection(funcidx wasm.Index) []byte {
	return encodeSection(wasm.SectionIDStart, leb128.EncodeUint32(funcidx))
}
