package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// compile-time check to ensure recorder implements FunctionListenerFactory
var _ FunctionListenerFactory = &recorder{}

type recorder struct {
	m map[string]struct{}
}

func (r *recorder) NewListener(definition api.FunctionDefinition) FunctionListener {
	r.m[definition.Name()] = struct{}{}
	return nil
}

func TestFunctionListenerFactory(t *testing.T) {
	// Set context to one that has an experimental listener
	factory := &recorder{map[string]struct{}{}}
	ctx := context.WithValue(context.Background(), FunctionListenerFactoryKey{}, factory)

	// Define a module with two functions
	bin := binary.EncodeModule(&wasm.Module{
		TypeSection:     []*wasm.FunctionType{{}},
		ImportSection:   []*wasm.Import{{}},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
		},
		NameSection: &wasm.NameSection{
			ModuleName: "test",
			FunctionNames: wasm.NameMap{
				{Index: 0, Name: "import"}, // should skip
				{Index: 1, Name: "fn1"},
				{Index: 2, Name: "fn2"},
			},
		},
	})

	if platform.CompilerSupported() {
		t.Run("fails on compile if compiler", func(t *testing.T) {
			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigCompiler())
			defer r.Close(testCtx) // This closes everything this Runtime created.
			_, err := r.CompileModule(ctx, bin, wazero.NewCompileConfig())
			require.EqualError(t, err,
				"context includes a FunctionListenerFactoryKey, which is only supported in the interpreter")
		})
	}

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx) // This closes everything this Runtime created.

	_, err := r.CompileModule(ctx, bin, wazero.NewCompileConfig())
	require.NoError(t, err)

	// Ensure each function was converted to a listener eagerly
	require.Equal(t, map[string]struct{}{
		"fn1": {},
		"fn2": {},
	}, factory.m)
}
