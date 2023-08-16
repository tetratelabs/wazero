package wazevoapi

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestNewModuleContextOffsetData(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    *wasm.Module
		exp  ModuleContextOffsetData
	}{
		{
			name: "empty",
			m:    &wasm.Module{},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
				TablesBegin:            -1,
				TotalSize:              8,
			},
		},
		{
			name: "local mem",
			m:    &wasm.Module{MemorySection: &wasm.Memory{}},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       8,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
				TablesBegin:            -1,
				TotalSize:              24,
			},
		},
		{
			name: "imported mem",
			m:    &wasm.Module{ImportMemoryCount: 1},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    8,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
				TablesBegin:            -1,
				TotalSize:              24,
			},
		},
		{
			name: "imported func",
			m:    &wasm.Module{ImportFunctionCount: 10},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: 8,
				GlobalsBegin:           -1,
				TablesBegin:            -1,
				TotalSize:              10*FunctionInstanceSize + 8,
			},
		},
		{
			name: "imported func/mem",
			m:    &wasm.Module{ImportMemoryCount: 1, ImportFunctionCount: 10},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    8,
				ImportedFunctionsBegin: 24,
				GlobalsBegin:           -1,
				TablesBegin:            -1,
				TotalSize:              10*FunctionInstanceSize + 24,
			},
		},
		{
			name: "local mem / imported func / globals / tables",
			m: &wasm.Module{
				ImportGlobalCount:   10,
				ImportFunctionCount: 10,
				ImportTableCount:    5,
				TableSection:        make([]wasm.Table, 10),
				MemorySection:       &wasm.Memory{},
				GlobalSection:       make([]wasm.Global, 20),
			},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       8,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: 24,
				GlobalsBegin:           24 + 10*FunctionInstanceSize,
				TablesBegin:            24 + 10*FunctionInstanceSize + 8*30,
				TotalSize:              24 + 10*FunctionInstanceSize + 8*30 + 8*15,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewModuleContextOffsetData(tc.m)
			require.Equal(t, tc.exp, got)
		})
	}
}
