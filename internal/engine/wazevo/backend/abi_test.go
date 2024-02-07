package backend

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

const (
	x0 = regalloc.RealRegInvalid + 1 + iota
	x1
	x2
	x3
	x4
	x5
	x6
	x7
	x8
	x9
	v0
	v1
	v2
	v3
	v4
	v5
	v6
	v7
	v8
	v9
)

var (
	x0VReg = regalloc.FromRealReg(x0, regalloc.RegTypeInt)
	x1VReg = regalloc.FromRealReg(x1, regalloc.RegTypeInt)
	x2VReg = regalloc.FromRealReg(x2, regalloc.RegTypeInt)
	x3VReg = regalloc.FromRealReg(x3, regalloc.RegTypeInt)
	x4VReg = regalloc.FromRealReg(x4, regalloc.RegTypeInt)
	x5VReg = regalloc.FromRealReg(x5, regalloc.RegTypeInt)
	x6VReg = regalloc.FromRealReg(x6, regalloc.RegTypeInt)
	x7VReg = regalloc.FromRealReg(x7, regalloc.RegTypeInt)

	v0VReg = regalloc.FromRealReg(v0, regalloc.RegTypeFloat)
	v1VReg = regalloc.FromRealReg(v1, regalloc.RegTypeFloat)
	v2VReg = regalloc.FromRealReg(v2, regalloc.RegTypeFloat)
	v3VReg = regalloc.FromRealReg(v3, regalloc.RegTypeFloat)
	v4VReg = regalloc.FromRealReg(v4, regalloc.RegTypeFloat)
	v5VReg = regalloc.FromRealReg(v5, regalloc.RegTypeFloat)
	v6VReg = regalloc.FromRealReg(v6, regalloc.RegTypeFloat)
	v7VReg = regalloc.FromRealReg(v7, regalloc.RegTypeFloat)

	intParamResultRegs   = []regalloc.RealReg{x0, x1, x2, x3, x4, x5, x6, x7}
	floatParamResultRegs = []regalloc.RealReg{v0, v1, v2, v3, v4, v5, v6, v7}
)

func TestAbiImpl_init(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  FunctionABI
	}{
		{
			name: "empty sig",
			sig:  &ssa.Signature{},
			exp:  FunctionABI{},
		},
		{
			name: "small sig",
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI32, ssa.TypeF32, ssa.TypeI32},
				Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64},
			},
			exp: FunctionABI{
				Args: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI32},
				},
				Rets: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI64},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF64},
				},
				ArgIntRealRegs: 2, ArgFloatRealRegs: 1,
				RetIntRealRegs: 1, RetFloatRealRegs: 1,
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
			exp: FunctionABI{
				ArgStackSize: 128, RetStackSize: 128,
				Args: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				Rets: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
					{Index: 16, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 0},
					{Index: 17, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 8},
					{Index: 18, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 16},
					{Index: 19, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 24},
					{Index: 20, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 32},
					{Index: 21, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 40},
					{Index: 22, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 48},
					{Index: 23, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 56},
					{Index: 24, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 64},
					{Index: 25, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 72},
					{Index: 26, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 80},
					{Index: 27, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 88},
					{Index: 28, Kind: ABIArgKindStack, Type: ssa.TypeI32, Offset: 96},
					{Index: 29, Kind: ABIArgKindStack, Type: ssa.TypeF32, Offset: 104},
					{Index: 30, Kind: ABIArgKindStack, Type: ssa.TypeI64, Offset: 112},
					{Index: 31, Kind: ABIArgKindStack, Type: ssa.TypeF64, Offset: 120},
				},
				ArgIntRealRegs: 8, ArgFloatRealRegs: 8,
				RetIntRealRegs: 8, RetFloatRealRegs: 8,
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
			exp: FunctionABI{
				Args: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				Rets: []ABIArg{
					{Index: 0, Kind: ABIArgKindReg, Reg: x0VReg, Type: ssa.TypeI32},
					{Index: 1, Kind: ABIArgKindReg, Reg: v0VReg, Type: ssa.TypeF32},
					{Index: 2, Kind: ABIArgKindReg, Reg: x1VReg, Type: ssa.TypeI64},
					{Index: 3, Kind: ABIArgKindReg, Reg: v1VReg, Type: ssa.TypeF64},
					{Index: 4, Kind: ABIArgKindReg, Reg: x2VReg, Type: ssa.TypeI32},
					{Index: 5, Kind: ABIArgKindReg, Reg: v2VReg, Type: ssa.TypeF32},
					{Index: 6, Kind: ABIArgKindReg, Reg: x3VReg, Type: ssa.TypeI64},
					{Index: 7, Kind: ABIArgKindReg, Reg: v3VReg, Type: ssa.TypeF64},
					{Index: 8, Kind: ABIArgKindReg, Reg: x4VReg, Type: ssa.TypeI32},
					{Index: 9, Kind: ABIArgKindReg, Reg: v4VReg, Type: ssa.TypeF32},
					{Index: 10, Kind: ABIArgKindReg, Reg: x5VReg, Type: ssa.TypeI64},
					{Index: 11, Kind: ABIArgKindReg, Reg: v5VReg, Type: ssa.TypeF64},
					{Index: 12, Kind: ABIArgKindReg, Reg: x6VReg, Type: ssa.TypeI32},
					{Index: 13, Kind: ABIArgKindReg, Reg: v6VReg, Type: ssa.TypeF32},
					{Index: 14, Kind: ABIArgKindReg, Reg: x7VReg, Type: ssa.TypeI64},
					{Index: 15, Kind: ABIArgKindReg, Reg: v7VReg, Type: ssa.TypeF64},
				},
				ArgIntRealRegs: 8, ArgFloatRealRegs: 8,
				RetIntRealRegs: 8, RetFloatRealRegs: 8,
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			abi := FunctionABI{}
			abi.Init(tc.sig, intParamResultRegs, floatParamResultRegs)
			require.Equal(t, tc.exp.Args, abi.Args)
			require.Equal(t, tc.exp.Rets, abi.Rets)
			require.Equal(t, tc.exp.ArgStackSize, abi.ArgStackSize)
			require.Equal(t, tc.exp.RetStackSize, abi.RetStackSize)
			require.Equal(t, tc.exp.RetIntRealRegs, abi.RetIntRealRegs)
			require.Equal(t, tc.exp.RetFloatRealRegs, abi.RetFloatRealRegs)
			require.Equal(t, tc.exp.ArgIntRealRegs, abi.ArgIntRealRegs)
			require.Equal(t, tc.exp.ArgFloatRealRegs, abi.ArgFloatRealRegs)
		})
	}
}
