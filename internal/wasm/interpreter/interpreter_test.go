package interpreter

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/wasm"
)

func TestInterpreter_PushFrame(t *testing.T) {
	f1 := &interpreterFrame{}
	f2 := &interpreterFrame{}

	it := interpreter{}
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

	it := interpreter{}
	it.pushFrame(f1)
	it.pushFrame(f2)
	it.pushFrame(f3)
	require.Panics(t, func() { it.pushFrame(f4) })
}

func TestInterpreter_CallHostFunc(t *testing.T) {
	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		memory := &internalwasm.MemoryInstance{}
		var ctxMemory wasm.Memory
		hostFn := reflect.ValueOf(func(ctx wasm.HostFunctionCallContext) {
			ctxMemory = ctx.Memory()
		})
		module := &internalwasm.ModuleInstance{Memory: memory}
		it := interpreter{functions: map[internalwasm.FunctionAddress]*interpreterFunction{
			0: {hostFn: &hostFn, funcInstance: &internalwasm.FunctionInstance{
				FunctionType: &internalwasm.TypeInstance{
					Type: &internalwasm.FunctionType{
						Params:  []wasm.ValueType{},
						Results: []wasm.ValueType{},
					},
				},
				ModuleInstance: module,
			},
			},
		}}

		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		it.callHostFunc(newHostFunctionCallContext(&it, module), it.functions[0])
		require.Same(t, memory, ctxMemory)
	})
}

func newHostFunctionCallContext(engine internalwasm.Engine, module *internalwasm.ModuleInstance) *internalwasm.HostFunctionCallContext {
	ctx := internalwasm.NewHostFunctionCallContext(&internalwasm.Store{
		Engine:          engine,
		ModuleInstances: map[string]*internalwasm.ModuleInstance{"test": module},
	}, module)
	return ctx
}
