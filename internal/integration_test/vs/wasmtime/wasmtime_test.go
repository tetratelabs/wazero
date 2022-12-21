//go:build cgo

package wasmtime

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

var runtime = newWasmtimeRuntime

func TestAllocation(t *testing.T) {
	vs.RunTestAllocation(t, runtime)
}

func BenchmarkAllocation(b *testing.B) {
	vs.RunBenchmarkAllocation(b, runtime)
}

func TestBenchmarkAllocation_Call_CompilerFastest(t *testing.T) {
	vs.RunTestBenchmarkAllocation_Call_CompilerFastest(t, runtime())
}

func TestFactorial(t *testing.T) {
	vs.RunTestFactorial(t, runtime)
}

func BenchmarkFactorial(b *testing.B) {
	vs.RunBenchmarkFactorial(b, runtime)
}

func TestBenchmarkFactorial_Call_CompilerFastest(t *testing.T) {
	vs.RunTestBenchmarkFactorial_Call_CompilerFastest(t, runtime())
}

func TestHostCall(t *testing.T) {
	vs.RunTestHostCall(t, runtime)
}

func BenchmarkHostCall(b *testing.B) {
	vs.RunBenchmarkHostCall(b, runtime)
}

func TestBenchmarkHostCall_CompilerFastest(t *testing.T) {
	vs.RunTestBenchmarkHostCall_CompilerFastest(t, runtime())
}

func TestMemory(t *testing.T) {
	vs.RunTestMemory(t, runtime)
}

func BenchmarkMemory(b *testing.B) {
	vs.RunBenchmarkMemory(b, runtime)
}

func TestBenchmarkMemory_CompilerFastest(t *testing.T) {
	vs.RunTestBenchmarkMemory_CompilerFastest(t, runtime())
}

func TestShorthash(t *testing.T) {
	vs.RunTestShorthash(t, runtime)
}

func BenchmarkShorthash(b *testing.B) {
	vs.RunBenchmarkShorthash(b, runtime)
}
