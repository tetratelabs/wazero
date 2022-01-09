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
	for _, f := range m.importFuncs {
		result.ImportSection = append(result.ImportSection, &wasm.ImportSegment{
			Module: f.module, Name: f.name,
			Desc: &wasm.ImportDesc{
				Kind:          wasm.ImportKindFunction,
				FuncTypeIndex: f.typeIndex,
			},
		})
	}

	// The start function is called on Module.Instantiate. To prevent runtime errors, resolve it into a valid function
	// index now. parseStartSection already returns a FormatError, so don't re-wrap it!
	if m.startFunction != nil {
		if result.StartSection, err = parseStartSection(m); err != nil {
			return nil, err
		}
	}

	// TODO: encode CustomSection["name"] with module function and local names
	return
}

// parseStartSection looks up the numerical index in module.importFuncs corresponding to startFunction.index or returns
// a FormatError.
func parseStartSection(m *module) (*uint32, error) {
	start := m.startFunction

	// Try to see if this is a numeric index like "2" first.
	if parsed, parseErr := strconv.ParseUint(start.index, 0, 32); parseErr == nil {
		if int(parsed) >= len(m.importFuncs) { // TODO len(m.importFuncs + m.funcs) when we add them!
			return nil, &FormatError{start.line, start.col, "module.start",
				fmt.Errorf("invalid function index: %d", parsed),
			}
		}
		startIdx := uint32(parsed)
		return &startIdx, nil
	}

	// Now, attempt to look up the symbolic name of any function imported or defined in this module.
	for i, f := range m.importFuncs {
		if f.funcName == start.index {
			startIdx := uint32(i)
			return &startIdx, nil
		}
	}

	// TODO: also search functions defined in this module, once we add them!
	return nil, &FormatError{start.line, start.col, "module.start",
		fmt.Errorf("unknown function name: %s", start.index),
	}
}
