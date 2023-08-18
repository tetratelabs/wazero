package wasm

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModuleInstance_LookupFunction(t *testing.T) {
	hostModule := &Module{
		IsHostModule: true,
		CodeSection: []Code{
			{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
			{GoFunc: api.GoModuleFunc(func(context.Context, api.Module, []uint64) {})},
		},
		FunctionDefinitionSection: []FunctionDefinition{{}, {}},
	}

	me := &mockModuleEngine{
		lookupEntries: map[Index]mockModuleEngineLookupEntry{
			0: {m: &ModuleInstance{Source: hostModule}, index: 0},
			1: {m: &ModuleInstance{Source: hostModule}, index: 1},
		},
	}
	m := &ModuleInstance{Engine: me}

	t.Run("host", func(t *testing.T) {
		gf, ok := m.LookupFunction(nil, 0, 0).(*lookedUpGoFunction)
		require.True(t, ok)
		require.Equal(t, m, gf.lookedUpModule)
		require.Equal(t, hostModule.CodeSection[0].GoFunc, gf.g)
		require.Equal(t, &hostModule.FunctionDefinitionSection[0], gf.def)

		gmf, ok := m.LookupFunction(nil, 0, 1).(*lookedUpGoModuleFunction)
		require.True(t, ok)
		require.Equal(t, m, gmf.lookedUpModule)
		require.Equal(t, hostModule.CodeSection[1].GoFunc, gmf.g)
		require.Equal(t, &hostModule.FunctionDefinitionSection[1], gmf.def)
	})

	t.Run("wasm", func(t *testing.T) {
		me.lookupEntries[2] = mockModuleEngineLookupEntry{
			m: &ModuleInstance{
				Source: &Module{},
				Engine: me,
			},
			index: 100,
		}
		wf, ok := m.LookupFunction(nil, 0, 2).(*mockCallEngine)
		require.True(t, ok)
		require.Equal(t, Index(100), wf.index)
	})
}

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
