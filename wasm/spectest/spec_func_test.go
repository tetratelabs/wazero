package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_func1(t *testing.T) {
	vm := requireInitVM(t, "func1", nil)

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\)
	// assertReturn("$1")
	assertReturn := func(name string) {
		values, types, err := vm.ExecExportedFunction(name)
		require.NoError(t, err)
		require.Len(t, values, 0)
		require.Len(t, types, 0)
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\)
	// assertReturn_I32_("$1", $2)
	assertReturn_I32_ := func(name string, arg uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}

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

	assertReturn_F32F32_F32 := func(name string, arg1, arg2, exp float32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(math.Float32bits(arg1)), uint64(math.Float32bits(arg2)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF32, types[0])
			require.Equal(t, exp, math.Float32frombits(uint32(values[0])))
		})
	}

	assertReturn_F64F64_F64 := func(name string, arg1, arg2, exp float64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, math.Float64bits(arg1), math.Float64bits(arg2))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF64, types[0])
			require.Equal(t, exp, math.Float64frombits(values[0]))
		})
	}

	// \(assert_return[\s\n]+\(invoke\s"([a-zA-Z-_0-9]+)"\s\(i32.const\s([a-z0-9-]+)\)\s\(f64.const\s([a-z0-9-]+)\)\s\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)(\n|)\)
	// assertReturn_I32F64I32_I32("$1", uint64(int32($2)), math.Float64bits($3), uint64(int32($4)), $5)
	assertReturn_I32F64I32_I32 := func(name string, arg1, arg2, arg3 uint64, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg1, arg2, arg3)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	assertReturnF64Vars := func(name string, exp uint64, args ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, args...)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeF64, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	assertReturn("type-use-1")
	assertReturn__I32("type-use-2", 0)
	assertReturn_I32_("type-use-3", 1)
	assertReturn_I32F64I32_I32("type-use-4", uint64(int32(1)), math.Float64bits(1), uint64(int32(1)), 0)
	assertReturn__I32("type-use-5", 0)
	assertReturn_I32_("type-use-6", 1)
	assertReturn_I32F64I32_I32("type-use-7", uint64(int32(1)), math.Float64bits(1), uint64(int32(1)), 0)

	assertReturn__I32("local-first-i32", 0)
	assertReturn__I64("local-first-i64", 0)
	assertReturn__F32("local-first-f32", math.Float32bits(0))
	assertReturn__F64("local-first-f64", math.Float64bits(0))
	assertReturn__I32("local-second-i32", 0)
	assertReturn__I64("local-second-i64", 0)
	assertReturn__F32("local-second-f32", math.Float32bits(0))
	assertReturn__F64("local-second-f64", math.Float64bits(0))
	assertReturn__F64("local-mixed", math.Float64bits(0))

	assertReturn_I32I32_I32("param-first-i32", 2, 3, 2)
	assertReturn_I64I64_I64("param-first-i64", 2, 3, 2)
	assertReturn_F32F32_F32("param-first-f32", 2, 3, 2)
	assertReturn_F64F64_F64("param-first-f64", 2, 3, 2)
	assertReturn_I32I32_I32("param-second-i32", 2, 3, 3)
	assertReturn_I64I64_I64("param-second-i64", 2, 3, 3)
	assertReturn_F32F32_F32("param-second-f32", 2, 3, 3)
	assertReturn_F64F64_F64("param-second-f64", 2, 3, 3)

	assertReturnF64Vars("param-mixed",
		math.Float64bits(5.5),
		uint64(math.Float32bits(1)), uint64(uint32(2)), uint64(int64(3)),
		uint64(uint32(4)), math.Float64bits(5.5), uint64(uint32(6)),
	)

	assertReturn("empty")
	assertReturn("value-void")
	assertReturn__I32("value-i32", 77)
	assertReturn__I64("value-i64", 7777)
	assertReturn__F32("value-f32", math.Float32bits(77.7))
	assertReturn__F64("value-f64", math.Float64bits(77.77))
	assertReturn("value-block-void")
	assertReturn__I32("value-block-i32", 77)

	assertReturn("return-empty")
	assertReturn__I32("return-i32", 78)
	assertReturn__I64("return-i64", 7878)
	assertReturn__F32("return-f32", math.Float32bits(78.7))
	assertReturn__F64("return-f64", math.Float64bits(78.78))
	assertReturn__I32("return-block-i32", 77)

	assertReturn("break-empty")
	assertReturn__I32("break-i32", 79)
	assertReturn__I64("break-i64", 7979)
	assertReturn__F32("break-f32", math.Float32bits(79.9))
	assertReturn__F64("break-f64", math.Float64bits(79.79))
	assertReturn__I32("break-block-i32", 77)

	assertReturn_I32_("break-br_if-empty", 0)
	assertReturn_I32_("break-br_if-empty", 2)
	assertReturn_I32_I32("break-br_if-num", 0, 51)
	assertReturn_I32_I32("break-br_if-num", 1, 50)

	assertReturn_I32_("break-br_table-empty", 0)
	assertReturn_I32_("break-br_table-empty", 1)
	assertReturn_I32_("break-br_table-empty", 5)
	assertReturn_I32_("break-br_table-empty", 0xffffffff)
	assertReturn_I32_I32("break-br_table-num", 0, 50)
	assertReturn_I32_I32("break-br_table-num", 1, 50)
	assertReturn_I32_I32("break-br_table-num", 10, 50)
	assertReturn_I32_I32("break-br_table-num", 0xffffff9c, 50)
	assertReturn_I32_("break-br_table-nested-empty", 0)
	assertReturn_I32_("break-br_table-nested-empty", 1)
	assertReturn_I32_("break-br_table-nested-empty", 3)
	assertReturn_I32_("break-br_table-nested-empty", 0xfffffffe)

	assertReturn_I32_I32("break-br_table-nested-num", 0, 52)
	assertReturn_I32_I32("break-br_table-nested-num", 1, 50)
	assertReturn_I32_I32("break-br_table-nested-num", 2, 52)
	assertReturn_I32_I32("break-br_table-nested-num", 0xfffffffd, 52)

	assertReturn__I32("init-local-i32", 0)
	assertReturn__I64("init-local-i64", 0)
	assertReturn__F32("init-local-f32", math.Float32bits(0))
	assertReturn__F64("init-local-f64", math.Float64bits(0))
}

func Test_func2(t *testing.T) {
	vm := requireInitVM(t, "func2", nil)

	// assertReturn_I32_("$1", $2)
	assertReturn__I32 := func(name string, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))
		})
	}

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

	assertReturn_I32_I32("f", 42, 0)
	assertReturn_I32_I32("g", 42, 0)
	assertReturn__I32("p", 42)
}

func Test_func3(t *testing.T) {
	vm := requireInitVM(t, "func3", nil)
	assertReturn := func(name string) {
		values, types, err := vm.ExecExportedFunction(name)
		require.NoError(t, err)
		require.Len(t, values, 0)
		require.Len(t, types, 0)
	}

	assertReturn("signature-explicit-reused")
	assertReturn("signature-implicit-reused")
	assertReturn("signature-explicit-duplicate")
	assertReturn("signature-implicit-duplicate")
}
