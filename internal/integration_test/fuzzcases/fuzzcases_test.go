package fuzzcases

import (
	"context"
	"embed"
	"encoding/hex"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/nodiff"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var ctx = context.Background()

// Note: the name of the test is the PR number. It may be followed by a letter
// if the PR includes more than one test (e.g. "1234a", "1234b").
//
//go:embed testdata/*.wasm
var testcases embed.FS

func getWasmBinary(t *testing.T, testId string) []byte {
	ret, err := testcases.ReadFile(fmt.Sprintf("testdata/%s.wasm", testId))
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
		module, err := r.Instantiate(ctx, getWasmBinary(t, "695"))
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
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.Instantiate(ctx, getWasmBinary(t, "696"))
		require.NoError(t, err)
		for _, tc := range []struct {
			fnName string
			in     uint64
			exp    [2]uint64
		}{
			{fnName: "select", in: 1 << 5, exp: [2]uint64{0xffffffffffffffff, 0xeeeeeeeeeeeeeeee}},
			{fnName: "select", in: 1, exp: [2]uint64{0xffffffffffffffff, 0xeeeeeeeeeeeeeeee}},
			{fnName: "select", in: 0, exp: [2]uint64{0x1111111111111111, 0x2222222222222222}},
			{fnName: "select", in: 0xffffff, exp: [2]uint64{0xffffffffffffffff, 0xeeeeeeeeeeeeeeee}},
			{fnName: "select", in: 0xffff00, exp: [2]uint64{0xffffffffffffffff, 0xeeeeeeeeeeeeeeee}},
			{fnName: "select", in: 0x000000, exp: [2]uint64{0x1111111111111111, 0x2222222222222222}},
			{fnName: "typed select", in: 1, exp: [2]uint64{0xffffffffffffffff, 0xeeeeeeeeeeeeeeee}},
			{fnName: "typed select", in: 0, exp: [2]uint64{0x1111111111111111, 0x2222222222222222}},
		} {
			res, err := module.ExportedFunction(tc.fnName).Call(ctx, tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.exp[:], res)
		}
	})
}

// Test699 ensures that accessing element instances and data instances works
// without crash even when the access happens in the nested function call.
func Test699(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		defer r.Close(ctx)
		_, err := r.Instantiate(ctx, getWasmBinary(t, "699"))
		require.NoError(t, err)
	})
}

// Test701 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test701(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		module, err := r.Instantiate(ctx, getWasmBinary(t, "701"))
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
		_, err := r.Instantiate(ctx, getWasmBinary(t, "704"))
		require.NoError(t, err)
	})
}

func Test708(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, "708"))
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "out of bounds memory access")
	})
}

func Test709(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "709"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "715"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "716"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "717"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "718"))
		require.NoError(t, err)

		f := mod.ExportedFunction("v128.load_zero on the ceil")
		require.NotNil(t, f)
		_, err = f.Call(ctx)
		require.NoError(t, err)
	})
}

func Test719(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "719"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "720"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "721"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "722"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "725"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "730"))
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
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "733"))
		require.NoError(t, err)

		t.Run("out of bounds", func(t *testing.T) {
			f := mod.ExportedFunction("out of bounds")
			require.NotNil(t, f)
			_, err = f.Call(ctx)
			require.Error(t, err)
			require.Contains(t, err.Error(), "out of bounds memory")
		})

		t.Run("store higher offset", func(t *testing.T) {
			if testing.Short() {
				// Note: this case uses large memory space, so can be slow like 1 to 2 seconds even without -race.
				// The reason is that this test requires roughly 2GB of in-Wasm memory.
				t.SkipNow()
			}
			f := mod.ExportedFunction("store higher offset")
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
		_, err := r.Instantiate(ctx, getWasmBinary(t, "873"))
		require.NoError(t, err)
	})
}

func Test874(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, "874"))
		require.NoError(t, err)
	})
}

