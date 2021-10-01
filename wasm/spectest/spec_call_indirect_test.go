package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_call_indirect1(t *testing.T) {
	vm := requireInitVM(t, "call_indirect1", nil)

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\)
	// assertReturn("$1")
	assertReturn := func(name string) {
		values, types, err := vm.ExecExportedFunction(name)
		require.NoError(t, err)
		require.Len(t, values, 0)
		require.Len(t, types, 0)
	}

	// // \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\)
	// // assertReturn_I32_("$1", $2)
	// assertReturn_I32_ := func(name string, arg uint32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 0)
	// 		require.Len(t, types, 0)
	// 	})
	// }

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I32("$1", $2, $3)
	assertReturn_I32_I32 := func(name string, arg, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I64_I64("$1", $2, $3)
	assertReturn_I64_I64 := func(name string, arg, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)
	// assertReturn_I32_F32($1, $2, $3)
	assertReturn_I32_F32 := func(name string, arg uint32, exp float32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF32, types[0])
			require.Equal(t, math.Float32bits(exp), uint32(values[0]))

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)
	// assertReturn_F32_F32($1, $2, $3)
	assertReturn_F32_F32 := func(name string, arg, exp float32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(math.Float32bits(arg)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF32, types[0])
			require.Equal(t, math.Float32bits(exp), uint32(values[0]))

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)
	// assertReturn_F64_F64($1, $2, $3)
	assertReturn_F64_F64 := func(name string, arg, exp float64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, math.Float64bits(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF64, types[0])
			require.Equal(t, math.Float64bits(exp), values[0])

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)
	// assertReturn_I32_F64($1, $2, $3)
	assertReturn_I32_F64 := func(name string, arg uint32, exp float64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF64, types[0])
			require.Equal(t, math.Float64bits(exp), values[0])

		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I64("$1", $2, $3)
	assertReturn_I32_I64 := func(name string, arg uint32, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])

		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn__I32("$1", $2)
	assertReturn__I32 := func(name string, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\s\(i64.const\s([a-z0-9-+]+)\)\)
	// assertReturn__I64("$1", $2)
	assertReturn__I64 := func(name string, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\s\(f32.const\s([.a-z0-9-+]+)\)\)
	// assertReturn__F32("$1", math.Float32bits($2))
	assertReturn__F32 := func(name string, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF32, types[0])
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\s\(f64.const\s([.a-z0-9-]+)\)\)
	// assertReturn__F64("$1", math.Float64bits($2))
	assertReturn__F64 := func(name string, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF64, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
	// // assertReturn_I32I32_I32("$1", $2, $3, $4)
	// assertReturn_I32I32_I32 := func(name string, arg1, arg2, exp uint32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg1), uint64(arg2))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI32, types[0])
	// 		require.Equal(t, exp, uint32(values[0]))
	// 	})
	// }

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32I64_I64("$1", $2, $3, $4)
	assertReturn_I32I64_I64 := func(name string, arg1 uint32, arg2, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg1), uint64(arg2))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	// assertReturn_I64I64_I64 := func(name string, arg1, arg2, exp uint64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, arg1, arg2)
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI64, types[0])
	// 		require.Equal(t, exp, values[0])
	// 	})
	// }

	// assertReturn_F32F32_F32 := func(name string, arg1, arg2, exp float32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(math.Float32bits(arg1)), uint64(math.Float32bits(arg2)))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF32, types[0])
	// 		require.Equal(t, exp, math.Float32frombits(uint32(values[0])))
	// 	})
	// }

	// assertReturn_F64F64_F64 := func(name string, arg1, arg2, exp float64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, math.Float64bits(arg1), math.Float64bits(arg2))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF64, types[0])
	// 		require.Equal(t, exp, math.Float64frombits(values[0]))
	// 	})
	// }

	// // \(assert_return[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\s\(f64.const\s([a-z0-9-]+)\)\s\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)(\n|)\)
	// // assertReturn_I32F64I32_I32("$1", uint64(int32($2)), math.Float64bits($3), uint64(int32($4)), $5)
	// assertReturn_I32F64I32_I32 := func(name string, arg1, arg2, arg3 uint64, exp uint32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, arg1, arg2, arg3)
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI32, types[0])
	// 		require.Equal(t, exp, uint32(values[0]))
	// 	})
	// }

	// \(assert_trap[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\s"[a-z\s]+"\)
	// assertTrap_I32($1, $2)
	assertTrap_I32 := func(name string, arg1 uint32) {
		t.Run("assert_trap_"+name, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(name, uint64(arg1))
			})
		})
	}

	// \(assert_trap[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\s\(i64.const\s([a-z0-9-]+)\)\)\s"[a-z\s]+"\)
	// assertTrap_I32I64($1, $2, $3)
	assertTrap_I32I64 := func(name string, arg1 uint32, arg2 uint64) {
		t.Run("assert_trap_"+name, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(name, uint64(arg1), arg2)
			})
		})
	}

	assertReturn__I32("type-i32", 0x132)
	assertReturn__I64("type-i64", 0x164)
	assertReturn__F32("type-f32", math.Float32bits(0xf32))
	assertReturn__F64("type-f64", math.Float64bits(0xf64))

	assertReturn__I64("type-index", 100)

	assertReturn__I32("type-first-i32", 32)
	assertReturn__I64("type-first-i64", 64)
	assertReturn__F32("type-first-f32", math.Float32bits(1.32))
	assertReturn__F64("type-first-f64", math.Float64bits(1.64))

	assertReturn__I32("type-second-i32", 32)
	assertReturn__I64("type-second-i64", 64)
	assertReturn__F32("type-second-f32", math.Float32bits(32))
	assertReturn__F64("type-second-f64", math.Float64bits(64.1))

	assertReturn_I32I64_I64("dispatch", 5, 2, 2)
	assertReturn_I32I64_I64("dispatch", 5, 5, 5)
	assertReturn_I32I64_I64("dispatch", 12, 5, 120)
	assertReturn_I32I64_I64("dispatch", 13, 5, 8)
	assertReturn_I32I64_I64("dispatch", 20, 2, 2)
	assertTrap_I32I64("dispatch", 0, 2)
	assertTrap_I32I64("dispatch", 15, 2)
	assertTrap_I32I64("dispatch", 32, 2)
	assertTrap_I32I64("dispatch", 0xffffffff, 2)
	assertTrap_I32I64("dispatch", 1213432423, 2)

	assertReturn_I32_I64("dispatch-structural-i64", 5, 9)
	assertReturn_I32_I64("dispatch-structural-i64", 12, 362880)
	assertReturn_I32_I64("dispatch-structural-i64", 13, 55)
	assertReturn_I32_I64("dispatch-structural-i64", 20, 9)
	assertTrap_I32("dispatch-structural-i64", 11)
	assertTrap_I32("dispatch-structural-i64", 22)

	assertReturn_I32_I32("dispatch-structural-i32", 4, 9)
	assertReturn_I32_I32("dispatch-structural-i32", 23, 362880)
	assertReturn_I32_I32("dispatch-structural-i32", 26, 55)
	assertReturn_I32_I32("dispatch-structural-i32", 19, 9)
	assertTrap_I32("dispatch-structural-i32", 9)
	assertTrap_I32("dispatch-structural-i32", 21)

	assertReturn_I32_F32("dispatch-structural-f32", 6, 9.0)
	assertReturn_I32_F32("dispatch-structural-f32", 24, 362880.0)
	assertReturn_I32_F32("dispatch-structural-f32", 27, 55.0)
	assertReturn_I32_F32("dispatch-structural-f32", 21, 9.0)
	assertTrap_I32("dispatch-structural-f32", 8)
	assertTrap_I32("dispatch-structural-f32", 19)

	assertReturn_I32_F64("dispatch-structural-f64", 7, 9.0)
	assertReturn_I32_F64("dispatch-structural-f64", 25, 362880.0)
	assertReturn_I32_F64("dispatch-structural-f64", 28, 55.0)
	assertReturn_I32_F64("dispatch-structural-f64", 22, 9.0)
	assertTrap_I32("dispatch-structural-f64", 10)
	assertTrap_I32("dispatch-structural-f64", 18)

	assertReturn_I64_I64("fac-i64", 0, 1)
	assertReturn_I64_I64("fac-i64", 1, 1)
	assertReturn_I64_I64("fac-i64", 5, 120)
	assertReturn_I64_I64("fac-i64", 25, 7034535277573963776)

	assertReturn_I32_I32("fac-i32", 0, 1)
	assertReturn_I32_I32("fac-i32", 1, 1)
	assertReturn_I32_I32("fac-i32", 5, 120)
	assertReturn_I32_I32("fac-i32", 10, 3628800)

	assertReturn_F32_F32("fac-f32", 0.0, 1.0)
	assertReturn_F32_F32("fac-f32", 1.0, 1.0)
	assertReturn_F32_F32("fac-f32", 5.0, 120.0)
	assertReturn_F32_F32("fac-f32", 10.0, 3628800.0)

	assertReturn_F64_F64("fac-f64", 0.0, 1.0)
	assertReturn_F64_F64("fac-f64", 1.0, 1.0)
	assertReturn_F64_F64("fac-f64", 5.0, 120.0)
	assertReturn_F64_F64("fac-f64", 10.0, 3628800.0)

	assertReturn_I64_I64("fib-i64", 0, 1)
	assertReturn_I64_I64("fib-i64", 1, 1)
	assertReturn_I64_I64("fib-i64", 2, 2)
	assertReturn_I64_I64("fib-i64", 5, 8)
	assertReturn_I64_I64("fib-i64", 20, 10946)

	assertReturn_I32_I32("fib-i32", 0, 1)
	assertReturn_I32_I32("fib-i32", 1, 1)
	assertReturn_I32_I32("fib-i32", 2, 2)
	assertReturn_I32_I32("fib-i32", 5, 8)
	assertReturn_I32_I32("fib-i32", 20, 10946)

	assertReturn_F32_F32("fib-f32", 0.0, 1.0)
	assertReturn_F32_F32("fib-f32", 1.0, 1.0)
	assertReturn_F32_F32("fib-f32", 2.0, 2.0)
	assertReturn_F32_F32("fib-f32", 5.0, 8.0)
	assertReturn_F32_F32("fib-f32", 20.0, 10946.0)

	assertReturn_F64_F64("fib-f64", 0.0, 1.0)
	assertReturn_F64_F64("fib-f64", 1.0, 1.0)
	assertReturn_F64_F64("fib-f64", 2.0, 2.0)
	assertReturn_F64_F64("fib-f64", 5.0, 8.0)
	assertReturn_F64_F64("fib-f64", 20.0, 10946.0)

	assertReturn_I32_I32("even", 0, 44)
	assertReturn_I32_I32("even", 1, 99)
	assertReturn_I32_I32("even", 100, 44)
	assertReturn_I32_I32("even", 77, 99)
	assertReturn_I32_I32("odd", 0, 99)
	assertReturn_I32_I32("odd", 1, 44)
	assertReturn_I32_I32("odd", 200, 99)
	assertReturn_I32_I32("odd", 77, 44)

	// (assert_exhaustion (invoke "runaway") "call stack exhausted")
	// (assert_exhaustion (invoke "mutual-runaway") "call stack exhausted")

	assertReturn__I32("as-select-first", 0x132)
	assertReturn__I32("as-select-mid", 2)
	assertReturn__I32("as-select-last", 2)

	assertReturn__I32("as-if-condition", 1)

	assertReturn__I64("as-br_if-first", 0x164)
	assertReturn__I32("as-br_if-last", 2)

	assertReturn__F32("as-br_table-first", math.Float32bits(0xf32))
	assertReturn__I32("as-br_table-last", 2)

	assertReturn("as-store-first")
	assertReturn("as-store-last")

	assertReturn__I32("as-memory.grow-value", 1)
	assertReturn__I32("as-return-value", 1)
	assertReturn("as-drop-operand")
	assertReturn__F32("as-br-value", math.Float32bits(1))
	assertReturn__F64("as-local.set-value", math.Float64bits(1))
	assertReturn__F64("as-local.tee-value", math.Float64bits(1))
	assertReturn__F64("as-global.set-value", math.Float64bits(1.0))
	assertReturn__I32("as-load-operand", 1)

	assertReturn__F32("as-unary-operand", math.Float32bits(0x0p+0))
	assertReturn__I32("as-binary-left", 11)
	assertReturn__I32("as-binary-right", 9)
	assertReturn__I32("as-test-operand", 0)
	assertReturn__I32("as-compare-left", 1)
	assertReturn__I32("as-compare-right", 1)
	assertReturn__I64("as-convert-operand", 1)
}
