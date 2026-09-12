package experimental_test

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func simpleAddModule() []byte {
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{{
			Results: []wasm.ValueType{wasm.ValueTypeI32},
		}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: "f", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeI32Const, 2,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			}},
		},
	})
}

func refuelHostModule() []byte {
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},
			{Results: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		ImportSection: []wasm.Import{{
			Module: "host", Name: "refuel",
			Type: wasm.ExternTypeFunc, DescFunc: 0,
		}},
		FunctionSection: []wasm.Index{1},
		ExportSection:   []wasm.Export{{Name: "f", Type: wasm.ExternTypeFunc, Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeI32Const, 40,
				wasm.OpcodeCall, 0,
				wasm.OpcodeI32Const, 2,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			}},
		},
	})
}

func newInterpreterRuntime(t *testing.T, ctx context.Context) wazero.Runtime {
	t.Helper()
	return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
}

func TestFuel_SufficientBudgetCompletes(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 1_000)
	results, err := m.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	remaining := experimental.GetFuel(ctx)
	require.True(t, remaining < 1_000, "fuel should have been consumed, got %d", remaining)
	require.True(t, remaining > 0, "fuel should not be fully exhausted, got %d", remaining)
}

func TestFuel_Exhaustion(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 1)
	_, err = m.ExportedFunction("f").Call(ctx)
	require.ErrorIs(t, err, experimental.ErrOutOfFuel)
}

func TestFuel_Deterministic(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)
	fn := m.ExportedFunction("f")

	const initial = uint64(1_000)

	ctxA := experimental.SetFuel(ctx, initial)
	_, err = fn.Call(ctxA)
	require.NoError(t, err)

	ctxB := experimental.SetFuel(ctx, initial)
	_, err = fn.Call(ctxB)
	require.NoError(t, err)

	require.Equal(t, experimental.GetFuel(ctxA), experimental.GetFuel(ctxB))
	require.NotEqual(t, initial, experimental.GetFuel(ctxA))
}

func TestFuel_AddFuel(t *testing.T) {
	ctx := context.Background()

	experimental.AddFuel(ctx, 100) // no-op: no fuel installed
	require.Equal(t, uint64(0), experimental.GetFuel(ctx))

	ctx = experimental.SetFuel(ctx, 10)
	experimental.AddFuel(ctx, 5)
	require.Equal(t, uint64(15), experimental.GetFuel(ctx))
}

func TestFuel_HostRefuelViaAddFuel(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	bin := refuelHostModule()

	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context) {
			experimental.AddFuel(ctx, 1_000)
		}).
		Export("refuel").
		Instantiate(ctx)
	require.NoError(t, err)

	m, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 3)
	results, err := m.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(42), results[0])
}

func TestFuel_RefuelInPlace(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)
	fn := m.ExportedFunction("f")

	ctx = experimental.SetFuel(ctx, 1_000)

	_, err = fn.Call(ctx)
	require.NoError(t, err)
	require.True(t, experimental.GetFuel(ctx) < 1_000)

	experimental.SetFuel(ctx, 1_000) // refuel in place
	require.Equal(t, uint64(1_000), experimental.GetFuel(ctx))

	_, err = fn.Call(ctx)
	require.NoError(t, err)
	require.True(t, experimental.GetFuel(ctx) < 1_000)
}

func TestFuel_HostRefuel(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	bin := refuelHostModule()

	var refueled atomic.Bool
	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context) {
			experimental.SetFuel(ctx, 1_000)
			refueled.Store(true)
		}).
		Export("refuel").
		Instantiate(ctx)
	require.NoError(t, err)

	m, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 3) // would exhaust without host refuel
	results, err := m.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.True(t, refueled.Load())
	require.Equal(t, uint64(42), results[0])
}

func TestFuel_HostNoRefuelTraps(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	bin := refuelHostModule()

	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().
		WithFunc(func() {}).
		Export("refuel").
		Instantiate(ctx)
	require.NoError(t, err)

	m, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 3)
	_, err = m.ExportedFunction("f").Call(ctx)
	require.ErrorIs(t, err, experimental.ErrOutOfFuel)
}