func Test888(t *testing.T) {
	// This tests that importing FuncRef type globals and using it as an initialization of the locally-defined
	// FuncRef global works fine.
	run(t, func(t *testing.T, r wazero.Runtime) {
		imported := binaryencoding.EncodeModule(&wasm.Module{
			MemorySection: &wasm.Memory{Min: 0, Max: 5, IsMaxEncoded: true},
			GlobalSection: []wasm.Global{
				{
					Type: wasm.GlobalType{
						ValType: wasm.ValueTypeFuncref,
						Mutable: false,
					},
					Init: wasm.ConstantExpression{
						Opcode: wasm.OpcodeRefNull,
						Data:   []byte{wasm.ValueTypeFuncref},
					},
				},
			},
			ExportSection: []wasm.Export{
				{Name: "", Type: wasm.ExternTypeGlobal, Index: 0},
				{Name: "s", Type: wasm.ExternTypeMemory, Index: 0},
			},
		})

		_, err := r.InstantiateWithConfig(ctx, imported, wazero.NewModuleConfig().WithName("host"))
		require.NoError(t, err)

		_, err = r.InstantiateWithConfig(ctx, getWasmBinary(t, "888"),
			wazero.NewModuleConfig().WithName("test"))
		require.NoError(t, err)
	})
}

func Test1054(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	modules := make([]api.Module, 0, 3)
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1054"))
		require.NoError(t, err)
		modules = append(modules, mod)
	})

	// Checks if the memory state is the same between engines.
	exp := modules[0].Memory().(*wasm.MemoryInstance).Buffer
	for i := 1; i < len(modules); i++ {
		actual := modules[i].Memory().(*wasm.MemoryInstance).Buffer
		require.Equal(t, exp, actual)
	}
}

// Test1777 tests that br_table with multiple args works fine even if
// there might be phi eliminations.
func Test1777(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1777"))
		require.NoError(t, err)
		f := mod.ExportedFunction("")
		require.NotNil(t, f)
		res, err := f.Call(ctx)
		require.NoError(t, err)
		require.Equal(t, []uint64{18446626425965379583, 4607736361554183979}, res)
	})
}

// Test1792a tests that v128.const i32x4 is not skipped when state is unreachable.
// This test fails at build-time.
func Test1792a(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, "1792a"))
		require.NoError(t, err)
	})
}

// Test1792b tests that OpcodeVhighBits (v128.Bitmask) is typed as V128.
// This test fails at build-time.
func Test1792b(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, "1792b"))
		require.NoError(t, err)
	})
}

// Test1792c tests that OpcodeVFcmp (f32x4.eq) is typed as V128.
func Test1792c(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1792c"))
		require.NoError(t, err)
		f := mod.ExportedFunction("")
		require.NotNil(t, f)
		_, err = f.Call(ctx, 0, 0, 0)
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)

		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(5044022786561933312), lo)
		require.Equal(t, uint64(9205357640488583168), hi)
	})
}

// Test1793a tests that OpcodeVAllTrue is lowered to the right registers.
func Test1793a(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1793a"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		lo, hi := m.Globals[2].Value()
		require.Equal(t, uint64(2531906066518671488), lo)
		require.Equal(t, uint64(18446744073709551615), hi)
	})
}

// Test1793b tests that OpcodeVIcmp, OpcodeVFcmp are lowered to the right registers.
func Test1793b(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1793b"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx, 0, 0, 0, 0)
		require.NoError(t, err)
		lo, hi := m.Globals[1].Value()
		require.Equal(t, uint64(18374967954648334335), lo)
		require.Equal(t, uint64(18446744073709551615), hi)
	})
}

// Test1793c tests that OpcodeVIcmp is lowered to the right registers.
func Test1793c(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1793c"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx, 0, 0)
		require.NoError(t, err)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(18446744073709551615), lo)
		require.Equal(t, uint64(18446744073709551615), hi)
	})
}

// Test1793c tests that OpcodeVShift is lowered to the right registers.
func Test1793d(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1793d"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(0), m.Globals[1].Val)
	})
}

// Test1797a tests that i8x16.shl uses the right register types when lowered.
func Test1797a(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1797a"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		res, err := m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(0), res[0])
	})
}

// Test1797a tests that i16x8.shr_u uses the right register types when lowered.
func Test1797b(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1797b"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("\x00\x00\x00\x00\x00").Call(ctx, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(2666130977255796624), lo)
		require.Equal(t, uint64(9223142857682330634), hi)
	})
}

