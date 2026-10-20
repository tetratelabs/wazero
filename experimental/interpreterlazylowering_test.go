package experimental_test

import (
	"context"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// lazyLoweringModule exercises every way the interpreter enters a function
// body: a direct call, call_indirect and tail calls.
var lazyLoweringModule = binaryencoding.EncodeModule(&wasm.Module{
	TypeSection: []wasm.FunctionType{
		{Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
		{Params: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
	},
	FunctionSection: []wasm.Index{0, 1, 0, 0},
	TableSection:    []wasm.Table{{Type: wasm.RefTypeFuncref, Min: 1}},
	ElementSection: []wasm.ElementSegment{{
		OffsetExpr: wasm.NewConstantExpressionFromI32(0), TableIndex: 0, Type: wasm.RefTypeFuncref, Mode: wasm.ElementModeActive,
		Init: []wasm.ConstantExpression{wasm.NewConstantExpressionFromOpcode(wasm.OpcodeRefFunc, []byte{0})},
	}},
	ExportSection: []wasm.Export{
		{Name: "fac", Type: wasm.ExternTypeFunc, Index: 0},
		{Name: "tail_sum", Type: wasm.ExternTypeFunc, Index: 1},
		{Name: "indirect_fac", Type: wasm.ExternTypeFunc, Index: 2},
		{Name: "tail_fac", Type: wasm.ExternTypeFunc, Index: 3},
	},
	CodeSection: []wasm.Code{
		// fac(n): n == 0 ? 1 : n * fac(n-1)
		{Body: []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Eqz,
			wasm.OpcodeIf, 0x7f, // result i32
			wasm.OpcodeI32Const, 1,
			wasm.OpcodeElse,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 1, wasm.OpcodeI32Sub,
			wasm.OpcodeCall, 0,
			wasm.OpcodeI32Mul,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}},
		// tail_sum(n, acc): n == 0 ? acc : return_call tail_sum(n-1, acc+n)
		{Body: []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Eqz,
			wasm.OpcodeIf, 0x40, // empty block type
			wasm.OpcodeLocalGet, 1, wasm.OpcodeReturn,
			wasm.OpcodeEnd,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 1, wasm.OpcodeI32Sub,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Add,
			wasm.OpcodeTailCallReturnCall, 1,
			wasm.OpcodeEnd,
		}},
		// indirect_fac(n): call_indirect table[0] (fac)
		{Body: []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 0,
			wasm.OpcodeCallIndirect, 0, 0,
			wasm.OpcodeEnd,
		}},
		// tail_fac(n): return_call fac(n)
		{Body: []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeTailCallReturnCall, 0,
			wasm.OpcodeEnd,
		}},
	},
})

func newLazyLoweringRuntime(ctx context.Context, cache wazero.CompilationCache) wazero.Runtime {
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesTailCall)
	if cache != nil {
		cfg = cfg.WithCompilationCache(cache)
	}
	return wazero.NewRuntimeWithConfig(ctx, cfg)
}

func TestWithInterpreterLazyLowering(t *testing.T) {
	tests := []struct {
		name     string
		params   []uint64
		expected uint32
	}{
		{name: "fac", params: []uint64{5}, expected: 120},
		{name: "tail_sum", params: []uint64{10, 0}, expected: 55},
		{name: "indirect_fac", params: []uint64{5}, expected: 120},
		{name: "tail_fac", params: []uint64{5}, expected: 120},
	}
	for _, mode := range []struct {
		name string
		ctx  context.Context
	}{
		{"eager", context.Background()},
		{"lazy", experimental.WithInterpreterLazyLowering(context.Background())},
	} {
		for _, tc := range tests {
			// A fresh runtime per case, so the call path under test is the
			// first to reach each function body.
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				ctx := mode.ctx
				r := newLazyLoweringRuntime(ctx, nil)
				defer r.Close(ctx)

				mod, err := r.Instantiate(ctx, lazyLoweringModule)
				require.NoError(t, err)

				for range 2 { // first call lowers, second reuses
					res, err := mod.ExportedFunction(tc.name).Call(ctx, tc.params...)
					require.NoError(t, err)
					require.Equal(t, tc.expected, api.DecodeU32(res[0]))
				}
			})
		}
	}
}

func TestWithInterpreterLazyLowering_ConcurrentFirstCall(t *testing.T) {
	ctx := experimental.WithInterpreterLazyLowering(context.Background())
	r := newLazyLoweringRuntime(ctx, nil)
	defer r.Close(ctx)

	mod, err := r.Instantiate(ctx, lazyLoweringModule)
	require.NoError(t, err)

	const goroutines = 16
	results := make([]uint32, goroutines)
	errs := make([]error, goroutines)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range goroutines {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			res, err := mod.ExportedFunction("fac").Call(ctx, 10)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = api.DecodeU32(res[0])
		}()
	}
	start.Done()
	done.Wait()

	for i := range goroutines {
		require.NoError(t, errs[i])
		require.Equal(t, uint32(3628800), results[i])
	}
}

// A module compiled lazily is shared through the compilation cache, and other
// runtimes lower the functions it has not called yet.
func TestWithInterpreterLazyLowering_SharedCompilationCache(t *testing.T) {
	ctx := context.Background()
	cache := wazero.NewCompilationCache()
	defer cache.Close(ctx)

	lazyCtx := experimental.WithInterpreterLazyLowering(ctx)
	r1 := newLazyLoweringRuntime(lazyCtx, cache)
	defer r1.Close(lazyCtx)
	mod1, err := r1.Instantiate(lazyCtx, lazyLoweringModule)
	require.NoError(t, err)
	res, err := mod1.ExportedFunction("fac").Call(lazyCtx, 5)
	require.NoError(t, err)
	require.Equal(t, uint32(120), api.DecodeU32(res[0]))

	// Without the option, but a cache hit: the lazily compiled module is reused.
	r2 := newLazyLoweringRuntime(ctx, cache)
	defer r2.Close(ctx)
	mod2, err := r2.Instantiate(ctx, lazyLoweringModule)
	require.NoError(t, err)
	res, err = mod2.ExportedFunction("tail_sum").Call(ctx, 10, 0)
	require.NoError(t, err)
	require.Equal(t, uint32(55), api.DecodeU32(res[0]))
	res, err = mod2.ExportedFunction("fac").Call(ctx, 6)
	require.NoError(t, err)
	require.Equal(t, uint32(720), api.DecodeU32(res[0]))
}
