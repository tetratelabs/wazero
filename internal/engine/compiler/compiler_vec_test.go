package compiler

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileV128Add(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		x1, x2, exp [16]byte
	}{
		{
			name:  "i8x16",
			shape: wazeroir.ShapeI8x16,
			x1:    [16]byte{0: 1, 2: 10, 10: 10},
			x2:    [16]byte{0: 10, 4: 5, 10: 5},
			exp:   [16]byte{0: 11, 2: 10, 4: 5, 10: 15},
		},
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			x1:    i16x8(1123, 0, 123, 1, 1, 5, 8, 1),
			x2:    i16x8(0, 123, 123, 0, 1, 5, 9, 1),
			exp:   i16x8(1123, 123, 246, 1, 2, 10, 17, 2),
		},
		{
			name:  "i32x4",
			shape: wazeroir.ShapeI32x4,
			x1:    i32x4(i32ToU32(-123), 5, 4, math.MaxUint32),
			x2:    i32x4(i32ToU32(-10), 1, i32ToU32(-104), math.MaxUint32),
			exp:   i32x4(i32ToU32(-133), 6, i32ToU32(-100), math.MaxUint32-1),
		},
		{
			name:  "i64x2",
			shape: wazeroir.ShapeI64x2,
			x1:    i64x2(i64ToU64(math.MinInt64), 12345),
			x2:    i64x2(i64ToU64(-1), i64ToU64(-12345)),
			exp:   i64x2(i64ToU64(math.MinInt64)+i64ToU64(-1), 0),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(1.0, 123, float32(math.Inf(1)), float32(math.Inf(-1))),
			x2:    f32x4(51234.12341, 123, math.MaxFloat32, -123),
			exp:   f32x4(51235.12341, 246, float32(math.Inf(1)), float32(math.Inf(-1))),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(1.123, math.Inf(1)),
			x2:    f64x2(1.123, math.MinInt64),
			exp:   f64x2(2.246, math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Add(&wazeroir.OperationV128Add{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Sub(t *testing.T) {

	tests := []struct {
		name        string
		shape       wazeroir.Shape
		x1, x2, exp [16]byte
	}{
		{
			name:  "i8x16",
			shape: wazeroir.ShapeI8x16,
			x1:    [16]byte{0: 1, 2: 10, 10: 10},
			x2:    [16]byte{0: 10, 4: 5, 10: 5},
			exp:   [16]byte{0: i8ToU8(-9), 2: 10, 4: i8ToU8(-5), 10: 5},
		},
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			x1:    i16x8(1123, 0, 123, 1, 1, 5, 8, 1),
			x2:    i16x8(0, 123, 123, 0, 1, 5, 9, 1),
			exp:   i16x8(1123, i16ToU16(-123), 0, 1, 0, 0, i16ToU16(-1), 0),
		},
		{
			name:  "i32x4",
			shape: wazeroir.ShapeI32x4,
			x1:    i32x4(i32ToU32(-123), 5, 4, math.MaxUint32),
			x2:    i32x4(i32ToU32(-10), 1, i32ToU32(-104), math.MaxUint32),
			exp:   i32x4(i32ToU32(-113), 4, 108, 0),
		},
		{
			name:  "i64x2",
			shape: wazeroir.ShapeI64x2,
			x1:    i64x2(i64ToU64(math.MinInt64), 12345),
			x2:    i64x2(i64ToU64(-1), i64ToU64(-12345)),
			exp:   i64x2(i64ToU64(math.MinInt64+1), 12345*2),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(1.0, 123, float32(math.Inf(1)), float32(math.Inf(-1))),
			x2:    f32x4(51234.12341, 123, math.MaxFloat32, -123),
			exp:   f32x4(-51233.12341, 0, float32(math.Inf(1)), float32(math.Inf(-1))),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(1.123, math.Inf(1)),
			x2:    f64x2(1.123, math.MinInt64),
			exp:   f64x2(0, math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Sub(&wazeroir.OperationV128Sub{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Load(t *testing.T) {
	tests := []struct {
		name       string
		memSetupFn func(buf []byte)
		loadType   wazeroir.V128LoadType
		offset     uint32
		exp        [16]byte
	}{
		{
			name: "v128 offset=0", loadType: wazeroir.V128LoadType128, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
			},
			exp: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		},
		{
			name: "v128 offset=2", loadType: wazeroir.V128LoadType128, offset: 2,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
			},
			exp: [16]byte{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		},
		{
			name: "8x8s offset=0", loadType: wazeroir.V128LoadType8x8s, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0, 0xff, 0xff, 3, 0, 0xff, 0xff, 5, 0, 0xff, 0xff, 7, 0, 0xff, 0xff,
			},
		},
		{
			name: "8x8s offset=3", loadType: wazeroir.V128LoadType8x8s, offset: 3,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				0xff, 0xff, 5, 0, 0xff, 0xff, 7, 0, 0xff, 0xff, 9, 0, 10, 0, 11, 0,
			},
		},
		{
			name: "8x8u offset=0", loadType: wazeroir.V128LoadType8x8u, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0, 0xff, 0, 3, 0, 0xff, 0, 5, 0, 0xff, 0, 7, 0, 0xff, 0,
			},
		},
		{
			name: "8x8i offset=3", loadType: wazeroir.V128LoadType8x8u, offset: 3,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				0xff, 0, 5, 0, 0xff, 0, 7, 0, 0xff, 0, 9, 0, 10, 0, 11, 0,
			},
		},
		{
			name: "16x4s offset=0", loadType: wazeroir.V128LoadType16x4s, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 0xff, 0xff,
				3, 0xff, 0xff, 0xff,
				5, 0xff, 0xff, 0xff,
				7, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "16x4s offset=3", loadType: wazeroir.V128LoadType16x4s, offset: 3,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 0xff, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				0xff, 5, 0, 0,
				6, 0xff, 0xff, 0xff,
				0xff, 9, 0, 0,
				10, 11, 0, 0,
			},
		},
		{
			name: "16x4u offset=0", loadType: wazeroir.V128LoadType16x4u, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 0xff, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 0, 0,
				3, 0xff, 0, 0,
				5, 0xff, 0, 0,
				7, 0xff, 0, 0,
			},
		},
		{
			name: "16x4u offset=3", loadType: wazeroir.V128LoadType16x4u, offset: 3,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 0xff, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				0xff, 5, 0, 0,
				6, 0xff, 0, 0,
				0xff, 9, 0, 0,
				10, 11, 0, 0,
			},
		},
		{
			name: "32x2s offset=0", loadType: wazeroir.V128LoadType32x2s, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 3, 0xff, 0xff, 0xff, 0xff, 0xff,
				5, 6, 7, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "32x2s offset=2", loadType: wazeroir.V128LoadType32x2s, offset: 2,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				3, 0xff, 5, 6, 0, 0, 0, 0,
				7, 0xff, 9, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "32x2u offset=0", loadType: wazeroir.V128LoadType32x2u, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 3, 0xff, 0, 0, 0, 0,
				5, 6, 7, 0xff, 0, 0, 0, 0,
			},
		},
		{
			name: "32x2u offset=2", loadType: wazeroir.V128LoadType32x2u, offset: 2,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				3, 0xff, 5, 6, 0, 0, 0, 0,
				7, 0xff, 9, 0xff, 0, 0, 0, 0,
			},
		},
		{
			name: "32zero offset=0", loadType: wazeroir.V128LoadType32zero, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 3, 0xff, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name: "32zero offset=3", loadType: wazeroir.V128LoadType32zero, offset: 3,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 0xff, 8, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				0xff, 5, 6, 0xff, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name: "32zero on ceil", loadType: wazeroir.V128LoadType32zero,
			offset: wasm.MemoryPageSize - 4,
			memSetupFn: func(buf []byte) {
				copy(buf[wasm.MemoryPageSize-8:], []byte{
					1, 0xff, 3, 0xff,
					5, 6, 0xff, 8,
				})
			},
			exp: [16]byte{5, 6, 0xff, 8},
		},
		{
			name: "64zero offset=0", loadType: wazeroir.V128LoadType64zero, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				1, 0xff, 3, 0xff, 5, 6, 7, 0xff,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name: "64zero offset=2", loadType: wazeroir.V128LoadType64zero, offset: 2,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				})
			},
			exp: [16]byte{
				3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name: "64zero on ceil", loadType: wazeroir.V128LoadType64zero,
			offset: wasm.MemoryPageSize - 8,
			memSetupFn: func(buf []byte) {
				copy(buf[wasm.MemoryPageSize-16:], []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff,
					9, 0xff, 11, 12, 13, 14, 15,
				})
			},
			exp: [16]byte{9, 0xff, 11, 12, 13, 14, 15, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "8splat offset=0", loadType: wazeroir.V128LoadType8Splat, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			name: "8splat offset=1", loadType: wazeroir.V128LoadType8Splat, offset: 1,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name: "16splat offset=0", loadType: wazeroir.V128LoadType16Splat, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff},
		},
		{
			name: "16splat offset=5", loadType: wazeroir.V128LoadType16Splat, offset: 5,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{6, 7, 6, 7, 6, 7, 6, 7, 6, 7, 6, 7, 6, 7, 6, 7},
		},
		{
			name: "32splat offset=0", loadType: wazeroir.V128LoadType32Splat, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{1, 0xff, 3, 0xff, 1, 0xff, 3, 0xff, 1, 0xff, 3, 0xff, 1, 0xff, 3, 0xff},
		},
		{
			name: "32splat offset=1", loadType: wazeroir.V128LoadType32Splat, offset: 1,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{0xff, 3, 0xff, 5, 0xff, 3, 0xff, 5, 0xff, 3, 0xff, 5, 0xff, 3, 0xff, 5},
		},
		{
			name: "64splat offset=0", loadType: wazeroir.V128LoadType64Splat, offset: 0,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 1, 0xff, 3, 0xff, 5, 6, 7, 0xff},
		},
		{
			name: "64splat offset=1", loadType: wazeroir.V128LoadType64Splat, offset: 1,
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff,
				})
			},
			exp: [16]byte{0xff, 3, 0xff, 5, 6, 7, 0xff, 9, 0xff, 3, 0xff, 5, 6, 7, 0xff, 9},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			tc.memSetupFn(env.memory())

			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileV128Load(&wazeroir.OperationV128Load{
				Type: tc.loadType, Arg: &wazeroir.MemoryArg{},
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
			loadedLocation := compiler.runtimeValueLocationStack().peek()
			require.True(t, loadedLocation.onRegister())

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			require.Equal(t, uint64(2), env.stackPointer())
			lo, hi := env.stackTopAsV128()

			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128LoadLane(t *testing.T) {
	originalVecLo, originalVecHi := uint64(0), uint64(0)
	tests := []struct {
		name                string
		memSetupFn          func(buf []byte)
		laneIndex, laneSize byte
		offset              uint32
		exp                 [16]byte
	}{
		{
			name: "8_lane offset=0 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff,
				})
			},
			laneSize:  8,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "8_lane offset=1 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff,
				})
			},
			laneSize:  8,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "8_lane offset=1 laneIndex=5",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff,
				})
			},
			laneSize:  8,
			laneIndex: 5,
			offset:    1,
			exp:       [16]byte{0, 0, 0, 0, 0, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "16_lane offset=0 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa,
				})
			},
			laneSize:  16,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "16_lane offset=1 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa,
				})
			},
			laneSize:  16,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0xff, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "16_lane offset=1 laneIndex=5",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa,
				})
			},
			laneSize:  16,
			laneIndex: 5,
			offset:    1,
			exp:       [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 1, 0, 0, 0, 0},
		},
		{
			name: "32_lane offset=0 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8,
				})
			},
			laneSize:  32,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 0xff, 1, 0xa, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "32_lane offset=1 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8,
				})
			},
			laneSize:  32,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0xff, 1, 0xa, 0x9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "32_lane offset=1 laneIndex=3",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8,
				})
			},
			laneSize:  32,
			laneIndex: 3,
			offset:    1,
			exp:       [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 1, 0xa, 0x9},
		},

		{
			name: "64_lane offset=0 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8, 0x1, 0x2, 0x3, 0x4,
				})
			},
			laneSize:  64,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 0xff, 1, 0xa, 0x9, 0x8, 0x1, 0x2, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "64_lane offset=1 laneIndex=0",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8, 0x1, 0x2, 0x3, 0x4,
				})
			},
			laneSize:  64,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0xff, 1, 0xa, 0x9, 0x8, 0x1, 0x2, 0x3, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "64_lane offset=3 laneIndex=1",
			memSetupFn: func(buf []byte) {
				copy(buf, []byte{
					1, 0xff, 1, 0xa, 0x9, 0x8, 0x1, 0x2, 0x3, 0x4, 0xa,
				})
			},
			laneSize:  64,
			laneIndex: 1,
			offset:    3,
			exp:       [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xa, 0x9, 0x8, 0x1, 0x2, 0x3, 0x4, 0xa},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			tc.memSetupFn(env.memory())

			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: originalVecLo,
				Hi: originalVecHi,
			})
			require.NoError(t, err)

			err = compiler.compileV128LoadLane(&wazeroir.OperationV128LoadLane{
				LaneIndex: tc.laneIndex, LaneSize: tc.laneSize, Arg: &wazeroir.MemoryArg{},
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
			loadedLocation := compiler.runtimeValueLocationStack().peek()
			require.True(t, loadedLocation.onRegister())

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, uint64(2), env.stackPointer())
			lo, hi := env.stackTopAsV128()

			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Store(t *testing.T) {
	tests := []struct {
		name   string
		offset uint32
	}{
		{name: "offset=1", offset: 1},
		{name: "offset=5", offset: 5},
		{name: "offset=10", offset: 10},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{Lo: ^uint64(0), Hi: ^uint64(0)})
			require.NoError(t, err)

			err = compiler.compileV128Store(&wazeroir.OperationV128Store{Arg: &wazeroir.MemoryArg{}})
			require.NoError(t, err)

			require.Equal(t, uint64(0), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 0, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, uint64(0), env.stackPointer())

			mem := env.memory()
			require.Equal(t, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
				mem[tc.offset:tc.offset+16])
		})
	}
}

