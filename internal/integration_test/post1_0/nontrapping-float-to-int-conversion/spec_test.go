package nontrapping_float_to_int_conversion

import (
	"context"
	_ "embed"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestNonTrappingFloatToIntConversion_JIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	testNonTrappingFloatToIntConversion(t, wazero.NewRuntimeConfigJIT)
}

func TestNonTrappingFloatToIntConversion_Interpreter(t *testing.T) {
	testNonTrappingFloatToIntConversion(t, wazero.NewRuntimeConfigInterpreter)
}

// conversions includes changes to test/core/conversions.wast from the commit that added
// "nontrapping-float-to-int-conversion" support.
//
// See https://github.com/WebAssembly/spec/commit/c8fd933fa51eb0b511bce027b573aef7ee373726
var nonTrappingFloatToIntConversion = []byte(`(module $conversions.wast
  (func $i32.trunc_sat_f32_s (param f32) (result i32) local.get 0 i32.trunc_sat_f32_s)
  (export "i32.trunc_sat_f32_s" (func $i32.trunc_sat_f32_s))

  (func $i32.trunc_sat_f32_u (param f32) (result i32) local.get 0 i32.trunc_sat_f32_u)
  (export "i32.trunc_sat_f32_u" (func $i32.trunc_sat_f32_u))

  (func $i32.trunc_sat_f64_s (param f64) (result i32) local.get 0 i32.trunc_sat_f64_s)
  (export "i32.trunc_sat_f64_s" (func $i32.trunc_sat_f64_s))

  (func $i32.trunc_sat_f64_u (param f64) (result i32) local.get 0 i32.trunc_sat_f64_u)
  (export "i32.trunc_sat_f64_u" (func $i32.trunc_sat_f64_u))

  (func $i64.trunc_sat_f32_s (param f32) (result i64) local.get 0 i64.trunc_sat_f32_s)
  (export "i64.trunc_sat_f32_s" (func $i64.trunc_sat_f32_s))

  (func $i64.trunc_sat_f32_u (param f32) (result i64) local.get 0 i64.trunc_sat_f32_u)
  (export "i64.trunc_sat_f32_u" (func $i64.trunc_sat_f32_u))

  (func $i64.trunc_sat_f64_s (param f64) (result i64) local.get 0 i64.trunc_sat_f64_s)
  (export "i64.trunc_sat_f64_s" (func $i64.trunc_sat_f64_s))

  (func $i64.trunc_sat_f64_u (param f64) (result i64) local.get 0 i64.trunc_sat_f64_u)
  (export "i64.trunc_sat_f64_u" (func $i64.trunc_sat_f64_u))
)
`)

