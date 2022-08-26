package vs

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

const (
	i32                  = "i32"
	i32ValueMemoryOffset = 32
	i64                  = "i64"
	i64ValueMemoryOffset = 64
	inWasmIteration      = 100
)

var (
	//go:embed testdata/memory.wasm
	memoryWasm      []byte
	memoryConfig    *RuntimeConfig
	memoryFunctions = []string{i32, i64}
)

func init() {
	memoryConfig = &RuntimeConfig{
		ModuleName:        "memory",
		ModuleWasm:        memoryWasm,
		FuncNames:         memoryFunctions,
		NeedsMemoryExport: true,
	}
}

func RunTestMemory(t *testing.T, runtime func() Runtime) {
	t.Run(i32, func(t *testing.T) {
		testCall(t, runtime, memoryConfig, func(t *testing.T, m Module, instantiation int, iteration int) {
			buf := m.Memory()
			binary.LittleEndian.PutUint32(buf[i32ValueMemoryOffset:], inWasmIteration)
			err := m.CallV_V(testCtx, i32)
			require.NoError(t, err)
			if 0 != binary.LittleEndian.Uint32(buf[i32ValueMemoryOffset:]) {
				panic(fmt.Sprintf("BUG at iteration %d: %d", iteration, binary.LittleEndian.Uint32(buf[i32ValueMemoryOffset:])))
			}
		})
	})

	t.Run(i64, func(t *testing.T) {
		testCall(t, runtime, memoryConfig, func(t *testing.T, m Module, instantiation int, iteration int) {
			buf := m.Memory()
			binary.LittleEndian.PutUint64(buf[i64ValueMemoryOffset:], inWasmIteration)
			err := m.CallV_V(testCtx, i64)
			require.NoError(t, err)
			if 0 != binary.LittleEndian.Uint64(buf[i64ValueMemoryOffset:]) {
				panic(fmt.Sprintf("BUG at iteration %d: %d", iteration, binary.LittleEndian.Uint64(buf[i32ValueMemoryOffset:])))
			}
		})
	})
}

func RunTestBenchmarkMemory_CompilerFastest(t *testing.T, vsRuntime Runtime) {
	t.Run(i32, func(t *testing.T) {
		runTestBenchmark_Call_CompilerFastest(t, memoryConfig, "/memory.i32", memoryI32, vsRuntime)
	})
	t.Run(i64, func(t *testing.T) {
		runTestBenchmark_Call_CompilerFastest(t, memoryConfig, "/memory.i64", memoryI64, vsRuntime)
	})
}

func RunBenchmarkMemory(b *testing.B, runtime func() Runtime) {
	b.Run(i32, func(b *testing.B) {
		benchmark(b, runtime, memoryConfig, memoryI32)
	})
	b.Run(i64, func(b *testing.B) {
		benchmark(b, runtime, memoryConfig, memoryI64)
	})
}

func memoryI32(m Module, iteration int) (err error) {
	buf := m.Memory()
	binary.LittleEndian.PutUint32(buf[i32ValueMemoryOffset:], inWasmIteration)
	err = m.CallV_V(testCtx, i32)
	if 0 != binary.LittleEndian.Uint32(buf[i32ValueMemoryOffset:]) {
		panic(fmt.Sprintf("BUG at iteration %d", iteration))
	}
	return
}

func memoryI64(m Module, iteration int) (err error) {
	buf := m.Memory()
	binary.LittleEndian.PutUint64(buf[i64ValueMemoryOffset:], inWasmIteration)
	err = m.CallV_V(testCtx, i64)
	if 0 != binary.LittleEndian.Uint64(buf[i64ValueMemoryOffset:]) {
		panic(fmt.Sprintf("BUG at iteration %d", iteration))
	}
	return
}