func TestCompiler_compileV128StoreLane(t *testing.T) {
	vecBytes := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	tests := []struct {
		name                string
		laneIndex, laneSize byte
		offset              uint32
		exp                 [16]byte
	}{
		{
			name:      "8_lane offset=0 laneIndex=0",
			laneSize:  8,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "8_lane offset=1 laneIndex=0",
			laneSize:  8,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "8_lane offset=3 laneIndex=5",
			laneSize:  8,
			laneIndex: 5,
			offset:    3,
			exp:       [16]byte{0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "16_lane offset=0 laneIndex=0",
			laneSize:  16,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "16_lane offset=1 laneIndex=0",
			laneSize:  16,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0, 1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "16_lane offset=5 laneIndex=7",
			laneSize:  16,
			laneIndex: 7,
			offset:    5,
			exp:       [16]byte{0, 0, 0, 0, 0, 15, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},

		{
			name:      "32_lane offset=0 laneIndex=0",
			laneSize:  32,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "32_lane offset=1 laneIndex=0",
			laneSize:  32,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0, 1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "32_lane offset=5 laneIndex=3",
			laneSize:  32,
			laneIndex: 3,
			offset:    5,
			exp:       [16]byte{0, 0, 0, 0, 0, 13, 14, 15, 16, 0, 0, 0, 0, 0, 0, 0},
		},

		{
			name:      "64_lane offset=0 laneIndex=0",
			laneSize:  64,
			laneIndex: 0,
			offset:    0,
			exp:       [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "64_lane offset=1 laneIndex=0",
			laneSize:  64,
			laneIndex: 0,
			offset:    1,
			exp:       [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:      "64_lane offset=5 laneIndex=3",
			laneSize:  64,
			laneIndex: 1,
			offset:    6,
			exp:       [16]byte{0, 0, 0, 0, 0, 0, 9, 10, 11, 12, 13, 14, 15, 16, 0, 0},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(vecBytes[:8]),
				Hi: binary.LittleEndian.Uint64(vecBytes[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128StoreLane(&wazeroir.OperationV128StoreLane{
				LaneIndex: tc.laneIndex, LaneSize: tc.laneSize, Arg: &wazeroir.MemoryArg{},
			})
			require.NoError(t, err)

			require.Equal(t, uint64(0), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 0, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, tc.exp[:], env.memory()[:16])
		})
	}
}

func TestCompiler_compileV128ExtractLane(t *testing.T) {
	tests := []struct {
		name      string
		vecBytes  [16]byte
		shape     wazeroir.Shape
		signed    bool
		laneIndex byte
		exp       uint64
	}{
		{
			name:      "i8x16 unsigned index=0",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			shape:     wazeroir.ShapeI8x16,
			signed:    false,
			laneIndex: 0,
			exp:       uint64(byte(1)),
		},
		{
			name:      "i8x16 unsigned index=15",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xff},
			shape:     wazeroir.ShapeI8x16,
			signed:    false,
			laneIndex: 15,
			exp:       uint64(byte(0xff)),
		},
		{
			name:      "i8x16 signed index=0",
			vecBytes:  [16]byte{0xf1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			shape:     wazeroir.ShapeI8x16,
			signed:    true,
			laneIndex: 0,
			exp:       uint64(0xff_ff_ff_f1),
		},
		{
			name:      "i8x16 signed index=1",
			vecBytes:  [16]byte{0xf0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			shape:     wazeroir.ShapeI8x16,
			signed:    true,
			laneIndex: 1,
			exp:       uint64(2),
		},
		{
			name:      "i16x8 unsigned index=0",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			shape:     wazeroir.ShapeI16x8,
			signed:    false,
			laneIndex: 0,
			exp:       uint64(uint16(0x2<<8 | 0x1)),
		},
		{
			name:      "i16x8 unsigned index=7",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xff},
			shape:     wazeroir.ShapeI16x8,
			signed:    false,
			laneIndex: 7,
			exp:       uint64(uint16(0xff<<8 | 15)),
		},
		{
			name:      "i16x8 signed index=0",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			shape:     wazeroir.ShapeI16x8,
			signed:    true,
			laneIndex: 0,
			exp:       uint64(uint16(0x2<<8 | 0x1)),
		},
		{
			name:      "i16x8 signed index=7",
			vecBytes:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xf1},
			shape:     wazeroir.ShapeI16x8,
			signed:    true,
			laneIndex: 7,
			exp:       uint64(uint32(0xffff<<16) | uint32(uint16(0xf1<<8|15))),
		},
		{
			name:      "i32x4 index=0",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeI32x4,
			laneIndex: 0,
			exp:       uint64(uint32(0x04_03_02_01)),
		},
		{
			name:      "i32x4 index=3",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeI32x4,
			laneIndex: 3,
			exp:       uint64(uint32(0x16_15_14_13)),
		},
		{
			name:      "i64x4 index=0",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeI64x2,
			laneIndex: 0,
			exp:       uint64(0x08_07_06_05_04_03_02_01),
		},
		{
			name:      "i64x4 index=1",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeI64x2,
			laneIndex: 1,
			exp:       uint64(0x16_15_14_13_12_11_10_09),
		},
		{
			name:      "f32x4 index=0",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 0,
			exp:       uint64(uint32(0x04_03_02_01)),
		},
		{
			name:      "f32x4 index=3",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 3,
			exp:       uint64(uint32(0x16_15_14_13)),
		},
		{
			name:      "f64x4 index=0",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 0,
			exp:       uint64(0x08_07_06_05_04_03_02_01),
		},
		{
			name:      "f64x4 index=1",
			vecBytes:  [16]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 1,
			exp:       uint64(0x16_15_14_13_12_11_10_09),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.vecBytes[:8]),
				Hi: binary.LittleEndian.Uint64(tc.vecBytes[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128ExtractLane(&wazeroir.OperationV128ExtractLane{
				LaneIndex: tc.laneIndex,
				Signed:    tc.signed,
				Shape:     tc.shape,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			vt := compiler.runtimeValueLocationStack().peek().valueType
			switch tc.shape {
			case wazeroir.ShapeI8x16, wazeroir.ShapeI16x8, wazeroir.ShapeI32x4:
				require.Equal(t, runtimeValueTypeI32, vt)
			case wazeroir.ShapeI64x2:
				require.Equal(t, runtimeValueTypeI64, vt)
			case wazeroir.ShapeF32x4:
				require.Equal(t, runtimeValueTypeF32, vt)
			case wazeroir.ShapeF64x2:
				require.Equal(t, runtimeValueTypeF64, vt)
			}

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			switch tc.shape {
			case wazeroir.ShapeI8x16, wazeroir.ShapeI16x8, wazeroir.ShapeI32x4, wazeroir.ShapeF32x4:
				require.Equal(t, uint32(tc.exp), env.stackTopAsUint32())
			case wazeroir.ShapeI64x2, wazeroir.ShapeF64x2:
				require.Equal(t, tc.exp, env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compileV128ReplaceLane(t *testing.T) {
	tests := []struct {
		name               string
		originValueSetupFn func(*testing.T, compilerImpl)
		shape              wazeroir.Shape
		laneIndex          byte
		exp                [16]byte
		lo, hi             uint64
	}{
		{
			name:      "i8x16 index=0",
			shape:     wazeroir.ShapeI8x16,
			laneIndex: 5,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xff})
				require.NoError(t, err)
			},
			exp: [16]byte{5: 0xff},
		},
		{
			name:      "i8x16 index=3",
			shape:     wazeroir.ShapeI8x16,
			laneIndex: 5,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xff << 8})
				require.NoError(t, err)
			},
			exp: [16]byte{},
		},
		{
			name:      "i8x16 index=5",
			shape:     wazeroir.ShapeI8x16,
			laneIndex: 5,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xff})
				require.NoError(t, err)
			},
			exp: [16]byte{5: 0xff},
		},
		{
			name:      "i16x8 index=0",
			shape:     wazeroir.ShapeI16x8,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xee_ff})
				require.NoError(t, err)
			},
			exp: [16]byte{0: 0xff, 1: 0xee},
		},
		{
			name:      "i16x8 index=3",
			shape:     wazeroir.ShapeI16x8,
			laneIndex: 3,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xaa_00})
				require.NoError(t, err)
			},
			exp: [16]byte{7: 0xaa},
		},
		{
			name:      "i16x8 index=7",
			shape:     wazeroir.ShapeI16x8,
			laneIndex: 3,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xaa_bb << 16})
				require.NoError(t, err)
			},
			exp: [16]byte{},
		},
		{
			name:      "i32x4 index=0",
			shape:     wazeroir.ShapeI32x4,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xaa_bb_cc_dd})
				require.NoError(t, err)
			},
			exp: [16]byte{0: 0xdd, 1: 0xcc, 2: 0xbb, 3: 0xaa},
		},
		{
			name:      "i32x4 index=3",
			shape:     wazeroir.ShapeI32x4,
			laneIndex: 3,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xaa_bb_cc_dd})
				require.NoError(t, err)
			},
			exp: [16]byte{12: 0xdd, 13: 0xcc, 14: 0xbb, 15: 0xaa},
		},
		{
			name:      "i64x2 index=0",
			shape:     wazeroir.ShapeI64x2,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI64(&wazeroir.OperationConstI64{Value: 0xaa_bb_cc_dd_01_02_03_04})
				require.NoError(t, err)
			},
			exp: [16]byte{0: 0x04, 1: 0x03, 2: 0x02, 3: 0x01, 4: 0xdd, 5: 0xcc, 6: 0xbb, 7: 0xaa},
		},
		{
			name:      "i64x2 index=1",
			shape:     wazeroir.ShapeI64x2,
			laneIndex: 1,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI64(&wazeroir.OperationConstI64{Value: 0xaa_bb_cc_dd_01_02_03_04})
				require.NoError(t, err)
			},
			exp: [16]byte{8: 0x04, 9: 0x03, 10: 0x02, 11: 0x01, 12: 0xdd, 13: 0xcc, 14: 0xbb, 15: 0xaa},
		},
		{
			name:      "f32x4 index=0",
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(0xaa_bb_cc_dd)})
				require.NoError(t, err)
			},
			exp: [16]byte{0: 0xdd, 1: 0xcc, 2: 0xbb, 3: 0xaa},
		},
		{
			name:      "f32x4 index=1",
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 1,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(0xaa_bb_cc_dd)})
				require.NoError(t, err)
			},
			exp: [16]byte{4: 0xdd, 5: 0xcc, 6: 0xbb, 7: 0xaa},
		},
		{
			name:      "f32x4 index=2",
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 2,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(0xaa_bb_cc_dd)})
				require.NoError(t, err)
			},
			exp: [16]byte{8: 0xdd, 9: 0xcc, 10: 0xbb, 11: 0xaa},
		},
		{
			name:      "f32x4 index=3",
			shape:     wazeroir.ShapeF32x4,
			laneIndex: 3,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(0xaa_bb_cc_dd)})
				require.NoError(t, err)
			},
			exp: [16]byte{12: 0xdd, 13: 0xcc, 14: 0xbb, 15: 0xaa},
		},
		{
			name:      "f64x2 index=0",
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(0xaa_bb_cc_dd_01_02_03_04)})
				require.NoError(t, err)
			},
			exp: [16]byte{0: 0x04, 1: 0x03, 2: 0x02, 3: 0x01, 4: 0xdd, 5: 0xcc, 6: 0xbb, 7: 0xaa},
		},
		{
			name:      "f64x2 index=1",
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 1,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(0xaa_bb_cc_dd_01_02_03_04)})
				require.NoError(t, err)
			},
			exp: [16]byte{8: 0x04, 9: 0x03, 10: 0x02, 11: 0x01, 12: 0xdd, 13: 0xcc, 14: 0xbb, 15: 0xaa},
		},
		{
			name:      "f64x2 index=0 / lo,hi = 1.0",
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 0,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(0.0)})
				require.NoError(t, err)
			},
			lo:  math.Float64bits(1.0),
			hi:  math.Float64bits(1.0),
			exp: [16]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf0, 0x3f},
		},
		{
			name:      "f64x2 index=1 / lo,hi = 1.0",
			shape:     wazeroir.ShapeF64x2,
			laneIndex: 1,
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(0.0)})
				require.NoError(t, err)
			},
			lo:  math.Float64bits(1.0),
			hi:  math.Float64bits(1.0),
			exp: [16]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf0, 0x3f, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{Lo: tc.lo, Hi: tc.hi})
			require.NoError(t, err)

			tc.originValueSetupFn(t, compiler)

			err = compiler.compileV128ReplaceLane(&wazeroir.OperationV128ReplaceLane{
				LaneIndex: tc.laneIndex,
				Shape:     tc.shape,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Splat(t *testing.T) {
	tests := []struct {
		name               string
		originValueSetupFn func(*testing.T, compilerImpl)
		shape              wazeroir.Shape
		exp                [16]byte
	}{
		{
			name: "i8x16",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0x1})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeI8x16,
			exp:   [16]byte{0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1},
		},
		{
			name: "i16x8",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xff_11})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeI16x8,
			exp:   [16]byte{0x11, 0xff, 0x11, 0xff, 0x11, 0xff, 0x11, 0xff, 0x11, 0xff, 0x11, 0xff, 0x11, 0xff, 0x11, 0xff},
		},
		{
			name: "i32x4",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI32(&wazeroir.OperationConstI32{Value: 0xff_11_ee_22})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeI32x4,
			exp:   [16]byte{0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff},
		},
		{
			name: "i64x2",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstI64(&wazeroir.OperationConstI64{Value: 0xff_00_ee_00_11_00_22_00})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeI64x2,
			exp:   [16]byte{0x00, 0x22, 0x00, 0x11, 0x00, 0xee, 0x00, 0xff, 0x00, 0x22, 0x00, 0x11, 0x00, 0xee, 0x00, 0xff},
		},
		{
			name: "f32x4",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(0xff_11_ee_22)})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeF32x4,
			exp:   [16]byte{0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff, 0x22, 0xee, 0x11, 0xff},
		},
		{
			name: "f64x2",
			originValueSetupFn: func(t *testing.T, c compilerImpl) {
				err := c.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(0xff_00_ee_00_11_00_22_00)})
				require.NoError(t, err)
			},
			shape: wazeroir.ShapeF64x2,
			exp:   [16]byte{0x00, 0x22, 0x00, 0x11, 0x00, 0xee, 0x00, 0xff, 0x00, 0x22, 0x00, 0x11, 0x00, 0xee, 0x00, 0xff},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			tc.originValueSetupFn(t, compiler)

			err = compiler.compileV128Splat(&wazeroir.OperationV128Splat{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128AnyTrue(t *testing.T) {
	tests := []struct {
		name   string
		lo, hi uint64
		exp    uint32
	}{
		{name: "lo == 0 && hi == 0", lo: 0, hi: 0, exp: 0},
		{name: "lo != 0", lo: 1, exp: 1},
		{name: "hi != 0", hi: 1, exp: 1},
		{name: "lo != 0 && hi != 0", lo: 1, hi: 1, exp: 1},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{Lo: tc.lo, Hi: tc.hi})
			require.NoError(t, err)

			err = compiler.compileV128AnyTrue(&wazeroir.OperationV128AnyTrue{})
			require.NoError(t, err)

			require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)
			require.Equal(t, uint64(1), env.stackPointer())
			require.Equal(t, tc.exp, env.stackTopAsUint32())
		})
	}
}

func TestCompiler_compileV128AllTrue(t *testing.T) {
	tests := []struct {
		name   string
		shape  wazeroir.Shape
		lo, hi uint64
		exp    uint32
	}{
		{
			name:  "i8x16 - true",
			shape: wazeroir.ShapeI8x16,
			lo:    0xffff_ffff_ffff_ffff,
			hi:    0x0101_0101_0101_0101,
			exp:   1,
		},
		{
			name:  "i8x16 - false on lo",
			shape: wazeroir.ShapeI8x16,
			lo:    0xffff_ffff_ffff_ffff,
			hi:    0x1111_1111_0011_1111,
			exp:   0,
		},
		{
			name:  "i8x16 - false on hi",
			shape: wazeroir.ShapeI8x16,
			lo:    0xffff_00ff_ffff_ffff,
			hi:    0x1111_1111_1111_1111,
			exp:   0,
		},
		{
			name:  "i16x8 - true",
			shape: wazeroir.ShapeI16x8,
			lo:    0x1000_0100_0010_0001,
			hi:    0x0101_0101_0101_0101,
			exp:   1,
		},
		{
			name:  "i16x8 - false on hi",
			shape: wazeroir.ShapeI16x8,
			lo:    0x1000_0100_0010_0001,
			hi:    0x1111_1111_0000_1111,
			exp:   0,
		},
		{
			name:  "i16x8 - false on lo",
			shape: wazeroir.ShapeI16x8,
			lo:    0xffff_0000_ffff_ffff,
			hi:    0x1111_1111_1111_1111,
			exp:   0,
		},
		{
			name:  "i32x4 - true",
			shape: wazeroir.ShapeI32x4,
			lo:    0x1000_0000_0010_0000,
			hi:    0x0000_0001_0000_1000,
			exp:   1,
		},
		{
			name:  "i32x4 - true",
			shape: wazeroir.ShapeI32x4,
			lo:    0x0000_1111_1111_0000,
			hi:    0x0000_0001_1000_0000,
			exp:   1,
		},
		{
			name:  "i32x4 - false on lo",
			shape: wazeroir.ShapeI32x4,
			lo:    0x1111_1111_0000_0000,
			hi:    0x1111_1111_1111_1111,
			exp:   0,
		},
		{
			name:  "i32x4 - false on lo",
			shape: wazeroir.ShapeI32x4,
			lo:    0x0000_0000_1111_1111,
			hi:    0x1111_1111_1111_1111,
			exp:   0,
		},
		{
			name:  "i32x4 - false on hi",
			shape: wazeroir.ShapeI32x4,
			lo:    0x1111_1111_1111_1111,
			hi:    0x1111_1111_0000_0000,
			exp:   0,
		},
		{
			name:  "i32x4 - false on hi",
			shape: wazeroir.ShapeI32x4,
			lo:    0x1111_1111_1111_1111,
			hi:    0x0000_0000_1111_1111,
			exp:   0,
		},

		{
			name:  "i64x2 - true",
			shape: wazeroir.ShapeI64x2,
			lo:    0x1000_0000_0000_0000,
			hi:    0x0000_0001_0000_0000,
			exp:   1,
		},
		{
			name:  "i64x2 - true",
			shape: wazeroir.ShapeI64x2,
			lo:    0x0000_0000_0010_0000,
			hi:    0x0000_0000_0000_0100,
			exp:   1,
		},
		{
			name:  "i64x2 - true",
			shape: wazeroir.ShapeI64x2,
			lo:    0x0000_0000_0000_1000,
			hi:    0x1000_0000_0000_0000,
			exp:   1,
		},
		{
			name:  "i64x2 - false on lo",
			shape: wazeroir.ShapeI64x2,
			lo:    0,
			hi:    0x1111_1111_1111_1111,
			exp:   0,
		},
		{
			name:  "i64x2 - false on hi",
			shape: wazeroir.ShapeI64x2,
			lo:    0x1111_1111_1111_1111,
			hi:    0,
			exp:   0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{Lo: tc.lo, Hi: tc.hi})
			require.NoError(t, err)

			err = compiler.compileV128AllTrue(&wazeroir.OperationV128AllTrue{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, 0, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)
			require.Equal(t, uint64(1), env.stackPointer())
			require.Equal(t, tc.exp, env.stackTopAsUint32())
		})
	}
}
func i8ToU8(v int8) byte {
	return byte(v)
}

