package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// encodeSection encodes the sectionID, the size of its contents in bytes, followed by the contents.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#sections%E2%91%A0
func encodeSection(sectionID wasm.SectionID, contents []byte) []byte {
	return append([]byte{sectionID}, encodeSizePrefixed(contents)...)
}

// encodeTypeSection encodes a wasm.SectionIDType for the given imports in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See EncodeFunctionType
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#type-section%E2%91%A0
func encodeTypeSection(types []wasm.FunctionType) []byte {
	contents := leb128.EncodeUint32(uint32(len(types)))
	for i := range types {
		t := &types[i]
		contents = append(contents, EncodeFunctionType(t)...)
	}
	return encodeSection(wasm.SectionIDType, contents)
}

// encodeImportSection encodes a wasm.SectionIDImport for the given imports in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See EncodeImport
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#import-section%E2%91%A0
func encodeImportSection(imports []wasm.Import) []byte {
	contents := leb128.EncodeUint32(uint32(len(imports)))
	for i := range imports {
		imp := &imports[i]
		contents = append(contents, EncodeImport(imp)...)
	}
	return encodeSection(wasm.SectionIDImport, contents)
}

// EncodeFunctionSection encodes a wasm.SectionIDFunction for the type indices associated with module-defined
// functions in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-section%E2%91%A0
func EncodeFunctionSection(typeIndices []wasm.Index) []byte {
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
func encodeCodeSection(code []wasm.Code) []byte {
	contents := leb128.EncodeUint32(uint32(len(code)))
	for i := range code {
		c := &code[i]
		contents = append(contents, encodeCode(c)...)
	}
	return encodeSection(wasm.SectionIDCode, contents)
}

// encodeTableSection encodes a wasm.SectionIDTable for the module-defined function in WebAssembly 1.0
// (20191205) Binary Format.
//
// See EncodeTable
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-section%E2%91%A0
func encodeTableSection(tables []wasm.Table) []byte {
	var contents []byte = leb128.EncodeUint32(uint32(len(tables)))
	for i := range tables {
		table := &tables[i]
		contents = append(contents, EncodeTable(table)...)
	}
	return encodeSection(wasm.SectionIDTable, contents)
}

// encodeMemorySection encodes a wasm.SectionIDMemory for the module-defined function in WebAssembly 1.0
// (20191205) Binary Format.
//
// See EncodeMemory
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-section%E2%91%A0
func encodeMemorySection(memory *wasm.Memory) []byte {
	contents := append([]byte{1}, EncodeMemory(memory)...)
	return encodeSection(wasm.SectionIDMemory, contents)
}

// encodeGlobalSection encodes a wasm.SectionIDGlobal for the given globals in WebAssembly 1.0 (20191205) Binary
// Format.
//
// See encodeGlobal
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-section%E2%91%A0
func encodeGlobalSection(globals []wasm.Global) []byte {
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
func encodeExportSection(exports []wasm.Export) []byte {
	contents := leb128.EncodeUint32(uint32(len(exports)))
	for i := range exports {
		e := &exports[i]
		contents = append(contents, encodeExport(e)...)
	}
	return encodeSection(wasm.SectionIDExport, contents)
}

// EncodeStartSection encodes a wasm.SectionIDStart for the given function index in WebAssembly 1.0 (20191205)
// Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-section%E2%91%A0
func EncodeStartSection(funcidx wasm.Index) []byte {
	return encodeSection(wasm.SectionIDStart, leb128.EncodeUint32(funcidx))
}

// encodeEelementSection encodes a wasm.SectionIDElement for the elements in WebAssembly 1.0 (20191205)
// Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#element-section%E2%91%A0
func encodeElementSection(elements []wasm.ElementSegment) []byte {
	contents := leb128.EncodeUint32(uint32(len(elements)))
	for i := range elements {
		e := &elements[i]
		contents = append(contents, encodeElement(e)...)
	}
	return encodeSection(wasm.SectionIDElement, contents)
}

// encodeDataSection encodes a wasm.SectionIDData for the data in WebAssembly 1.0 (20191205)
// Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#data-section%E2%91%A0
func encodeDataSection(datum []wasm.DataSegment) []byte {
	contents := leb128.EncodeUint32(uint32(len(datum)))
	for i := range datum {
		d := &datum[i]
		contents = append(contents, encodeDataSegment(d)...)
	}
	return encodeSection(wasm.SectionIDData, contents)
}
