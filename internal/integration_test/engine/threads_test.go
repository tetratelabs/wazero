package adhoc

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
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

var threadTests = map[string]testCase{
	"increment guarded by mutex": {f: incrementGuardedByMutex},
	"atomic add":                 {f: atomicAdd},
	"atomic sub":                 {f: atomicSub},
	"atomic xor":                 {f: atomicXor},
}

func TestThreadsNotEnabled(t *testing.T) {
	r := wazero.NewRuntime(testCtx)
	_, err := r.Instantiate(testCtx, mutexWasm)
	require.EqualError(t, err, "section memory: shared memory requested but threads feature not enabled")
}

func TestThreadsCompiler_hammer(t *testing.T) {
	if !platform.CompilerSupports(api.CoreFeaturesV2 | experimental.CoreFeaturesThreads) {
		t.Skip()
	}
	runAllTests(t, threadTests, wazero.NewRuntimeConfigCompiler().WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesThreads), false)
}

func TestThreadsInterpreter_hammer(t *testing.T) {
	runAllTests(t, threadTests, wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesThreads), false)
}

func incrementGuardedByMutex(t *testing.T, r wazero.Runtime) {
	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
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

			fns := make([]api.Function, P)
			hammer.NewHammer(t, P, 30000).Run(func(p, n int) {
				_, err := mustGetFn(mod, tt.fn, fns, p).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(8)
			require.True(t, ok)
			require.Equal(t, uint32(P*30000), res)
		})
	}
}

func atomicAdd(t *testing.T, r wazero.Runtime) {
	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
	tests := []struct {
		fn  string
		exp int
	}{
		{
			fn:  "run32",
			exp: P * 30000,
		},
		{
			fn:  "run64",
			exp: P * 30000,
		},
		{
			fn: "run32_8",
			// Overflows
			exp: (P * 30000) % (1 << 8),
		},
		{
			fn: "run32_16",
			// Overflows
			exp: (P * 30000) % (1 << 16),
		},
		{
			fn: "run64_8",
			// Overflows
			exp: (P * 30000) % (1 << 8),
		},
		{
			fn: "run64_16",
			// Overflows
			exp: (P * 30000) % (1 << 16),
		},
		{
			fn:  "run64_32",
			exp: P * 30000,
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, addWasm)
			require.NoError(t, err)

			fns := make([]api.Function, P)
			hammer.NewHammer(t, P, 30000).Run(func(p, n int) {
				_, err := mustGetFn(mod, tt.fn, fns, p).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(0)
			require.True(t, ok)
			require.Equal(t, uint32(tt.exp), res)
		})
	}
}

func atomicSub(t *testing.T, r wazero.Runtime) {
	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
	tests := []struct {
		fn  string
		exp int
	}{
		{
			fn:  "run32",
			exp: -(P * 30000),
		},
		{
			fn:  "run64",
			exp: -(P * 30000),
		},
		{
			fn: "run32_8",
			// Overflows
			exp: (1 << 8) - ((P * 30000) % (1 << 8)),
		},
		{
			fn: "run32_16",
			// Overflows
			exp: (1 << 16) - ((P * 30000) % (1 << 16)),
		},
		{
			fn: "run64_8",
			// Overflows
			exp: (1 << 8) - ((P * 30000) % (1 << 8)),
		},
		{
			fn: "run64_16",
			// Overflows
			exp: (1 << 16) - ((P * 30000) % (1 << 16)),
		},
		{
			fn:  "run64_32",
			exp: -(P * 30000),
		},
	}
	for _, tc := range tests {
		tt := tc
		t.Run(tt.fn, func(t *testing.T) {
			mod, err := r.Instantiate(testCtx, subWasm)
			require.NoError(t, err)

			fns := make([]api.Function, P)
			hammer.NewHammer(t, P, 30000).Run(func(p, n int) {
				_, err := mustGetFn(mod, tt.fn, fns, p).Call(testCtx)
				require.NoError(t, err)
			}, func() {})

			// Cheat that LE encoding can read both 32 and 64 bits
			res, ok := mod.Memory().ReadUint32Le(0)
			require.True(t, ok)
			require.Equal(t, int32(tt.exp), int32(res))
		})
	}
}

func atomicXor(t *testing.T, r wazero.Runtime) {
	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
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

			fns := make([]api.Function, P)
			hammer.NewHammer(t, P, 30000).Run(func(p, n int) {
				_, err := mustGetFn(mod, tt.fn, fns, p).Call(testCtx)
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

// mustGetFn is a helper to get a function from a module, caching the result to avoid repeated allocations.
//
// Creating ExportedFunction per invocation costs a lot here since each time the runtime allocates the execution stack,
// so only do it once per goroutine of the hammer.
func mustGetFn(m api.Module, name string, fns []api.Function, p int) api.Function {
	if fns[p] == nil {
		fns[p] = m.ExportedFunction(name)
	}
	return fns[p]
}
