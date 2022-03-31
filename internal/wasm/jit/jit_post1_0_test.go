package jit

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileSignExtend(t *testing.T) {
	type fromKind byte
	from8, from16, from32 := fromKind(0), fromKind(1), fromKind(2)

	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int32
			expected int32
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x012345_00, expected: 0, fromKind: from8},
			{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x0123_0000, expected: 0, fromKind: from16},
			{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, newCompiler, nil)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend32From8()
				} else {
					err = compiler.compileSignExtend32From16()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int64
			expected int64
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L271-L277
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x01234567_89abcd_00, expected: 0, fromKind: from8},
			{in: 81985529216486784 /* = 0xfedcba98_765432_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L279-L285
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x12345678_9abc_0000, expected: 0, fromKind: from16},
			{in: 81985529216466944 /* = 0xfedcba98_7654_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L287-L296
			{in: 0, expected: 0, fromKind: from32},
			{in: 0x7fff, expected: 32767, fromKind: from32},
			{in: 0x8000, expected: 32768, fromKind: from32},
			{in: 0xffff, expected: 65535, fromKind: from32},
			{in: 0x7fffffff, expected: 0x7fffffff, fromKind: from32},
			{in: 0x80000000, expected: -0x80000000, fromKind: from32},
			{in: 0xffffffff, expected: -1, fromKind: from32},
			{in: 0x01234567_00000000, expected: 0, fromKind: from32},
			{in: -81985529054232576 /* = 0xfedcba98_80000000 bit pattern */, expected: -0x80000000, fromKind: from32},
			{in: -1, expected: -1, fromKind: from32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, newCompiler, nil)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend64From8()
				} else if tc.fromKind == from16 {
					err = compiler.compileSignExtend64From16()
				} else {
					err = compiler.compileSignExtend64From32()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt64())
			})
		}
	})
}
