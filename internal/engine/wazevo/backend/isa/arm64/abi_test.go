package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAbiImpl_init(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  abiImpl
	}{
		{
			name: "empty sig",
			sig:  &ssa.Signature{},
			exp:  abiImpl{},
		},
		{
			name: "small sig",
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI32, ssa.TypeF32, ssa.TypeI32},
				Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64},
			},
			exp: abiImpl{
				m: nil,
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI32},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI64},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF64},
				},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg},
			},
		},
		{
			name: "regs stack mix and match",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
				Results: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
			},
			exp: abiImpl{
				argStackSize: 128, retStackSize: 128,
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: backend.ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: backend.ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: backend.ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: backend.ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
			},
		},
		{
			name: "all regs",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
				Results: []ssa.Type{
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
					ssa.TypeI32, ssa.TypeF32,
					ssa.TypeI64, ssa.TypeF64,
				},
			},
			exp: abiImpl{
				args: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				rets: []backend.ABIArg{
					{Index: 0, Kind: backend.ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: backend.ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: backend.ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: backend.ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: backend.ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: backend.ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: backend.ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: backend.ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: backend.ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: backend.ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: backend.ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: backend.ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: backend.ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: backend.ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: backend.ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: backend.ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				retRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
				argRealRegs: []regalloc.VReg{x0VReg, v0VReg, x1VReg, v1VReg, x2VReg, v2VReg, x3VReg, v3VReg, x4VReg, v4VReg, x5VReg, v5VReg, x6VReg, v6VReg, x7VReg, v7VReg},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			abi := abiImpl{}
			abi.init(tc.sig)
			require.Equal(t, tc.exp.args, abi.args)
			require.Equal(t, tc.exp.rets, abi.rets)
			require.Equal(t, tc.exp.argStackSize, abi.argStackSize)
			require.Equal(t, tc.exp.retStackSize, abi.retStackSize)
			require.Equal(t, tc.exp.retRealRegs, abi.retRealRegs)
			require.Equal(t, tc.exp.argRealRegs, abi.argRealRegs)
		})
	}
}
