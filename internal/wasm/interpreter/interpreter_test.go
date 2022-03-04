package interpreter

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

func TestCallEngine_PushFrame(t *testing.T) {
	f1 := &callFrame{}
	f2 := &callFrame{}

	vm := callEngine{}
	require.Empty(t, vm.frames)

	vm.pushFrame(f1)
	require.Equal(t, []*callFrame{f1}, vm.frames)

	vm.pushFrame(f2)
	require.Equal(t, []*callFrame{f1, f2}, vm.frames)
}

func TestCallEngine_PushFrame_StackOverflow(t *testing.T) {
	defer func() { callStackCeiling = buildoptions.CallStackCeiling }()

	callStackCeiling = 3

	f1 := &callFrame{}
	f2 := &callFrame{}
	f3 := &callFrame{}
	f4 := &callFrame{}

	vm := callEngine{}
	vm.pushFrame(f1)
	vm.pushFrame(f2)
	vm.pushFrame(f3)
	require.Panics(t, func() { vm.pushFrame(f4) })
}

func TestEngine_Call(t *testing.T) {
	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
		ExportSection:   map[string]*wasm.Export{"fn": {Type: wasm.ExternTypeFunc, Index: 0, Name: "fn"}},
	}

	// Use exported functions to simplify instantiation of a Wasm function
	e := NewEngine()
	store := wasm.NewStore(context.Background(), e, wasm.Features20191205)
	mod, err := store.Instantiate(m, "")
	require.NoError(t, err)

	fn := mod.Function("fn")
	require.NotNil(t, fn)

	// ensure base case doesn't fail
	results, err := fn.Call(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := fn.Call(context.Background())
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := fn.Call(context.Background(), 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func TestEngine_Call_HostFn(t *testing.T) {
	memory := &wasm.MemoryInstance{}
	var ctxMemory publicwasm.Memory
	hostFn := reflect.ValueOf(func(ctx publicwasm.ModuleContext, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := NewEngine()
	module := &wasm.ModuleInstance{MemoryInstance: memory}
	modCtx := wasm.NewModuleContext(context.Background(), e, module)
	f := &wasm.FunctionInstance{
		HostFunction: &hostFn,
		FunctionKind: wasm.FunctionKindGoModuleContext,
		FunctionType: &wasm.TypeInstance{
			Type: &wasm.FunctionType{
				Params:  []wasm.ValueType{wasm.ValueTypeI64},
				Results: []wasm.ValueType{wasm.ValueTypeI64},
			},
		},
		ModuleInstance: module,
	}
	require.NoError(t, e.Compile(f))

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := e.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := e.Call(modCtx, f)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := e.Call(modCtx, f, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}
