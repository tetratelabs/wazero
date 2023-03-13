package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// compile-time check to ensure recorder implements FunctionListenerFactory
var _ FunctionListenerFactory = &recorder{}

type recorder struct {
	m                       map[string]struct{}
	beforeNames, afterNames []string
}

func (r *recorder) Before(ctx context.Context, _ api.Module, def api.FunctionDefinition, _ []uint64) context.Context {
	r.beforeNames = append(r.beforeNames, def.DebugName())
	return ctx
}

func (r *recorder) After(_ context.Context, _ api.Module, def api.FunctionDefinition, _ error, _ []uint64) {
	r.afterNames = append(r.afterNames, def.DebugName())
}

func (r *recorder) NewListener(definition api.FunctionDefinition) FunctionListener {
	r.m[definition.Name()] = struct{}{}
	return r
}

func TestFunctionListenerFactory(t *testing.T) {
	// Set context to one that has an experimental listener
	factory := &recorder{m: map[string]struct{}{}}
	ctx := context.WithValue(context.Background(), FunctionListenerFactoryKey{}, factory)

	// Define a module with two functions
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		ImportSection:   []wasm.Import{{}},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []*wasm.Code{
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

	_, err := r.NewHostModuleBuilder("").NewFunctionBuilder().WithFunc(func() {}).Export("").Instantiate(ctx)
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
