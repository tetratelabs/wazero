package hostfunc

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func TestModuleBuilder_SetFunction(t *testing.T) {
	t.Run("register", func(t *testing.T) {
		builder := NewModuleBuilder()
		cases := []struct {
			modName, funcName string
			expIndex          uint32
		}{
			{modName: "env", funcName: "foo", expIndex: 0},
			{modName: "env", funcName: "bar", expIndex: 1},
			{modName: "golang", funcName: "hello", expIndex: 0},
			{modName: "env", funcName: "great", expIndex: 2},
		}

		for _, c := range cases {
			builder.MustSetFunction(c.modName, c.funcName, func(machine *wasm.VirtualMachine) reflect.Value {
				return reflect.ValueOf(func() {})
			})
		}

		ms := builder.Done()
		for _, c := range cases {
			mod, ok := ms[c.modName]
			require.True(t, ok)
			e, ok := mod.SecExports[c.funcName]
			require.True(t, ok)
			require.Equal(t, c.funcName, e.Name)
			require.Equal(t, wasm.ExportKindFunction, e.Desc.Kind)
			require.Equal(t, c.expIndex, e.Desc.Index)
		}
	})

	t.Run("type", func(t *testing.T) {
		builder := NewModuleBuilder()
		builder.MustSetFunction("a", "b", func(machine *wasm.VirtualMachine) reflect.Value {
			return reflect.ValueOf(func(int32, int64) (float32, float64, int32) {
				return 0, 0, 0
			})
		})
		ms := builder.Done()
		mod, ok := ms["a"]
		require.True(t, ok)
		actualType := mod.IndexSpace.Function[0].FunctionType()
		require.Equal(t, &wasm.FunctionType{
			InputTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32},
		}, actualType)
	})
}

func Test_getSignature(t *testing.T) {
	v := reflect.ValueOf(func(int32, int64, float32, float64) (int32, float64) { return 0, 0 })
	actual, err := getSignature(v.Type())
	require.NoError(t, err)
	require.Equal(t, &wasm.FunctionType{
		InputTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64},
		ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeF64},
	}, actual)
}

func Test_getTypeOf(t *testing.T) {
	for _, c := range []struct {
		kind reflect.Kind
		exp  wasm.ValueType
	}{
		{kind: reflect.Int32, exp: wasm.ValueTypeI32},
		{kind: reflect.Uint32, exp: wasm.ValueTypeI32},
		{kind: reflect.Int64, exp: wasm.ValueTypeI64},
		{kind: reflect.Uint64, exp: wasm.ValueTypeI64},
		{kind: reflect.Float32, exp: wasm.ValueTypeF32},
		{kind: reflect.Float64, exp: wasm.ValueTypeF64},
	} {
		actual, err := getTypeOf(c.kind)
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
	}
}