// Test1797c tests that the program counter for V128*Shuffle is advanced correctly
// even when an unreachable instruction is present.
func Test1797c(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1797c"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		params := make([]uint64, 20)
		_, err = m.ExportedFunction("~zz\x00E1E\x00EE\x00$").Call(ctx, params...)
		require.Error(t, err, "wasm error: unreachable")
	})
}

// Test1797d tests that the registers are allocated correctly in Vbitselect.
func Test1797d(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1797d"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		params := make([]uint64, 20)
		_, err = m.ExportedFunction("p").Call(ctx, params...)
		require.NoError(t, err)
		lo, hi := m.Globals[2].Value()
		require.Equal(t, uint64(15092115255309870764), lo)
		require.Equal(t, uint64(9241386435284803069), hi)
	})
}

// Test1802 tests that load32_splat computes the load from the right offset
// when a nonzero value is on the stack.
func Test1802(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1802"))
		require.NoError(t, err, "wasm binary should build successfully")
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.Contains(t, err.Error(), "wasm error: unreachable")
	})
}

// Test1812 tests that many constant block params work fine.
func Test1812(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1812"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		res, err := m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		require.Equal(t,
			[]uint64{
				0x8301fd00, 0xfd838783, 0x87878383, 0x9b000087, 0x170001fd,
				0xfd8383fd, 0x87838301, 0x878787, 0x83fd9b00, 0x201fd83, 0x878783,
				0x83fd9b00, 0x9b00fd83, 0xfd8383fd, 0x87838301, 0x87878787,
				0xfd9b0000, 0x87878383, 0x1fd8383,
			}, res)
	})
}

// Test1817 tests that v128.store uses the right memory layout.
func Test1817(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1817"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		buf, ok := m.Memory().Read(15616, 16)
		require.True(t, ok)
		require.Equal(t, []uint8{0, 0, 0, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, buf)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(0x8000000080000000), lo)
		require.Equal(t, uint64(0x8000000080000000), hi)
	})
}

// Test1820 tests that i16x8.narrow_i32x4_u assigns the dest register correctly.
func Test1820(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1820"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		lo, hi := m.Globals[1].Value()
		require.Equal(t, uint64(0xFFFFFFFFFFFF0000), lo)
		require.Equal(t, uint64(0xFFFF), hi)
	})
}

// Test1823 tests that f64x2.pmin lowers to BSL with the right register usage
// (condition register gets overwritten).
func Test1823(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1823"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(17282609607625994159), lo)
		require.Equal(t, uint64(4671060543367625455), hi)
	})
}

// Test1825 tests that OpcodeInsertlane allocates correctly the temporary registers.
func Test1825(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1825"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		lo, hi := m.Globals[6].Value()
		require.Equal(t, uint64(1099511627775), lo)
		require.Equal(t, uint64(18446744073709551615), hi)
	})
}

// Test1826 tests that lowerFcopysignImpl allocates correctly the temporary registers.
func Test1826(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1826"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("3").Call(ctx, 0, 0)
		require.NoError(t, err)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(1608723901141126568), lo)
		require.Equal(t, uint64(0), hi)
	})
}

func Test1846(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1846"))
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		_, err = m.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, math.Float64bits(2), lo)
		require.Equal(t, uint64(0), hi)
	})
}

// Test1847 verifies that an attempt to write a v128 value to an OOB memory location
// does not result in a partial write (e.g. lower 64 bits) to memory.
func Test1949(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	const offset = 65526
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1949"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.Error(t, err)

		read, ok := mod.Memory().Read(offset, 8)
		require.True(t, ok)
		require.Equal(t, []byte{0xfe, 0xca, 0xfe, 0xca, 0, 0, 0, 0}, read)
	})
}

func Test1999(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "1999"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unreachable")
	})
}

func Test2000a(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2000a"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
	})
}

func Test2000b(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2000b"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "integer overflow")
	})
}

func Test2001(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2001"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
	})
}

func Test2006(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2006"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
	})
}

func Test2007(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2007"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "integer overflow")
	})
}

func Test2008(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2008"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unreachable")
	})
}

func Test2009(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2009"))
		require.NoError(t, err)
		res, err := mod.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		require.Equal(t, res, []uint64{0})
	})
}

