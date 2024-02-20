package nodiff

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_ensureMutableGlobalsMatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cm, im *wasm.ModuleInstance
		expErr string
	}{
		{
			name: "no globals",
			cm:   &wasm.ModuleInstance{},
			im:   &wasm.ModuleInstance{},
		},
		{
			name: "i32 match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}},
				},
			},
		},
		{
			name: "i32 match not match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 11, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}},
				},
			},
			expErr: `mutable globals mismatch:
	[1] i32: 10 != 11`,
		},
		{
			name: "i64 match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI64}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI64}},
				},
			},
		},
		{
			name: "i64 match not match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI64}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 63, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI64}},
				},
			},
			expErr: `mutable globals mismatch:
	[2] i64: 4611686018427387904 != 9223372036854775808`,
		},
		{
			name: "f32 match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF32}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF32}},
				},
			},
		},
		{
			name: "f32 match not match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 10, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF32}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 11, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF32}},
				},
			},
			expErr: `mutable globals mismatch:
	[1] f32: 10 != 11`,
		},
		{
			name: "f64 match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF64}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF64}},
				},
			},
		},
		{
			name: "f64 match not match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF64}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 63, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeF64}},
				},
			},
			expErr: `mutable globals mismatch:
	[2] f64: 4611686018427387904 != 9223372036854775808`,
		},

		{
			name: "v128 match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{ValHi: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeV128}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{ValHi: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeV128}},
				},
			},
		},
		{
			name: "v128 match not match",
			cm: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeV128}},
				},
			},
			im: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Type: wasm.GlobalType{ValType: wasm.ValueTypeV128}},
					{Val: 1 << 62, ValHi: 1234, Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeV128}},
				},
			},
			expErr: `mutable globals mismatch:
	[2] v128: (4611686018427387904,0) != (4611686018427387904,1234)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var actualErr error

			// Append the "fuel" inserted by the fuzzer, which will be ignored by ensureMutableGlobalsMatch.
			tc.im.Globals = append(tc.im.Globals, &wasm.GlobalInstance{Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}, Val: 10000})
			tc.cm.Globals = append(tc.cm.Globals, &wasm.GlobalInstance{Type: wasm.GlobalType{Mutable: true, ValType: wasm.ValueTypeI32}, Val: 1})
			ensureMutableGlobalsMatch(tc.cm, tc.im, func(err error) {
				actualErr = err
			})
			if tc.expErr == "" {
				require.NoError(t, actualErr)
			} else {
				require.Equal(t, tc.expErr, actualErr.Error())
			}
		})
	}
}
