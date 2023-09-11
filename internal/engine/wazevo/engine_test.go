package wazevo

import (
	"context"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_builtinFunctionFinalizer(t *testing.T) {
	bf := &builtinFunctions{}

	b1, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)

	b2, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	bf.memoryGrowExecutable = b1
	bf.stackGrowExecutable = b2

	builtinFunctionFinalizer(bf)
	require.Nil(t, bf.memoryGrowExecutable)
	require.Nil(t, bf.stackGrowExecutable)
}

func Test_compiledModuleFinalizer(t *testing.T) {
	cm := &compiledModule{}

	b, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	cm.executable = b
	compiledModuleFinalizer(cm)
	require.Nil(t, cm.executable)
}

type fakeFinalizer map[*compiledModule]func(module *compiledModule)

func (f fakeFinalizer) setFinalizer(obj interface{}, finalizer interface{}) {
	cf := obj.(*compiledModule)
	if _, ok := f[cf]; ok { // easier than adding a field for testing.T
		panic(fmt.Sprintf("BUG: %v already had its finalizer set", cf))
	}
	f[cf] = finalizer.(func(*compiledModule))
}

func TestEngine_CompileModule(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx, 0, nil).(*engine)
	ff := fakeFinalizer{}
	e.setFinalizer = ff.setFinalizer

	okModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0, 0, 0, 0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
		},
		ID: wasm.ModuleID{},
	}

	err := e.CompileModule(ctx, okModule, nil, false)
	require.NoError(t, err)

	// Compiling same module shouldn't be compiled again, but instead should be cached.
	err = e.CompileModule(ctx, okModule, nil, false)
	require.NoError(t, err)

	// Pretend the finalizer executed, by invoking them one-by-one.
	for k, v := range ff {
		v(k)
	}
}
