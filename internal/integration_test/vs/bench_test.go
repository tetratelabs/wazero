package vs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"
	"text/tabwriter"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/wasi"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// ensureJITFastest is overridable via ldflags. Ex.
//	-ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'
var ensureJITFastest = "false"

const jitRuntime = "wazero/jit"

var jitFastestBench = benchFacInvoke

var runtimeTesters = map[string]func() runtimeTester{
	"wazero/interpreter": newWazeroInterpreterTester,
	jitRuntime:           newWazeroJITTester,
}

// TestFac_JIT_Fastest ensures that JIT is the fastest engine for function invocations.
// This is disabled by default, and can be run with -ldflags '-X github.com/tetratelabs/wazero/vs.ensureJITFastest=true'.
func TestFac_JIT_Fastest(t *testing.T) {
	if ensureJITFastest != "true" {
		t.Skip()
	}

	type benchResult struct {
		name string
		nsOp float64
	}
	results := make([]benchResult, 0, len(runtimeTesters))

	for name, rtFn := range runtimeTesters {
		result := testing.Benchmark(jitFastestBench(rtFn()))
		// https://github.com/golang/go/blob/fd09e88722e0af150bf8960e95e8da500ad91001/src/testing/benchmark.go#L428-L432
		nsOp := float64(result.T.Nanoseconds()) / float64(result.N)
		results = append(results, benchResult{name: name, nsOp: nsOp})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].nsOp < results[j].nsOp
	})

	// Print results before deciding if this failed
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	for _, result := range results {
		w.Write([]byte(fmt.Sprintf("%s:\t%.2f\n", result.name, result.nsOp)))
	}
	w.Flush()

	// Fail if jit wasn't fastest!
	require.Equal(t, jitRuntime, results[0].name, "%s is faster than %s. "+
		"Run BenchmarkFac_Invoke with ensureJITFastest=false instead to see the detailed result",
		results[0].name, jitRuntime)
}

type runtimeTester interface {
	Init(ctx context.Context, wasm []byte, funcNames ...string) error
	CallI64_I64(ctx context.Context, funcName string, param uint64) (uint64, error)
	io.Closer
}

func newWazeroInterpreterTester() runtimeTester {
	return newWazeroTester(wazero.NewRuntimeConfigInterpreter().WithFinishedFeatures())
}

func newWazeroJITTester() runtimeTester {
	return newWazeroTester(wazero.NewRuntimeConfigJIT().WithFinishedFeatures())
}

func newWazeroTester(config *wazero.RuntimeConfig) runtimeTester {
	return &wazeroTester{config: config, funcs: map[string]api.Function{}}
}

type wazeroTester struct {
	config    *wazero.RuntimeConfig
	wasi, mod api.Module
	funcs     map[string]api.Function
}

func (w *wazeroTester) Init(ctx context.Context, wasm []byte, funcNames ...string) (err error) {
	r := wazero.NewRuntimeWithConfig(w.config)

	if w.wasi, err = wasi.InstantiateSnapshotPreview1(ctx, r); err != nil {
		return
	}
	if w.mod, err = r.InstantiateModuleFromCode(ctx, wasm); err != nil {
		return
	}
	for _, funcName := range funcNames {
		if fn := w.mod.ExportedFunction(funcName); fn == nil {
			return fmt.Errorf("%s is not an exported function", fn)
		} else {
			w.funcs[funcName] = fn
		}
	}
	return
}

func (w *wazeroTester) CallI64_I64(ctx context.Context, funcName string, param uint64) (uint64, error) {
	if results, err := w.funcs[funcName].Call(ctx, param); err != nil {
		return 0, err
	} else if len(results) > 0 {
		return results[0], nil
	}
	return 0, nil
}

func (w *wazeroTester) Close() (err error) {
	for _, closer := range []io.Closer{w.mod, w.wasi} {
		if closer == nil {
			continue
		}
		if nextErr := closer.Close(); nextErr != nil {
			err = nextErr
		}
	}
	w.mod = nil
	w.wasi = nil
	return
}
