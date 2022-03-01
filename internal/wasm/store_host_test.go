package internalwasm

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestStore_AddHostFunction(t *testing.T) {
	t.Skip() // TODO: fix
	s := NewStore(context.Background(), &catchContext{})

	hf, err := NewGoFunc("fn", func(wasm.ModuleContext) {})
	require.NoError(t, err)

	// Add the host module
	hostModule := &ModuleInstance{Name: "test", Exports: make(map[string]*ExportInstance, 1)}
	s.moduleInstances[hostModule.Name] = hostModule

	err = s.exportHostFunction(hostModule, hf)
	require.NoError(t, err)

	// The function was added to the store, prefixed by the owning module name
	require.Equal(t, 1, len(s.functions))
	fn := s.functions[0]
	require.Equal(t, "test.fn", fn.Name)

	// The function was exported in the module
	require.Equal(t, 1, len(hostModule.Exports))
	exp, ok := hostModule.Exports["fn"]
	require.True(t, ok)

	// Trying to register it again should fail
	err = s.exportHostFunction(hostModule, hf)
	require.EqualError(t, err, `"fn" is already exported in module "test"`)

	// Any side effects should be reverted
	require.Equal(t, []*FunctionInstance{fn, nil}, s.functions)
	require.Equal(t, map[string]*ExportInstance{"fn": exp}, hostModule.Exports)
}

func TestStore_ExportImportedHostFunction(t *testing.T) {
	s := NewStore(context.Background(), &catchContext{})

	hf, err := NewGoFunc("host_fn", func(wasm.ModuleContext) {})
	require.NoError(t, err)

	// Add the host module
	hostModule := &ModuleInstance{Name: "", Exports: make(map[string]*ExportInstance, 1)}
	s.moduleInstances[hostModule.Name] = hostModule
	err = s.exportHostFunction(hostModule, hf)
	require.NoError(t, err)

	t.Run("ModuleInstance is the importing module", func(t *testing.T) {
		_, err = s.Instantiate(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Type: ExternTypeFunc, Name: "host_fn", DescFunc: 0}},
			MemorySection: []*MemoryType{{1, nil}},
			ExportSection: map[string]*Export{"host.fn": {Type: ExternTypeFunc, Name: "host.fn", Index: 0}},
		}, "test")
		require.NoError(t, err)

		ei, err := s.getExport("test", "host.fn", ExternTypeFunc)
		require.NoError(t, err)
		os.Environ()
		// We expect the host function to be called in context of the importing module.
		// Otherwise, it would be the pseudo-module of the host, which only includes types and function definitions.
		// Notably, this ensures the host function call context has the correct memory (from the importing module).
		require.Equal(t, s.moduleInstances["test"], ei.Function.ModuleInstance)
	})
}
