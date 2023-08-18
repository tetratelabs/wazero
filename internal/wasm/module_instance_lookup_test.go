package wasm

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_lookedUpGoFunction(t *testing.T) {
	def := &FunctionDefinition{
		Functype: &FunctionType{
			ParamNumInUint64: 2, ResultNumInUint64: 1,
			Params: []ValueType{ValueTypeI64, ValueTypeF64}, Results: []ValueType{ValueTypeI64},
		},
	}

	var called bool
	l := &lookedUpGoFunction{
		def: def,
		g: api.GoFunc(
			func(ctx context.Context, stack []uint64) {
				require.Equal(t, []uint64{math.MaxUint64, math.Float64bits(math.Pi)}, stack)
				stack[0] = 1
				called = true
			},
		),
	}
	require.Equal(t, l.Definition().(*FunctionDefinition), l.def)
	result, err := l.Call(context.Background(), math.MaxUint64, math.Float64bits(math.Pi))
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []uint64{1}, result)
	require.Equal(t, 1, len(result))
}

func Test_lookedUpGoModuleFunction(t *testing.T) {
	def := &FunctionDefinition{
		Functype: &FunctionType{
			ParamNumInUint64: 2, ResultNumInUint64: 1,
			Params: []ValueType{ValueTypeI64, ValueTypeF64}, Results: []ValueType{ValueTypeI64},
		},
	}

	expMod := &ModuleInstance{}
	var called bool
	l := &lookedUpGoModuleFunction{
		lookedUpModule: expMod,
		def:            def,
		g: api.GoModuleFunc(
			func(ctx context.Context, mod api.Module, stack []uint64) {
				require.Equal(t, expMod, mod)
				require.Equal(t, []uint64{math.MaxUint64, math.Float64bits(math.Pi)}, stack)
				stack[0] = 1
				called = true
			},
		),
	}
	require.Equal(t, l.Definition().(*FunctionDefinition), l.def)
	result, err := l.Call(context.Background(), math.MaxUint64, math.Float64bits(math.Pi))
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []uint64{1}, result)
	require.Equal(t, 1, len(result))
}
