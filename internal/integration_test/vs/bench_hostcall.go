package vs

import (
	_ "embed"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"
)

var (
	//go:embed testdata/hostcall.wasm
	hostCallWasm     []byte
	hostCallConfig   *RuntimeConfig
	hostCallFunction = "call_host_func"
	hostCallParam    = uint64(12345)
)

func init() {
	hostCallConfig = &RuntimeConfig{
		ModuleName:      "hostcall",
		ModuleWasm:      hostCallWasm,
		FuncNames:       []string{hostCallFunction},
		EnvFReturnValue: 0xffff,
	}
}

func RunTestHostCall(t *testing.T, runtime func() Runtime) {
	testCall(t, runtime, hostCallConfig, testHostCall)
}

func testHostCall(t *testing.T, m Module, instantiation, iteration int) {
	res, err := m.CallI64_I64(testCtx, hostCallFunction, hostCallParam)
	require.NoError(t, err, "instantiation[%d] iteration[%d] failed", instantiation, iteration)
	require.Equal(t, hostCallConfig.EnvFReturnValue, res)
}

func RunTestBenchmarkHostCall_CompilerFastest(t *testing.T, vsRuntime Runtime) {
	runTestBenchmark_Call_CompilerFastest(t, hostCallConfig, "Host Call", hostCall, vsRuntime)
}

func RunBenchmarkHostCall(b *testing.B, runtime func() Runtime) {
	benchmark(b, runtime, hostCallConfig, hostCall)
}

func hostCall(m Module) error {
	_, err := m.CallI64_I64(testCtx, hostCallFunction, hostCallParam)
	return err
}
