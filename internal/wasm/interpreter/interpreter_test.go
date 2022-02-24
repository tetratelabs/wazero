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

func TestInterpreter_PushFrame(t *testing.T) {
	f1 := &interpreterFrame{}
	f2 := &interpreterFrame{}

	it := engine{}
	require.Empty(t, it.frames)

	it.pushFrame(f1)
	require.Equal(t, []*interpreterFrame{f1}, it.frames)

	it.pushFrame(f2)
	require.Equal(t, []*interpreterFrame{f1, f2}, it.frames)
}

func TestInterpreter_PushFrame_StackOverflow(t *testing.T) {
	defer func() { callStackCeiling = buildoptions.CallStackCeiling }()

	callStackCeiling = 3

	f1 := &interpreterFrame{}
	f2 := &interpreterFrame{}
	f3 := &interpreterFrame{}
	f4 := &interpreterFrame{}

	it := engine{}
	it.pushFrame(f1)
	it.pushFrame(f2)
	it.pushFrame(f3)
	require.Panics(t, func() { it.pushFrame(f4) })
}

func TestEngine_Call(t *testing.T) {
	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
	}

	// Use exported functions to simplify instantiation of a Wasm function
	e := NewEngine()
	store := wasm.NewStore(context.Background(), e)
	_, err := store.Instantiate(m, "")
	require.NoError(t, err)

	// ensure base case doesn't fail
	results, err := e.Call(store.ModuleContexts[""], store.Functions[0], 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := e.Call(store.ModuleContexts[""], store.Functions[0])
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := e.Call(store.ModuleContexts[""], store.Functions[0], 1, 2)
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
