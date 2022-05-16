package vs

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var (
	// catFS is an embedded filesystem limited to test.txt
	//go:embed testdata/fac.wasm
	factorialWasm   []byte
	factorialParam  = uint64(30)
	factorialResult = uint64(9682165104862298112)
	factorialConfig *RuntimeConfig
)

func init() {
	factorialConfig = &RuntimeConfig{
		ModuleName: "math",
		ModuleWasm: factorialWasm,
		FuncNames:  []string{"fac-ssa"},
	}
}

func factorialCall(m Module) error {
	_, err := m.CallI64_I64(testCtx, "fac-ssa", factorialParam)
	return err
}

func RunTestFactorial(t *testing.T, runtime func() Runtime) {
	testCall(t, runtime, factorialConfig, testFactorialCall)
}

func testFactorialCall(t *testing.T, m Module, instantiation, iteration int) {
	res, err := m.CallI64_I64(testCtx, "fac-ssa", factorialParam)
	require.NoError(t, err, "instantiation[%d] iteration[%d] failed", instantiation, iteration)
	require.Equal(t, factorialResult, res)
}

func RunTestBenchmarkFactorial_Call_CompilerFastest(t *testing.T, vsRuntime Runtime) {
	runTestBenchmark_Call_CompilerFastest(t, factorialConfig, "Factorial", factorialCall, vsRuntime)
}

func RunBenchmarkFactorial(b *testing.B, runtime func() Runtime) {
	benchmark(b, runtime, factorialConfig, factorialCall)
}
