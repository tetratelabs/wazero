package internalwasm

import "fmt"

// Table describes the limits of (function) elements in a table.
type Table = limitsType

// ElementSegment are initialization instructions for a TableInstance
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-elem
type ElementSegment struct {
	// OffsetExpr returns the table element offset to apply to Init indices.
	// Note: This can be validated prior to instantiation unless it includes OpcodeGlobalGet (an imported global).
	OffsetExpr *ConstantExpression

	// Init indices are table elements relative to the result of OffsetExpr. The values are positions in the function
	// index namespace that initialize the corresponding element.
	Init []Index
}

// TableInstance represents a table instance in a store.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-instances%E2%91%A0
type TableInstance struct {
	// Table holds the table elements managed by this table instance.
	Table []uintptr

	// Min is the minimum (function) elements in this table.
	Min uint32

	// Max if present is the maximum (function) elements in this table, or nil if unbounded.
	Max *uint32
}

func (m *Module) buildTable() *TableInstance {
	table := m.TableSection
	if table != nil {
		return &TableInstance{
			Table: make([]uintptr, table.Min),
			Min:   table.Min,
			Max:   table.Max,
		}
	}
	return nil
}

func (m *Module) validateTable(table *Table, globals []*GlobalType) error {
	for _, elem := range m.ElementSection {
		if table == nil {
			return fmt.Errorf("table index out of range")
		}
		err := validateConstExpression(globals, elem.OffsetExpr, ValueTypeI32)
		if err != nil {
			return fmt.Errorf("invalid const expression for element: %w", err)
		}
	}
	return nil
}

func (m *ModuleInstance) validateElements(elements []*ElementSegment) (err error) {
	for _, elem := range elements {
		offset := int(executeConstExpression(m.Globals, elem.OffsetExpr).(int32))
		ceil := offset + len(elem.Init)

		if offset < 0 || ceil > len(m.Table.Table) {
			return fmt.Errorf("out of bounds table access")
		}
		for _, elm := range elem.Init {
			if elm >= uint32(len(m.Functions)) {
				return fmt.Errorf("unknown function specified by element")
			}
		}
	}
	return
}

func (m *ModuleInstance) applyElements(elements []*ElementSegment) {
	for _, elem := range elements {
		offset := int(executeConstExpression(m.Globals, elem.OffsetExpr).(int32))
		table := m.Table.Table
		for i, elm := range elem.Init {
			pos := i + offset
			table[pos] = m.Engine.FunctionAddress(elm)
		}
	}
}
