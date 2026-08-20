package adhoc

import (
	"context"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Guest code can run unboundedly without ever crossing a `loop` back-edge, which used to be
// the only place WithCloseOnContextDone emitted its module-closed check. Both shapes below
// ran forever under a canceled context before the check was also emitted at function entry.
//
// Wasm's structured control flow makes `loop` the only backward branch within a function, so
// the call graph is the only other way to build one.

const (
	opNop        = 0x01
	opEnd        = 0x0b
	opCall       = 0x10
	opReturnCall = 0x12
)

func voidFuncType() wasm.FunctionType {
	return wasm.FunctionType{ParamNumInUint64: 0, ResultNumInUint64: 0}
}

// tailRecursionModule is infinite `return_call` self-recursion. The tail call reuses the
// frame, so this runs forever at a constant stack depth with no `loop` opcode anywhere.
func tailRecursionModule() []byte {
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{voidFuncType()},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: "run", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection: []wasm.Code{
			{Body: []byte{opReturnCall, 0x00, opEnd}},
		},
	})
}

// callTreeModule builds a loop-free exponential call tree: f_i calls f_{i+1} twice, so f_0
// performs 2^(n-1) calls while never exceeding a stack depth of n.
func callTreeModule(n int) []byte {
	funcs := make([]wasm.Index, n)
	code := make([]wasm.Code, n)
	for i := 0; i < n; i++ {
		funcs[i] = 0
		if i == n-1 {
			code[i] = wasm.Code{Body: []byte{opNop, opEnd}}
			continue
		}
		callee := byte(i + 1) // n stays below 128, so a single-byte LEB128 index is fine.
		code[i] = wasm.Code{Body: []byte{opCall, callee, opCall, callee, opEnd}}
	}
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{voidFuncType()},
		FunctionSection: funcs,
		ExportSection:   []wasm.Export{{Name: "run", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection:     code,
	})
}

// requireInterrupted calls "run" under a context that expires after timeout and requires the
// call to come back with the cancellation error. A regression fails the test at hardLimit
// rather than hanging it.
func requireInterrupted(t *testing.T, newCfg func() wazero.RuntimeConfig, bin []byte) {
	t.Helper()

	const timeout = 200 * time.Millisecond
	const hardLimit = 30 * time.Second

	cfg := newCfg().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesTailCall).
		WithCloseOnContextDone(true)

	r := wazero.NewRuntimeWithConfig(context.Background(), cfg)
	defer r.Close(context.Background())

	mod, err := r.Instantiate(context.Background(), bin)
	require.NoError(t, err)
	run := mod.ExportedFunction("run")
	require.NotNil(t, run)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, callErr := run.Call(ctx)
		done <- callErr
	}()

	select {
	case callErr := <-done:
		require.Error(t, callErr)
		require.Contains(t, callErr.Error(), "module closed with context deadline exceeded")
	case <-time.After(hardLimit):
		t.Fatal("call was never interrupted: the module-closed check is not reached on this shape")
	}
}

func TestEnsureTerminationWithoutLoops(t *testing.T) {
	engines := []struct {
		name   string
		newCfg func() wazero.RuntimeConfig
	}{
		{"compiler", wazero.NewRuntimeConfigCompiler},
		{"interpreter", wazero.NewRuntimeConfigInterpreter},
	}

	shapes := []struct {
		name string
		bin  []byte
	}{
		{"exponential call tree", callTreeModule(45)},
		{"tail call self recursion", tailRecursionModule()},
	}

	for _, eng := range engines {
		for _, sh := range shapes {
			t.Run(eng.name+"/"+sh.name, func(t *testing.T) {
				if eng.name == "compiler" && !platform.CompilerSupported() {
					t.Skip()
				}
				t.Parallel()
				requireInterrupted(t, eng.newCfg, sh.bin)
			})
		}
	}
}
