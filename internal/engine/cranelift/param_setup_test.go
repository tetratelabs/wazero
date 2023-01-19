package cranelift

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestArm64_paramSetupFn_amd64(t *testing.T) {
	t.Skip("TODO")
}

func TestArm64_paramSetupFn_arm64(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil).(*engine)
	require.NotNil(t, e)
	defer func() {
		require.NoError(t, e.Close())
	}()

	for _, tc := range []struct {
		name string
		sig  *wasm.FunctionType
		exp  string
	}{
		{
			name: "i32_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{i32},
			},
			// ldr w2, [x10]
			// ret
			exp: "420140b9c0035fd60000000000000000",
		},
		{
			name: "i32f32_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{i32, f32},
			},
			// ldr w2, [x10]
			// ldr s0, [x10, #8]
			// ret
			exp: "420140b9400940bdc0035fd600000000",
		},
		{
			name: "f32i32_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{f32, i32},
			},
			// ldr s0, [x10]
			// ldr w2, [x10, #8]
			// ret
			exp: "400140bd420940b9c0035fd600000000",
		},
		{
			name: "f32_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{f32},
			},
			// ldr s0, [x10]
			// ret
			exp: "400140bdc0035fd60000000000000000",
		},
		{
			name: "f64_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{f64},
			},
			// ldr d0, [x10]
			// ret
			exp: "400140fdc0035fd60000000000000000",
		},
		{
			name: "f64i64_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{f64, i64},
			},
			// ldr d0, [x10]
			// ldr x2, [x10, #8]
			// ret
			exp: "400140fd420540f9c0035fd600000000",
		},
		{
			name: "f64i64f32f32f32i32i32_v",
			sig: &wasm.FunctionType{
				Params: []wasm.ValueType{f64, i64, f32, f32, f32, i64, i32},
			},
			// ldr d0, [x10]
			// ldr x2, [x10, #8]
			// ldr s1, [x10, #0x10]
			// ldr s2, [x10, #0x18]
			// ldr s3, [x10, #0x20]
			// ldr x3, [x10, #0x28]
			// ldr w4, [x10, #0x30]
			// ret
			exp: "400140fd420540f9411140bd421940bd432140bd431540f9443140b9c0035fd6",
		},
		// TODO: multi results and/or stack allocation cases.
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actual, err := e.paramSetupFnArm64(tc.sig)
			require.NoError(t, err)
			act := hex.EncodeToString(actual)
			require.Equal(t, act, tc.exp, act)
		})
	}
}
