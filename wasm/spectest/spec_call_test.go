package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_call(t *testing.T) {
	vm := requireInitVM(t, "call", nil)

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\)
	// assertReturn("$1")
	assertReturn := func(name string) {
		values, types, err := vm.ExecExportedFunction(name)
		require.NoError(t, err)
		require.Len(t, values, 0)
		require.Len(t, types, 0)
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

	// \(assert_return\s\(invoke "([a-zA-Z-_]+)"\s\(i64.const\s([a-z0-9-]+)\)\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I64_I32("$1", $2, $3)
	assertReturn_I64_I32 := func(name string, arg uint64, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	// \(assert_return\s\(invoke "([a-zA-Z-_]+)"\s\(i64.const\s([a-z0-9-]+)\)\)\s\(i64.const\s([a-z0-9-]+)\)\)
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

	// \(assert_return\s\(invoke "([a-zA-Z-_]+)"\s\(i64.const\s([a-z0-9-]+)\)\s\(i64.const\s([a-z0-9-]+)\)\)\s\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I64I64_I64("$1", $2, $3, $4)
	assertReturn_I64I64_I64 := func(name string, arg1, arg2, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg1, arg2)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I32("$1", $2, $3)
	assertReturn_I32_I32 := func(name string, arg, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	assertTrap := func(name string) {
		t.Run("assert_trap_"+name, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(name)
			})
		})
	}

	assertReturn__I32("type-i32", 0x132)
	assertReturn__I64("type-i64", 0x164)
	assertReturn__F32("type-f32", math.Float32bits(0xf32))
	assertReturn__F64("type-f64", math.Float64bits(0xf64))

	assertReturn__I32("type-first-i32", 32)
	assertReturn__I64("type-first-i64", 64)
	assertReturn__F32("type-first-f32", math.Float32bits(1.32))
	assertReturn__F64("type-first-f64", math.Float64bits(1.64))

	assertReturn__I32("type-second-i32", 32)
	assertReturn__I64("type-second-i64", 64)
	assertReturn__F32("type-second-f32", math.Float32bits(32))
	assertReturn__F64("type-second-f64", math.Float64bits(64.1))

	assertReturn__I32("as-binary-all-operands", 7)
	assertReturn__I32("as-mixed-operands", 32)

	assertReturn_I64_I64("fac", 0, 1)
	assertReturn_I64_I64("fac", 1, 1)
	assertReturn_I64_I64("fac", 5, 120)
	assertReturn_I64_I64("fac", 25, 7034535277573963776)
	assertReturn_I64I64_I64("fac-acc", 0, 1, 1)
	assertReturn_I64I64_I64("fac-acc", 1, 1, 1)
	assertReturn_I64I64_I64("fac-acc", 5, 1, 120)
	assertReturn_I64I64_I64("fac-acc", 25, 1, 7034535277573963776)

	assertReturn_I64_I64("fib", 0, 1)
	assertReturn_I64_I64("fib", 1, 1)
	assertReturn_I64_I64("fib", 2, 2)
	assertReturn_I64_I64("fib", 5, 8)
	assertReturn_I64_I64("fib", 20, 10946)

	assertReturn_I64_I32("even", 0, 44)
	assertReturn_I64_I32("even", 1, 99)
	assertReturn_I64_I32("even", 100, 44)
	assertReturn_I64_I32("even", 77, 99)
	assertReturn_I64_I32("odd", 0, 99)
	assertReturn_I64_I32("odd", 1, 44)
	assertReturn_I64_I32("odd", 200, 99)
	assertReturn_I64_I32("odd", 77, 44)
	// FIXME.
	// (assert_exhaustion (invoke "runaway") "call stack exhausted")
	// (assert_exhaustion (invoke "mutual-runaway") "call stack exhausted")

	assertReturn__I32("as-select-first", 0x132)
	assertReturn__I32("as-select-mid", 2)
	assertReturn__I32("as-select-last", 2)

	assertReturn__I32("as-if-condition", 1)

	assertReturn__I32("as-br_if-first", 0x132)
	assertReturn__I32("as-br_if-last", 2)

	assertReturn__I32("as-br_table-first", 0x132)
	assertReturn__I32("as-br_table-last", 2)

	assertReturn__I32("as-call_indirect-first", 0x132)
	assertReturn__I32("as-call_indirect-mid", 2)
	assertTrap("as-call_indirect-last")

	assertReturn("as-store-first")
	assertReturn("as-store-last")

	assertReturn__I32("as-memory.grow-value", 1)
	assertReturn__I32("as-return-value", 0x132)
	assertReturn("as-drop-operand")
	assertReturn__I32("as-br-value", 0x132)
	assertReturn__I32("as-local.set-value", 0x132)
	assertReturn__I32("as-local.tee-value", 0x132)
	assertReturn__I32("as-global.set-value", 0x132)
	assertReturn__I32("as-load-operand", 1)

	assertReturn__F32("as-unary-operand", math.Float32bits(0x0p+0))
	assertReturn__I32("as-binary-left", 11)
	assertReturn__I32("as-binary-right", 9)
	assertReturn__I32("as-test-operand", 0)
	assertReturn__I32("as-compare-left", 1)
	assertReturn__I32("as-compare-right", 1)
	assertReturn__I64("as-convert-operand", 1)

	assertReturn_I32_I32("return-from-long-argument-list", 42, 42)

}
