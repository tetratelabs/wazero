package jit

// import (
// 	"context"
// 	_ "embed"
// 	"testing"

// 	"github.com/stretchr/testify/require"
// 	wasm "github.com/tetratelabs/wazero/internal/wasm"
// 	"github.com/tetratelabs/wazero/internal/wasm/binary"
// )

// //go:embed tmp.wasm
// var bin []byte

// //go:embed tmp2.wasm
// var bin2 []byte

// func Test_a(t *testing.T) {
// 	eng := newEngine()
// 	store := wasm.NewStore(eng)
// 	addSpectestModule(t, store)

// 	{
// 		// Register.
// 		mod, err := binary.DecodeModule(bin2)
// 		require.NoError(t, err)
// 		_, err = store.Instantiate(mod, "test")
// 		require.NoError(t, err)
// 	}

// 	mod, err := binary.DecodeModule(bin)
// 	require.NoError(t, err)
// 	ctx, err := store.Instantiate(mod, "original")
// 	require.NoError(t, err)

// 	fn := ctx.Function("print64")

// 	_, err = fn(context.Background())
// 	require.NoError(t, err)
// }

// func addSpectestModule(t *testing.T, store *wasm.Store) {
// 	var printV = func() {}
// 	var printI32 = func(uint32) {}
// 	var printF32 = func(float32) {}
// 	var printI64 = func(uint64) {}
// 	var printF64 = func(float64) {}
// 	var printI32F32 = func(uint32, float32) {}
// 	var printF64F64 = func(float64, float64) {}

// 	for n, v := range map[string]interface{}{
// 		"print":         printV,
// 		"print_i32":     printI32,
// 		"print_f32":     printF32,
// 		"print_i64":     printI64,
// 		"print_f64":     printF64,
// 		"print_i32_f32": printI32F32,
// 		"print_f64_f64": printF64F64,
// 	} {
// 		fn, err := wasm.NewGoFunc(n, v)
// 		require.NoError(t, err)
// 		_, err = store.AddHostFunction("spectest", fn)
// 		require.NoError(t, err, "AddHostFunction(%s)", n)
// 	}

// 	for _, g := range []struct {
// 		name      string
// 		valueType wasm.ValueType
// 		value     uint64
// 	}{
// 		{name: "global_i32", valueType: wasm.ValueTypeI32, value: uint64(int32(666))},
// 		{name: "global_i64", valueType: wasm.ValueTypeI64, value: uint64(int64(666))},
// 		{name: "global_f32", valueType: wasm.ValueTypeF32, value: uint64(uint32(0x44268000))},
// 		{name: "global_f64", valueType: wasm.ValueTypeF64, value: uint64(0x4084d00000000000)},
// 	} {
// 		require.NoError(t, store.AddGlobal("spectest", g.name, g.value, g.valueType, false), "AddGlobal(%s)", g.name)
// 	}

// 	tableLimitMax := uint32(20)
// 	require.NoError(t, store.AddTableInstance("spectest", "table", 10, &tableLimitMax))

// 	memoryLimitMax := uint32(2)
// 	require.NoError(t, store.AddMemoryInstance("spectest", "memory", 1, &memoryLimitMax))
// }
