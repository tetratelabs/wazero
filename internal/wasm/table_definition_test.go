package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestModule_BuildTableDefinitions mirrors TestModule_BuildMemoryDefinitions -
// added alongside ExportedTable (wazero#2461) so CompiledModule.ExportedTables
// gets the same coverage ExportedMemories already has.
func TestModule_BuildTableDefinitions(t *testing.T) {
	max := uint32(3)

	tests := []struct {
		name            string
		m               *Module
		expected        []TableDefinition
		expectedImports []api.TableDefinition
		expectedExports map[string]api.TableDefinition
	}{
		{
			name:            "no exports",
			m:               &Module{},
			expectedExports: map[string]api.TableDefinition{},
		},
		{
			name: "no tables",
			m: &Module{
				ExportSection: []Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection: []Global{{}},
			},
			expectedExports: map[string]api.TableDefinition{},
		},
		{
			name:            "defines table{2,}",
			m:               &Module{TableSection: []Table{{Min: 2, Type: RefTypeFuncref}}},
			expected:        []TableDefinition{{index: 0, table: &Table{Min: 2, Type: RefTypeFuncref}}},
			expectedExports: map[string]api.TableDefinition{},
		},
		{
			name: "exports defined table{2,3}",
			m: &Module{
				ExportSection: []Export{
					{Name: "table_index=0", Type: ExternTypeTable, Index: 0},
					{Name: "", Type: ExternTypeGlobal, Index: 0},
				},
				GlobalSection: []Global{{}},
				TableSection:  []Table{{Min: 2, Max: &max, Type: RefTypeExternref}},
			},
			expected: []TableDefinition{
				{
					index:       0,
					exportNames: []string{"table_index=0"},
					table:       &Table{Min: 2, Max: &max, Type: RefTypeExternref},
				},
			},
			expectedExports: map[string]api.TableDefinition{
				"table_index=0": &TableDefinition{
					index:       0,
					exportNames: []string{"table_index=0"},
					table:       &Table{Min: 2, Max: &max, Type: RefTypeExternref},
				},
			},
		},
		{
			name: "exports imported table{0,} and defined table{2,3}",
			m: &Module{
				ImportSection: []Import{{
					Type:      ExternTypeTable,
					DescTable: Table{Min: 0, Type: RefTypeFuncref},
				}},
				ExportSection: []Export{
					{Name: "imported_table", Type: ExternTypeTable, Index: 0},
					{Name: "table_index=1", Type: ExternTypeTable, Index: 1},
				},
				TableSection: []Table{{Min: 2, Max: &max, Type: RefTypeExternref}},
			},
			expected: []TableDefinition{
				{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_table"},
					table:       &Table{Min: 0, Type: RefTypeFuncref},
				},
				{
					index:       1,
					exportNames: []string{"table_index=1"},
					table:       &Table{Min: 2, Max: &max, Type: RefTypeExternref},
				},
			},
			expectedImports: []api.TableDefinition{
				&TableDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_table"},
					table:       &Table{Min: 0, Type: RefTypeFuncref},
				},
			},
			expectedExports: map[string]api.TableDefinition{
				"imported_table": &TableDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_table"},
					table:       &Table{Min: 0, Type: RefTypeFuncref},
				},
				"table_index=1": &TableDefinition{
					index:       1,
					exportNames: []string{"table_index=1"},
					table:       &Table{Min: 2, Max: &max, Type: RefTypeExternref},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.m.BuildTableDefinitions()
			require.Equal(t, tc.expected, tc.m.TableDefinitionSection)
			require.Equal(t, tc.expectedImports, tc.m.ImportedTables())
			require.Equal(t, tc.expectedExports, tc.m.ExportedTables())
		})
	}
}