func testNonTrappingFloatToIntConversion(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("disabled", func(t *testing.T) {
		// Non-trapping Float-to-int Conversions are disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(testCtx, nonTrappingFloatToIntConversion)
		require.Error(t, err)
	})
	t.Run("enabled", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureNonTrappingFloatToIntConversion(true))
		module, err := r.InstantiateModuleFromCode(testCtx, nonTrappingFloatToIntConversion)
		require.NoError(t, err)

		// https://github.com/WebAssembly/spec/commit/c8fd933fa51eb0b511bce027b573aef7ee373726#diff-68f5d3026030825a35400ba547214701a409a89ce4b1bdb525d5eb98a5e03a38R259-R447
		// Skip -0.0 due to SA4026: in Go, the floating-point literal '-0.0' is the same as '0.0'
		// Skip nan:0x200000, -nan, -nan:0x200000 as there is no representation of these in Go.
		testFunctions(t, module, []funcTest{
			{name: "i32.trunc_sat_f32_s", param: 0, expected: 0},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(0x1p-149), expected: 0},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-0x1p-149), expected: 0},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(1.0), expected: 1},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(0x1.19999ap+0), expected: 1},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(1.5), expected: 1},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-1.0), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-0x1.19999ap+0), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-1.5), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-1.9), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-2.0), expected: api.EncodeI32(-2)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(2147483520.0), expected: 2147483520},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-2147483648.0), expected: api.EncodeI32(-2147483648)},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(2147483648.0), expected: 0x7fffffff},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(-2147483904.0), expected: 0x80000000},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(float32(math.Inf(1))), expected: 0x7fffffff},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(float32(math.Inf(-11))), expected: 0x80000000},
			{name: "i32.trunc_sat_f32_s", param: api.EncodeF32(float32(math.NaN())), expected: 0},

			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(0.0), expected: 0},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(0x1p-149), expected: 0},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(-0x1p-149), expected: 0},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(1.0), expected: 1},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(0x1.19999ap+0), expected: 1},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(1.5), expected: 1},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(1.9), expected: 1},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(2.0), expected: 2},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(2147483648), expected: api.EncodeI32(-2147483648)}, // 0x1.00000p+31 -> 8000 0000
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(4294967040.0), expected: api.EncodeI32(-256)},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(-0x1.ccccccp-1), expected: 0},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(-0x1.fffffep-1), expected: 0},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(4294967296.0), expected: 0xffffffff},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(-1.0), expected: 0x00000000},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(float32(math.Inf(1))), expected: 0xffffffff},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(float32(math.Inf(-1))), expected: 0x00000000},
			{name: "i32.trunc_sat_f32_u", param: api.EncodeF32(float32(math.NaN())), expected: 0},

			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(0.0), expected: 0},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(0x0.0000000000001p-1022), expected: 0},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-0x0.0000000000001p-1022), expected: 0},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(1.0), expected: 1},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(0x1.199999999999ap+0), expected: 1},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(1.5), expected: 1},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-1.0), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-0x1.199999999999ap+0), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-1.5), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-1.9), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-2.0), expected: api.EncodeI32(-2)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(2147483647.0), expected: 2147483647},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-2147483648.0), expected: api.EncodeI32(-2147483648)},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(2147483648.0), expected: 0x7fffffff},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(-2147483649.0), expected: 0x80000000},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(math.Inf(1)), expected: 0x7fffffff},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(math.Inf(-1)), expected: 0x80000000},
			{name: "i32.trunc_sat_f64_s", param: api.EncodeF64(math.NaN()), expected: 0},

			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(0.0), expected: 0},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(0x0.0000000000001p-1022), expected: 0},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(-0x0.0000000000001p-1022), expected: 0},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1.0), expected: 1},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(0x1.199999999999ap+0), expected: 1},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1.5), expected: 1},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1.9), expected: 1},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(2.0), expected: 2},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(2147483648), expected: api.EncodeI32(-2147483648)}, // 0x1.00000p+31 -> 8000 0000
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(4294967295.0), expected: api.EncodeI32(-1)},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(-0x1.ccccccccccccdp-1), expected: 0},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(-0x1.fffffffffffffp-1), expected: 0},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1e8), expected: 100000000},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(4294967296.0), expected: 0xffffffff},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(-1.0), expected: 0x00000000},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1e16), expected: 0xffffffff},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(1e30), expected: 0xffffffff},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(9223372036854775808), expected: 0xffffffff},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(math.Inf(1)), expected: 0xffffffff},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(math.Inf(-1)), expected: 0x00000000},
			{name: "i32.trunc_sat_f64_u", param: api.EncodeF64(math.NaN()), expected: 0},

			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(0.0), expected: 0},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(0x1p-149), expected: 0},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-0x1p-149), expected: 0},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(1.0), expected: 1},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(0x1.19999ap+0), expected: 1},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(1.5), expected: 1},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-1.0), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-0x1.19999ap+0), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-1.5), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-1.9), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-2.0), expected: api.EncodeI64(-2)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(4294967296), expected: 4294967296},                  // 0x1.00000p+32 -> 1 0000 0000
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-4294967296), expected: api.EncodeI64(-4294967296)}, // -0x1.00000p+32 -> ffff ffff 0000 0000
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(9223371487098961920.0), expected: 9223371487098961920},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-9223372036854775808.0), expected: api.EncodeI64(-9223372036854775808)},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(9223372036854775808.0), expected: 0x7fffffffffffffff},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(-9223373136366403584.0), expected: 0x8000000000000000},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(float32(math.Inf(1))), expected: 0x7fffffffffffffff},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(float32(math.Inf(-1))), expected: 0x8000000000000000},
			{name: "i64.trunc_sat_f32_s", param: api.EncodeF32(float32(math.NaN())), expected: 0},

			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(0.0), expected: 0},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(0x1p-149), expected: 0},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(-0x1p-149), expected: 0},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(1.0), expected: 1},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(0x1.19999ap+0), expected: 1},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(1.5), expected: 1},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(4294967296), expected: 4294967296},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(18446742974197923840.0), expected: api.EncodeI64(-1099511627776)},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(-0x1.ccccccp-1), expected: 0},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(-0x1.fffffep-1), expected: 0},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(18446744073709551616.0), expected: 0xffffffffffffffff},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(-1.0), expected: 0x0000000000000000},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(float32(math.Inf(1))), expected: 0xffffffffffffffff},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(float32(math.Inf(-1))), expected: 0x0000000000000000},
			{name: "i64.trunc_sat_f32_u", param: api.EncodeF32(float32(math.NaN())), expected: 0},

			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(0.0), expected: 0},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(0x0.0000000000001p-1022), expected: 0},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-0x0.0000000000001p-1022), expected: 0},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(1.0), expected: 1},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(0x1.199999999999ap+0), expected: 1},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(1.5), expected: 1},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-1.0), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-0x1.199999999999ap+0), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-1.5), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-1.9), expected: api.EncodeI64(-1)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-2.0), expected: api.EncodeI64(-2)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(4294967296), expected: 4294967296},                  // 0x1.00000p+32 -> 1 0000 0000
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-4294967296), expected: api.EncodeI64(-4294967296)}, // -0x1.00000p+32 -> ffff ffff 0000 0000
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(9223372036854774784.0), expected: 9223372036854774784},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-9223372036854775808.0), expected: api.EncodeI64(-9223372036854775808)},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(9223372036854775808.0), expected: 0x7fffffffffffffff},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(-9223372036854777856.0), expected: 0x8000000000000000},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(math.Inf(1)), expected: 0x7fffffffffffffff},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(math.Inf(-1)), expected: 0x8000000000000000},
			{name: "i64.trunc_sat_f64_s", param: api.EncodeF64(math.NaN()), expected: 0},

			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(0.0), expected: 0},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(0x0.0000000000001p-1022), expected: 0},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(-0x0.0000000000001p-1022), expected: 0},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(1.0), expected: 1},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(0x1.199999999999ap+0), expected: 1},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(1.5), expected: 1},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(4294967295), expected: 0xffffffff},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(4294967296), expected: 0x100000000},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(18446744073709549568.0), expected: api.EncodeI64(-2048)},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(-0x1.ccccccccccccdp-1), expected: 0},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(-0x1.fffffffffffffp-1), expected: 0},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(1e8), expected: 100000000},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(1e16), expected: 10000000000000000},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(9223372036854775808), expected: api.EncodeI64(-9223372036854775808)},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(18446744073709551616.0), expected: 0xffffffffffffffff},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(-1.0), expected: 0x0000000000000000},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(math.Inf(1)), expected: 0xffffffffffffffff},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(math.Inf(-1)), expected: 0x0000000000000000},
			{name: "i64.trunc_sat_f64_u", param: api.EncodeF64(math.NaN()), expected: 0},
		})
	})
}

type funcTest struct {
	name            string
	param, expected uint64
}

func testFunctions(t *testing.T, module api.Module, tests []funcTest) {
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			results, err := module.ExportedFunction(tc.name).Call(testCtx, tc.param)
			require.NoError(t, err)
			require.Equal(t, tc.expected, results[0])
		})
	}
}
