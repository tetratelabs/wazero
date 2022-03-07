package post1_0

import (
	"context"
	_ "embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

func TestJIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runOptionalFeatureTests(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreter(t *testing.T) {
	runOptionalFeatureTests(t, wazero.NewRuntimeConfigInterpreter)
}

// runOptionalFeatureTests tests features enabled by feature flags (internalwasm.Features) as they were unfinished when
// WebAssembly 1.0 (20191205) was released.
//
// See https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
func runOptionalFeatureTests(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("sign-extension-ops", func(t *testing.T) {
		testSignExtensionOps(t, newRuntimeConfig)
	})
}

// signExtend is a WebAssembly 1.0 (20191205) Text Format source, except that it uses opcodes from 'sign-extension-ops'.
//
// See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
var signExtend = []byte(`(module
  (func $i32.extend8_s (param i32) (result i32) local.get 0 i32.extend8_s)
  (export "i32.extend8_s" (func $i32.extend8_s))

  (func $i32.extend16_s (param i32) (result i32) local.get 0 i32.extend16_s)
  (export "i32.extend16_s" (func $i32.extend16_s))

  (func $i64.extend8_s (param i64) (result i64) local.get 0 i64.extend8_s)
  (export "i64.extend8_s" (func $i64.extend8_s))

  (func $i64.extend16_s (param i64) (result i64) local.get 0 i64.extend16_s)
  (export "i64.extend16_s" (func $i64.extend16_s))

  (func $i64.extend32_s (param i64) (result i64) local.get 0 i64.extend32_s)
  (export "i64.extend32_s" (func $i64.extend32_s))
)
`)

func testSignExtensionOps(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("disabled", func(t *testing.T) {
		// Sign-extension is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.NewModuleFromSource(signExtend)
		require.Error(t, err)
	})
	t.Run("enabled", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureSignExtensionOps(true))
		module, err := r.NewModuleFromSource(signExtend)
		require.NoError(t, err)

		signExtend32from8Name, signExtend32from16Name := "i32.extend8_s", "i32.extend16_s"
		t.Run("32bit", func(t *testing.T) {
			for _, tc := range []struct {
				in       int32
				expected int32
				funcname string
			}{
				// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
				{in: 0, expected: 0, funcname: signExtend32from8Name},
				{in: 0x7f, expected: 127, funcname: signExtend32from8Name},
				{in: 0x80, expected: -128, funcname: signExtend32from8Name},
				{in: 0xff, expected: -1, funcname: signExtend32from8Name},
				{in: 0x012345_00, expected: 0, funcname: signExtend32from8Name},
				{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, funcname: signExtend32from8Name},
				{in: -1, expected: -1, funcname: signExtend32from8Name},

				// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
				{in: 0, expected: 0, funcname: signExtend32from16Name},
				{in: 0x7fff, expected: 32767, funcname: signExtend32from16Name},
				{in: 0x8000, expected: -32768, funcname: signExtend32from16Name},
				{in: 0xffff, expected: -1, funcname: signExtend32from16Name},
				{in: 0x0123_0000, expected: 0, funcname: signExtend32from16Name},
				{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, funcname: signExtend32from16Name},
				{in: -1, expected: -1, funcname: signExtend32from16Name},
			} {
				tc := tc
				t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
					fn := module.Function(tc.funcname)
					require.NotNil(t, fn)

					actual, err := fn.Call(context.Background(), uint64(uint32(tc.in)))
					require.NoError(t, err)
					require.Equal(t, tc.expected, int32(actual[0]))
				})
			}
		})
		signExtend64from8Name, signExtend64from16Name, signExtend64from32Name := "i64.extend8_s", "i64.extend16_s", "i64.extend32_s"
		t.Run("64bit", func(t *testing.T) {
			for _, tc := range []struct {
				in       int64
				expected int64
				funcname string
			}{
				// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L271-L277
				{in: 0, expected: 0, funcname: signExtend64from8Name},
				{in: 0x7f, expected: 127, funcname: signExtend64from8Name},
				{in: 0x80, expected: -128, funcname: signExtend64from8Name},
				{in: 0xff, expected: -1, funcname: signExtend64from8Name},
				{in: 0x01234567_89abcd_00, expected: 0, funcname: signExtend64from8Name},
				{in: 81985529216486784 /* = 0xfedcba98_765432_80 bit pattern */, expected: -0x80, funcname: signExtend64from8Name},
				{in: -1, expected: -1, funcname: signExtend64from8Name},

				// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L279-L285
				{in: 0, expected: 0, funcname: signExtend64from16Name},
				{in: 0x7fff, expected: 32767, funcname: signExtend64from16Name},
				{in: 0x8000, expected: -32768, funcname: signExtend64from16Name},
				{in: 0xffff, expected: -1, funcname: signExtend64from16Name},
				{in: 0x12345678_9abc_0000, expected: 0, funcname: signExtend64from16Name},
				{in: 81985529216466944 /* = 0xfedcba98_7654_8000 bit pattern */, expected: -0x8000, funcname: signExtend64from16Name},
				{in: -1, expected: -1, funcname: signExtend64from16Name},

				// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L287-L296
				{in: 0, expected: 0, funcname: signExtend64from32Name},
				{in: 0x7fff, expected: 32767, funcname: signExtend64from32Name},
				{in: 0x8000, expected: 32768, funcname: signExtend64from32Name},
				{in: 0xffff, expected: 65535, funcname: signExtend64from32Name},
				{in: 0x7fffffff, expected: 0x7fffffff, funcname: signExtend64from32Name},
				{in: 0x80000000, expected: -0x80000000, funcname: signExtend64from32Name},
				{in: 0xffffffff, expected: -1, funcname: signExtend64from32Name},
				{in: 0x01234567_00000000, expected: 0, funcname: signExtend64from32Name},
				{in: -81985529054232576 /* = 0xfedcba98_80000000 bit pattern */, expected: -0x80000000, funcname: signExtend64from32Name},
				{in: -1, expected: -1, funcname: signExtend64from32Name},
			} {
				tc := tc
				t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
					fn := module.Function(tc.funcname)
					require.NotNil(t, fn)

					actual, err := fn.Call(context.Background(), uint64(tc.in))
					require.NoError(t, err)
					require.Equal(t, tc.expected, int64(actual[0]))
				})
			}
		})
	})
}
