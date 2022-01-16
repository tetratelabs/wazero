package wat

import (
	"github.com/tetratelabs/wazero/wasm"
)

// TextToBinary parses the configured source into a wasm.Module. This function returns when the source is exhausted or
// an error occurs.
//
// Here's a description of the return values:
// * result is the module parsed or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
func TextToBinary(source []byte) (result *wasm.Module, err error) {
	// First, attempt to parse the module into a basic structure representing the text format. If this errs, return
	// immediately without wrapping parseModule returns FormatError, which is pre-formatted.
	var m *module
	if m, err = parseModule(source); err != nil {
		return nil, err
	}

	// Next, we need to convert the types from the text format into the binary one. This is easy because the only
	// difference is that the text format has type names and the binary format does not.
	result = &wasm.Module{}
	names := wasm.CustomNameSection{
		ModuleName:    m.name,
		FunctionNames: map[uint32]string{},
		LocalNames:    map[uint32]map[uint32]string{},
	}
	for _, t := range m.types {
		var results []wasm.ValueType
		if t.result != 0 {
			results = []wasm.ValueType{t.result}
		}
		result.TypeSection = append(result.TypeSection, &wasm.FunctionType{Params: t.params, Results: results})
	}

	// Now, handle any imported functions. Notably, we retain the same insertion order as defined in the text format in
	// case a numeric index is used for the start function (or another reason such as the call instruction).
	for i, f := range m.importFuncs {
		funcidx := uint32(i)
		if f.funcName != "" {
			names.FunctionNames[funcidx] = f.funcName
		}
		if f.paramNames != nil {
			locals := map[uint32]string{}
			names.LocalNames[funcidx] = locals
			for _, pn := range f.paramNames {
				locals[pn.index] = string(pn.name)
			}
		}
		result.ImportSection = append(result.ImportSection, &wasm.ImportSegment{
			Module: f.module, Name: f.name,
			Desc: &wasm.ImportDesc{
				Kind:          wasm.ImportKindFunction,
				FuncTypeIndex: f.typeIndex.numeric,
			},
		})
	}

	// TODO: module function and local names

	// The start function is called on Module.Instantiate.
	if m.startFunction != nil {
		result.StartSection = &m.startFunction.numeric
	}

	// Encode the custom name section, if there is any data in it
	if nameData := names.EncodeData(); nameData != nil {
		result.CustomSections = map[string][]byte{
			"name": nameData,
		}
	}
	return
}
