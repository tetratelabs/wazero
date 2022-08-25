package vs

import (
	_ "embed"
	"encoding/binary"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"
)

var (
	//go:embed testdata/memory.wasm
	memoryWasm      []byte
	memoryConfig    *RuntimeConfig
	memoryFunctions = []string{"i32", "i64"}
)

func init() {
	memoryConfig = &RuntimeConfig{
		ModuleName: "memory",
		ModuleWasm: memoryWasm,
		FuncNames:  memoryFunctions,
	}
}

func RunTestMemory(t *testing.T, runtime func() Runtime) {
	for _, fn := range memoryFunctions {
		fn := fn
		t.Run(fn, func(t *testing.T) {
			testCall(t, runtime, memoryConfig, func(t *testing.T, m Module, instantiation int, iteration int) {
				err := m.CallV_V(testCtx, fn)
				require.NoError(t, err)

				buf := m.Memory()
				switch fn {
				case "i32":
					require.Equal(t, uint32(iteration)+1, binary.LittleEndian.Uint32(buf[64:]))
				case "i64":
					require.Equal(t, uint64(iteration)+1, binary.LittleEndian.Uint64(buf[128:]))
				}
			})
		})
	}
}

func RunTestBenchmarkMemory_CompilerFastest(t *testing.T, vsRuntime Runtime) {
	for _, fn := range memoryFunctions {
		fn := fn
		t.Run(fn, func(t *testing.T) {
			runTestBenchmark_Call_CompilerFastest(t, memoryConfig, fn+".memory", func(m Module) (err error) {
				err = m.CallV_V(testCtx, fn)
				return
			}, vsRuntime)
		})
	}
}

func RunBenchmarkMemory(b *testing.B, runtime func() Runtime) {
	for _, fn := range memoryFunctions {
		fn := fn
		b.Run(fn, func(b *testing.B) {
			benchmark(b, runtime, memoryConfig, func(m Module) (err error) {
				err = m.CallV_V(testCtx, fn)
				return
			})
		})
	}
}
