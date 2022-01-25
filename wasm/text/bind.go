package text

import (
	"fmt"
	"github.com/tetratelabs/wazero/wasm"
)

// bindIndices ensures any indices point are numeric or returns a FormatError if they cannot be bound.
func bindIndices(m *module, typeNameToIndex, funcNameToIndex map[string]wasm.Index) error {
	typeToIndex := map[*wasm.FunctionType]wasm.Index{}
	for i, t := range m.types {
		ui := wasm.Index(i)
		typeToIndex[t] = ui
	}

	if err := bindFunctionTypes(m, typeNameToIndex, typeToIndex); err != nil {
		return err
	}

	indexCount := uint32(len(m.importFuncs) + len(m.code))

	if err := bindExportFuncs(m, funcNameToIndex, indexCount); err != nil {
		return err
	}

	if m.startFunction != nil {
		err := bindIndex(indexCount, funcNameToIndex, m.startFunction, "module.start", -1)
		if err != nil {
			return err
		}
	}
	return nil
}

// bindFunctionTypes ensures that all module.importFuncs point to a valid numeric index or returns a FormatError if one
// couldn't be bound. A mapping of function names to their index are returned for convenience.
//
// Failure cases are when a symbolic identifier points nowhere or a numeric index is out of range.
// Ex. (import "Math" "PI" (func (type $t0))) exists, but (type $t0 (func ...)) does not.
//  or (import "Math" "PI" (func (type 32))) exists, but there are only 10 types.
func bindFunctionTypes(m *module, typeNameToIndex map[string]wasm.Index, typeToIndex map[*wasm.FunctionType]wasm.Index) error {
	typeCount := uint32(len(m.types))
	importCount := uint32(len(m.importFuncs))

	for i, tu := range m.typeUses {

		var context string
		if uint32(i) >= importCount {
			context = "module.func[%d].type"
		} else {
			context = "module.import[%d].func.type"
		}

		idx := tu.typeIndex
		if idx == nil { // inlined type
			ti := tu.typeInlined
			tu.typeIndex = &index{numeric: typeToIndex[ti.typeFunc], line: ti.line, col: ti.col}
			tu.typeInlined = nil
			continue
		}

		err := bindIndex(typeCount, typeNameToIndex, idx, context, int64(i))
		if err != nil {
			return err
		}

		// If there's an inlined type now, it must contain the same signature as the index, and may contain names.
		if tu.typeInlined != nil {
			realType := m.types[idx.numeric]
			ti := tu.typeInlined
			if !funcTypeEquals(realType, ti.typeFunc.Params, ti.typeFunc.Results) {
				return &FormatError{ti.line, ti.col, fmt.Sprintf(context, i),
					fmt.Errorf("inlined type doesn't match type index %d", idx.numeric),
				}
			}
		}
	}
	return nil
}

// bindExportFuncs ensures all module.exportFuncs point to valid numeric indices or returns a FormatError if one
// cannot be bound.
func bindExportFuncs(m *module, funcNameToIndex map[string]wasm.Index, indexCount uint32) (err error) {
	for _, e := range m.exportFuncs {
		err = bindIndex(indexCount, funcNameToIndex, e.funcIndex, "module.exports[%d].func", int64(e.exportIndex))
		if err != nil {
			return
		}
	}
	return
}

// bindIndex ensures the idx points to a valid numeric function index or returns a FormatError if it cannot be bound.
//
// Failure cases are when a symbolic identifier points nowhere or a numeric index is out of range.
// Ex. (start $t0) exists, but there's no import or module defined function with that name.
//  or (start 32) exists, but there are only 10 functions.
func bindIndex(indexCount uint32, nameToIndex map[string]uint32, idx *index, context string, contextArg0 int64) error {
	if idx.ID == "" { // already bound to a numeric index, but we have to verify it is in range
		return checkIndexInRange(idx, indexCount, context, contextArg0)
	}

	return bindSymbolicIDToNumericIndex(nameToIndex, idx, context, contextArg0)
}

func bindSymbolicIDToNumericIndex(idToIndex map[string]uint32, idx *index, context string, contextArg0 int64) error {
	if numeric, ok := idToIndex[idx.ID]; ok {
		idx.ID = ""
		idx.numeric = numeric
		return nil
	}
	// This check allows us to defer Sprintf until there's an error, and reuse the same logic for non-indexed types.
	if contextArg0 >= 0 {
		context = fmt.Sprintf(context, contextArg0)
	}
	return &FormatError{idx.line, idx.col, context,
		fmt.Errorf("unknown ID $%s", idx.ID), // re-attach '$' as that was in the text format!
	}
}

func checkIndexInRange(idx *index, count uint32, context string, contextArg0 int64) error {
	if idx.numeric >= count {
		// This check allows us to defer Sprintf until there's an error, and reuse the same logic for non-indexed types.
		if contextArg0 >= 0 {
			context = fmt.Sprintf(context, contextArg0)
		}
		return &FormatError{idx.line, idx.col, context,
			fmt.Errorf("index %d is out of range [0..%d]", idx.numeric, count-1)}
	}
	return nil
}

// mergeLocalNames produces wasm.NameSection LocalNames. This has to be done post-parse as types can be defined after
// functions that use them.
func mergeLocalNames(m *module, typeParamIDContext map[wasm.Index]idContext) wasm.IndirectNameMap {
	paramNames := m.names.LocalNames
	j, jLen := 0, len(paramNames)
	if typeParamIDContext == nil && jLen == 0 {
		return nil
	}

	// Parameters can be named on the type, and overridden via a function. This loop collects the final name for each
	// function's parameters regardless of if it is an imported function or module defined.
	var result wasm.IndirectNameMap
	typeParamNames := map[wasm.Index]wasm.NameMap{}
	funcIndexSize := uint32(len(m.typeUses))
	for i := uint32(0); i < funcIndexSize; i++ {
		// Seek to see if we have any function-defined parameter names
		var inlinedNames wasm.NameMap
		for ; j < jLen; j++ {
			next := paramNames[j]
			if next.Index > i {
				break // we have parameter names, but starting at a later index
			} else if next.Index == i {
				inlinedNames = next.NameMap
				break
			}
		}

		// Use any inlined names or default to any on the type
		typeIndex := m.typeUses[i].typeIndex.numeric
		typeNames, hasType := typeParamIDContext[typeIndex]
		var localNames wasm.NameMap
		if inlinedNames == nil && !hasType {
			continue
		} else if inlinedNames == nil {
			if localNames, hasType = typeParamNames[typeIndex]; !hasType {
				localNames = wasm.NewNameMap(typeNames)
				typeParamNames[typeIndex] = localNames
			}
		} else {
			// On conflict, choose the function names, as merge rules aren't defined in the specification. If there are
			// names on the function, the user added them. They may not intend to inherit names they didn't define!
			localNames = inlinedNames
		}
		result = append(result, &wasm.NameMapAssoc{Index: i, NameMap: localNames})
	}
	// TODO: function local names
	return result
}
