package text

import (
	"github.com/tetratelabs/wazero/wasm"
)

// DecodeModule implements wasm.DecodeModule for the WebAssembly 1.0 (MVP) Text Format
// See https://www.w3.org/TR/wasm-core-1/#text-format%E2%91%A0
func DecodeModule(source []byte) (result *wasm.Module, err error) {
	// First, attempt to parse the module into a basic structure representing the text format. If this errs, return
	// immediately without wrapping parseModule returns FormatError, which is pre-formatted.
	var m *module
	if m, err = parseModule(source); err != nil {
		return nil, err
	}

	result = &wasm.Module{}
	result.TypeSection = m.types

	// Now, handle any imported functions. Notably, we retain the same insertion order as defined in the text format in
	// case a numeric index is used for the start function (or another reason such as the call instruction).
	importFuncCount := len(m.importFuncs)
	if importFuncCount > 0 {
		result.ImportSection = make([]*wasm.Import, importFuncCount)
		for i, f := range m.importFuncs {
			result.ImportSection[i] = &wasm.Import{
				Module: f.module, Name: f.name,
				Kind:     wasm.ImportKindFunc,
				DescFunc: m.typeUses[i].typeIndex.numeric,
			}
		}
	}

	// Add the type use of the function into the function section.
	funcCount := len(m.code)
	if funcCount != 0 {
		result.FunctionSection = make([]wasm.Index, funcCount)
		for i := 0; i < funcCount; i++ {
			result.FunctionSection[i] = m.typeUses[i+importFuncCount].typeIndex.numeric
		}
		result.CodeSection = m.code
	}

	// Now, handle any exported functions. Notably, we retain the same insertion order as defined in the text format.
	exportFuncCount := len(m.exportFuncs)
	if exportFuncCount > 0 {
		result.ExportSection = make(map[string]*wasm.Export, exportFuncCount)
		for _, f := range m.exportFuncs {
			e := &wasm.Export{
				Name:  f.name,
				Kind:  wasm.ExportKindFunc,
				Index: f.funcIndex.numeric,
			}
			result.ExportSection[e.Name] = e
		}
	}

	// The start function is called on Module.Instantiate.
	if m.startFunction != nil {
		result.StartSection = &m.startFunction.numeric
	}
	result.NameSection = m.names
	return
}
