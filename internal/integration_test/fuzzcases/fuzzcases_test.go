package fuzzcases

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

var ctx = context.Background()

var (
	//go:embed testdata/695.wasm
	case695 []byte
	//go:embed testdata/696.wasm
	case696 []byte
	//go:embed testdata/699.wasm
	case699 []byte
	//go:embed testdata/701.wasm
	case701 []byte
	//go:embed testdata/704.wasm
	case704 []byte
	//go:embed testdata/708.wasm
	case708 []byte
	//go:embed testdata/709.wasm
	case709 []byte
	//go:embed testdata/715.wasm
	case715 []byte
	//go:embed testdata/716.wasm
	case716 []byte
)

func runWithCompiler(t *testing.T, runner func(t *testing.T, r wazero.Runtime)) {
	if !platform.CompilerSupported() {
		return
	}
	t.Run("compiler", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigCompiler().WithWasmCore2())
		defer r.Close(ctx)
		runner(t, r)
	})
}

func runWithInterpreter(t *testing.T, runner func(t *testing.T, r wazero.Runtime)) {
	t.Run("interpreter", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter().WithWasmCore2())
		defer r.Close(ctx)
		runner(t, r)
	})
}

func run(t *testing.T, runner func(t *testing.T, r wazero.Runtime)) {
	runWithInterpreter(t, runner)
	runWithCompiler(t, runner)
}

// Test695 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test695(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.InstantiateModuleFromBinary(ctx, case695)
		require.NoError(t, err)

		_, err = module.ExportedFunction("i8x16s").Call(ctx)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")

		_, err = module.ExportedFunction("i16x8s").Call(ctx)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test696(t *testing.T) {
	functionNames := [4]string{
		"select with 0 / after calling dummy",
		"select with 0",
		"typed select with 1 / after calling dummy",
		"typed select with 1",
	}

	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.InstantiateModuleFromBinary(ctx, case696)
		require.NoError(t, err)

		for _, name := range functionNames {
			_, err := module.ExportedFunction(name).Call(ctx)
			require.NoError(t, err)
		}
	})
}

// Test699 ensures that accessing element instances and data instances works
// without crash even when the access happens in the nested function call.
func Test699(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		defer r.Close(ctx)
		_, err := r.InstantiateModuleFromBinary(ctx, case699)
		require.NoError(t, err)
	})
}

// Test701 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test701(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.InstantiateModuleFromBinary(ctx, case701)
		require.NoError(t, err)

		_, err = module.ExportedFunction("i32.extend16_s").Call(ctx)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")

		_, err = module.ExportedFunction("i32.extend8_s").Call(ctx)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test704(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.InstantiateModuleFromBinary(ctx, case704)
		require.NoError(t, err)
	})
}

func Test708(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.InstantiateModuleFromBinary(ctx, case708)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test709(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, case709)
		require.NoError(t, err)

		f := mod.ExportedFunction("f64x2.promote_low_f32x4")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)

		require.NotEqual(t, uint64(0), res[0])
		require.NotEqual(t, uint64(0), res[1])
	})
}

func Test715(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, case715)
		require.NoError(t, err)

		f := mod.ExportedFunction("select on conditional value after table.size")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), res[0])
	})
}

func Test716(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, case716)
		require.NoError(t, err)

		f := mod.ExportedFunction("select on ref.func")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), res[0])
	})
}