func Test2017(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2017"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "integer divide by zero")
	})
}

func Test2031(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2031"))
		require.NoError(t, err)
		res, err := mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
		require.Equal(t, []uint64{2139095040, 9218868437227405312}, res)
	})
}

func Test2034(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2034"))
		require.NoError(t, err)
		res, err := mod.ExportedFunction("").Call(ctx, make([]uint64, 20)...)
		require.NoError(t, err)
		require.Equal(t, []uint64{0xf0f0f280f0f280f, 0x0, 0x0, 0x0, 0x0}, res)
	})
}

func Test2037(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2037"))
		require.NoError(t, err)
		res, err := mod.ExportedFunction("").Call(ctx)
		require.NoError(t, err)
		require.Equal(t, []uint64{0x0, 0x0, 0xbbbbbbbbbbbbbbbb, 0xbbbbbbbbbbbbbbbb, 0xcb6151c8d497b060, 0xbbbbbbbbbbbbbbbb, 0xe71c3971a22b233b, 0xa0a0a0a0a0a0a0a, 0x0, 0xfffffffb00000030, 0x0, 0x6c6cbbbbbbbbbbbb, 0xfeb44590ef194fa2, 0x1313131313131313, 0x1898a98e9daf4f22, 0xf8f8f8f80a0a0aa0, 0x6c6c6c6c6c6cf1f8, 0x6c6c6c6c6c6c6c6c, 0x9abbbbbbbbbbbb6c, 0x9a9ad39a9a9a9a9a}, res)
	})
}

func Test2040(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2040"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
	})
}

func Test2057(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2057"))
		require.NoError(t, err)
		res, err := mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
		require.Equal(t, []uint64{0xe2012900e20129, 0xe2012900e20129}, res)
	})
}

func Test2058(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2058"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0, 0, 0)
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(18446744073709551615), lo)
		require.Equal(t, uint64(0), hi)
	})
}

func Test2060(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	run(t, func(t *testing.T, r wazero.Runtime) {
		mod, err := r.Instantiate(ctx, getWasmBinary(t, "2060"))
		require.NoError(t, err)
		_, err = mod.ExportedFunction("").Call(ctx, 0)
		require.NoError(t, err)
		m := mod.(*wasm.ModuleInstance)
		lo, hi := m.Globals[0].Value()
		require.Equal(t, uint64(1), lo)
		require.Equal(t, uint64(1), hi)
		lo, hi = m.Globals[1].Value()
		require.Equal(t, uint64(18446744073709551615), lo)
		require.Equal(t, uint64(18446744073709551615), hi)
	})
}

func Test2070(t *testing.T) {
	run(t, func(t *testing.T, r wazero.Runtime) {
		_, err := r.Instantiate(ctx, getWasmBinary(t, "2070"))
		require.NoError(t, err)
	})
}

func Test2078(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2078"), true, true)
}

func Test2082(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2082"), true, true)
}

func Test2084(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2084"), true, true)
}

func Test2096(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2096"), true, true)
}

func Test2097(t *testing.T) {
	bin, err := hex.DecodeString("0061736d010000000107016000037f7f7f02010003030200000a49022f0010010004001001027d00024002400240024002400240440000000000000000000b0b0b0b0b0b000b0005000b000b1703017c017c017e00000000000000000000000b43420b0b")
	require.NoError(t, err)

	r := wazero.NewRuntime(context.Background())
	_, err = r.Instantiate(ctx, bin)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected end of function")
}

func Test2112(t *testing.T) {
	bin, err := hex.DecodeString("0061736d0100000001050160017e00020100030201000404017000000a06010400fc300b")
	require.NoError(t, err)
	if !platform.CompilerSupported() {
		return
	}

	r := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigCompiler())
	_, err = r.Instantiate(ctx, bin)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid function[0]: unknown misc opcode 0x30")
}

func Test2118(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2118"), true, true)
}

func Test2131(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2131"), true, true)
}

func Test2136(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2136"), true, true)
}

func Test2137(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2137"), true, true)
}

func Test2140(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2140"), true, true)
}

func Test2201(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2201"), false, false)
}

func Test2260(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	nodiff.RequireNoDiffT(t, getWasmBinary(t, "2260"), false, false)
}
