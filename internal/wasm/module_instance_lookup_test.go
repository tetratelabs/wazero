package wasm

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModuleInstance_LookupFunction(t *testing.T) {
	var called int
	hostModule := &Module{
		IsHostModule: true,
		CodeSection: []Code{
			{GoFunc: api.GoFunc(func(context.Context, []uint64) {
				called++
			})},
			{GoFunc: api.GoModuleFunc(func(context.Context, api.Module, []uint64) {
				called++
			})},
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
		require.Nil(t, gf.lookedUpModule) // GoFunction doesn't need looked up module.
		require.Equal(t, &hostModule.FunctionDefinitionSection[0], gf.def)
		err := gf.CallWithStack(context.Background(), nil)
		require.NoError(t, err)

		gmf, ok := m.LookupFunction(nil, 0, 1).(*lookedUpGoFunction)
		require.True(t, ok)
		require.Equal(t, m, gmf.lookedUpModule)
		require.Equal(t, hostModule.CodeSection[1].GoFunc, gmf.g)
		require.Equal(t, &hostModule.FunctionDefinitionSection[1], gmf.def)
		err = gmf.CallWithStack(context.Background(), nil)
		require.NoError(t, err)

		require.Equal(t, 2, called)
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

func Test_lookedUpGoModuleFunction(t *testing.T) {
	def := &FunctionDefinition{
		Functype: &FunctionType{
			ParamNumInUint64: 2, ResultNumInUint64: 1,
			Params: []ValueType{ValueTypeI64, ValueTypeF64}, Results: []ValueType{ValueTypeI64},
		},
	}

	expMod := &ModuleInstance{}
	var called bool
	l := &lookedUpGoFunction{
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
