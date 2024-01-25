package backend

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_goFunctionCallRequiredStackSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sig      *ssa.Signature
		argBegin int
		exp      int64
	}{
		{
			name: "no param",
			sig:  &ssa.Signature{},
			exp:  0,
		},
		{
			name: "only param",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128}},
			exp:  32,
		},
		{
			name: "only result",
			sig:  &ssa.Signature{Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}},
			exp:  32,
		},
		{
			name: "param < result",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}},
			exp:  32,
		},
		{
			name: "param > result",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128}},
			exp:  32,
		},
		{
			name:     "param < result / argBegin=2",
			argBegin: 2,
			sig:      &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64}},
			exp:      16,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			requiredSize, _ := GoFunctionCallRequiredStackSize(tc.sig, tc.argBegin)
			require.Equal(t, tc.exp, requiredSize)
		})
	}
}
