package wat

import (
	"fmt"
	"strconv"

	"github.com/tetratelabs/wazero/wasm"
)

// TextToBinary parses the configured source into a wasm.Module. This function returns when the source is exhausted or
// an error occurs.
//
// Here's a description of the return values:
// * result is the module parsed or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
func TextToBinary(source []byte) (result *wasm.Module, err error) {
	var m *module
	m, err = parseModule(source)
	if err != nil {
		return nil, err
	}
	result = &wasm.Module{}
	for _, t := range m.types {
		result.TypeSection = append(result.TypeSection, &wasm.FunctionType{Params: t.params, Results: t.results})
	}
	for _, f := range m.importFuncs {
		result.ImportSection = append(result.ImportSection, &wasm.ImportSegment{
			Module: f.module, Name: f.name,
			Desc: &wasm.ImportDesc{
				Kind:          wasm.ImportKindFunction,
				FuncTypeIndex: uint32(f.typeIndex),
			},
		})
	}
	if m.startFunction != nil {
		if result.StartSection, err = parseStartSection(m); err != nil {
			return nil, err
		}
	}

	// TODO: encode CustomSection["name"] with module function and local names
	return
}

func parseStartSection(m *module) (*uint32, error) {
	start := m.startFunction
	if parsed, e := strconv.ParseUint(start.index, 0, 32); e == nil {
		if int(parsed) >= len(m.importFuncs) { // TODO len(m.importFuncs + m.funcs) when we add them!
			return nil, &FormatError{start.line, start.col, "module.start",
				fmt.Errorf("invalid function index: %d", parsed),
			}
		}
		startIdx := uint32(parsed)
		return &startIdx, nil
	} // attempt to look up the symbolic name
	for i, f := range m.importFuncs {
		if f.funcName == start.index {
			startIdx := uint32(i)
			return &startIdx, nil
		}
	}
	// TODO: also search funcs when we add them
	return nil, &FormatError{start.line, start.col, "module.start",
		fmt.Errorf("unknown function name: %s", start.index),
	}
}
