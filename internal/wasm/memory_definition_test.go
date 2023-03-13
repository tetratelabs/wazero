package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_BuildMemoryDefinitions(t *testing.T) {
	tests := []struct {
		name            string
		m               *Module
		expected        []MemoryDefinition
		expectedImports []api.MemoryDefinition
		expectedExports map[string]api.MemoryDefinition
	}{
		{
			name:            "no exports",
			m:               &Module{},
			expectedExports: map[string]api.MemoryDefinition{},
		},
		{
			name: "no memories",
			m: &Module{
				ExportSection: []Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection: []Global{{}},
			},
			expectedExports: map[string]api.MemoryDefinition{},
		},
		{
			name:            "defines memory{0,}",
			m:               &Module{MemorySection: &Memory{Min: 0}},
			expected:        []MemoryDefinition{{index: 0, memory: &Memory{Min: 0}}},
			expectedExports: map[string]api.MemoryDefinition{},
		},
		{
			name: "exports defined memory{2,3}",
			m: &Module{
				ExportSection: []Export{
					{Name: "memory_index=0", Type: ExternTypeMemory, Index: 0},
					{Name: "", Type: ExternTypeGlobal, Index: 0},
				},
				GlobalSection: []Global{{}},
				MemorySection: &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
			},
			expected: []MemoryDefinition{
				{
					index:       0,
					exportNames: []string{"memory_index=0"},
					memory:      &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				},
			},
			expectedExports: map[string]api.MemoryDefinition{
				"memory_index=0": &MemoryDefinition{
					index:       0,
					exportNames: []string{"memory_index=0"},
					memory:      &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				},
			},
		},
		{ // NOTE: not yet supported https://github.com/WebAssembly/multi-memory
			name: "exports imported memory{0,} and defined memory{2,3}",
			m: &Module{
				ImportSection: []Import{{
					Type:    ExternTypeMemory,
					DescMem: &Memory{Min: 0},
				}},
				ExportSection: []Export{
					{Name: "imported_memory", Type: ExternTypeMemory, Index: 0},
					{Name: "memory_index=1", Type: ExternTypeMemory, Index: 1},
				},
				MemorySection: &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
			},
			expected: []MemoryDefinition{
				{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_memory"},
					memory:      &Memory{Min: 0},
				},
				{
					index:       1,
					exportNames: []string{"memory_index=1"},
					memory:      &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				},
			},
			expectedImports: []api.MemoryDefinition{
				&MemoryDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_memory"},
					memory:      &Memory{Min: 0},
				},
			},
			expectedExports: map[string]api.MemoryDefinition{
				"imported_memory": &MemoryDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_memory"},
					memory:      &Memory{Min: 0},
				},
				"memory_index=1": &MemoryDefinition{
					index:       1,
					exportNames: []string{"memory_index=1"},
					memory:      &Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.m.BuildMemoryDefinitions()
			require.Equal(t, tc.expected, tc.m.MemoryDefinitionSection)
			require.Equal(t, tc.expectedImports, tc.m.ImportedMemories())
			require.Equal(t, tc.expectedExports, tc.m.ExportedMemories())
		})
	}
}
