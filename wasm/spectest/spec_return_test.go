package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_return(t *testing.T) {
	vm := requireInitVM(t, "return", nil)

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

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
	// // assertReturn_I32_I32("$1", $2, $3)
	// assertReturn_I32_I32 := func(name string, arg, exp uint32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI32, types[0])
	// 		require.Equal(t, exp, uint32(values[0]))

	// 	})
	// }

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// // assertReturn_I64_I64("$1", $2, $3)
	// assertReturn_I64_I64 := func(name string, arg, exp uint64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, arg)
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI64, types[0])
	// 		require.Equal(t, exp, values[0])

	// 	})
	// }

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)
	// // assertReturn_I32_F32($1, $2, $3)
	// assertReturn_I32_F32 := func(name string, arg uint32, exp float32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF32, types[0])
	// 		require.Equal(t, math.Float32bits(exp), uint32(values[0]))

	// 	})
	// }

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)[\s\n]+\(f32.const\s([a-z0-9-.]+)\)\)
	// // assertReturn_F32_F32($1, $2, $3)
	// assertReturn_F32_F32 := func(name string, arg, exp float32) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(math.Float32bits(arg)))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF32, types[0])
	// 		require.Equal(t, math.Float32bits(exp), uint32(values[0]))

	// 	})
	// }

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)
	// assertReturn_F64_F64($1, $2, $3)
	// assertReturn_F64_F64 := func(name string, arg, exp float64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, math.Float64bits(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF64, types[0])
	// 		require.Equal(t, math.Float64bits(exp), values[0])

	// 	})
	// }

	// // \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(f64.const\s([a-z0-9-.]+)\)\)
	// // assertReturn_I32_F64($1, $2, $3)
	// assertReturn_I32_F64 := func(name string, arg uint32, exp float64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeF64, types[0])
	// 		require.Equal(t, math.Float64bits(exp), values[0])

	// 	})
	// }

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_0-9]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I64("$1", $2, $3)
	// assertReturn_I32_I64 := func(name string, arg uint32, exp uint64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI64, types[0])
	// 		require.Equal(t, exp, values[0])

	// 	})
	// }

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

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32I32_I32("$1", $2, $3, $4)
	assertReturn_I32I32_I32 := func(name string, arg1, arg2, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg1), uint64(arg2))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32I64_I64("$1", $2, $3, $4)
	// assertReturn_I32I64_I64 := func(name string, arg1 uint32, arg2, exp uint64) {
	// 	t.Run("assert_return_"+name, func(t *testing.T) {
	// 		values, types, err := vm.ExecExportedFunction(name, uint64(arg1), uint64(arg2))
	// 		require.NoError(t, err)
	// 		require.Len(t, values, 1)
	// 		require.Len(t, types, 1)
	// 		require.Equal(t, wasm.ValueTypeI64, types[0])
	// 		require.Equal(t, exp, values[0])
	// 	})
	// }

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

	// // \(assert_trap[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\s"[a-z\s]+"\)
	// // assertTrap_I32($1, $2)
	// assertTrap_I32 := func(name string, arg1 uint32) {
	// 	t.Run("assert_trap_"+name, func(t *testing.T) {
	// 		require.Panics(t, func() {
	// 			// Memory out of bounds.
	// 			_, _, _ = vm.ExecExportedFunction(name, uint64(arg1))
	// 		})
	// 	})
	// }

	// // \(assert_trap[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\s\(i64.const\s([a-z0-9-]+)\)\)\s"[a-z\s]+"\)
	// // assertTrap_I32I64($1, $2, $3)
	// assertTrap_I32I64 := func(name string, arg1 uint32, arg2 uint64) {
	// 	t.Run("assert_trap_"+name, func(t *testing.T) {
	// 		require.Panics(t, func() {
	// 			// Memory out of bounds.
	// 			_, _, _ = vm.ExecExportedFunction(name, uint64(arg1), arg2)
	// 		})
	// 	})
	// }

	assertReturn("type-i32")
	assertReturn("type-i64")
	assertReturn("type-f32")
	assertReturn("type-f64")

	assertReturn__I32("type-i32-value", 1)
	assertReturn__I64("type-i64-value", 2)
	assertReturn__F32("type-f32-value", math.Float32bits(3))
	assertReturn__F64("type-f64-value", math.Float64bits(4))

	assertReturn("nullary")
	assertReturn__F64("unary", math.Float64bits(3))

	assertReturn__I32("as-func-first", 1)
	assertReturn__I32("as-func-mid", 2)
	assertReturn("as-func-last")
	assertReturn__I32("as-func-value", 3)

	assertReturn("as-block-first")
	assertReturn("as-block-mid")
	assertReturn("as-block-last")
	assertReturn__I32("as-block-value", 2)

	assertReturn__I32("as-loop-first", 3)
	assertReturn__I32("as-loop-mid", 4)
	assertReturn__I32("as-loop-last", 5)

	assertReturn__I32("as-br-value", 9)

	assertReturn("as-br_if-cond")
	assertReturn__I32("as-br_if-value", 8)
	assertReturn__I32("as-br_if-value-cond", 9)

	assertReturn__I64("as-br_table-index", 9)
	assertReturn__I32("as-br_table-value", 10)
	assertReturn__I32("as-br_table-value-index", 11)

	assertReturn__I64("as-return-value", 7)

	assertReturn__I32("as-if-cond", 2)
	assertReturn_I32I32_I32("as-if-then", 1, 6, 3)
	assertReturn_I32I32_I32("as-if-then", 0, 6, 6)
	assertReturn_I32I32_I32("as-if-else", 0, 6, 4)
	assertReturn_I32I32_I32("as-if-else", 1, 6, 6)

	assertReturn_I32I32_I32("as-select-first", 0, 6, 5)
	assertReturn_I32I32_I32("as-select-first", 1, 6, 5)
	assertReturn_I32I32_I32("as-select-second", 0, 6, 6)
	assertReturn_I32I32_I32("as-select-second", 1, 6, 6)
	assertReturn__I32("as-select-cond", 7)

	assertReturn__I32("as-call-first", 12)
	assertReturn__I32("as-call-mid", 13)
	assertReturn__I32("as-call-last", 14)

	assertReturn__I32("as-call_indirect-func", 20)
	assertReturn__I32("as-call_indirect-first", 21)
	assertReturn__I32("as-call_indirect-mid", 22)
	assertReturn__I32("as-call_indirect-last", 23)

	assertReturn__I32("as-local.set-value", 17)
	assertReturn__I32("as-local.tee-value", 1)
	assertReturn__I32("as-global.set-value", 1)

	assertReturn__F32("as-load-address", math.Float32bits(1.7))
	assertReturn__I64("as-loadN-address", 30)

	assertReturn__I32("as-store-address", 30)
	assertReturn__I32("as-store-value", 31)
	assertReturn__I32("as-storeN-address", 32)
	assertReturn__I32("as-storeN-value", 33)

	assertReturn__F32("as-unary-operand", math.Float32bits(3.4))

	assertReturn__I32("as-binary-left", 3)
	assertReturn__I64("as-binary-right", 45)

	assertReturn__I32("as-test-operand", 44)

	assertReturn__I32("as-compare-left", 43)
	assertReturn__I32("as-compare-right", 42)

	assertReturn__I32("as-convert-operand", 41)

	assertReturn__I32("as-memory.grow-size", 40)
}
