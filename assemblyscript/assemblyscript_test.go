package assemblyscript

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// abortWasm compiled from testdata/abort.ts
//go:embed testdata/abort.wasm
var abortWasm []byte

// randomWasm compiled from testdata/random.ts
//go:embed testdata/random.wasm
var randomWasm []byte

// traceWasm compiled from testdata/trace.ts
//go:embed testdata/trace.wasm
var traceWasm []byte

var testCtx = context.Background()

func TestAbort(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	out := &bytes.Buffer{}

	_, err := NewModuleBuilder().
		WithAbortWriter(out).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateModuleFromCode(testCtx, abortWasm)
	require.NoError(t, err)

	run := mod.ExportedFunction("run")
	_, err = run.Call(testCtx)
	require.Error(t, err)

	require.Equal(t, "error thrown at abort.ts:4:5\n", out.String())
}

func TestRandom(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	seed := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	_, err := NewModuleBuilder().
		WithSeedSource(bytes.NewReader(seed)).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateModuleFromCode(testCtx, randomWasm)
	require.NoError(t, err)

	rand := mod.ExportedFunction("rand")

	var nums []float64

	for i := 0; i < 3; i++ {
		res, err := rand.Call(testCtx)
		require.NoError(t, err)
		nums = append(nums, api.DecodeF64(res[0]))
	}

	// Fixed seed so output is deterministic.
	require.Equal(t, 0.3730638488484388, nums[0])
	require.Equal(t, 0.4447017452657911, nums[1])
	require.Equal(t, 0.30214158564115867, nums[2])
}

func TestTrace(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	out := &bytes.Buffer{}

	_, err := NewModuleBuilder().
		WithTraceWriter(out).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	// _start will execute the calls to trace
	_, err = r.InstantiateModuleFromCode(testCtx, traceWasm)
	require.NoError(t, err)

	require.Equal(t, `trace: zero_implicit
trace: zero_explicit
trace: one_int 1
trace: two_int 1,2
trace: three_int 1,2,3
trace: four_int 1,2,3,4
trace: five_int 1,2,3,4,5
trace: five_dbl 1.1,2.2,3.3,4.4,5.5
`, out.String())
}