func TestFuel_NoOpWhenDisabled(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)

	require.Equal(t, uint64(0), experimental.GetFuel(ctx))

	results, err := m.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])
}

func TestFuel_ConcurrentUse(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	compiled, err := r.CompileModule(ctx, simpleAddModule())
	require.NoError(t, err)

	probeInst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
	require.NoError(t, err)
	probeCtx := experimental.SetFuel(ctx, 10_000)
	_, err = probeInst.ExportedFunction("f").Call(probeCtx)
	require.NoError(t, err)
	perCall := 10_000 - experimental.GetFuel(probeCtx)
	require.True(t, perCall > 0)
	require.NoError(t, probeInst.Close(ctx))

	const goroutines = 16
	const callsPerGoroutine = 32
	const totalCalls = goroutines * callsPerGoroutine

	initial := perCall * totalCalls * 2
	sharedCtx := experimental.SetFuel(ctx, initial)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			inst, instErr := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
			if instErr != nil {
				t.Errorf("instantiate: %v", instErr)
				return
			}
			defer inst.Close(ctx)
			fn := inst.ExportedFunction("f")
			for j := 0; j < callsPerGoroutine; j++ {
				if _, callErr := fn.Call(sharedCtx); callErr != nil {
					t.Errorf("call: %v", callErr)
					return
				}
			}
		}()
	}
	wg.Wait()

	expectedConsumed := perCall * totalCalls
	actualConsumed := initial - experimental.GetFuel(sharedCtx)
	require.Equal(t, expectedConsumed, actualConsumed)
}

func TestFuel_RejectsAmountsAboveMaxInt64(t *testing.T) {
	ctx := context.Background()
	err := require.CapturePanic(func() {
		experimental.SetFuel(ctx, math.MaxInt64+1)
	})
	require.EqualError(t, err, "fuel: units 9223372036854775808 exceed max supported value 9223372036854775807")
}

func TestFuel_CallWithStack(t *testing.T) {
	ctx := context.Background()
	r := newInterpreterRuntime(t, ctx)
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)
	fn := m.ExportedFunction("f")

	fueledCtx := experimental.SetFuel(ctx, 1_000)
	stack := make([]uint64, 1)
	require.NoError(t, fn.CallWithStack(fueledCtx, stack))
	require.Equal(t, uint64(3), stack[0])
	require.True(t, experimental.GetFuel(fueledCtx) < 1_000)

	tightCtx := experimental.SetFuel(ctx, 1)
	stack = make([]uint64, 1)
	err = fn.CallWithStack(tightCtx, stack)
	require.ErrorIs(t, err, experimental.ErrOutOfFuel)
}

func TestFuel_StartFunctionMetered(t *testing.T) {
	ctx := context.Background()

	startIdx := wasm.Index(0)
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		StartSection:    &startIdx,
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeDrop,
				wasm.OpcodeI32Const, 2,
				wasm.OpcodeDrop,
				wasm.OpcodeI32Const, 3,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			}},
		},
	})

	t.Run("completes", func(t *testing.T) {
		r := newInterpreterRuntime(t, ctx)
		defer r.Close(ctx)

		fueledCtx := experimental.SetFuel(ctx, 1_000)
		_, err := r.Instantiate(fueledCtx, bin)
		require.NoError(t, err)
		require.True(t, experimental.GetFuel(fueledCtx) < 1_000)
	})

	t.Run("traps", func(t *testing.T) {
		r := newInterpreterRuntime(t, ctx)
		defer r.Close(ctx)

		fueledCtx := experimental.SetFuel(ctx, 1)
		_, err := r.Instantiate(fueledCtx, bin)
		require.ErrorIs(t, err, experimental.ErrOutOfFuel)
	})
}

func TestFuel_CompilerEngineRejected(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	defer r.Close(ctx)

	m, err := r.Instantiate(ctx, simpleAddModule())
	require.NoError(t, err)

	ctx = experimental.SetFuel(ctx, 1_000)
	_, err = m.ExportedFunction("f").Call(ctx)
	require.ErrorIs(t, err, experimental.ErrFuelNotSupported)
}
