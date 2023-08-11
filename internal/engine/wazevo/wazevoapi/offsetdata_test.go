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
				TotalSize:              0,
			},
		},
		{
			name: "local mem",
			m:    &wasm.Module{MemorySection: &wasm.Memory{}},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       0,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
				TotalSize:              16,
			},
		},
		{
			name: "imported mem",
			m:    &wasm.Module{ImportMemoryCount: 1},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    0,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
				TotalSize:              8,
			},
		},
		{
			name: "imported func",
			m:    &wasm.Module{ImportFunctionCount: 10},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: 0,
				GlobalsBegin:           -1,
				TotalSize:              160,
			},
		},
		{
			name: "imported func/mem",
			m:    &wasm.Module{ImportMemoryCount: 1, ImportFunctionCount: 10},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    0,
				ImportedFunctionsBegin: 8,
				GlobalsBegin:           -1,
				TotalSize:              168,
			},
		},
		{
			name: "local mem / imported func / globals",
			m: &wasm.Module{
				ImportGlobalCount:   10,
				ImportFunctionCount: 10,
				MemorySection:       &wasm.Memory{},
				GlobalSection:       make([]wasm.Global, 20),
			},
			exp: ModuleContextOffsetData{
				LocalMemoryBegin:       0,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: 16,
				GlobalsBegin:           16 + 16*10,
				TotalSize:              16 + 16*10 + 8*30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewModuleContextOffsetData(tc.m)
			require.Equal(t, tc.exp, got)
		})
	}
}
