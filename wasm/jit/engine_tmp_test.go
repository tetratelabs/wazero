package jit

import (
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

func Test_tmp(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip()
	}
	buf, err := os.ReadFile("testdata/tmp.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)
	eng := newEngine()
	store := wasm.NewStore(eng)
	require.NoError(t, err)
	addSpectestModule(t, store)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	ret, _, err := store.CallFunction("test", "i32_align_switch", 0, 0)
	require.NoError(t, err)

	interpreter := wasm.NewStore(interpreter.NewEngine())
	err = interpreter.Instantiate(mod, "test")
	require.NoError(t, err)

	exp, _, err := interpreter.CallFunction("test", "i32_align_switch", 0, 0)
	require.NoError(t, err)

	require.Equal(t, uint32(exp[0]), uint32(ret[0]))
}

func addSpectestModule(t *testing.T, store *wasm.Store) {
	for n, v := range map[string]reflect.Value{
		"print":         reflect.ValueOf(func(*wasm.HostFunctionCallContext) {}),
		"print_i32":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint32) {}),
		"print_f32":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, float32) {}),
		"print_i64":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint64) {}),
		"print_f64":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, float64) {}),
		"print_i32_f32": reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint32, float32) {}),
		"print_f64_f64": reflect.ValueOf(func(*wasm.HostFunctionCallContext, float64, float64) {}),
	} {
		require.NoError(t, store.AddHostFunction("spectest", n, v), "AddHostFunction(%s)", n)
	}

	for _, g := range []struct {
		name      string
		valueType wasm.ValueType
		value     uint64
	}{
		{name: "global_i32", valueType: wasm.ValueTypeI32, value: uint64(int32(666))},
		{name: "global_i64", valueType: wasm.ValueTypeI64, value: uint64(int64(666))},
		{name: "global_f32", valueType: wasm.ValueTypeF32, value: uint64(uint32(0x44268000))},
		{name: "global_f64", valueType: wasm.ValueTypeF64, value: uint64(0x4084d00000000000)},
	} {
		require.NoError(t, store.AddGlobal("spectest", g.name, g.value, g.valueType, false), "AddGlobal(%s)", g.name)
	}

	tableLimitMax := uint32(20)
	require.NoError(t, store.AddTableInstance("spectest", "table", 10, &tableLimitMax))

	memoryLimitMax := uint32(2)
	require.NoError(t, store.AddMemoryInstance("spectest", "memory", 1, &memoryLimitMax))
}
