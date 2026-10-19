package adhoc

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/copysign_localset.wasm
var copysignLocalsetWasm []byte

//go:embed testdata/copysign_localset_f64.wasm
var copysignLocalsetF64Wasm []byte

// TestCopysignWithSelectAfterCallResultLocalSet is a regression test for #2555
// where the optimizing compiler miscompiled f32.copysign/f64.copysign when the
// operands were defined by calls whose results were moved into locals before
// the select/copysign sequence: it returned -x although the sign operand was
// positive.
//
// The bug is specific to the amd64 backend: the mask constants used by
// lowerFcopysign were materialized after register allocation, and the register
// allocator could assign the call result register (xmm0) as their backing
// machine register, clobbering the operands.
func TestCopysignWithSelectAfterCallResultLocalSet(t *testing.T) {
	tests := []struct {
		name           string
		wasm           []byte
		isF64          bool
		input          float64
		selectCaseWant float64
		negativeSign   float64
	}{
		{
			name:           "f32",
			wasm:           copysignLocalsetWasm,
			input:          1.5,
			selectCaseWant: 1.5,
			negativeSign:   -1.5,
		},
		{
			name:           "f64",
			wasm:           copysignLocalsetF64Wasm,
			isF64:          true,
			input:          2.5,
			selectCaseWant: 2.5,
			negativeSign:   -2.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range []struct {
				name string
				cfg  func(context.Context) (wazero.Runtime, error)
			}{
				{
					name: "compiler",
					cfg: func(ctx context.Context) (wazero.Runtime, error) {
						return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler()), nil
					},
				},
				{
					name: "interpreter",
					cfg: func(ctx context.Context) (wazero.Runtime, error) {
						return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter()), nil
					},
				},
			} {
				t.Run(mode.name, func(t *testing.T) {
					// Use a fresh runtime per call so that previous compilations
					// in the same engine cannot mask the arithmetic.
					ctx := context.Background()
					r, err := mode.cfg(ctx)
					require.NoError(t, err)
					defer r.Close(ctx)

					mod, err := r.Instantiate(ctx, tt.wasm)
					require.NoError(t, err)
					defer mod.Close(ctx)

					selectCaseWantBits := encodeBits(tt.isF64, tt.selectCaseWant)
					negativeSignBits := encodeBits(tt.isF64, tt.negativeSign)

					ret, err := mod.ExportedFunction("select_case").Call(ctx, encodeBits(tt.isF64, tt.input))
					require.NoError(t, err)
					require.Equal(t, selectCaseWantBits, ret[0])

					ret, err = mod.ExportedFunction("negative_sign").Call(ctx, encodeBits(tt.isF64, tt.input))
					require.NoError(t, err)
					require.Equal(t, negativeSignBits, ret[0])
				})
			}
		})
	}
}

// encodeBits packs v as the raw bits expected by the wasm function signature.
func encodeBits(isF64 bool, v float64) uint64 {
	if isF64 {
		return api.EncodeF64(v)
	}
	return uint64(api.EncodeF32(float32(v)))
}