func i16ToU16(v int16) uint16 {
	return uint16(v)
}

func i32ToU32(v int32) uint32 {
	return uint32(v)
}

func i64ToU64(v int64) uint64 {
	return uint64(v)
}

func TestCompiler_compileV128Swizzle(t *testing.T) {

	tests := []struct {
		name              string
		indexVec, baseVec [16]byte
		expVec            [16]byte
	}{
		{
			name:     "1",
			baseVec:  [16]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			indexVec: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			expVec:   [16]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
		},
		{
			name: "2",
			baseVec: [16]byte{i8ToU8(-16), i8ToU8(-15), i8ToU8(-14), i8ToU8(-13), i8ToU8(-12),
				i8ToU8(-11), i8ToU8(-10), i8ToU8(-9), i8ToU8(-8), i8ToU8(-7), i8ToU8(-6), i8ToU8(-5),
				i8ToU8(-4), i8ToU8(-3), i8ToU8(-2), i8ToU8(-1)},
			indexVec: [16]byte{i8ToU8(-8), i8ToU8(-7), i8ToU8(-6), i8ToU8(-5), i8ToU8(-4),
				i8ToU8(-3), i8ToU8(-2), i8ToU8(-1), 16, 17, 18, 19, 20, 21, 22, 23},
			expVec: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:     "3",
			baseVec:  [16]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115},
			indexVec: [16]byte{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
			expVec:   [16]byte{115, 114, 113, 112, 111, 110, 109, 108, 107, 106, 105, 104, 103, 102, 101, 100},
		},
		{
			name:    "4",
			baseVec: [16]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115},
			indexVec: [16]byte{
				9, 16, 10, 17, 11, 18, 12, 19, 13, 20, 14, 21, 15, 22, 16, 23,
			},
			expVec: [16]byte{109, 0, 110, 0, 111, 0, 112, 0, 113, 0, 114, 0, 115, 0, 0, 0},
		},
		{
			name:     "5",
			baseVec:  [16]byte{0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70, 0x71, 0x72, 0x73},
			indexVec: [16]byte{9, 16, 10, 17, 11, 18, 12, 19, 13, 20, 14, 21, 15, 22, 16, 23},
			expVec:   [16]byte{0x6d, 0, 0x6e, 0, 0x6f, 0, 0x70, 0, 0x71, 0, 0x72, 0, 0x73, 0, 0, 0},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.baseVec[:8]),
				Hi: binary.LittleEndian.Uint64(tc.baseVec[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.indexVec[:8]),
				Hi: binary.LittleEndian.Uint64(tc.indexVec[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Swizzle(&wazeroir.OperationV128Swizzle{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.expVec, actual)
		})
	}
}

func TestCompiler_compileV128Shuffle(t *testing.T) {
	tests := []struct {
		name             string
		lanes, w, v, exp [16]byte
	}{
		{
			name:  "v only",
			lanes: [16]byte{1, 1, 1, 1, 0, 0, 0, 0, 10, 10, 10, 10, 0, 0, 0, 0},
			v:     [16]byte{0: 0xa, 1: 0xb, 10: 0xc},
			w:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			exp: [16]byte{
				0xb, 0xb, 0xb, 0xb,
				0xa, 0xa, 0xa, 0xa,
				0xc, 0xc, 0xc, 0xc,
				0xa, 0xa, 0xa, 0xa,
			},
		},
		{
			name:  "w only",
			lanes: [16]byte{17, 17, 17, 17, 16, 16, 16, 16, 26, 26, 26, 26, 16, 16, 16, 16},
			v:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			w:     [16]byte{0: 0xa, 1: 0xb, 10: 0xc},
			exp: [16]byte{
				0xb, 0xb, 0xb, 0xb,
				0xa, 0xa, 0xa, 0xa,
				0xc, 0xc, 0xc, 0xc,
				0xa, 0xa, 0xa, 0xa,
			},
		},
		{
			name:  "mix",
			lanes: [16]byte{0, 17, 2, 19, 4, 21, 6, 23, 8, 25, 10, 27, 12, 29, 14, 31},
			v: [16]byte{
				0x1, 0xff, 0x2, 0xff, 0x3, 0xff, 0x4, 0xff,
				0x5, 0xff, 0x6, 0xff, 0x7, 0xff, 0x8, 0xff,
			},
			w: [16]byte{
				0xff, 0x11, 0xff, 0x12, 0xff, 0x13, 0xff, 0x14,
				0xff, 0x15, 0xff, 0x16, 0xff, 0x17, 0xff, 0x18,
			},
			exp: [16]byte{
				0x1, 0x11, 0x2, 0x12, 0x3, 0x13, 0x4, 0x14,
				0x5, 0x15, 0x6, 0x16, 0x7, 0x17, 0x8, 0x18,
			},
		},
		{
			name:  "mix",
			lanes: [16]byte{0, 17, 2, 19, 4, 21, 6, 23, 8, 25, 10, 27, 12, 29, 14, 31},
			v: [16]byte{
				0x1, 0xff, 0x2, 0xff, 0x3, 0xff, 0x4, 0xff,
				0x5, 0xff, 0x6, 0xff, 0x7, 0xff, 0x8, 0xff,
			},
			w: [16]byte{
				0xff, 0x11, 0xff, 0x12, 0xff, 0x13, 0xff, 0x14,
				0xff, 0x15, 0xff, 0x16, 0xff, 0x17, 0xff, 0x18,
			},
			exp: [16]byte{
				0x1, 0x11, 0x2, 0x12, 0x3, 0x13, 0x4, 0x14,
				0x5, 0x15, 0x6, 0x16, 0x7, 0x17, 0x8, 0x18,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.w[:8]),
				Hi: binary.LittleEndian.Uint64(tc.w[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Shuffle(&wazeroir.OperationV128Shuffle{Lanes: tc.lanes})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Bitmask(t *testing.T) {

	u16x8 := func(u1, u2, u3, u4, u5, u6, u7, u8 uint16) (ret [16]byte) {
		binary.LittleEndian.PutUint16(ret[0:], u1)
		binary.LittleEndian.PutUint16(ret[2:], u2)
		binary.LittleEndian.PutUint16(ret[4:], u3)
		binary.LittleEndian.PutUint16(ret[6:], u4)
		binary.LittleEndian.PutUint16(ret[8:], u5)
		binary.LittleEndian.PutUint16(ret[10:], u6)
		binary.LittleEndian.PutUint16(ret[12:], u7)
		binary.LittleEndian.PutUint16(ret[14:], u8)
		return
	}
	u32x4 := func(u1, u2, u3, u4 uint32) (ret [16]byte) {
		binary.LittleEndian.PutUint32(ret[0:], u1)
		binary.LittleEndian.PutUint32(ret[4:], u2)
		binary.LittleEndian.PutUint32(ret[8:], u3)
		binary.LittleEndian.PutUint32(ret[12:], u4)
		return
	}
	u64x2 := func(u1, u2 uint64) (ret [16]byte) {
		binary.LittleEndian.PutUint64(ret[0:], u1)
		binary.LittleEndian.PutUint64(ret[8:], u2)
		return
	}

	tests := []struct {
		name  string
		shape wazeroir.Shape
		v     [16]byte
		exp   uint32
	}{
		{
			name: wasm.OpcodeVecI8x16BitMaskName,
			v: [16]byte{
				i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1,
				i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1,
			},
			shape: wazeroir.ShapeI8x16,
			exp:   0b0101_0101_0101_0101,
		},
		{
			name: wasm.OpcodeVecI8x16BitMaskName,
			v: [16]byte{
				i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			shape: wazeroir.ShapeI8x16,
			exp:   0b0000_0000_0101_0101,
		},
		{
			name: wasm.OpcodeVecI8x16BitMaskName,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1, i8ToU8(-1), 1,
			},
			shape: wazeroir.ShapeI8x16,
			exp:   0b0101_0101_0000_0000,
		},
		{
			name:  wasm.OpcodeVecI16x8BitMaskName,
			v:     u16x8(0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff),
			shape: wazeroir.ShapeI16x8,
			exp:   0b1111_1111,
		},
		{
			name:  wasm.OpcodeVecI16x8BitMaskName,
			v:     u16x8(0, 0xffff, 0, 0xffff, 0, 0xffff, 0, 0xffff),
			shape: wazeroir.ShapeI16x8,
			exp:   0b1010_1010,
		},
		{
			name:  wasm.OpcodeVecI32x4BitMaskName,
			v:     u32x4(0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff),
			shape: wazeroir.ShapeI32x4,
			exp:   0b1111,
		},
		{
			name:  wasm.OpcodeVecI32x4BitMaskName,
			v:     u32x4(0, 0xffffffff, 0xffffffff, 0),
			shape: wazeroir.ShapeI32x4,
			exp:   0b0110,
		},
		{
			name:  wasm.OpcodeVecI64x2BitMaskName,
			v:     u64x2(0, 0xffffffffffffffff),
			shape: wazeroir.ShapeI64x2,
			exp:   0b10,
		},
		{
			name:  wasm.OpcodeVecI64x2BitMaskName,
			v:     u64x2(0xffffffffffffffff, 0xffffffffffffffff),
			shape: wazeroir.ShapeI64x2,
			exp:   0b11,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128BitMask(&wazeroir.OperationV128BitMask{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.

			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			actual := env.stackTopAsUint32()
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128_Not(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler,
		&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	var originalLo, originalHi uint64 = 0xffff_0000_ffff_0000, 0x0000_ffff_0000_ffff

	err = compiler.compileV128Const(&wazeroir.OperationV128Const{
		Lo: originalLo,
		Hi: originalHi,
	})
	require.NoError(t, err)

	err = compiler.compileV128Not(&wazeroir.OperationV128Not{})
	require.NoError(t, err)

	require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
	require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	lo, hi := env.stackTopAsV128()
	require.Equal(t, ^originalLo, lo)
	require.Equal(t, ^originalHi, hi)
}

func TestCompiler_compileV128_And_Or_Xor_AndNot(t *testing.T) {

	tests := []struct {
		name        string
		op          wazeroir.OperationKind
		x1, x2, exp [16]byte
	}{
		{
			name: "AND",
			op:   wazeroir.OperationKindV128And,
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x2:  [16]byte{},
			exp: [16]byte{},
		},
		{
			name: "AND",
			op:   wazeroir.OperationKindV128And,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1:  [16]byte{},
			exp: [16]byte{},
		},
		{
			name: "AND",
			op:   wazeroir.OperationKindV128And,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1:  [16]byte{0: 0x1, 5: 0x1, 15: 0x1},
			exp: [16]byte{0: 0x1, 5: 0x1, 15: 0x1},
		},
		{
			name: "OR",
			op:   wazeroir.OperationKindV128Or,
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x2: [16]byte{},
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "OR",
			op:   wazeroir.OperationKindV128Or,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1: [16]byte{},
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "OR",
			op:   wazeroir.OperationKindV128Or,
			x2:   [16]byte{},
			x1:   [16]byte{0: 0x1, 5: 0x1, 15: 0x1},
			exp:  [16]byte{0: 0x1, 5: 0x1, 15: 0x1},
		},
		{
			name: "OR",
			op:   wazeroir.OperationKindV128Or,
			x2:   [16]byte{8: 0x1, 10: 0x1},
			x1:   [16]byte{0: 0x1},
			exp:  [16]byte{0: 0x1, 8: 0x1, 10: 0x1},
		},
		{
			name: "XOR",
			op:   wazeroir.OperationKindV128Xor,
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x2: [16]byte{},
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "XOR",
			op:   wazeroir.OperationKindV128Xor,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1: [16]byte{},
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "XOR",
			op:   wazeroir.OperationKindV128Xor,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{},
		},
		{
			name: "XOR",
			op:   wazeroir.OperationKindV128Xor,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1: [16]byte{0: 0x1, 15: 0x2},
			exp: [16]byte{
				0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfd,
			},
		},

		{
			name: "AndNot",
			op:   wazeroir.OperationKindV128AndNot,
			x2:   [16]byte{},
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name: "AndNot",
			op:   wazeroir.OperationKindV128AndNot,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1:  [16]byte{},
			exp: [16]byte{},
		},
		{
			name: "AndNot",
			op:   wazeroir.OperationKindV128AndNot,
			x2: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{},
		},
		{
			name: "AndNot",
			op:   wazeroir.OperationKindV128AndNot,
			x2: [16]byte{
				0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfd,
			},
			x1:  [16]byte{0: 0x1, 15: 0x2},
			exp: [16]byte{0: 0x1, 15: 0x2},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			switch tc.op {
			case wazeroir.OperationKindV128And:
				err = compiler.compileV128And(nil) // And doesn't use the param.
			case wazeroir.OperationKindV128Or:
				err = compiler.compileV128Or(nil) // Or doesn't use the param.
			case wazeroir.OperationKindV128Xor:
				err = compiler.compileV128Xor(nil) // Xor doesn't use the param.
			case wazeroir.OperationKindV128AndNot:
				err = compiler.compileV128AndNot(nil) // AndNot doesn't use the param.
			}
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Bitselect(t *testing.T) {
	tests := []struct {
		name                  string
		selector, x1, x2, exp [16]byte
	}{
		{
			name: "all x1",
			selector: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			x1:  [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			x2:  [16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			exp: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			name:     "all x2",
			selector: [16]byte{},
			x1:       [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			x2:       [16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			exp:      [16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
		},
		{
			name: "mix",
			selector: [16]byte{
				0b1111_0000, 0b1111_0000, 0b1111_0000, 0b1111_0000, 0b1111_0000, 0b1111_0000, 0b1111_0000, 0b1111_0000,
				0b0000_0000, 0b0000_0000, 0b0000_0000, 0b0000_0000, 0b1111_1111, 0b1111_1111, 0b1111_1111, 0b1111_1111,
			},
			x1: [16]byte{
				0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010,
				0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010,
			},
			x2: [16]byte{
				0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101,
				0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101,
			},
			exp: [16]byte{
				0b1010_0101, 0b1010_0101, 0b1010_0101, 0b1010_0101, 0b1010_0101, 0b1010_0101, 0b1010_0101, 0b1010_0101,
				0b0101_0101, 0b0101_0101, 0b0101_0101, 0b0101_0101, 0b1010_1010, 0b1010_1010, 0b1010_1010, 0b1010_1010,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.selector[:8]),
				Hi: binary.LittleEndian.Uint64(tc.selector[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Bitselect(nil)
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Shl(t *testing.T) {
	tests := []struct {
		name   string
		shape  wazeroir.Shape
		s      uint32
		x, exp [16]byte
	}{
		{
			name:  "i8x16/shift=0",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s: 0,
		},
		{
			name:  "i8x16/shift=1",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff,
				1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff,
			},
			exp: [16]byte{
				2, 0xfe, 2, 0xfe, 2, 0xfe, 2, 0xfe,
				2, 0xfe, 2, 0xfe, 2, 0xfe, 2, 0xfe,
			},
			s: 1,
		},
		{
			name:  "i8x16/shift=2",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				4, 0, 4, 0, 4, 0, 4, 0,
				4, 0, 4, 0, 4, 0, 4, 0,
			},
			s: 2,
		},
		{
			name:  "i8x16/shift=3",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff,
				1, 0xff, 1, 0xff, 1, 0xff, 1, 0xff,
			},
			exp: [16]byte{
				8, 0xff & ^0b111, 8, 0xff & ^0b111, 8, 0xff & ^0b111, 8, 0xff & ^0b111,
				8, 0xff & ^0b111, 8, 0xff & ^0b111, 8, 0xff & ^0b111, 8, 0xff & ^0b111,
			},
			s: 3,
		},
		{
			name:  "i8x16/shift=4",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1,
				0xff, 1, 0xff, 1, 0xff, 1, 0xff, 1,
			},
			exp: [16]byte{
				0xff & ^0b1111, 16, 0xff & ^0b1111, 16, 0xff & ^0b1111, 16, 0xff & ^0b1111, 16,
				0xff & ^0b1111, 16, 0xff & ^0b1111, 16, 0xff & ^0b1111, 16, 0xff & ^0b1111, 16,
			},
			s: 4,
		},
		{
			name:  "i8x16/shift=5",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				0xff, 0xff, 0xff, 0xff, 1, 1, 1, 1,
				0xff, 0xff, 0xff, 0xff, 1, 1, 1, 1,
			},
			exp: [16]byte{
				0xff & ^0b11111, 0xff & ^0b11111, 0xff & ^0b11111, 0xff & ^0b11111, 32, 32, 32, 32,
				0xff & ^0b11111, 0xff & ^0b11111, 0xff & ^0b11111, 0xff & ^0b11111, 32, 32, 32, 32,
			},
			s: 5,
		},
		{
			name:  "i8x16/shift=6",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				0xff, 0x81, 0xff, 0x81, 0xff, 0x81, 0xff, 0x81,
				0xff, 0x81, 0xff, 0x81, 0xff, 0x81, 0xff, 0x81,
			},
			exp: [16]byte{
				0xc0, 1 << 6, 0xc0, 1 << 6, 0xc0, 1 << 6, 0xc0, 1 << 6,
				0xc0, 1 << 6, 0xc0, 1 << 6, 0xc0, 1 << 6, 0xc0, 1 << 6,
			},
			s: 6,
		},
		{
			name:  "i8x16/shift=7",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{
				0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80,
				0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80,
			},
			s: 7,
		},
		{
			name:  "i16x8/shift=0",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s: 0,
		},
		{
			name:  "i16x8/shift=1",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				2, 0, 2, 0, 2, 0, 2, 0,
				2, 0, 2, 0, 2, 0, 2, 0,
			},
			s: 1,
		},
		{
			name:  "i16x8/shift=7",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				1, 1, 1, 1, 0x80, 0x80, 0x80, 0x80,
				0, 0x80, 0, 0x80, 0b11, 0b11, 0b11, 0b11,
			},
			exp: [16]byte{
				0, 1, 0, 1, 0, 0x80, 0, 0x80,
				0, 0, 0, 0, 0, 0b11, 0, 0b11,
			},
			s: 8,
		},
		{
			name:  "i16x8/shift=15",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{
				0, 0x80, 0, 0x80, 0, 0x80, 0, 0x80,
				0, 0x80, 0, 0x80, 0, 0x80, 0, 0x80,
			},
			s: 15,
		},
		{
			name:  "i32x4/shift=0",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s: 0,
		},
		{
			name:  "i32x4/shift=1",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				1, 0x80, 0, 0x80, 1, 0x80, 0, 0x80,
				1, 0x80, 0, 0x80, 1, 0x80, 0, 0x80,
			},
			exp: [16]byte{
				2, 0, 1, 0, 2, 0, 1, 0,
				2, 0, 1, 0, 2, 0, 1, 0,
			},
			s: 1,
		},
		{
			name:  "i32x4/shift=31",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
			},
			s: 31,
		},
		{
			name:  "i64x2/shift=0",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s: 0,
		},
		{
			name:  "i64x2/shift=5",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1 << 5, 0, 1<<4 | 1<<5, 0, 1<<4 | 1<<5, 0, 1<<4 | 1<<5, 0,
				1 << 5, 0, 1<<4 | 1<<5, 0, 1<<4 | 1<<5, 0, 1<<4 | 1<<5, 0,
			},
			s: 5,
		},
		{
			name:  "i64x2/shift=63",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
			exp: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0x80,
				0, 0, 0, 0, 0, 0, 0, 0x80,
			},
			s: 63,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.s})
			require.NoError(t, err)

			err = compiler.compileV128Shl(&wazeroir.OperationV128Shl{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Shr(t *testing.T) {
	tests := []struct {
		name   string
		signed bool
		shape  wazeroir.Shape
		s      uint32
		x, exp [16]byte
	}{
		{
			name:  "i8x16/shift=0/signed=false",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: false,
		},
		{
			name:  "i8x16/shift=7/signed=false",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80,
				0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80,
			},
			exp: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			s:      7,
			signed: false,
		},
		{
			name:  "i8x16/shift=0/signed=false",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: true,
		},
		{
			name:  "i8x16/shift=7/signed=false",
			shape: wazeroir.ShapeI8x16,
			x: [16]byte{
				1, 0x80, 0x7e, 0x80, 1, 0x80, 0x7e, 0x80,
				1, 0x80, 0x7e, 0x80, 1, 0x80, 0x7e, 0x80,
			},
			exp: [16]byte{
				0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff,
				0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff,
			},
			s:      7,
			signed: true,
		},
		{
			name:  "i16x8/shift=0/signed=false",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: false,
		},
		{
			name:  "i16x8/shift=8/signed=false",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80,
				0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80,
			},
			exp: [16]byte{
				0x80, 0, 0x80, 0, 0x80, 0, 0x80, 0,
				0x80, 0, 0x80, 0, 0x80, 0, 0x80, 0,
			},
			s:      8,
			signed: false,
		},
		{
			name:  "i16x8/shift=0/signed=true",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: true,
		},
		{
			name:  "i16x8/shift=8/signed=true",
			shape: wazeroir.ShapeI16x8,
			x: [16]byte{
				0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80,
				0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80,
			},
			exp: [16]byte{
				0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff,
				0x80, 0xff, 0x80, 0xff, 0x80, 0xff, 0x80, 0xff,
			},
			s:      8,
			signed: true,
		},
		{
			name:  "i32x4/shift=0/signed=false",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: false,
		},
		{
			name:  "i32x4/shift=16/signed=false",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
			},
			exp: [16]byte{
				0, 0x80, 0, 0, 0, 0x80, 0, 0,
				0, 0x80, 0, 0, 0, 0x80, 0, 0,
			},
			s:      16,
			signed: false,
		},
		{
			name:  "i32x4/shift=0/signed=true",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: true,
		},
		{
			name:  "i32x4/shift=16/signed=true",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
			},
			exp: [16]byte{
				0, 0x80, 0xff, 0xff, 0, 0x80, 0xff, 0xff,
				0, 0x80, 0xff, 0xff, 0, 0x80, 0xff, 0xff,
			},
			s:      16,
			signed: true,
		},
		{
			name:  "i64x2/shift=0/signed=false",
			shape: wazeroir.ShapeI32x4,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: false,
		},
		{
			name:  "i64x2/shift=16/signed=false",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
			},
			exp: [16]byte{
				0, 0x80, 0, 0, 0, 0x80, 0, 0,
				0, 0x80, 0, 0, 0, 0x80, 0, 0,
			},
			s:      16,
			signed: false,
		},
		{
			name:  "i64x2/shift=0/signed=true",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			exp: [16]byte{
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
				1, 0x80, 1, 0x80, 1, 0x80, 1, 0x80,
			},
			s:      0,
			signed: true,
		},
		{
			name:  "i64x2/shift=16/signed=true",
			shape: wazeroir.ShapeI64x2,
			x: [16]byte{
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
				0, 0, 0, 0x80, 0, 0, 0, 0x80,
			},
			exp: [16]byte{
				0, 0x80, 0, 0, 0, 0x80, 0xff, 0xff,
				0, 0x80, 0, 0, 0, 0x80, 0xff, 0xff,
			},
			s:      16,
			signed: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.s})
			require.NoError(t, err)

			err = compiler.compileV128Shr(&wazeroir.OperationV128Shr{Shape: tc.shape, Signed: tc.signed})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func i16x8(i1, i2, i3, i4, i5, i6, i7, i8 uint16) (ret [16]byte) {
	binary.LittleEndian.PutUint16(ret[0:], i1)
	binary.LittleEndian.PutUint16(ret[2:], i2)
	binary.LittleEndian.PutUint16(ret[4:], i3)
	binary.LittleEndian.PutUint16(ret[6:], i4)
	binary.LittleEndian.PutUint16(ret[8:], i5)
	binary.LittleEndian.PutUint16(ret[10:], i6)
	binary.LittleEndian.PutUint16(ret[12:], i7)
	binary.LittleEndian.PutUint16(ret[14:], i8)
	return
}

func i32x4(i1, i2, i3, i4 uint32) (ret [16]byte) {
	binary.LittleEndian.PutUint32(ret[0:], i1)
	binary.LittleEndian.PutUint32(ret[4:], i2)
	binary.LittleEndian.PutUint32(ret[8:], i3)
	binary.LittleEndian.PutUint32(ret[12:], i4)
	return
}

func f32x4(f1, f2, f3, f4 float32) (ret [16]byte) {
	binary.LittleEndian.PutUint32(ret[0:], math.Float32bits(f1))
	binary.LittleEndian.PutUint32(ret[4:], math.Float32bits(f2))
	binary.LittleEndian.PutUint32(ret[8:], math.Float32bits(f3))
	binary.LittleEndian.PutUint32(ret[12:], math.Float32bits(f4))
	return
}

func i64x2(i1, i2 uint64) (ret [16]byte) {
	binary.LittleEndian.PutUint64(ret[0:], i1)
	binary.LittleEndian.PutUint64(ret[8:], i2)
	return
}

func f64x2(f1, f2 float64) (ret [16]byte) {
	binary.LittleEndian.PutUint64(ret[0:], math.Float64bits(f1))
	binary.LittleEndian.PutUint64(ret[8:], math.Float64bits(f2))
	return
}

func TestCompiler_compileV128Cmp(t *testing.T) {
	tests := []struct {
		name        string
		cmpType     wazeroir.V128CmpType
		x1, x2, exp [16]byte
	}{
		{
			name:    "f32x4 eq",
			cmpType: wazeroir.V128CmpTypeF32x4Eq,
			x1:      f32x4(1.0, -123.123, 0, 3214231),
			x2:      f32x4(0, 0, 0, 0),
			exp:     [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0},
		},
		{
			name:    "f32x4 ne",
			cmpType: wazeroir.V128CmpTypeF32x4Ne,
			x1:      f32x4(1.0, -123.123, 123, 3214231),
			x2:      f32x4(2.0, 213123123.1231, 123, 0),
			exp:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f32x4 lt",
			cmpType: wazeroir.V128CmpTypeF32x4Lt,
			x1:      f32x4(2.0, -123.123, 1234, 3214231),
			x2:      f32x4(2.0, 213123123.1231, 123, 0),
			exp:     [16]byte{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:    "f32x4 le",
			cmpType: wazeroir.V128CmpTypeF32x4Le,
			x1:      f32x4(2.0, -123.123, 1234, 3214231),
			x2:      f32x4(2.0, 213123123.1231, 123, 0),
			exp:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:    "f32x4 gt",
			cmpType: wazeroir.V128CmpTypeF32x4Gt,
			x1:      f32x4(2.0, -123.123, 1234, 3214231),
			x2:      f32x4(2.0, 213123123.1231, 123, 0),
			exp:     [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f32x4 ge",
			cmpType: wazeroir.V128CmpTypeF32x4Ge,
			x1:      f32x4(2.0, -123.123, 1234, 3214231),
			x2:      f32x4(2.0, 213123123.1231, 123, 0),
			exp:     [16]byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f64x2 eq",
			cmpType: wazeroir.V128CmpTypeF64x2Eq,
			x1:      f64x2(1.0, -123.12412),
			x2:      f64x2(1.0, 123.123124),
			exp:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:    "f64x2 ne",
			cmpType: wazeroir.V128CmpTypeF64x2Ne,
			x1:      f64x2(1.0, -123.12412),
			x2:      f64x2(1.0, 123.123124),
			exp:     [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f64x2 lt",
			cmpType: wazeroir.V128CmpTypeF64x2Lt,
			x1:      f64x2(-123, math.Inf(-1)),
			x2:      f64x2(-123, -1234515),
			exp:     [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f64x2 le",
			cmpType: wazeroir.V128CmpTypeF64x2Le,
			x1:      f64x2(-123, 123),
			x2:      f64x2(-123, math.MaxFloat64),
			exp:     [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "f64x2 gt",
			cmpType: wazeroir.V128CmpTypeF64x2Gt,
			x1:      f64x2(math.MaxFloat64, -123.0),
			x2:      f64x2(123, -123.0),
			exp: [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:    "f64x2 ge",
			cmpType: wazeroir.V128CmpTypeF64x2Ge,
			x1:      f64x2(math.MaxFloat64, -123.0),
			x2:      f64x2(123, -123.0),
			exp: [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "i8x16 eq",
			cmpType: wazeroir.V128CmpTypeI8x16Eq,
			x1:      [16]byte{0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1},
			x2:      [16]byte{1, 1, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 1, 0},
			exp:     [16]byte{0, 0xff, 0, 0xff, 0xff, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0xff, 0xff, 0},
		},
		{
			name:    "i8x16 ne",
			cmpType: wazeroir.V128CmpTypeI8x16Ne,
			x1:      [16]byte{0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1},
			x2:      [16]byte{1, 1, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 1, 0},
			exp:     [16]byte{0xff, 0, 0xff, 0, 0, 0xff, 0, 0, 0, 0, 0, 0, 0xff, 0, 0, 0xff},
		},
		{
			name:    "i8x16 lt_s",
			cmpType: wazeroir.V128CmpTypeI8x16LtS,
			x1:      [16]byte{0: i8ToU8(-1), 15: 0},
			x2:      [16]byte{0: 0x7f, 15: i8ToU8(-1)},
			exp:     [16]byte{0: 0xff},
		},
		{
			name:    "i8x16 lt_u",
			cmpType: wazeroir.V128CmpTypeI8x16LtU,
			x1:      [16]byte{0: 0xff, 15: 0},
			x2:      [16]byte{0: 0x7f, 15: 0xff},
			exp:     [16]byte{15: 0xff},
		},
		{
			name:    "i8x16 gt_s",
			cmpType: wazeroir.V128CmpTypeI8x16GtS,
			x1:      [16]byte{0: i8ToU8(-1), 15: 0},
			x2:      [16]byte{0: 0x7f, 15: i8ToU8(-1)},
			exp:     [16]byte{15: 0xff},
		},
		{
			name:    "i8x16 gt_u",
			cmpType: wazeroir.V128CmpTypeI8x16GtU,
			x1:      [16]byte{0: 0xff, 15: 0},
			x2:      [16]byte{0: 0x7f, 15: 0xff},
			exp:     [16]byte{0: 0xff},
		},
		{
			name:    "i8x16 le_s",
			cmpType: wazeroir.V128CmpTypeI8x16LeS,
			x1:      [16]byte{i8ToU8(-1), 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, i8ToU8(-1)},
			x2:      [16]byte{0: 0x7f, 15: i8ToU8(-1)},
			exp:     [16]byte{0: 0xff, 15: 0xff},
		},
		{
			name:    "i8x16 le_u",
			cmpType: wazeroir.V128CmpTypeI8x16LeU,
			x1:      [16]byte{0x80, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0xff},
			x2:      [16]byte{0: 0x7f, 5: 0x1, 15: 0xff},
			exp:     [16]byte{5: 0xff, 15: 0xff},
		},
		{
			name:    "i8x16 ge_s",
			cmpType: wazeroir.V128CmpTypeI8x16GeS,
			x1:      [16]byte{0: 0x7f, 15: i8ToU8(-1)},
			x2:      [16]byte{i8ToU8(-1), 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, i8ToU8(-1)},
			exp:     [16]byte{0: 0xff, 15: 0xff},
		},
		{
			name:    "i8x16 ge_u",
			cmpType: wazeroir.V128CmpTypeI8x16GeU,
			x1:      [16]byte{0: 0x7f, 3: 0xe, 15: 0xff},
			x2:      [16]byte{0xff, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0xff},
			exp:     [16]byte{3: 0xff, 15: 0xff},
		},
		{
			name:    "i16x8 eq",
			cmpType: wazeroir.V128CmpTypeI16x8Eq,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0xffff, 0, 0xffff, 0xffff, 0, 0xffff, 0xffff, 0),
		},
		{
			name:    "i8x16 ne",
			cmpType: wazeroir.V128CmpTypeI16x8Ne,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0, 0xffff, 0, 0, 0xffff, 0, 0, 0xffff),
		},
		{
			name:    "i8x16 lt_s",
			cmpType: wazeroir.V128CmpTypeI16x8LtS,
			x1:      i16x8(0xffff, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0xffff, 0, 0, 0, 0xffff, 0, 0, 0),
		},
		{
			name:    "i8x16 lt_u",
			cmpType: wazeroir.V128CmpTypeI16x8LtU,
			x1:      i16x8(0xffff, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0, 0, 0, 0, 0xffff, 0, 0, 0),
		},
		{
			name:    "i8x16 gt_s",
			cmpType: wazeroir.V128CmpTypeI16x8GtS,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0xffff, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0xffff, 0xffff, 0, 0, 0, 0, 0, 0xffff),
		},
		{
			name:    "i8x16 gt_u",
			cmpType: wazeroir.V128CmpTypeI16x8GtU,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0xffff, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0, 0xffff, 0, 0, 0, 0, 0, 0xffff),
		},
		{
			name:    "i8x16 le_s",
			cmpType: wazeroir.V128CmpTypeI16x8LeS,
			x1:      i16x8(0xffff, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0xffff, 0, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0),
		},
		{
			name:    "i8x16 le_u",
			cmpType: wazeroir.V128CmpTypeI16x8LeU,
			x1:      i16x8(0xffff, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0, 0, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0),
		},
		{
			name:    "i8x16 ge_s",
			cmpType: wazeroir.V128CmpTypeI16x8GeS,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0xffff, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0xffff, 0xffff, 0xffff, 0xffff, 0, 0xffff, 0xffff, 0xffff),
		},
		{
			name:    "i8x16 ge_u",
			cmpType: wazeroir.V128CmpTypeI16x8GeU,
			x1:      i16x8(0, 1, 0, 1, 0, 1, 0, 1),
			x2:      i16x8(0xffff, 0, 0, 1, 1, 1, 0, 0),
			exp:     i16x8(0, 0xffff, 0xffff, 0xffff, 0, 0xffff, 0xffff, 0xffff),
		},
		{
			name:    "i32x4 eq",
			cmpType: wazeroir.V128CmpTypeI32x4Eq,
			x1:      i32x4(0, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0xffffffff, 0xffffffff, 0, 0),
		},
		{
			name:    "i32x4 ne",
			cmpType: wazeroir.V128CmpTypeI32x4Ne,
			x1:      i32x4(0, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0, 0, 0xffffffff, 0xffffffff),
		},
		{
			name:    "i32x4 lt_s",
			cmpType: wazeroir.V128CmpTypeI32x4LtS,
			x1:      i32x4(0xffffffff, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0xffffffff, 0, 0, 0xffffffff),
		},
		{
			name:    "i32x4 lt_u",
			cmpType: wazeroir.V128CmpTypeI32x4LtU,
			x1:      i32x4(0xffffffff, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0, 0, 0, 0xffffffff),
		},
		{
			name:    "i32x4 gt_s",
			cmpType: wazeroir.V128CmpTypeI32x4GtS,
			x1:      i32x4(0, 1, 1, 1),
			x2:      i32x4(0xffffffff, 1, 0, 0),
			exp:     i32x4(0xffffffff, 0, 0xffffffff, 0xffffffff),
		},
		{
			name:    "i32x4 gt_u",
			cmpType: wazeroir.V128CmpTypeI32x4GtU,
			x1:      i32x4(0, 1, 1, 1),
			x2:      i32x4(0xffffffff, 1, 0, 0),
			exp:     i32x4(0, 0, 0xffffffff, 0xffffffff),
		},
		{
			name:    "i32x4 le_s",
			cmpType: wazeroir.V128CmpTypeI32x4LeS,
			x1:      i32x4(0xffffffff, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0xffffffff, 0xffffffff, 0, 0xffffffff),
		},
		{
			name:    "i32x4 le_u",
			cmpType: wazeroir.V128CmpTypeI32x4LeU,
			x1:      i32x4(0xffffffff, 1, 1, 0),
			x2:      i32x4(0, 1, 0, 1),
			exp:     i32x4(0, 0xffffffff, 0, 0xffffffff),
		},
		{
			name:    "i32x4 ge_s",
			cmpType: wazeroir.V128CmpTypeI32x4GeS,
			x1:      i32x4(0, 1, 1, 0),
			x2:      i32x4(0xffffffff, 1, 0, 1),
			exp:     i32x4(0xffffffff, 0xffffffff, 0xffffffff, 0),
		},
		{
			name:    "i32x4 ge_u",
			cmpType: wazeroir.V128CmpTypeI32x4GeU,
			x1:      i32x4(0, 1, 1, 0),
			x2:      i32x4(0xffffffff, 1, 0, 1),
			exp:     i32x4(0, 0xffffffff, 0xffffffff, 0),
		},
		{
			name:    "i64x2 eq",
			cmpType: wazeroir.V128CmpTypeI64x2Eq,
			x1:      i64x2(1, 0),
			x2:      i64x2(0, 0),
			exp:     i64x2(0, 0xffffffffffffffff),
		},
		{
			name:    "i64x2 ne",
			cmpType: wazeroir.V128CmpTypeI64x2Ne,
			x1:      i64x2(1, 0),
			x2:      i64x2(0, 0),
			exp:     i64x2(0xffffffffffffffff, 0),
		},
		{
			name:    "i64x2 lt_s",
			cmpType: wazeroir.V128CmpTypeI64x2LtS,
			x1:      i64x2(0xffffffffffffffff, 0),
			x2:      i64x2(0, 0),
			exp:     i64x2(0xffffffffffffffff, 0),
		},
		{
			name:    "i64x2 gt_s",
			cmpType: wazeroir.V128CmpTypeI64x2GtS,
			x1:      i64x2(123, 0),
			x2:      i64x2(123, 0xffffffffffffffff),
			exp:     i64x2(0, 0xffffffffffffffff),
		},
		{
			name:    "i64x2 le_s",
			cmpType: wazeroir.V128CmpTypeI64x2LeS,
			x1:      i64x2(123, 0xffffffffffffffff),
			x2:      i64x2(123, 0),
			exp:     i64x2(0xffffffffffffffff, 0xffffffffffffffff),
		},
		{
			name:    "i64x2 ge_s",
			cmpType: wazeroir.V128CmpTypeI64x2GeS,
			x1:      i64x2(123, 0),
			x2:      i64x2(123, 0xffffffffffffffff),
			exp:     i64x2(0xffffffffffffffff, 0xffffffffffffffff),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Cmp(&wazeroir.OperationV128Cmp{Type: tc.cmpType})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128AvgrU(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		x1, x2, exp [16]byte
	}{
		{
			name:  "i8x16",
			shape: wazeroir.ShapeI8x16,
			x1:    [16]byte{0: 1, 2: 10, 10: 10, 15: math.MaxUint8},
			x2:    [16]byte{0: 10, 4: 5, 10: 5, 15: 10},
			exp: [16]byte{
				0:  byte((uint16(1) + uint16(10) + 1) / 2),
				2:  byte((uint16(10) + 1) / 2),
				4:  byte((uint16(5) + 1) / 2),
				10: byte((uint16(10) + uint16(5) + 1) / 2),
				15: byte((uint16(math.MaxUint8) + uint16(10) + 1) / 2),
			},
		},
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			x1:    i16x8(1, 0, 100, 0, 0, math.MaxUint16, 0, 0),
			x2:    i16x8(10, 0, math.MaxUint16, 0, 0, 1, 0, 0),
			exp: i16x8(
				uint16((uint32(1)+uint32(10)+1)/2),
				0,
				uint16((uint32(100)+uint32(math.MaxUint16)+1)/2),
				0,
				0,
				uint16((uint32(1)+uint32(math.MaxUint16)+1)/2),
				0, 0,
			),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128AvgrU(&wazeroir.OperationV128AvgrU{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Sqrt(t *testing.T) {

	tests := []struct {
		name   string
		shape  wazeroir.Shape
		v, exp [16]byte
	}{
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(1.23, -123.1231, math.MaxFloat32, float32(math.Inf(1))),
			exp: f32x4(
				float32(math.Sqrt(float64(float32(1.23)))),
				float32(math.Sqrt(float64(float32(-123.1231)))),
				float32(math.Sqrt(float64(float32(math.MaxFloat32)))),
				float32(math.Sqrt(float64(float32(math.Inf(1))))),
			),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(1.2314, math.MaxFloat64),
			exp:   f64x2(math.Sqrt(1.2314), math.Sqrt(math.MaxFloat64)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Sqrt(&wazeroir.OperationV128Sqrt{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Mul(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		x1, x2, exp [16]byte
	}{
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			x1:    i16x8(1123, 0, 123, 1, 1, 5, 8, 1),
			x2:    i16x8(0, 123, 123, 0, 1, 5, 9, 1),
			exp:   i16x8(0, 0, 123*123, 0, 1, 25, 8*9, 1),
		},
		{
			name:  "i32x4",
			shape: wazeroir.ShapeI32x4,
			x1:    i32x4(i32ToU32(-123), 5, 4, math.MaxUint32),
			x2:    i32x4(i32ToU32(-10), 1, i32ToU32(-104), 0),
			exp:   i32x4(1230, 5, i32ToU32(-416), 0),
		},
		{
			name:  "i64x2",
			shape: wazeroir.ShapeI64x2,
			x1:    i64x2(1, 12345),
			x2:    i64x2(100, i64ToU64(-10)),
			exp:   i64x2(100, i64ToU64(-123450)),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(1.0, 123, float32(math.Inf(1)), float32(math.Inf(-1))),
			x2:    f32x4(51234.12341, 123, math.MaxFloat32, -123),
			exp:   f32x4(51234.12341, 123*123, float32(math.Inf(1)), float32(math.Inf(1))),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(1.123, math.Inf(1)),
			x2:    f64x2(1.123, math.MinInt64),
			exp:   f64x2(1.123*1.123, math.Inf(-1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Mul(&wazeroir.OperationV128Mul{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Neg(t *testing.T) {
	tests := []struct {
		name   string
		shape  wazeroir.Shape
		v, exp [16]byte
	}{
		{
			name:  "i8x16",
			shape: wazeroir.ShapeI8x16,
			v:     [16]byte{1: 123, 5: i8ToU8(-1), 15: i8ToU8(-125)},
			exp:   [16]byte{1: i8ToU8(-123), 5: 1, 15: 125},
		},
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			v:     i16x8(0, 0, i16ToU16(-123), 0, 1, 25, 8, i16ToU16(-1)),
			exp:   i16x8(0, 0, 123, 0, i16ToU16(-1), i16ToU16(-25), i16ToU16(-8), 1),
		},
		{
			name:  "i32x4",
			shape: wazeroir.ShapeI32x4,
			v:     i32x4(1230, 5, i32ToU32(-416), 0),
			exp:   i32x4(i32ToU32(-1230), i32ToU32(-5), 416, 0),
		},
		{
			name:  "i64x2",
			shape: wazeroir.ShapeI64x2,
			v:     i64x2(100, i64ToU64(-123450)),
			exp:   i64x2(i64ToU64(-100), 123450),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, -123, float32(math.Inf(1)), 0.1),
			exp:   f32x4(-51234.12341, 123, float32(math.Inf(-1)), -0.1),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, 0, float32(math.Inf(1)), 0.1),
			exp:   f32x4(-51234.12341, float32(math.Copysign(0, -1)), float32(math.Inf(-1)), -0.1),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(1.123, math.Inf(-1)),
			exp:   f64x2(-1.123, math.Inf(1)),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(0, math.Inf(-1)),
			exp:   f64x2(math.Copysign(0, -1), math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Neg(&wazeroir.OperationV128Neg{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Abs(t *testing.T) {
	tests := []struct {
		name   string
		shape  wazeroir.Shape
		v, exp [16]byte
	}{
		{
			name:  "i8x16",
			shape: wazeroir.ShapeI8x16,
			v:     [16]byte{1: 123, 5: i8ToU8(-1), 15: i8ToU8(-125)},
			exp:   [16]byte{1: 123, 5: 1, 15: 125},
		},
		{
			name:  "i16x8",
			shape: wazeroir.ShapeI16x8,
			v:     i16x8(0, 0, i16ToU16(-123), 0, 1, 25, 8, i16ToU16(-1)),
			exp:   i16x8(0, 0, 123, 0, 1, 25, 8, 1),
		},
		{
			name:  "i32x4",
			shape: wazeroir.ShapeI32x4,
			v:     i32x4(i32ToU32(-1230), 5, i32ToU32(-416), 0),
			exp:   i32x4(1230, 5, 416, 0),
		},
		{
			name:  "i64x2",
			shape: wazeroir.ShapeI64x2,
			v:     i64x2(i64ToU64(-100), i64ToU64(-123450)),
			exp:   i64x2(100, 123450),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, -123, float32(math.Inf(1)), 0.1),
			exp:   f32x4(51234.12341, 123, float32(math.Inf(1)), 0.1),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, 0, float32(math.Inf(1)), -0.1),
			exp:   f32x4(51234.12341, 0, float32(math.Inf(1)), 0.1),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(-1.123, math.Inf(-1)),
			exp:   f64x2(1.123, math.Inf(1)),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(0, math.Inf(-1)),
			exp:   f64x2(0, math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Abs(&wazeroir.OperationV128Abs{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Div(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		x1, x2, exp [16]byte
	}{
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(1.0, 123, float32(math.Inf(1)), float32(math.Inf(-1))),
			x2:    f32x4(123.12, 123, math.MaxFloat32, -123),
			exp:   f32x4(float32(1.0)/float32(123.12), 1, float32(math.Inf(1)), float32(math.Inf(1))),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(1.123, math.Inf(1)),
			x2:    f64x2(1.123, math.MinInt64),
			exp:   f64x2(1.0, math.Inf(-1)),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(0, math.Inf(1)),
			x2:    f64x2(1.123, math.MaxInt64),
			exp:   f64x2(0, math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Div(&wazeroir.OperationV128Div{Shape: tc.shape})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Min(t *testing.T) {

	tests := []struct {
		name        string
		shape       wazeroir.Shape
		signed      bool
		x1, x2, exp [16]byte
	}{
		{
			name:   "i8x16s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			x1:     [16]byte{0: 123, 5: i8ToU8(-1), 15: 2},
			x2:     [16]byte{0: 1, 5: 0, 15: i8ToU8(-1)},
			exp:    [16]byte{0: 1, 5: i8ToU8(-1), 15: i8ToU8(-1)},
		},
		{
			name:   "i8x16u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			x1:     [16]byte{0: 123, 5: i8ToU8(-1), 15: 2},
			x2:     [16]byte{0: 1, 5: 0, 15: i8ToU8(-1)},
			exp:    [16]byte{0: 1, 5: 0, 15: 2},
		},
		{
			name:   "i16x8s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, i16ToU16(-123), 1),
			x2:     i16x8(0, 123, i16ToU16(-123), 3, 1, 4, 5, 1),
			exp:    i16x8(0, 0, i16ToU16(-123), 1, 1, 4, i16ToU16(-123), 1),
		},
		{
			name:   "i16x8u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, i16ToU16(-123), 1),
			x2:     i16x8(0, 123, i16ToU16(-123), 3, 1, 4, 5, 1),
			exp:    i16x8(0, 0, 123, 1, 1, 4, 5, 1),
		},
		{
			name:   "i32x4s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(i32ToU32(-123), 0, 1, i32ToU32(math.MinInt32)),
			x2:     i32x4(123, 5, 1, 0),
			exp:    i32x4(i32ToU32(-123), 0, 1, i32ToU32(math.MinInt32)),
		},
		{
			name:   "i32x4u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(i32ToU32(-123), 0, 1, i32ToU32(math.MinInt32)),
			x2:     i32x4(123, 5, 1, 0),
			exp:    i32x4(123, 0, 1, 0),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.NaN()), -123.12, 2.3, float32(math.Inf(1))),
			x2:    f32x4(5.5, 123.12, 5.0, float32(math.Inf(-1))),
			exp:   f32x4(float32(math.NaN()), -123.12, 2.3, float32(math.Inf(-1))),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(5.5, 123.12, -5.0, float32(math.Inf(-1))),
			x2:    f32x4(-123.12, float32(math.NaN()), 2.3, float32(math.Inf(-1))),
			exp:   f32x4(-123.12, float32(math.NaN()), -5.0, float32(math.Inf(-1))),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.MinInt64, 0),
			x2:    f64x2(math.MaxInt64, -12.3),
			exp:   f64x2(math.MinInt64, -12.3),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.MaxInt64, -12.3),
			x2:    f64x2(math.MinInt64, 0),
			exp:   f64x2(math.MinInt64, -12.3),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(math.Inf(1), math.Inf(-1)),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.Inf(1), math.Inf(-1)),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Min(&wazeroir.OperationV128Min{Shape: tc.shape, Signed: tc.signed})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			switch tc.shape {
			case wazeroir.ShapeF64x2:
				for _, vs := range [][2]float64{
					{math.Float64frombits(lo), math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[:8]))},
					{math.Float64frombits(hi), math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[8:]))},
				} {
					actual, exp := vs[0], vs[1]
					if math.IsNaN(exp) {
						require.True(t, math.IsNaN(actual))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			case wazeroir.ShapeF32x4:
				for _, vs := range [][2]float32{
					{math.Float32frombits(uint32(lo)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[:4]))},
					{math.Float32frombits(uint32(lo >> 32)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[4:8]))},
					{math.Float32frombits(uint32(hi)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[8:12]))},
					{math.Float32frombits(uint32(hi >> 32)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[12:]))},
				} {
					actual, exp := vs[0], vs[1]
					if math.IsNaN(float64(exp)) {
						require.True(t, math.IsNaN(float64(actual)))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			default:
				var actual [16]byte
				binary.LittleEndian.PutUint64(actual[:8], lo)
				binary.LittleEndian.PutUint64(actual[8:], hi)
				require.Equal(t, tc.exp, actual)
			}
		})
	}
}

func TestCompiler_compileV128Max(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		signed      bool
		x1, x2, exp [16]byte
	}{
		{
			name:   "i8x16s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			x1:     [16]byte{0: 123, 5: i8ToU8(-1), 15: 2},
			x2:     [16]byte{0: 1, 5: 0, 15: i8ToU8(-1)},
			exp:    [16]byte{0: 123, 5: 0, 15: 2},
		},
		{
			name:   "i8x16u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			x1:     [16]byte{0: 123, 5: i8ToU8(-1), 15: 2},
			x2:     [16]byte{0: 1, 5: 0, 15: i8ToU8(-1)},
			exp:    [16]byte{0: 123, 5: i8ToU8(-1), 15: i8ToU8(-1)},
		},
		{
			name:   "i16x8s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, i16ToU16(-123), 1),
			x2:     i16x8(0, 123, i16ToU16(-123), 3, 1, 4, 5, 1),
			exp:    i16x8(1123, 123, 123, 3, 1, 6, 5, 1),
		},
		{
			name:   "i16x8u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, i16ToU16(-123), 1),
			x2:     i16x8(0, 123, i16ToU16(-123), 3, 1, 4, 5, 1),
			exp:    i16x8(1123, 123, i16ToU16(-123), 3, 1, 6, i16ToU16(-123), 1),
		},
		{
			name:   "i32x4s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(i32ToU32(-123), 0, 1, i32ToU32(math.MinInt32)),
			x2:     i32x4(123, 5, 1, 0),
			exp:    i32x4(123, 5, 1, 0),
		},
		{
			name:   "i32x4u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(i32ToU32(-123), 0, 1, i32ToU32(math.MinInt32)),
			x2:     i32x4(123, 5, 1, 0),
			exp:    i32x4(i32ToU32(-123), 5, 1, i32ToU32(math.MinInt32)),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.NaN()), -123.12, 2.3, float32(math.Inf(1))),
			x2:    f32x4(5.5, 123.12, 5.0, float32(math.Inf(-1))),
			exp:   f32x4(float32(math.NaN()), 123.12, 5.0, float32(math.Inf(1))),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(5.5, 123.12, -5.0, float32(math.Inf(-1))),
			x2:    f32x4(-123.12, float32(math.NaN()), 2.3, float32(math.Inf(-1))),
			exp:   f32x4(5.5, float32(math.NaN()), 2.3, float32(math.Inf(-1))),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.MinInt64, 0),
			x2:    f64x2(math.MaxInt64, -12.3),
			exp:   f64x2(math.MaxInt64, 0),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.MaxInt64, -12.3),
			x2:    f64x2(math.MinInt64, 0),
			exp:   f64x2(math.MaxInt64, 0),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.NaN(), -12.3),
			x2:    f64x2(math.MinInt64, math.NaN()),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.MinInt64, math.NaN()),
			x2:    f64x2(math.NaN(), -12.3),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(math.Inf(1), math.Inf(-1)),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			x1:    f64x2(math.Inf(1), math.Inf(-1)),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Max(&wazeroir.OperationV128Max{Shape: tc.shape, Signed: tc.signed})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			switch tc.shape {
			case wazeroir.ShapeF64x2:
				for _, vs := range [][2]float64{
					{math.Float64frombits(lo), math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[:8]))},
					{math.Float64frombits(hi), math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[8:]))},
				} {
					actual, exp := vs[0], vs[1]
					if math.IsNaN(exp) {
						require.True(t, math.IsNaN(actual))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			case wazeroir.ShapeF32x4:
				for _, vs := range [][2]float32{
					{math.Float32frombits(uint32(lo)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[:4]))},
					{math.Float32frombits(uint32(lo >> 32)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[4:8]))},
					{math.Float32frombits(uint32(hi)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[8:12]))},
					{math.Float32frombits(uint32(hi >> 32)), math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[12:]))},
				} {
					actual, exp := vs[0], vs[1]
					if math.IsNaN(float64(exp)) {
						require.True(t, math.IsNaN(float64(actual)))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			default:
				var actual [16]byte
				binary.LittleEndian.PutUint64(actual[:8], lo)
				binary.LittleEndian.PutUint64(actual[8:], hi)
				require.Equal(t, tc.exp, actual)
			}
		})
	}
}

func TestCompiler_compileV128AddSat(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		signed      bool
		x1, x2, exp [16]byte
	}{
		{
			name:   "i8x16s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			x1: [16]byte{
				0:  i8ToU8(math.MaxInt8),
				5:  i8ToU8(-1),
				15: i8ToU8(math.MinInt8),
			},
			x2: [16]byte{
				0:  1,
				5:  0,
				15: i8ToU8(-1),
			},
			exp: [16]byte{
				0:  i8ToU8(math.MaxInt8),
				5:  i8ToU8(-1),
				15: i8ToU8(math.MinInt8),
			},
		},
		{
			name:   "i8x16u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			x1: [16]byte{
				0:  i8ToU8(math.MaxInt8),
				5:  0,
				15: math.MaxUint8,
			},
			x2: [16]byte{
				0:  1,
				5:  i8ToU8(-1),
				15: 1,
			},
			exp: [16]byte{
				0:  i8ToU8(math.MaxInt8) + 1,
				5:  i8ToU8(-1),
				15: math.MaxUint8,
			},
		},
		{
			name:   "i16x8s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(i16ToU16(math.MinInt16), 0, 123, 1, 1, 6, i16ToU16(-123), i16ToU16(math.MaxInt16)),
			x2:     i16x8(i16ToU16(-1), 123, i16ToU16(-123), 3, 1, 4, 5, 1),
			exp:    i16x8(i16ToU16(math.MinInt16), 123, 0, 4, 2, 10, i16ToU16(-118), i16ToU16(math.MaxInt16)),
		},
		{
			name:   "i16x8u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, i16ToU16(-123), math.MaxUint16),
			x2:     i16x8(0, 123, math.MaxUint16, 3, 1, 4, 0, 1),
			exp:    i16x8(1123, 123, math.MaxUint16, 4, 2, 10, i16ToU16(-123), math.MaxUint16),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128AddSat(&wazeroir.OperationV128AddSat{Shape: tc.shape, Signed: tc.signed})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128SubSat(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		signed      bool
		x1, x2, exp [16]byte
	}{
		{
			name:   "i8x16s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			x1: [16]byte{
				0:  i8ToU8(math.MinInt8),
				5:  i8ToU8(-1),
				15: i8ToU8(math.MaxInt8),
			},
			x2: [16]byte{
				0:  1,
				5:  0,
				15: i8ToU8(-1),
			},
			exp: [16]byte{
				0:  i8ToU8(math.MinInt8),
				5:  i8ToU8(-1),
				15: i8ToU8(math.MaxInt8),
			},
		},
		{
			name:   "i8x16u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			x1: [16]byte{
				0:  i8ToU8(math.MinInt8),
				5:  i8ToU8(-1),
				15: 0,
			},
			x2: [16]byte{
				0:  1,
				5:  0,
				15: 1,
			},
			exp: [16]byte{
				0:  i8ToU8(math.MinInt8) - 1,
				5:  i8ToU8(-1),
				15: 0,
			},
		},
		{
			name:   "i16x8s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(i16ToU16(math.MinInt16), 0, 123, 1, 1, 6, i16ToU16(-123), i16ToU16(math.MaxInt16)),
			x2:     i16x8(1, 123, i16ToU16(-123), 3, 1, 4, 5, i16ToU16(-123)),
			exp:    i16x8(i16ToU16(math.MinInt16), i16ToU16(-123), 246, i16ToU16(-2), 0, 2, i16ToU16(-128), i16ToU16(math.MaxInt16)),
		},
		{
			name:   "i16x8u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(1123, 0, 123, 1, 1, 6, 200, math.MaxUint16),
			x2:     i16x8(0, 123, math.MaxUint16, 3, 1, 4, i16ToU16(-1), 12),
			exp:    i16x8(1123, 0, 0, 0, 0, 2, 0, math.MaxUint16-12),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128SubSat(&wazeroir.OperationV128SubSat{Shape: tc.shape, Signed: tc.signed})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Popcnt(t *testing.T) {
	tests := []struct {
		name   string
		v, exp [16]byte
	}{
		{
			name: "ones",
			v: [16]byte{
				1, 1 << 1, 1 << 2, 1 << 3, 1 << 4, 1 << 5, 1 << 6, 1 << 7,
				0, 1 << 2, 0, 1 << 4, 0, 1 << 6, 0, 0,
			},
			exp: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				0, 1, 0, 1, 0, 1, 0, 0,
			},
		},
		{
			name: "mix",
			v: [16]byte{
				0b1, 0b11, 0b111, 0b1111, 0b11111, 0b111111, 0b1111111, 0b11111111,
				0b10000001, 0b10000010, 0b10000100, 0b10001000, 0b10010000, 0b10100000, 0b11000000, 0,
			},
			exp: [16]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				2, 2, 2, 2, 2, 2, 2, 0,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Popcnt(&wazeroir.OperationV128Popcnt{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Round(t *testing.T) {
	tests := []struct {
		name  string
		shape wazeroir.Shape
		kind  wazeroir.OperationKind
		v     [16]byte
	}{
		{
			name:  "f32 ceil",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Ceil,
			v:     f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 ceil",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Ceil,
			v:     f32x4(math.Pi, -1231231.123, float32(math.NaN()), float32(math.Inf(-1))),
		},
		{
			name:  "f64 ceil",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Ceil,
			v:     f64x2(1.231, -123.12313),
		},
		{
			name:  "f64 ceil",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Ceil,
			v:     f64x2(math.Inf(1), math.NaN()),
		},
		{
			name:  "f64 ceil",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Ceil,
			v:     f64x2(math.Inf(-1), math.Pi),
		},
		{
			name:  "f32 floor",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Floor,
			v:     f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 floor",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Floor,
			v:     f32x4(math.Pi, -1231231.123, float32(math.NaN()), float32(math.Inf(-1))),
		},
		{
			name:  "f64 floor",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Floor,
			v:     f64x2(1.231, -123.12313),
		},
		{
			name:  "f64 floor",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Floor,
			v:     f64x2(math.Inf(1), math.NaN()),
		},
		{
			name:  "f64 floor",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Floor,
			v:     f64x2(math.Inf(-1), math.Pi),
		},
		{
			name:  "f32 trunc",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Trunc,
			v:     f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 trunc",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Trunc,
			v:     f32x4(math.Pi, -1231231.123, float32(math.NaN()), float32(math.Inf(-1))),
		},
		{
			name:  "f64 trunc",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Trunc,
			v:     f64x2(1.231, -123.12313),
		},
		{
			name:  "f64 trunc",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Trunc,
			v:     f64x2(math.Inf(1), math.NaN()),
		},
		{
			name:  "f64 trunc",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Trunc,
			v:     f64x2(math.Inf(-1), math.Pi),
		},
		{
			name:  "f32 nearest",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Nearest,
			v:     f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 nearest",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Nearest,
			v:     f32x4(math.Pi, -1231231.123, float32(math.NaN()), float32(math.Inf(-1))),
		},
		{
			name:  "f64 nearest",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Nearest,
			v:     f64x2(1.231, -123.12313),
		},
		{
			name:  "f64 nearest",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Nearest,
			v:     f64x2(math.Inf(1), math.NaN()),
		},
		{
			name:  "f64 nearest",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Nearest,
			v:     f64x2(math.Inf(-1), math.Pi),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			is32bit := tc.shape == wazeroir.ShapeF32x4
			switch tc.kind {
			case wazeroir.OperationKindV128Ceil:
				err = compiler.compileV128Ceil(&wazeroir.OperationV128Ceil{Shape: tc.shape})
			case wazeroir.OperationKindV128Floor:
				err = compiler.compileV128Floor(&wazeroir.OperationV128Floor{Shape: tc.shape})
			case wazeroir.OperationKindV128Trunc:
				err = compiler.compileV128Trunc(&wazeroir.OperationV128Trunc{Shape: tc.shape})
			case wazeroir.OperationKindV128Nearest:
				err = compiler.compileV128Nearest(&wazeroir.OperationV128Nearest{Shape: tc.shape})
			}
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()

			if is32bit {
				actualFs := [4]float32{
					math.Float32frombits(uint32(lo)),
					math.Float32frombits(uint32(lo >> 32)),
					math.Float32frombits(uint32(hi)),
					math.Float32frombits(uint32(hi >> 32))}
				f1Original, f2Original, f3Original, f4Original :=
					math.Float32frombits(binary.LittleEndian.Uint32(tc.v[:4])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.v[4:8])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.v[8:12])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.v[12:]))

				var expFs [4]float32
				switch tc.kind {
				case wazeroir.OperationKindV128Ceil:
					expFs[0] = float32(math.Ceil(float64(f1Original)))
					expFs[1] = float32(math.Ceil(float64(f2Original)))
					expFs[2] = float32(math.Ceil(float64(f3Original)))
					expFs[3] = float32(math.Ceil(float64(f4Original)))
				case wazeroir.OperationKindV128Floor:
					expFs[0] = float32(math.Floor(float64(f1Original)))
					expFs[1] = float32(math.Floor(float64(f2Original)))
					expFs[2] = float32(math.Floor(float64(f3Original)))
					expFs[3] = float32(math.Floor(float64(f4Original)))
				case wazeroir.OperationKindV128Trunc:
					expFs[0] = float32(math.Trunc(float64(f1Original)))
					expFs[1] = float32(math.Trunc(float64(f2Original)))
					expFs[2] = float32(math.Trunc(float64(f3Original)))
					expFs[3] = float32(math.Trunc(float64(f4Original)))
				case wazeroir.OperationKindV128Nearest:
					expFs[0] = moremath.WasmCompatNearestF32(f1Original)
					expFs[1] = moremath.WasmCompatNearestF32(f2Original)
					expFs[2] = moremath.WasmCompatNearestF32(f3Original)
					expFs[3] = moremath.WasmCompatNearestF32(f4Original)
				}

				for i := range expFs {
					exp, actual := expFs[i], actualFs[i]
					if math.IsNaN(float64(exp)) {
						require.True(t, math.IsNaN(float64(actual)))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			} else {
				actualFs := [2]float64{math.Float64frombits(lo), math.Float64frombits(hi)}
				f1Original, f2Original :=
					math.Float64frombits(binary.LittleEndian.Uint64(tc.v[:8])), math.Float64frombits(binary.LittleEndian.Uint64(tc.v[8:]))

				var expFs [2]float64
				switch tc.kind {
				case wazeroir.OperationKindV128Ceil:
					expFs[0] = math.Ceil(f1Original)
					expFs[1] = math.Ceil(f2Original)
				case wazeroir.OperationKindV128Floor:
					expFs[0] = math.Floor(f1Original)
					expFs[1] = math.Floor(f2Original)
				case wazeroir.OperationKindV128Trunc:
					expFs[0] = math.Trunc(f1Original)
					expFs[1] = math.Trunc(f2Original)
				case wazeroir.OperationKindV128Nearest:
					expFs[0] = moremath.WasmCompatNearestF64(f1Original)
					expFs[1] = moremath.WasmCompatNearestF64(f2Original)
				}

				for i := range expFs {
					exp, actual := expFs[i], actualFs[i]
					if math.IsNaN(exp) {
						require.True(t, math.IsNaN(actual))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			}
		})
	}
}

func TestCompiler_compileV128_Pmax_Pmin(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		kind        wazeroir.OperationKind
		x1, x2, exp [16]byte
	}{
		{
			name:  "f32 pmin",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f32x4(float32(math.Inf(1)), -1.5, 1123.5, float32(math.Inf(1))),
			x2:    f32x4(1.4, float32(math.Inf(-1)), -1231.5, float32(math.Inf(1))),
			exp:   f32x4(1.4, float32(math.Inf(-1)), -1231.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 pmin",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f32 pmin",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 pmin",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
		},
		{
			name:  "f32 pmin",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f64 pmin",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f64x2(math.Inf(1), -123123.1231),
			x2:    f64x2(-123123.1, math.Inf(-1)),
			exp:   f64x2(-123123.1, math.Inf(-1)),
		},
		{
			name:  "f64 pmin",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(-123123.1, 1.0),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64 pmin",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f64x2(-123123.1, 1.0),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(-123123.1, 1.0),
		},
		{
			name:  "f64 pmin",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(math.Inf(1), math.Inf(-1)),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64 pmin",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmin,
			x1:    f64x2(math.Inf(1), math.Inf(-1)),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(math.Inf(1), math.Inf(-1)),
		},
		{
			name:  "f32 pmax",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f32x4(float32(math.Inf(1)), -1.5, 1123.5, float32(math.Inf(1))),
			x2:    f32x4(1.4, float32(math.Inf(-1)), -1231.5, float32(math.Inf(1))),
			exp:   f32x4(float32(math.Inf(1)), -1.5, 1123.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 pmax",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f32 pmax",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(1.4, -1.5, 1.5, float32(math.Inf(1))),
		},
		{
			name:  "f32 pmax",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			x2:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			exp:   f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
		},
		{
			name:  "f32 pmax",
			shape: wazeroir.ShapeF32x4,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
			x2:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(1))),
			exp:   f32x4(float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())),
		},
		{
			name:  "f64 pmax",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f64x2(math.Inf(1), -123123.1231),
			x2:    f64x2(-123123.1, math.Inf(-1)),
			exp:   f64x2(math.Inf(1), -123123.1231),
		},
		{
			name:  "f64 pmax",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(-123123.1, 1.0),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64 pmax",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f64x2(-123123.1, 1.0),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(-123123.1, 1.0),
		},
		{
			name:  "f64 pmax",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f64x2(math.NaN(), math.NaN()),
			x2:    f64x2(math.Inf(1), math.Inf(-1)),
			exp:   f64x2(math.NaN(), math.NaN()),
		},
		{
			name:  "f64 pmax",
			shape: wazeroir.ShapeF64x2,
			kind:  wazeroir.OperationKindV128Pmax,
			x1:    f64x2(math.Inf(1), math.Inf(-1)),
			x2:    f64x2(math.NaN(), math.NaN()),
			exp:   f64x2(math.Inf(1), math.Inf(-1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			is32bit := tc.shape == wazeroir.ShapeF32x4
			switch tc.kind {
			case wazeroir.OperationKindV128Pmin:
				err = compiler.compileV128Pmin(&wazeroir.OperationV128Pmin{Shape: tc.shape})
			case wazeroir.OperationKindV128Pmax:
				err = compiler.compileV128Pmax(&wazeroir.OperationV128Pmax{Shape: tc.shape})
			}
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()

			if is32bit {
				actualFs := [4]float32{
					math.Float32frombits(uint32(lo)),
					math.Float32frombits(uint32(lo >> 32)),
					math.Float32frombits(uint32(hi)),
					math.Float32frombits(uint32(hi >> 32))}
				expFs := [4]float32{
					math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[:4])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[4:8])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[8:12])),
					math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[12:])),
				}
				for i := range expFs {
					exp, actual := expFs[i], actualFs[i]
					if math.IsNaN(float64(exp)) {
						require.True(t, math.IsNaN(float64(actual)))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			} else {
				actualFs := [2]float64{
					math.Float64frombits(lo), math.Float64frombits(hi),
				}
				expFs := [2]float64{
					math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[:8])),
					math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[8:])),
				}
				for i := range expFs {
					exp, actual := expFs[i], actualFs[i]
					if math.IsNaN(exp) {
						require.True(t, math.IsNaN(actual))
					} else {
						require.Equal(t, exp, actual)
					}
				}
			}
		})
	}
}

func TestCompiler_compileV128ExtMul(t *testing.T) {
	tests := []struct {
		name           string
		shape          wazeroir.Shape
		signed, useLow bool
		x1, x2, exp    [16]byte
	}{
		{
			name:   "i8x16s low",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			x1:     [16]byte{}, x2: [16]byte{},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16s low",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			x1: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(128, 128, 128, 128, 128, 128, 128, 128),
		},
		{
			name:   "i8x16s low",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			x1: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16s low",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			x1: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(16384, 16384, 16384, 16384, 16384, 16384, 16384, 16384),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			x1:     [16]byte{}, x2: [16]byte{},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			x2: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(128, 128, 128, 128, 128, 128, 128, 128),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			x2: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			x2: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(16384, 16384, 16384, 16384, 16384, 16384, 16384, 16384),
		},
		{
			name:   "i8x16u low",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			x1:     [16]byte{}, x2: [16]byte{},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16u low",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			x1: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0,
			},
			exp: i16x8(32640, 32640, 32640, 32640, 32640, 32640, 32640, 32640),
		},
		{
			name:   "i8x16u low",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			x1: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				255, 255, 255, 255, 255, 255, 255, 255,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(i16ToU16(-511), i16ToU16(-511), i16ToU16(-511), i16ToU16(-511),
				i16ToU16(-511), i16ToU16(-511), i16ToU16(-511), i16ToU16(-511)),
		},
		{
			name:   "i8x16u low",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			x1: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			x2: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(16384, 16384, 16384, 16384, 16384, 16384, 16384, 16384),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			x1:     [16]byte{}, x2: [16]byte{},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			x2: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(32640, 32640, 32640, 32640, 32640, 32640, 32640, 32640),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			x2: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				255, 255, 255, 255, 255, 255, 255, 255,
			},
			exp: i16x8(i16ToU16(-511), i16ToU16(-511), i16ToU16(-511), i16ToU16(-511),
				i16ToU16(-511), i16ToU16(-511), i16ToU16(-511), i16ToU16(-511)),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			x1: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			x2: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(16384, 16384, 16384, 16384, 16384, 16384, 16384, 16384),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			x1: i16x8(
				16383, 16383, 16383, 16383,
				0, 0, 1, 0,
			),
			x2: i16x8(
				16384, 16384, 16384, 16384,
				0, 0, 1, 0,
			),
			exp: i32x4(268419072, 268419072, 268419072, 268419072),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			x1: i16x8(
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
				0, 0, 1, 0,
			),
			x2: i16x8(
				i16ToU16(-32767), 0, i16ToU16(-32767), 0,
				0, 0, 1, 0,
			),
			exp: i32x4(1073709056, 0, 1073709056, 0),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			x1: i16x8(
				65535, 65535, 65535, 65535,
				0, 0, 1, 0,
			),
			x2: i16x8(
				65535, 0, 65535, 0,
				0, 0, 1, 0,
			),
			exp: i32x4(1, 0, 1, 0),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				16383, 16383, 16383, 16383,
			),
			x2: i16x8(
				0, 0, 1, 0,
				16384, 16384, 16384, 16384,
			),
			exp: i32x4(268419072, 268419072, 268419072, 268419072),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
			),
			x2: i16x8(
				0, 0, 1, 0,
				i16ToU16(-32767), 0, i16ToU16(-32767), 0,
			),
			exp: i32x4(1073709056, 0, 1073709056, 0),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				65535, 65535, 65535, 65535,
			),
			x2: i16x8(
				0, 0, 1, 0,

				65535, 0, 65535, 0,
			),
			exp: i32x4(1, 0, 1, 0),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			x1: i16x8(
				16383, 16383, 16383, 16383,
				0, 0, 1, 0,
			),
			x2: i16x8(
				16384, 16384, 16384, 16384,
				0, 0, 1, 0,
			),
			exp: i32x4(268419072, 268419072, 268419072, 268419072),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			x1: i16x8(
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
				0, 0, 1, 0,
			),
			x2: i16x8(
				i16ToU16(-32767), 0, i16ToU16(-32767), 0,
				0, 0, 1, 0,
			),
			exp: i32x4(1073774592, 0, 1073774592, 0),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			x1: i16x8(
				65535, 65535, 65535, 65535,
				0, 0, 1, 0,
			),
			x2: i16x8(
				65535, 0, 65535, 0,
				0, 0, 1, 0,
			),
			exp: i32x4(i32ToU32(-131071), 0, i32ToU32(-131071), 0),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				16383, 16383, 16383, 16383,
			),
			x2: i16x8(
				0, 0, 1, 0,
				16384, 16384, 16384, 16384,
			),
			exp: i32x4(268419072, 268419072, 268419072, 268419072),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
			),
			x2: i16x8(
				0, 0, 1, 0,
				i16ToU16(-32767), 0, i16ToU16(-32767), 0,
			),
			exp: i32x4(1073774592, 0, 1073774592, 0),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			x1: i16x8(
				0, 0, 1, 0,
				65535, 65535, 65535, 65535,
			),
			x2: i16x8(
				0, 0, 1, 0,
				65535, 0, 65535, 0,
			),
			exp: i32x4(i32ToU32(-131071), 0, i32ToU32(-131071), 0),
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			x1: i32x4(
				1, i32ToU32(-1),
				0, 0,
			),
			x2: i32x4(
				i32ToU32(-1), 1,
				0, 0,
			),
			exp: i64x2(i64ToU64(-1), i64ToU64(-1)),
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			x1: i32x4(
				1073741824, 4294967295,
				0, 0,
			),
			x2: i32x4(
				1073741824, 4294967295,
				0, 0,
			),
			exp: i64x2(1152921504606846976, 1),
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			x1: i32x4(
				0, 0,
				1, i32ToU32(-1),
			),
			x2: i32x4(
				0, 0,
				i32ToU32(-1), 1,
			),
			exp: i64x2(i64ToU64(-1), i64ToU64(-1)),
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			x1: i32x4(
				0, 0,
				1073741824, 4294967295,
			),
			x2: i32x4(
				0, 0,
				1073741824, 4294967295,
			),
			exp: i64x2(1152921504606846976, 1),
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			x1: i32x4(
				1, i32ToU32(-1),
				0, 0,
			),
			x2: i32x4(
				i32ToU32(-1), 1,
				0, 0,
			),
			exp: i64x2(4294967295, 4294967295),
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			x1: i32x4(
				1073741824, 4294967295,
				0, 0,
			),
			x2: i32x4(
				1073741824, 4294967295,
				0, 0,
			),
			exp: i64x2(1152921504606846976, i64ToU64(-8589934591)),
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			x1:     [16]byte{},
			x2:     [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			x1: i32x4(
				0, 0,
				1, i32ToU32(-1),
			),
			x2: i32x4(
				0, 0,
				i32ToU32(-1), 1,
			),
			exp: i64x2(4294967295, 4294967295),
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			x1: i32x4(
				0, 0,
				1073741824, 4294967295,
			),
			x2: i32x4(
				0, 0,
				1073741824, 4294967295,
			),
			exp: i64x2(1152921504606846976, i64ToU64(-8589934591)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128ExtMul(&wazeroir.OperationV128ExtMul{
				OriginShape: tc.shape, Signed: tc.signed, UseLow: tc.useLow,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Extend(t *testing.T) {
	tests := []struct {
		name           string
		shape          wazeroir.Shape
		signed, useLow bool
		v, exp         [16]byte
	}{
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1)),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16s hi",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: false,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16s lo",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16s lo",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1)),
		},
		{
			name:   "i8x16s lo",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			v: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16s lo",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			useLow: true,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		// unsigned
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(255, 255, 255, 255, 255, 255, 255, 255),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16u hi",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: false,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i8x16u lo",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16u lo",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(255, 255, 255, 255, 255, 255, 255, 255),
		},
		{
			name:   "i8x16u lo",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			v: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			exp: i16x8(1, 1, 1, 1, 1, 1, 1, 1),
		},
		{
			name:   "i8x16u lo",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			useLow: true,
			v: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			v:      i16x8(1, 1, 1, 1, 0, 0, 0, 0),
			exp:    i32x4(0, 0, 0, 0),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			v:      i16x8(0, 0, 0, 0, i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1)),
			exp:    i32x4(i32ToU32(-1), i32ToU32(-1), i32ToU32(-1), i32ToU32(-1)),
		},
		{
			name:   "i16x8s hi",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: false,
			v:      i16x8(0, 0, 0, 0, 123, 0, 123, 0),
			exp:    i32x4(123, 0, 123, 0),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			v:      i16x8(0, 0, 0, 0, 1, 1, 1, 1),
			exp:    i32x4(0, 0, 0, 0),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			v:      i16x8(i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), 0, 0, 0, 0),
			exp:    i32x4(i32ToU32(-1), i32ToU32(-1), i32ToU32(-1), i32ToU32(-1)),
		},
		{
			name:   "i16x8s lo",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			useLow: true,
			v:      i16x8(123, 0, 123, 0, 0, 0, 0, 0),
			exp:    i32x4(123, 0, 123, 0),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			v:      i16x8(1, 1, 1, 1, 0, 0, 0, 0),
			exp:    i32x4(0, 0, 0, 0),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			v:      i16x8(0, 0, 0, 0, i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1)),
			exp:    i32x4(65535, 65535, 65535, 65535),
		},
		{
			name:   "i16x8u hi",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: false,
			v:      i16x8(0, 0, 0, 0, 123, 0, 123, 0),
			exp:    i32x4(123, 0, 123, 0),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			v:      i16x8(0, 0, 0, 0, 1, 1, 1, 1),
			exp:    i32x4(0, 0, 0, 0),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			v:      i16x8(i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), 0, 0, 0, 0),
			exp:    i32x4(65535, 65535, 65535, 65535),
		},
		{
			name:   "i16x8u lo",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			useLow: true,
			v:      i16x8(123, 0, 123, 0, 0, 0, 0, 0),
			exp:    i32x4(123, 0, 123, 0),
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			v:      i32x4(0, 0, 1, i32ToU32(-1)),
			exp:    i64x2(1, i64ToU64(-1)),
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			v:      i32x4(1, i32ToU32(-1), 0, 0),
			exp:    i64x2(0, 0),
		},
		{
			name:   "i32x4s hi",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: false,
			v:      i32x4(1, i32ToU32(-1), 123, 123),
			exp:    i64x2(123, 123),
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			v:      i32x4(1, i32ToU32(-1), 0, 0),
			exp:    i64x2(1, i64ToU64(-1)),
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			v:      i32x4(0, 0, 1, i32ToU32(-1)),
			exp:    i64x2(0, 0),
		},
		{
			name:   "i32x4s lo",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			useLow: true,
			v:      i32x4(123, 123, 1, i32ToU32(-1)),
			exp:    i64x2(123, 123),
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			v:      i32x4(0, 0, 1, i32ToU32(-1)),
			exp:    i64x2(1, 4294967295),
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			v:      i32x4(1, i32ToU32(-1), 0, 0),
			exp:    i64x2(0, 0),
		},
		{
			name:   "i32x4u hi",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: false,
			v:      i32x4(1, i32ToU32(-1), 123, 123),
			exp:    i64x2(123, 123),
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			v:      i32x4(1, i32ToU32(-1), 0, 0),
			exp:    i64x2(1, 4294967295),
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			v:      i32x4(0, 0, 1, i32ToU32(-1)),
			exp:    i64x2(0, 0),
		},
		{
			name:   "i32x4u lo",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			useLow: true,
			v:      i32x4(123, 123, 1, i32ToU32(-1)),
			exp:    i64x2(123, 123),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Extend(&wazeroir.OperationV128Extend{
				OriginShape: tc.shape, Signed: tc.signed, UseLow: tc.useLow,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Q15mulrSatS(t *testing.T) {

	tests := []struct {
		name        string
		x1, x2, exp [16]byte
	}{
		{
			name: "1",
			x1:   i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:   i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp:  i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name: "2",
			x1:   i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:   i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp:  i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name: "3",
			x1:   i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			x2:   i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp:  i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name: "4",
			x1:   i16x8(65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535),
			x2:   i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp:  i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name: "5",
			x1:   i16x8(32767, 32767, 32767, 32767, 32767, 32767, 32767, 32767),
			x2:   i16x8(32767, 32767, 32767, 32767, 32767, 32767, 32767, 32767),
			exp:  i16x8(32766, 32766, 32766, 32766, 32766, 32766, 32766, 32766),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Q15mulrSatS(&wazeroir.OperationV128Q15mulrSatS{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileFloatPromote(t *testing.T) {
	tests := []struct {
		name   string
		v, exp [16]byte
	}{
		{
			name: "1",
			v:    f32x4(float32(0x1.8f867ep+125), float32(0x1.8f867ep+125), float32(0x1.8f867ep+125), float32(0x1.8f867ep+125)),
			exp:  f64x2(6.6382536710104395e+37, 6.6382536710104395e+37),
		},
		{
			name: "2",
			v:    f32x4(float32(0x1.8f867ep+125), float32(0x1.8f867ep+125), 0, 0),
			exp:  f64x2(6.6382536710104395e+37, 6.6382536710104395e+37),
		},
		{
			name: "3",
			v:    f32x4(0, 0, float32(0x1.8f867ep+125), float32(0x1.8f867ep+125)),
			exp:  f64x2(0, 0),
		},
		{
			name: "4",
			v:    f32x4(float32(math.NaN()), float32(math.NaN()), float32(0x1.8f867ep+125), float32(0x1.8f867ep+125)),
			exp:  f64x2(math.NaN(), math.NaN()),
		},
		{
			name: "5",
			v:    f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), float32(0x1.8f867ep+125), float32(0x1.8f867ep+125)),
			exp:  f64x2(math.Inf(1), math.Inf(-1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128FloatPromote(&wazeroir.OperationV128FloatPromote{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			actualFs := [2]float64{
				math.Float64frombits(lo), math.Float64frombits(hi),
			}
			expFs := [2]float64{
				math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[:8])),
				math.Float64frombits(binary.LittleEndian.Uint64(tc.exp[8:])),
			}
			for i := range expFs {
				exp, actual := expFs[i], actualFs[i]
				if math.IsNaN(exp) {
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			}
		})
	}
}

func TestCompiler_compileV128FloatDemote(t *testing.T) {
	tests := []struct {
		name   string
		v, exp [16]byte
	}{
		{
			name: "1",
			v:    f64x2(0, 0),
			exp:  f32x4(0, 0, 0, 0),
		},
		{
			name: "2",
			v:    f64x2(0x1.fffffe0000000p-127, 0x1.fffffe0000000p-127),
			exp:  f32x4(0x1p-126, 0x1p-126, 0, 0),
		},
		{
			name: "3",
			v:    f64x2(0x1.fffffep+127, 0x1.fffffep+127),
			exp:  f32x4(0x1.fffffep+127, 0x1.fffffep+127, 0, 0),
		},
		{
			name: "4",
			v:    f64x2(math.NaN(), math.NaN()),
			exp:  f32x4(float32(math.NaN()), float32(math.NaN()), 0, 0),
		},
		{
			name: "5",
			v:    f64x2(math.Inf(1), math.Inf(-1)),
			exp:  f32x4(float32(math.Inf(1)), float32(math.Inf(-1)), 0, 0),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128FloatDemote(&wazeroir.OperationV128FloatDemote{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			actualFs := [4]float32{
				math.Float32frombits(uint32(lo)),
				math.Float32frombits(uint32(lo >> 32)),
				math.Float32frombits(uint32(hi)),
				math.Float32frombits(uint32(hi >> 32))}
			expFs := [4]float32{
				math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[:4])),
				math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[4:8])),
				math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[8:12])),
				math.Float32frombits(binary.LittleEndian.Uint32(tc.exp[12:])),
			}
			for i := range expFs {
				exp, actual := expFs[i], actualFs[i]
				if math.IsNaN(float64(exp)) {
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			}
		})
	}
}

func TestCompiler_compileV128ExtAddPairwise(t *testing.T) {

	tests := []struct {
		name   string
		shape  wazeroir.Shape
		signed bool
		v, exp [16]byte
	}{
		{
			name:   "i8x16 s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16 s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			v:      [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			exp:    i16x8(2, 2, 2, 2, 2, 2, 2, 2),
		},
		{
			name:   "i8x16 s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(
				i16ToU16(-2), i16ToU16(-2), i16ToU16(-2), i16ToU16(-2),
				i16ToU16(-2), i16ToU16(-2), i16ToU16(-2), i16ToU16(-2),
			),
		},
		{
			name:   "i8x16 s",
			shape:  wazeroir.ShapeI8x16,
			signed: true,
			v: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(
				i16ToU16(-256), i16ToU16(-256), i16ToU16(-256), i16ToU16(-256),
				i16ToU16(-256), i16ToU16(-256), i16ToU16(-256), i16ToU16(-256),
			),
		},
		{
			name:   "i8x16 u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i8x16 u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			v:      [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			exp:    i16x8(2, 2, 2, 2, 2, 2, 2, 2),
		},
		{
			name:   "i8x16 u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			v: [16]byte{
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
				i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1), i8ToU8(-1),
			},
			exp: i16x8(510, 510, 510, 510, 510, 510, 510, 510),
		},
		{
			name:   "i8x16 u",
			shape:  wazeroir.ShapeI8x16,
			signed: false,
			v: [16]byte{
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
				i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128), i8ToU8(-128),
			},
			exp: i16x8(256, 256, 256, 256, 256, 256, 256, 256),
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			v:      i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp:    i32x4(2, 2, 2, 2),
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			v: i16x8(
				i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1),
				i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1),
			),
			exp: i32x4(i32ToU32(-2), i32ToU32(-2), i32ToU32(-2), i32ToU32(-2)),
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			v: i16x8(
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
			),
			exp: i32x4(i32ToU32(-65536), i32ToU32(-65536), i32ToU32(-65536), i32ToU32(-65536)),
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			v:      [16]byte{},
			exp:    [16]byte{},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			v:      i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp:    i32x4(2, 2, 2, 2),
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			v: i16x8(
				i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1),
				i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1),
			),
			exp: i32x4(131070, 131070, 131070, 131070),
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			v: i16x8(
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
				i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768), i16ToU16(-32768),
			),
			exp: i32x4(65536, 65536, 65536, 65536),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128ExtAddPairwise(&wazeroir.OperationV128ExtAddPairwise{
				OriginShape: tc.shape, Signed: tc.signed,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Narrow(t *testing.T) {
	tests := []struct {
		name        string
		shape       wazeroir.Shape
		signed      bool
		x1, x2, exp [16]byte
	}{
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp:    [16]byte{},
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:     i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			x2:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0),
			x2:     i16x8(0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff),
			exp: [16]byte{
				0x80, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x00,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			},
		},
		{
			name:   "i16x8 s",
			shape:  wazeroir.ShapeI16x8,
			signed: true,
			x1:     i16x8(0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff),
			x2:     i16x8(i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0),
			exp: [16]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0x80, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x00,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp:    [16]byte{},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:     i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			exp: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(1, 1, 1, 1, 1, 1, 1, 1),
			x2:     i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp: [16]byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0),
			x2:     i16x8(0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff),
			exp: [16]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff),
			x2:     i16x8(i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0, i16ToU16(-0x8000), 0),
			exp: [16]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(i16ToU16(-1), 0, i16ToU16(-1), 0, i16ToU16(-1), 0, i16ToU16(-1), 0),
			x2:     i16x8(0, 0x100, 0, 0x100, 0, 0x100, 0, 0x100),
			exp: [16]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff,
			},
		},
		{
			name:   "i16x8 u",
			shape:  wazeroir.ShapeI16x8,
			signed: false,
			x1:     i16x8(0, 0x100, 0, 0x100, 0, 0x100, 0, 0x100),
			x2:     i16x8(i16ToU16(-1), 0, i16ToU16(-1), 0, i16ToU16(-1), 0, i16ToU16(-1), 0),
			exp: [16]byte{
				0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:   "i32x4 s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(0, 0, 0, 0),
			x2:     i32x4(0, 0, 0, 0),
			exp:    i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i32x4 s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(0, 0, 0, 0),
			x2:     i32x4(1, 1, 1, 1),
			exp:    i16x8(0, 0, 0, 0, 1, 1, 1, 1),
		},
		{
			name:   "i32x4 s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(1, 1, 1, 1),
			x2:     i32x4(0, 0, 0, 0),
			exp:    i16x8(1, 1, 1, 1, 0, 0, 0, 0),
		},
		{
			name:   "i32x4 s",
			shape:  wazeroir.ShapeI32x4,
			signed: true,
			x1:     i32x4(0x8000, 0x8000, 0x7fff, 0x7fff),
			x2:     i32x4(0x7fff, 0x7fff, 0x8000, 0x8000),
			exp:    i16x8(0x7fff, 0x7fff, 0x7fff, 0x7fff, 0x7fff, 0x7fff, 0x7fff, 0x7fff),
		},
		{
			name:   "i32x4 u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(0, 0, 0, 0),
			x2:     i32x4(0, 0, 0, 0),
			exp:    i16x8(0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			name:   "i32x4 u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(0, 0, 0, 0),
			x2:     i32x4(1, 1, 1, 1),
			exp:    i16x8(0, 0, 0, 0, 1, 1, 1, 1),
		},
		{
			name:   "i32x4 u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(1, 1, 1, 1),
			x2:     i32x4(0, 0, 0, 0),
			exp:    i16x8(1, 1, 1, 1, 0, 0, 0, 0),
		},
		{
			name:   "i32x4 u",
			shape:  wazeroir.ShapeI32x4,
			signed: false,
			x1:     i32x4(0x8000, 0x8000, 0x7fff, 0x7fff),
			x2:     i32x4(0x7fff, 0x7fff, 0x8000, 0x8000),
			exp:    i16x8(0x8000, 0x8000, 0x7fff, 0x7fff, 0x7fff, 0x7fff, 0x8000, 0x8000),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Narrow(&wazeroir.OperationV128Narrow{
				OriginShape: tc.shape, Signed: tc.signed,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128FConvertFromI(t *testing.T) {
	tests := []struct {
		name      string
		destShape wazeroir.Shape
		signed    bool
		v, exp    [16]byte
	}{
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    true,
			v:         i32x4(0, 0, 0, 0),
			exp:       f32x4(0, 0, 0, 0),
		},
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    true,
			v:         i32x4(1, 0, 2, 3),
			exp:       f32x4(1, 0, 2.0, 3),
		},
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    true,
			v:         i32x4(1234567890, i32ToU32(-2147483648.0), 2147483647, 1234567890),
			exp:       f32x4(0x1.26580cp+30, -2147483648.0, 2147483647, 0x1.26580cp+30),
		},
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    false,
			v:         i32x4(0, 0, 0, 0),
			exp:       f32x4(0, 0, 0, 0),
		},
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    false,
			v:         i32x4(1, 0, 2, 3),
			exp:       f32x4(1, 0, 2.0, 3),
		},
		{
			name:      "f32x4 s",
			destShape: wazeroir.ShapeF32x4,
			signed:    false,
			v:         i32x4(2147483647, i32ToU32(-2147483648.0), 2147483647, i32ToU32(-1)),
			exp:       f32x4(2147483648.0, 2147483648.0, 2147483648.0, 4294967295.0),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    true,
			v:         i32x4(0, 0, 0, 0),
			exp:       f64x2(0, 0),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    true,
			v:         i32x4(0, 0, i32ToU32(-1), i32ToU32(-32)),
			exp:       f64x2(0, 0),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    true,
			v:         i32x4(2147483647, i32ToU32(-2147483648), 0, 0),
			exp:       f64x2(2147483647, -2147483648),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    false,
			v:         i32x4(0, 0, 0, 0),
			exp:       f64x2(0, 0),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    false,
			v:         i32x4(0, 0, i32ToU32(-1), i32ToU32(-32)),
			exp:       f64x2(0, 0),
		},
		{
			name:      "f64x2 s",
			destShape: wazeroir.ShapeF64x2,
			signed:    false,
			v:         i32x4(2147483647, i32ToU32(-2147483648), 0, 0),
			exp:       f64x2(2147483647, 2147483648),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128FConvertFromI(&wazeroir.OperationV128FConvertFromI{
				DestinationShape: tc.destShape,
				Signed:           tc.signed,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128Dot(t *testing.T) {
	tests := []struct {
		name        string
		x1, x2, exp [16]byte
	}{
		{
			name: "1",
			x1:   i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			x2:   i16x8(0, 0, 0, 0, 0, 0, 0, 0),
			exp:  i32x4(0, 0, 0, 0),
		},
		{
			name: "2",
			x1:   i16x8(1, 1, 1, 1, i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1)),
			x2:   i16x8(i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), i16ToU16(-1), 2, 2, 2, 2),
			exp:  i32x4(i32ToU32(-2), i32ToU32(-2), i32ToU32(-4), i32ToU32(-4)),
		},
		{
			name: "3",
			x1:   i16x8(65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535),
			x2:   i16x8(65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535),
			exp:  i32x4(2, 2, 2, 2),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x2[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x2[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.x1[:8]),
				Hi: binary.LittleEndian.Uint64(tc.x1[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128Dot(&wazeroir.OperationV128Dot{})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestCompiler_compileV128ITruncSatFromF(t *testing.T) {

	tests := []struct {
		name        string
		originShape wazeroir.Shape
		signed      bool
		v, exp      [16]byte
	}{
		{
			name:        "f32x4 s",
			originShape: wazeroir.ShapeF32x4,
			signed:      true,
			v:           i32x4(0, 0, 0, 0),
			exp:         f32x4(0, 0, 0, 0),
		},
		{
			name:        "f32x4 s",
			originShape: wazeroir.ShapeF32x4,
			signed:      true,
			v:           f32x4(1.5, -1.9, -1.9, 1.5),
			exp:         i32x4(1, i32ToU32(-1), i32ToU32(-1), 1),
		},
		{
			name:        "f32x4 s",
			originShape: wazeroir.ShapeF32x4,
			signed:      true,
			v:           f32x4(float32(math.NaN()), -4294967294.0, float32(math.Inf(-1)), float32(math.Inf(1))),
			exp:         i32x4(0, i32ToU32(-2147483648), i32ToU32(-2147483648), 2147483647),
		},
		{
			name:        "f32x4 u",
			originShape: wazeroir.ShapeF32x4,
			signed:      false,
			v:           i32x4(0, 0, 0, 0),
			exp:         f32x4(0, 0, 0, 0),
		},
		{
			name:        "f32x4 u",
			originShape: wazeroir.ShapeF32x4,
			signed:      false,
			v:           f32x4(1.5, -1.9, -1.9, 1.5),
			exp:         i32x4(1, 0, 0, 1),
		},
		{
			name:        "f32x4 u",
			originShape: wazeroir.ShapeF32x4,
			signed:      false,
			v:           f32x4(float32(math.NaN()), -4294967294.0, 4294967294.0, float32(math.Inf(1))),
			exp:         i32x4(0, 0, 4294967295, 4294967295),
		},
		{
			name:        "f64x2 s",
			originShape: wazeroir.ShapeF64x2,
			signed:      true,
			v:           f64x2(0, 0),
			exp:         i32x4(0, 0, 0, 0),
		},
		{
			name:        "f64x2 s",
			originShape: wazeroir.ShapeF64x2,
			signed:      true,
			v:           f64x2(5.123, -2.0),
			exp:         i32x4(5, i32ToU32(-2), 0, 0),
		},
		{
			name:        "f64x2 s",
			originShape: wazeroir.ShapeF64x2,
			signed:      true,
			v:           f64x2(math.NaN(), math.Inf(1)),
			exp:         i32x4(0, 2147483647, 0, 0),
		},
		{
			name:        "f64x2 s",
			originShape: wazeroir.ShapeF64x2,
			signed:      true,
			v:           f64x2(math.Inf(-1), 4294967294.0),
			exp:         i32x4(i32ToU32(-2147483648), 2147483647, 0, 0),
		},
		{
			name:        "f64x2 u",
			originShape: wazeroir.ShapeF64x2,
			signed:      false,
			v:           f64x2(0, 0),
			exp:         i32x4(0, 0, 0, 0),
		},
		{
			name:        "f64x2 u",
			originShape: wazeroir.ShapeF64x2,
			signed:      false,
			v:           f64x2(5.123, -2.0),
			exp:         i32x4(5, 0, 0, 0),
		},
		{
			name:        "f64x2 u",
			originShape: wazeroir.ShapeF64x2,
			signed:      false,
			v:           f64x2(math.NaN(), math.Inf(1)),
			exp:         i32x4(0, 4294967295, 0, 0),
		},
		{
			name:        "f64x2 u",
			originShape: wazeroir.ShapeF64x2,
			signed:      false,
			v:           f64x2(math.Inf(-1), 4294967296.0),
			exp:         i32x4(0, 4294967295, 0, 0),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileV128ITruncSatFromF(&wazeroir.OperationV128ITruncSatFromF{
				OriginShape: tc.originShape,
				Signed:      tc.signed,
			})
			require.NoError(t, err)

			require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}

// TestCompiler_compileSelect_v128 is for select instructions on vector values.
func TestCompiler_compileSelect_v128(t *testing.T) {
	const x1Lo, x1Hi = uint64(0x1), uint64(0x2)
	const x2Lo, x2Hi = uint64(0x3), uint64(0x4)

	for _, selector := range []uint32{0, 1} {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler,
			&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

		err := compiler.compilePreamble()
		require.NoError(t, err)

		err = compiler.compileV128Const(&wazeroir.OperationV128Const{
			Lo: x1Lo,
			Hi: x1Hi,
		})
		require.NoError(t, err)

		err = compiler.compileV128Const(&wazeroir.OperationV128Const{
			Lo: x2Lo,
			Hi: x2Hi,
		})
		require.NoError(t, err)

		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: selector})
		require.NoError(t, err)

		err = compiler.compileSelect(&wazeroir.OperationSelect{IsTargetVector: true})
		require.NoError(t, err)

		require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
		require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// Generate and run the code under test.
		code, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

		lo, hi := env.stackTopAsV128()
		if selector == 0 {
			require.Equal(t, x2Lo, lo)
			require.Equal(t, x2Hi, hi)
		} else {
			require.Equal(t, x1Lo, lo)
			require.Equal(t, x1Hi, hi)
		}
	}
}
