package vs

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

const (
	facArgument = uint64(30)
	facResult   = uint64(9682165104862298112)
)

// facWasm is compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

var facConfig = &runtimeConfig{
	moduleName: "math",
	moduleWasm: facWasm,
	funcNames:  []string{"fac"},
}

func facCall(m module) (uint64, error) {
	return m.CallI64_I64(testCtx, "fac", facArgument)
}

func testFacCall(t *testing.T, m module) {
	res, err := m.CallI64_I64(testCtx, "fac", facArgument)
	require.NoError(t, err)
	require.Equal(t, facResult, res)
}

// TestFac ensures that the code in BenchmarkFac works as expected.
func TestFac(t *testing.T) {
	testCall(t, facConfig, testFacCall)
}

func BenchmarkFac_Compile(b *testing.B) {
	benchmarkCompile(b, facConfig)
}

func BenchmarkFac_Instantiate(b *testing.B) {
	benchmarkInstantiate(b, facConfig)
}

func BenchmarkFac_Call(b *testing.B) {
	benchmarkCall(b, facConfig, facCall)
}
