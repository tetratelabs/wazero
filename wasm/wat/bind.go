package wat

import (
	"fmt"
)

// bindIndices ensures any indices point are numeric or returns a FormatError if they cannot be bound.
func bindIndices(m *module) error {
	if m.startFunction != nil {
		if err := bindStartFunction(m); err != nil {
			return err
		}
	}
	return nil
}

// bindStartFunction ensures the module.startFunction points to a valid numeric index or returns a FormatError if it
// cannot be bound.
//
// Failure cases are when a symbolic identifier points nowhere or a numeric index is out of range.
// Ex. (start $t0) exists, but there's no import or module defined function with that name.
//  or (start 32) exists, but there are only 10 functions.
func bindStartFunction(m *module) error {
	start := m.startFunction

	if start.ID == "" { // already bound to a numeric index, but we have to verify it is in range
		if int(start.numeric) >= len(m.importFuncs) { // TODO len(m.importFuncs + m.funcs) when we add them!
			return &FormatError{start.line, start.col, "module.start",
				fmt.Errorf("function index %d is out of range [0..%d]", start.numeric, len(m.importFuncs)-1),
			}
		}
	}

	// Now, attempt to look up the symbolic name of any function imported or defined in this module.
	for i, f := range m.importFuncs {
		if f.funcName == start.ID {
			start.ID = ""
			start.numeric = uint32(i)
			return nil
		}
	}

	// TODO: also search functions defined in this module, once we add them!
	return &FormatError{start.line, start.col, "module.start",
		fmt.Errorf("unknown function name %s", start.ID),
	}
}
