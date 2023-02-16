package fuzzcases

import (
	"context"
	"embed"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
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
		r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
		defer r.Close(ctx)
		runner(t, r)
	})
}

func runWithInterpreter(t *testing.T, runner func(t *testing.T, r wazero.Runtime)) {
	t.Run("interpreter", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
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
		module, err := r.Instantiate(ctx, getWasmBinary(t, 695))
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
		module, err := r.Instantiate(ctx, getWasmBinary(t, 696))
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
		_, err := r.Instantiate(ctx, getWasmBinary(t, 699))
		require.NoError(t, err)
	})
}

// Test701 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test701(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.Instantiate(ctx, getWasmBinary(t, 701))
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
		_, err := r.Instantiate(ctx, getWasmBinary(t, 704))
		require.NoError(t, err)
	})
}

func Test708(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, 708))
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test709(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 709))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 715))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 716))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 717))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 718))
		require.NoError(t, err)

		f := mod.ExportedFunction("v128.load_zero on the ceil")
		require.NotNil(t, f)
		_, err = f.Call(ctx)
		require.NoError(t, err)
	})
}

func Test719(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 719))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 720))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 721))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 722))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 725))
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

// Test730 ensures that the vector min/max operations comply with the spec wrt sign bits of zeros:
//
//   - min(0, 0) = 0, min(-0, 0) = -0, min(0, -0) = -0, min(-0, -0) = -0
//   - max(0, 0) = 0, max(-0, 0) =  0, max(0, -0) =  0, max(-0, -0) = -0
func Test730(t *testing.T) {
	tests := []struct {
		name string
		exp  [2]uint64
	}{
		{name: "f32x4.max", exp: [2]uint64{0x80000000 << 32, 0x00000000}},
		{name: "f32x4.min", exp: [2]uint64{0x80000000, 0x80000000<<32 | 0x80000000}},
		{name: "f64x2.max", exp: [2]uint64{0, 0}},
		{name: "f64x2.min", exp: [2]uint64{1 << 63, 1 << 63}},
		{name: "f64x2.max/mix", exp: [2]uint64{0, 1 << 63}},
		{name: "f64x2.min/mix", exp: [2]uint64{1 << 63, 0}},
	}

	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 730))
		require.NoError(t, err)

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				f := mod.ExportedFunction(tc.name)
				require.NotNil(t, f)
				actual, err := f.Call(ctx)
				require.NoError(t, err)
				require.Equal(t, tc.exp[:], actual)
			})
		}
	})
}

func Test733(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 733))
		require.NoError(t, err)

		name := "out of bounds"
		t.Run(name, func(t *testing.T) {
			f := mod.ExportedFunction(name)
			require.NotNil(t, f)
			_, err = f.Call(ctx)
			require.Error(t, err)
			require.Contains(t, err.Error(), "out of bounds memory")
		})

		name = "store higher offset"
		t.Run(name, func(t *testing.T) {
			if testing.Short() {
				// Note: this case uses large memory space, so can be slow like 1 to 2 seconds even without -race.
				t.SkipNow()
			}
			f := mod.ExportedFunction(name)
			require.NotNil(t, f)
			_, err = f.Call(ctx)
			require.NoError(t, err)

			mem := mod.Memory()
			require.NotNil(t, mem)

			v, ok := mem.ReadUint64Le(0x80000100)
			require.True(t, ok)
			require.Equal(t, uint64(0xffffffffffffffff), v)
		})
	})
}

func Test873(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, 873))
		require.NoError(t, err)
	})
}

func Test874(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, 874))
		require.NoError(t, err)
	})
}

func Test888(t *testing.T) {
	// This tests that importing FuncRef type globals and using it as an initialization of the locally-defined
	// FuncRef global works fine.
	run(t, func(t *testing.T, r wazero.Runtime) {
		imported := binary.EncodeModule(&wasm.Module{
			MemorySection: &wasm.Memory{Min: 0, Max: 5, IsMaxEncoded: true},
			GlobalSection: []*wasm.Global{
				{
					Type: &wasm.GlobalType{
						ValType: wasm.ValueTypeFuncref,
						Mutable: false,
					},
					Init: &wasm.ConstantExpression{
						Opcode: wasm.OpcodeRefNull,
						Data:   []byte{wasm.ValueTypeFuncref},
					},
				},
			},
			ExportSection: []*wasm.Export{
				{Name: "", Type: wasm.ExternTypeGlobal, Index: 0},
				{Name: "s", Type: wasm.ExternTypeMemory, Index: 0},
			},
		})

		_, err := r.Instantiate(ctx, imported)
		require.NoError(t, err)

		_, err = r.InstantiateWithConfig(ctx, getWasmBinary(t, 888),
			wazero.NewModuleConfig().WithName("test"))
		require.NoError(t, err)
	})
}

func Test1054(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	modules := make([]api.Module, 0, 2)
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, 1054))
		require.NoError(t, err)
		modules = append(modules, mod)
	})

	// Checks if the memory state is the same between engines.
	require.Equal(t,
		modules[0].Memory().(*wasm.MemoryInstance).Buffer,
		modules[1].Memory().(*wasm.MemoryInstance).Buffer,
	)
}
