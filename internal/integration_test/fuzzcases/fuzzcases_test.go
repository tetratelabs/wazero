package fuzzcases

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

var ctx = context.Background()

//go:embed testdata/*.wasm
var testcases embed.FS

func getWasmBinary(t *testing.T, number int) []byte {
	ret, err := testcases.ReadFile(fmt.Sprintf("testdata/%d.wasm", number))
	require.NoError(t, err)
	return ret
}

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
		module, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 695))
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
		module, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 696))
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
		_, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 699))
		require.NoError(t, err)
	})
}

// Test701 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test701(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 701))
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
		_, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 704))
		require.NoError(t, err)
	})
}

func Test708(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 708))
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test709(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 709))
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
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 715))
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
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 716))
		require.NoError(t, err)

		f := mod.ExportedFunction("select on ref.func")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), res[0])
	})
}

func Test717(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 717))
		require.NoError(t, err)

		f := mod.ExportedFunction("vectors")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)

		const expectedLen = 35
		require.Equal(t, expectedLen, len(res))
		for i := 0; i < expectedLen; i++ {
			require.Equal(t, uint64(i), res[i])
		}
	})
}

func Test718(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 718))
		require.NoError(t, err)

		f := mod.ExportedFunction("v128.load_zero on the ceil")
		require.NotNil(t, f)
		_, err = f.Call(ctx)
		require.NoError(t, err)
	})
}

func Test719(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 719))
		require.NoError(t, err)

		f := mod.ExportedFunction("require unreachable")
		require.NotNil(t, f)
		_, err = f.Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "wasm error: unreachable\nwasm stack trace:")
	})
}

func Test720(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 720))
		require.NoError(t, err)

		f := mod.ExportedFunction("access memory after table.grow")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)
		require.Equal(t, uint32(0xffffffff), uint32(res[0]))
	})
}

func Test721(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 721))
		require.NoError(t, err)

		f := mod.ExportedFunction("conditional before elem.drop")
		require.NotNil(t, f)
		ret, err := f.Call(ctx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), ret[0])
	})
}

func Test722(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 722))
		require.NoError(t, err)

		f := mod.ExportedFunction("conditional before data.drop")
		require.NotNil(t, f)
		ret, err := f.Call(ctx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), ret[0])
	})
}

func Test725(t *testing.T) {
	functions := []string{"i32.load8_s", "i32.load16_s"}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.InstantiateModuleFromBinary(ctx, getWasmBinary(t, 725))
		require.NoError(t, err)

		for _, fn := range functions {
			f := mod.ExportedFunction(fn)
			require.NotNil(t, f)
			_, err := f.Call(ctx)
			require.Error(t, err)
			require.Contains(t, err.Error(), "out of bounds memory")
		}
	})
}
