package wat

import (
	"fmt"
)

// bindIndices ensures any indices point are numeric or returns a FormatError if they cannot be bound.
func bindIndices(m *module) error {
	typeToIndex := map[*typeFunc]uint32{}
	typeNameToIndex := map[string]uint32{}
	for i, t := range m.typeFuncs {
		if t.name != "" {
			typeNameToIndex[t.name] = uint32(i)
		}
		typeToIndex[t] = uint32(i)
	}

	funcNameToIndex, err := bindFunctionTypes(m, typeToIndex, typeNameToIndex)
	if err != nil {
		return err
	}

	if m.startFunction != nil {
		if err = bindStartFunction(m, funcNameToIndex); err != nil {
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
func bindFunctionTypes(m *module, typeToIndex map[*typeFunc]uint32, typeNameToIndex map[string]uint32) (map[string]uint32, error) {
	funcNameToIndex := map[string]uint32{}
	typeCount := uint32(len(m.typeFuncs))
	for i, f := range m.importFuncs {
		if f.funcName != "" {
			funcNameToIndex[f.funcName] = uint32(i)
		}

		idx := f.typeIndex
		if idx == nil { // inlined type
			f.typeIndex = &index{numeric: typeToIndex[f.typeInlined] /* TODO: 0, 0 aren't valid line/col */}
			f.typeInlined = nil
			continue
		}

		// TODO: inlined types can contain "verification types" basically where typeIndex != nil if there is a type we need
		// to verify it matches the corresponding index exactly.
		if idx.ID == "" { // already bound to a numeric index: verify it is in range
			if err := checkIndexInRange(idx, typeCount, "module.import[%d].func.type", i); err != nil {
				return nil, err
			}
		} else if err := bindSymbolicIDToNumericIndex(typeNameToIndex, idx, "module.import[%d].func.type", i); err != nil {
			return nil, err
		}

	}
	return funcNameToIndex, nil
}

// bindStartFunction ensures the module.startFunction points to a valid numeric index or returns a FormatError if it
// cannot be bound.
//
// Failure cases are when a symbolic identifier points nowhere or a numeric index is out of range.
// Ex. (start $t0) exists, but there's no import or module defined function with that name.
//  or (start 32) exists, but there are only 10 functions.
func bindStartFunction(m *module, funcNameToIndex map[string]uint32) error {
	idx := m.startFunction

	if idx.ID == "" { // already bound to a numeric index, but we have to verify it is in range
		indexCount := uint32(len(m.importFuncs)) // TODO len(m.importFuncs + m.funcs) when we add them!
		return checkIndexInRange(idx, indexCount, "module.start", -1)
	}

	return bindSymbolicIDToNumericIndex(funcNameToIndex, idx, "module.start", -1)
}

func bindSymbolicIDToNumericIndex(idToIndex map[string]uint32, idx *index, context string, contextArg0 int) error {
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
		fmt.Errorf("unknown ID %s", idx.ID),
	}
}

func checkIndexInRange(idx *index, count uint32, context string, contextArg0 int) error {
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
