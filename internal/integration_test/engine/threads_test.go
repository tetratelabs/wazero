package adhoc

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// We do not currently have hammer tests for bitwise and/or operations. The tests are designed to have
// input that changes deterministically every iteration, which is difficult to model with these operations.
// This is likely why atomic and/or do not show up in the wild very often if at all.
var (
	// memory.atomic.notify, memory.atomic.wait32, memory.atomic.wait64
	// i32.atomic.store, i32.atomic.rmw.cmpxchg
	// i64.atomic.store, i64.atomic.rmw.cmpxchg
	// i32.atomic.store8, i32.atomic.rmw8.cmpxchg_u
	// i32.atomic.store16, i32.atomic.rmw16.cmpxchg_u
	// i64.atomic.store8, i64.atomic.rmw8.cmpxchg_u
	// i64.atomic.store16, i64.atomic.rmw16.cmpxchg_u
	// i64.atomic.store32, i64.atomic.rmw32.cmpxchg_u
	//go:embed testdata/threads/mutex.wasm
	mutexWasm []byte

	// i32.atomic.rmw.add, i64.atomic.rmw.add, i32.atomic.rmw8.add_u, i32.atomic.rmw16.add_u, i64.atomic.rmw8.add_u, i64.atomic.rmw16.add_u, i64.atomic.rmw32.add_u
	//go:embed testdata/threads/add.wasm
	addWasm []byte

	// i32.atomic.rmw.sub, i64.atomic.rmw.sub, i32.atomic.rmw8.sub_u, i32.atomic.rmw16.sub_u, i64.atomic.rmw8.sub_u, i64.atomic.rmw16.sub_u, i64.atomic.rmw32.sub_u
	//go:embed testdata/threads/sub.wasm
	subWasm []byte

	// i32.atomic.rmw.xor, i64.atomic.rmw.xor, i32.atomic.rmw8.xor_u, i32.atomic.rmw16.xor_u, i64.atomic.rmw8.xor_u, i64.atomic.rmw16.xor_u, i64.atomic.rmw32.xor_u
	//go:embed testdata/threads/xor.wasm
	xorWasm []byte
)

var threadTests = map[string]func(t *testing.T, r wazero.Runtime){
	"increment guarded by mutex": incrementGuardedByMutex,
	"atomic add":                 atomicAdd,
	"atomic sub":                 atomicSub,
	"atomic xor":                 atomicXor,
}

func TestThreadsNotEnabled(t *testing.T) {
	r := wazero.NewRuntime(testCtx)
	_, err := r.Instantiate(testCtx, mutexWasm)
	require.EqualError(t, err, "invalid function[0]: i32.atomic.rmw.cmpxchg invalid as feature \"threads\" is disabled")
}

func TestThreadsInterpreter(t *testing.T) {
	runAllTests(t, threadTests, wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesThreads))
}

func incrementGuardedByMutex(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		fn string
	}{
		{
			fn: "run32",
		},
		{
			fn: "run64",
		},
		{
			fn: "run32_8",
		},
		{
			fn: "run32_16",
		},
		{
			fn: "run64_8",
		},
		{
			fn: "run64_16",
		},
		{
			fn: "run64_32",
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, mutexWasm)
			require.NoError(t, err)

			hammer.NewHammer(t, 200, 1000).Run(func(name string) {
				_, err = mod.ExportedFunction(tt.fn).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(8)
			require.True(t, ok)
			require.Equal(t, uint32(200*1000), res)
		})
	}
}

func atomicAdd(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		fn  string
		exp uint32
	}{
		{
			fn:  "run32",
			exp: 200 * 1000,
		},
		{
			fn:  "run64",
			exp: 200 * 1000,
		},
		{
			fn: "run32_8",
			// Overflows
			exp: (200 * 1000) % (1 << 8),
		},
		{
			fn: "run32_16",
			// Overflows
			exp: (200 * 1000) % (1 << 16),
		},
		{
			fn: "run64_8",
			// Overflows
			exp: (200 * 1000) % (1 << 8),
		},
		{
			fn: "run64_16",
			// Overflows
			exp: (200 * 1000) % (1 << 16),
		},
		{
			fn:  "run64_32",
			exp: 200 * 1000,
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, addWasm)
			require.NoError(t, err)

			hammer.NewHammer(t, 200, 1000).Run(func(name string) {
				_, err = mod.ExportedFunction(tt.fn).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(0)
			require.True(t, ok)
			require.Equal(t, tt.exp, res)
		})
	}
}

func atomicSub(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		fn  string
		exp int32
	}{
		{
			fn:  "run32",
			exp: -(200 * 1000),
		},
		{
			fn:  "run64",
			exp: -(200 * 1000),
		},
		{
			fn: "run32_8",
			// Overflows
			exp: (1 << 8) - ((200 * 1000) % (1 << 8)),
		},
		{
			fn: "run32_16",
			// Overflows
			exp: (1 << 16) - ((200 * 1000) % (1 << 16)),
		},
		{
			fn: "run64_8",
			// Overflows
			exp: (1 << 8) - ((200 * 1000) % (1 << 8)),
		},
		{
			fn: "run64_16",
			// Overflows
			exp: (1 << 16) - ((200 * 1000) % (1 << 16)),
		},
		{
			fn:  "run64_32",
			exp: -(200 * 1000),
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, subWasm)
			require.NoError(t, err)

			hammer.NewHammer(t, 200, 1000).Run(func(name string) {
				_, err = mod.ExportedFunction(tt.fn).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(0)
			require.True(t, ok)
			require.Equal(t, tt.exp, int32(res))
		})
	}
}

func atomicXor(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		fn string
	}{
		{
			fn: "run32",
		},
		{
			fn: "run64",
		},
		{
			fn: "run32_8",
		},
		{
			fn: "run32_16",
		},
		{
			fn: "run64_8",
		},
		{
			fn: "run64_16",
		},
		{
			fn: "run64_32",
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, xorWasm)
			require.NoError(t, err)

			mod.Memory().WriteUint32Le(0, 12345)

			hammer.NewHammer(t, 200, 1000).Run(func(name string) {
				_, err = mod.ExportedFunction(tt.fn).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(0)
			require.True(t, ok)
			// Even number of iterations, the value should be unchanged.
			require.Equal(t, uint32(12345), res)
		})
	}
}
