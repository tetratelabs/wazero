package binary

import (
	"github.com/tetratelabs/wazero/wasm"
)

var sizePrefixedName = []byte{4, 'n', 'a', 'm', 'e'}

// EncodeModule implements wasm.EncodeModule for the WebAssembly 1.0 (MVP) Binary Format.
// Note: If saving to a file, the conventional extension is wasm
// See https://www.w3.org/TR/wasm-core-1/#binary-format%E2%91%A0
func EncodeModule(m *wasm.Module) (bytes []byte) {
	bytes = append(magic, version...)
	if m.SectionSize(wasm.SectionIDType) > 0 {
		bytes = append(bytes, encodeTypeSection(m.TypeSection)...)
	}
	if m.SectionSize(wasm.SectionIDImport) > 0 {
		bytes = append(bytes, encodeImportSection(m.ImportSection)...)
	}
	if m.SectionSize(wasm.SectionIDFunction) > 0 {
		bytes = append(bytes, encodeFunctionSection(m.FunctionSection)...)
	}
	if m.SectionSize(wasm.SectionIDTable) > 0 {
		panic("TODO: TableSection")
	}
	if m.SectionSize(wasm.SectionIDMemory) > 0 {
		bytes = append(bytes, encodeMemorySection(m.MemorySection)...)
	}
	if m.SectionSize(wasm.SectionIDGlobal) > 0 {
		panic("TODO: GlobalSection")
	}
	if m.SectionSize(wasm.SectionIDExport) > 0 {
		bytes = append(bytes, encodeExportSection(m.ExportSection)...)
	}
	if m.SectionSize(wasm.SectionIDStart) > 0 {
		bytes = append(bytes, encodeStartSection(*m.StartSection)...)
	}
	if m.SectionSize(wasm.SectionIDElement) > 0 {
		panic("TODO: ElementSection")
	}
	if m.SectionSize(wasm.SectionIDCode) > 0 {
		bytes = append(bytes, encodeCodeSection(m.CodeSection)...)
	}
	if m.SectionSize(wasm.SectionIDData) > 0 {
		panic("TODO: DataSection")
	}
	if m.SectionSize(wasm.SectionIDCustom) > 0 {
		for name, data := range m.CustomSections {
			bytes = append(bytes, encodeCustomSection(name, data)...)
		}

		// >> The name section should appear only once in a module, and only after the data section.
		// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
		if m.NameSection != nil {
			nameSection := append(sizePrefixedName, encodeNameSectionData(m.NameSection)...)
			bytes = append(bytes, encodeSection(wasm.SectionIDCustom, nameSection)...)
		}
	}
	return
}
