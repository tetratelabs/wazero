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
	for _, t := range m.types {
		var results []wasm.ValueType
		if t.result != 0 {
			results = []wasm.ValueType{t.result}
		}
		result.TypeSection = append(result.TypeSection, &wasm.FunctionType{Params: t.params, Results: results})
	}

	// Now, handle any imported functions. Notably, we retain the same insertion order as defined in the text format in
	// case a numeric index is used for the start function (or another reason such as the call instruction).
	var functionNames map[uint32]string
	var localNames map[uint32]map[uint32]string
	for i, f := range m.importFuncs {
		funcidx := uint32(i)
		if f.funcName != "" {
			if functionNames == nil {
				functionNames = map[uint32]string{funcidx: f.funcName}
			} else {
				functionNames[funcidx] = f.funcName
			}
		}
		if f.paramNames != nil {
			locals := map[uint32]string{}
			for _, pn := range f.paramNames {
				locals[pn.index] = string(pn.name)
			}
			if localNames == nil {
				localNames = map[uint32]map[uint32]string{funcidx: locals}
			} else {
				localNames[funcidx] = locals
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

	// Don't set the name section unless we found a name!
	if m.name != "" || functionNames != nil || localNames != nil {
		result.NameSection = &wasm.NameSection{
			ModuleName:    m.name,
			FunctionNames: functionNames,
			LocalNames:    localNames,
		}
	}
	return
}
