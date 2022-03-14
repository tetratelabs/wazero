package internalwasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStore_resolveImports_table(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("ok", func(t *testing.T) {
		s := newStore()
		max := uint32(10)
		tableInst := &TableInstance{Max: &max}
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
			Type:  ExternTypeTable,
			Table: tableInst,
		}}, Name: moduleName}
		_, _, table, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &Table{Max: &max}}}})
		require.NoError(t, err)
		require.Equal(t, table, tableInst)
	})
	t.Run("minimum size mismatch", func(t *testing.T) {
		s := newStore()
		importTableType := &Table{Min: 2}
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
			Type:  ExternTypeTable,
			Table: &TableInstance{Min: importTableType.Min - 1},
		}}, Name: moduleName}
		_, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
		require.EqualError(t, err, "import[0] table[test.target]: minimum size mismatch: 2 > 1")
	})
	t.Run("maximum size mismatch", func(t *testing.T) {
		s := newStore()
		max := uint32(10)
		importTableType := &Table{Max: &max}
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
			Type:  ExternTypeTable,
			Table: &TableInstance{Min: importTableType.Min - 1},
		}}, Name: moduleName}
		_, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
		require.EqualError(t, err, "import[0] table[test.target]: maximum size mismatch: 10, but actual has no max")
	})
}

func TestModule_buildTableInstance(t *testing.T) {
	m := Module{TableSection: &Table{Min: 1}}
	table := m.buildTable()
	require.Equal(t, uint32(1), table.Min)
}

func TestModule_validateTable(t *testing.T) {
	t.Run("invalid const expr", func(t *testing.T) {
		m := Module{ElementSection: []*ElementSegment{{
			OffsetExpr: &ConstantExpression{
				Opcode: OpcodeUnreachable, // Invalid!
			},
		}}}
		err := m.validateTable(&Table{}, nil)
		require.Error(t, err)
		require.EqualError(t, err, "invalid const expression for element: invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{ElementSection: []*ElementSegment{{
			OffsetExpr: &ConstantExpression{
				Opcode: OpcodeI32Const,
				Data:   []byte{0x1},
			},
		}}}
		err := m.validateTable(&Table{}, nil)
		require.NoError(t, err)
	})
}

func TestModuleInstance_applyElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		Table:     &TableInstance{Table: make([]uintptr, 10)},
		Functions: make([]*FunctionInstance, 10),
		Engine:    &mockModuleEngine{},
	}
	targetIndex, targetOffset := uint32(1), byte(0)
	targetIndex2, targetOffset2 := functionCounts-1, byte(0x8)
	m.Functions[targetIndex] = &FunctionInstance{Type: &FunctionType{}, Index: Index(targetIndex)}
	m.Functions[targetIndex2] = &FunctionInstance{Type: &FunctionType{}, Index: Index(targetIndex2)}
	m.applyElements([]*ElementSegment{
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset}}, Init: []uint32{targetIndex}},
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset2}}, Init: []uint32{targetIndex2}},
	})
	require.Equal(t, uintptr(targetIndex), m.Table.Table[targetOffset])
	require.Equal(t, uintptr(targetIndex2), m.Table.Table[targetOffset2])
}

func TestModuleInstance_validateElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		Table:     &TableInstance{Table: make([]uintptr, 10)},
		Functions: make([]*FunctionInstance, 10),
	}
	for _, tc := range []struct {
		name     string
		elements []*ElementSegment
		expErr   bool
	}{
		{
			name: "ok",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "ok on edge",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "out of bounds",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x9}}, Init: []uint32{0, functionCounts - 1}},
			},
			expErr: true,
		},
		{
			name: "unknown function",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts}},
			},
			expErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateElements(tc.elements)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
