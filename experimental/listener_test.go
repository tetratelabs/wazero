package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/wazerotest"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// compile-time check to ensure recorder implements FunctionListenerFactory
var _ experimental.FunctionListenerFactory = &recorder{}

type recorder struct {
	m           map[string]struct{}
	beforeNames []string
	afterNames  []string
	abortNames  []string
}

func (r *recorder) Before(ctx context.Context, _ api.Module, def api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
	r.beforeNames = append(r.beforeNames, def.DebugName())
}

func (r *recorder) After(_ context.Context, _ api.Module, def api.FunctionDefinition, _ []uint64) {
	r.afterNames = append(r.afterNames, def.DebugName())
}

func (r *recorder) Abort(_ context.Context, _ api.Module, def api.FunctionDefinition, _ error) {
	r.abortNames = append(r.abortNames, def.DebugName())
}

func (r *recorder) NewFunctionListener(definition api.FunctionDefinition) experimental.FunctionListener {
	r.m[definition.Name()] = struct{}{}
	return r
}

func TestFunctionListenerFactory(t *testing.T) {
	// Set context to one that has an experimental listener
	factory := &recorder{m: map[string]struct{}{}}
	ctx := experimental.WithFunctionListenerFactory(context.Background(), factory)

	// Define a module with two functions
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		ImportSection:   []wasm.Import{{Module: "host"}},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			// fn1
			{Body: []byte{
				// call fn2 twice
				wasm.OpcodeCall, 2,
				wasm.OpcodeCall, 2,
				wasm.OpcodeEnd,
			}},
			// fn2
			{Body: []byte{wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{{Name: "fn1", Type: wasm.ExternTypeFunc, Index: 1}},
		NameSection: &wasm.NameSection{
			ModuleName: "test",
			FunctionNames: wasm.NameMap{
				{Index: 0, Name: "import"}, // should skip for building listeners.
				{Index: 1, Name: "fn1"},
				{Index: 2, Name: "fn2"},
			},
		},
	})

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	_, err := r.NewHostModuleBuilder("host").NewFunctionBuilder().WithFunc(func() {}).Export("").Instantiate(ctx)
	require.NoError(t, err)

	// Ensure the imported function was converted to a listener.
	require.Equal(t, map[string]struct{}{"": {}}, factory.m)

	compiled, err := r.CompileModule(ctx, bin)
	require.NoError(t, err)

	// Ensure each function was converted to a listener eagerly
	require.Equal(t, map[string]struct{}{
		"":    {},
		"fn1": {},
		"fn2": {},
	}, factory.m)

	// Ensures that FunctionListener is a compile-time option, so passing
	// context.Background here is ok to use listeners at runtime.
	m, err := r.InstantiateModule(context.Background(), compiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	fn1 := m.ExportedFunction("fn1")
	require.NotNil(t, fn1)

	_, err = fn1.Call(context.Background())
	require.NoError(t, err)

	require.Equal(t, []string{"test.fn1", "test.fn2", "test.fn2"}, factory.beforeNames)
	require.Equal(t, []string{"test.fn2", "test.fn2", "test.fn1"}, factory.afterNames) // after is in the reverse order.
}

func TestMultiFunctionListenerFactory(t *testing.T) {
	module := wazerotest.NewModule(nil,
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
	)

	stack := []experimental.StackFrame{
		{Function: module.Function(0), Params: []uint64{1}},
		{Function: module.Function(1), Params: []uint64{2}},
		{Function: module.Function(2), Params: []uint64{3}},
	}

	n := 0
	f := func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, stackIterator experimental.StackIterator) {
		n++
		i := 0
		for stackIterator.Next() {
			i++
		}
		if i != 3 {
			t.Errorf("wrong number of call frames: want=3 got=%d", i)
		}
	}

	factory := experimental.MultiFunctionListenerFactory(
		experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
			return experimental.FunctionListenerFunc(f)
		}),
		experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
			return experimental.FunctionListenerFunc(f)
		}),
	)

	function := module.Function(0).Definition()
	listener := factory.NewFunctionListener(function)
	listener.Before(context.Background(), module, function, stack[2].Params, experimental.NewStackIterator(stack...))

	if n != 2 {
		t.Errorf("wrong number of function calls: want=2 got=%d", n)
	}
}

func BenchmarkMultiFunctionListener(b *testing.B) {
	module := wazerotest.NewModule(nil,
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module, value int32) {}),
	)

	stack := []experimental.StackFrame{
		{Function: module.Function(0), Params: []uint64{1}},
		{Function: module.Function(1), Params: []uint64{2}},
		{Function: module.Function(2), Params: []uint64{3}},
	}

	tests := []struct {
		scenario string
		function func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator)
	}{
		{
			scenario: "simple function listener",
			function: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, stackIterator experimental.StackIterator) {
			},
		},
		{
			scenario: "stack iterator",
			function: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, stackIterator experimental.StackIterator) {
				for stackIterator.Next() {
				}
			},
		},
	}

	for _, test := range tests {
		b.Run(test.scenario, func(b *testing.B) {
			factory := experimental.MultiFunctionListenerFactory(
				experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
					return experimental.FunctionListenerFunc(test.function)
				}),
				experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
					return experimental.FunctionListenerFunc(test.function)
				}),
			)
			function := module.Function(0).Definition()
			listener := factory.NewFunctionListener(function)
			experimental.BenchmarkFunctionListener(b.N, module, stack, listener)
		})
	}
}
